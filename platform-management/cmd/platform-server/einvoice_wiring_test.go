package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/console"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

const (
	wiringSecret = "0123456789abcdef0123456789abcdef"
	wiringOrigin = "https://console.example.com"
)

type wiringCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *wiringCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *wiringCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
	return nil
}

func (c *wiringCache) Del(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

// mainSetsConsoleEinvoices reports whether run() passes Einvoices to
// console.New. The retry route answers 503 when that field is nil.
func mainSetsConsoleEinvoices(t *testing.T) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "New" || !isIdent(sel.X, "console") {
			return true
		}
		for _, a := range call.Args {
			lit, ok := a.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, el := range lit.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "Einvoices") && isIdent(kv.Value, "einvoices") {
					found = true
				}
			}
		}
		return true
	})
	return found
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// TestRetryEinvoiceReachesTheProviderWhenConfigured: with the SePay provider
// configured, the retry route queues a failed invoice instead of answering 503
// einvoice_provider_not_configured; with no provider it still answers 503.
// main.go must hand the same service to the console.
func TestRetryEinvoiceReachesTheProviderWhenConfigured(t *testing.T) {
	if !mainSetsConsoleEinvoices(t) {
		t.Fatal("main.go does not set Einvoices: einvoices in console.Deps, so the retry route answers 503 einvoice_provider_not_configured")
	}

	pool := testdb.Open(t)
	ctx := context.Background()
	sepay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected call", http.StatusTeapot)
	}))
	t.Cleanup(sepay.Close)

	var staffID, order, ws string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Wiring Test', 'wiring-'||gen_random_uuid()::text||'@example.test') RETURNING id::text`).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&order, &ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, `DELETE FROM einvoice_issuance WHERE order_id = $1`, order)
		pool.Exec(bg, `DELETE FROM audit_event WHERE target_id = $1 OR actor_id = $2`, order, staffID)
		pool.Exec(bg, `DELETE FROM platform_staff WHERE user_id = $1`, staffID)
		pool.Exec(bg, `DELETE FROM "user" WHERE id = $1`, staffID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO platform_staff (user_id, role) VALUES ($1, 'billing')`, staffID); err != nil {
		t.Fatal(err)
	}
	store := invoice.NewPgxStore(pool)
	if _, err := store.Insert(ctx, invoice.NewRecord{OrderID: order, WorkspaceID: ws, Provider: "sepay", Currency: "VND",
		Amount: 600000, VATAmount: 60000,
		Buyer: []byte(`{"name":"Cong ty A","tax_code":"0312345678","address":"1 Le Loi","email":"a@example.com"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE einvoice_issuance SET state = 'failed', attempts = 4, last_error_code = 'rejected',
		next_attempt_at = now() + interval '30 days' WHERE order_id = $1`, order); err != nil {
		t.Fatal(err)
	}

	q := db.New(pool)
	sstore := staff.NewStore(q)
	guard := accountguard.New(sstore, &wiringCache{m: map[string][]byte{}})
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: wiringSecret, ConsoleOrigin: wiringOrigin},
		Directory: sstore, Guard: guard, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	retry := func(ops console.EinvoiceOps) (int, map[string]any) {
		h := console.New(console.Deps{
			RequireStaff: auth.RequirePlatformStaff, RequireOrigin: auth.RequireOrigin,
			Reader: console.NewPgxReader(pool), Einvoices: ops, Emails: sstore,
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/platform/einvoices/"+order+"/retry", nil)
		tok, err := platformauth.SignMFASession(wiringSecret, staffID, time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
		req.Header.Set("Origin", wiringOrigin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	var cfg config.Config
	vat, err := billing.ParseRate("0.10")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Billing.VATRate = vat
	cfg.Einvoice = config.Einvoice{Provider: "sepay", SepayAPIKey: "id:secret", SepayBaseURL: sepay.URL, SepayProviderAccountID: "acct"}
	configured, err := newEinvoiceService(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	code, body := retry(configured)
	if code == http.StatusServiceUnavailable || body["code"] == "einvoice_provider_not_configured" {
		t.Fatalf("configured provider: retry answered %d %v", code, body)
	}
	if code != http.StatusOK || body["state"] != "pending" {
		t.Errorf("configured provider: retry = %d %v, want 200 pending", code, body)
	}

	if _, err := pool.Exec(ctx, `UPDATE einvoice_issuance SET state = 'failed' WHERE order_id = $1`, order); err != nil {
		t.Fatal(err)
	}
	off, err := newEinvoiceService(config.Config{}, store)
	if err != nil {
		t.Fatal(err)
	}
	code, body = retry(off)
	if code != http.StatusServiceUnavailable || body["code"] != "einvoice_provider_not_configured" {
		t.Errorf("no provider: retry = %d %v, want 503 einvoice_provider_not_configured", code, body)
	}
}
