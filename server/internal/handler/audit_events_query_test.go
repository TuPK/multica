package handler

import (
	"encoding/csv"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

const auditTestUUID = "0190a2f4-7b1c-7c3e-9a55-2d1f6c0a4b11"

func TestAuditCursorRoundTrip(t *testing.T) {
	at := time.Date(2025, 3, 4, 5, 6, 7, 123456000, time.UTC)
	cur, err := decodeAuditCursor(encodeAuditCursor(at, auditTestUUID))
	if err != nil {
		t.Fatal(err)
	}
	if !cur.createdAt.Equal(at) || cur.id != auditTestUUID {
		t.Fatalf("got %+v", cur)
	}
}

func TestAuditCursorRejectsGarbage(t *testing.T) {
	for _, s := range []string{
		"", "not base64!", "bm9waXBl", // "nopipe"
		encodeAuditCursor(time.Now(), "not-a-uuid"),
	} {
		if _, err := decodeAuditCursor(s); err == nil {
			t.Errorf("cursor %q accepted", s)
		}
	}
}

func TestParseAuditListParams(t *testing.T) {
	from := "2025-01-01T00:00:00Z"
	to := "2025-02-01T00:00:00+02:00"
	cases := []struct {
		name  string
		query string
		code  string // expected error code, "" for success
		check func(t *testing.T, p auditListParams)
	}{
		{"defaults", "", "", func(t *testing.T, p auditListParams) {
			if p.limit != 50 || p.csv || p.cursor != nil || p.action != nil || p.from != nil {
				t.Fatalf("%+v", p)
			}
		}},
		{"limit bounds ok", "limit=1", "", nil},
		{"limit 100 ok", "limit=100", "", nil},
		{"limit zero", "limit=0", "invalid_filter", nil},
		{"limit 101", "limit=101", "invalid_filter", nil},
		{"limit text", "limit=abc", "invalid_filter", nil},
		{"limit empty", "limit=", "invalid_filter", nil},
		{"known action", "action=member.removed", "", func(t *testing.T, p auditListParams) {
			if p.action == nil || *p.action != "member.removed" {
				t.Fatal("action not parsed")
			}
		}},
		{"unknown action", "action=nope", "invalid_filter", nil},
		{"actor", "actor_id=abc", "", func(t *testing.T, p auditListParams) {
			if p.actorID == nil || *p.actorID != "abc" {
				t.Fatal("actor not parsed")
			}
		}},
		{"from to", "from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to), "", func(t *testing.T, p auditListParams) {
			if !p.from.Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("from %v", p.from)
			}
			if !p.to.Equal(time.Date(2025, 1, 31, 22, 0, 0, 0, time.UTC)) {
				t.Fatalf("to %v", p.to)
			}
		}},
		{"bad from", "from=yesterday", "invalid_filter", nil},
		{"from after to", "from=2025-02-01T00:00:00Z&to=2025-01-01T00:00:00Z", "invalid_filter", nil},
		{"from equals to is an empty range, not an error", "from=2025-02-01T00:00:00Z&to=2025-02-01T00:00:00Z", "", nil},
		{"csv", "format=csv", "", func(t *testing.T, p auditListParams) {
			if !p.csv {
				t.Fatal("csv not set")
			}
		}},
		{"json", "format=json", "", nil},
		{"bad format", "format=xml", "invalid_filter", nil},
		{"bad cursor", "cursor=zzz", "invalid_cursor", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, _ := url.ParseQuery(c.query)
			p, perr := parseAuditListParams(q, true)
			if c.code != "" {
				if perr == nil || perr.code != c.code {
					t.Fatalf("err = %v, want code %s", perr, c.code)
				}
				return
			}
			if perr != nil {
				t.Fatalf("unexpected error %v", perr)
			}
			if c.check != nil {
				c.check(t, p)
			}
		})
	}
}

func TestParseAuditListParamsAcceptsEveryCatalogAction(t *testing.T) {
	for _, a := range auditActionCatalog {
		q := url.Values{"action": {a}}
		if _, perr := parseAuditListParams(q, true); perr != nil {
			t.Errorf("%s rejected: %v", a, perr)
		}
	}
	for _, a := range []string{audit.ActionMemberRoleChanged, audit.ActionPATRevoked, audit.ActionConsentAccepted} {
		if !isAuditAction(a) {
			t.Errorf("%s missing from catalog", a)
		}
	}
}

func TestParseAuditListParamsAccountIgnoresFilters(t *testing.T) {
	q, _ := url.ParseQuery("action=nope&format=xml&limit=5")
	p, perr := parseAuditListParams(q, false)
	if perr != nil {
		t.Fatal(perr)
	}
	if p.limit != 5 || p.action != nil || p.csv {
		t.Fatalf("%+v", p)
	}
	q, _ = url.ParseQuery("limit=500")
	if _, perr := parseAuditListParams(q, false); perr == nil || perr.code != "invalid_filter" {
		t.Fatal("limit still validated on account route")
	}
}

func TestAuditCSVEscapesFormulaCells(t *testing.T) {
	e := auditEventJSON{ID: auditTestUUID, ActorLabel: "=HYPERLINK(\"x\")", TargetLabel: "+1", Action: "member.removed", Detail: []byte(`{"a":1}`), CreatedAt: "2025-01-01T00:00:00Z"}
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := writeAuditCSV(w, []auditEventJSON{e}); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(strings.NewReader(sb.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || len(recs[0]) != 14 || len(recs[1]) != 14 {
		t.Fatalf("shape: %v", recs)
	}
	if recs[0][0] != "id" || recs[0][13] != "created_at" {
		t.Fatalf("header %v", recs[0])
	}
	if recs[1][4] != "'=HYPERLINK(\"x\")" || recs[1][8] != "'+1" {
		t.Fatalf("formula cells not neutralised: %q %q", recs[1][4], recs[1][8])
	}
	if recs[1][9] != `{"a":1}` {
		t.Fatalf("detail %q", recs[1][9])
	}
}
