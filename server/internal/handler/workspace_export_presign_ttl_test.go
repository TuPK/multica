package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/workspaceexport"
)

type ttlRecordingStore struct {
	storage.Storage
	ttl time.Duration
}

func (s *ttlRecordingStore) PresignGetWithContentDisposition(_ context.Context, _ string, ttl time.Duration, _ string) (string, error) {
	s.ttl = ttl
	return "https://example.test/signed", nil
}

// A link signed right after the export finishes lives for the whole expiry
// window, and a link signed later lives only for what is left of it.
func TestExportPresignedLinkTTLMatchesTheExpiry(t *testing.T) {
	finished := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	expires := workspaceexport.ExpiresAt(finished)
	e := workspaceexport.Export{Status: workspaceexport.StatusDone, FinishedAt: &finished, ExpiresAt: &expires}

	for _, tc := range []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{"at finish", finished, 7 * 24 * time.Hour},
		{"two days in", finished.Add(48 * time.Hour), 5 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			window, ok := workspaceexport.DownloadWindow(e, tc.now)
			if !ok {
				t.Fatal("no download window for a finished export")
			}
			store := &ttlRecordingStore{}
			h := &Handler{Storage: store}
			url, err := h.exportDownloadURL(httptest.NewRequest("GET", "/", nil), "exports/x.zip", window)
			if err != nil || url == "" {
				t.Fatalf("exportDownloadURL = %q, %v", url, err)
			}
			if store.ttl != tc.want || store.ttl != expires.Sub(tc.now) {
				t.Fatalf("presign ttl = %v, want %v (expiry - now = %v)", store.ttl, tc.want, expires.Sub(tc.now))
			}
		})
	}
}
