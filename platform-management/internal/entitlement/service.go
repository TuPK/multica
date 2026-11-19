package entitlement

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// PolicyPattern is the route Multica's client calls (client.go: fetch).
const PolicyPattern = "GET /api/v1/internal/entitlement-policies/{workspaceId}"

const defaultMaxEntries = 10_000

// Source loads the inputs of a policy.
type Source interface {
	Inputs(ctx context.Context, workspaceID string) (Inputs, error)
}

type entry struct {
	in      Inputs
	expires time.Time
}

// Service builds policies and caches their inputs in process for ValidFor, per
// workspace. A staff write clears the entry (or all of them) in the process
// that made it. Several platform processes lag one another by at most ValidFor,
// which spec 5.5 accepts.
//
// The cache holds inputs, not policies, so the period and valid_until are those
// of the instant a request is served.
type Service struct {
	src Source
	now func() time.Time
	log *slog.Logger

	maxEntries int

	mu      sync.Mutex
	entries map[string]entry
	// gen advances on every Invalidate. A load that started before one is not
	// stored: it may hold the data the write just replaced.
	gen uint64
}

// NewService returns a Service. now is time.Now in production.
func NewService(src Source, now func() time.Time, log *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{src: src, now: now, log: log, maxEntries: defaultMaxEntries, entries: map[string]entry{}}
}

// Policy returns the workspace's policy at the current instant.
func (s *Service) Policy(ctx context.Context, workspaceID string) (Policy, error) {
	now := s.now()
	s.mu.Lock()
	if e, ok := s.entries[workspaceID]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return Build(e.in, now), nil
	}
	gen := s.gen
	s.mu.Unlock()

	in, err := s.src.Inputs(ctx, workspaceID)
	if err != nil {
		return Policy{}, err
	}

	s.mu.Lock()
	if s.gen == gen {
		s.store(workspaceID, entry{in: in, expires: now.Add(ValidFor)}, now)
	}
	s.mu.Unlock()
	return Build(in, now), nil
}

// store requires s.mu. The service answers any workspace id it is asked about,
// so the map is bounded: when it is full, expired entries go first, then all.
func (s *Service) store(workspaceID string, e entry, now time.Time) {
	if _, ok := s.entries[workspaceID]; !ok && len(s.entries) >= s.maxEntries {
		for k, old := range s.entries {
			if !now.Before(old.expires) {
				delete(s.entries, k)
			}
		}
		if len(s.entries) >= s.maxEntries {
			clear(s.entries)
		}
	}
	s.entries[workspaceID] = e
}

// Invalidate drops one workspace's cached inputs.
func (s *Service) Invalidate(workspaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	delete(s.entries, workspaceID)
}

// InvalidateAll drops every cached entry. A change to a plan affects every
// workspace on it.
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	clear(s.entries)
}

func (s *Service) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Handler serves PolicyPattern. It requires no credential: the client sends
// none, and the API listener is internal (see package trust). An X-User-ID that
// is present must still be one UUID, as trust.RequireUser demands.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PolicyPattern, s.servePolicy)
	return refuseMalformedUser(mux)
}

func refuseMalformedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if values := r.Header.Values(trust.UserHeader); len(values) > 0 {
			if _, ok := trust.CanonicalUUID(values[0]); !ok || len(values) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "X-User-ID must be one UUID", "code": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Service) servePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := trust.CanonicalUUID(r.PathValue("workspaceId"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found", "code": "not_found"})
		return
	}
	p, err := s.Policy(r.Context(), id)
	if err != nil {
		// The client reads any 5xx as "platform unavailable". The cause may name
		// the database, so it goes to the log only.
		s.log.Error("entitlement policy failed", "workspace_id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error", "code": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}
