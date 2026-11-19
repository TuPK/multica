package realtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/auth"
)

// shortLivedCache is an in-memory accountguard.Cache whose entries expire
// after a fixed short time, whatever TTL the guard asks for, so a store change
// becomes visible within the test's window.
type shortLivedCache struct {
	mu   sync.Mutex
	vals map[string][]byte
	set  map[string]time.Time
}

const shortLivedCacheTTL = 5 * time.Millisecond

func (c *shortLivedCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if at, ok := c.set[k]; ok && time.Since(at) < shortLivedCacheTTL {
		return c.vals[k], true, nil
	}
	return nil, false, nil
}

func (c *shortLivedCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vals == nil {
		c.vals, c.set = map[string][]byte{}, map[string]time.Time{}
	}
	c.vals[k], c.set[k] = v, time.Now()
	return nil
}

func (c *shortLivedCache) Del(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.vals, k)
	delete(c.set, k)
	return nil
}

const watchTestInterval = 20 * time.Millisecond

// watchedConn opens a cookie-authenticated socket for an account served by the
// real guard over store, with the recheck interval shortened.
func watchedConn(t *testing.T, store *fakeAccountStore) (*Hub, *websocket.Conn) {
	t.Helper()
	prev := accountStatusRecheckInterval
	accountStatusRecheckInterval = watchTestInterval
	t.Cleanup(func() { accountStatusRecheckInterval = prev })

	hub := NewHub()
	go hub.Run()
	mc := &mockMembershipChecker{}
	hub.SetMembershipChecker(mc)
	guard := accountguard.New(store, &shortLivedCache{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, nil, nil, guard, w, r)
	}))
	t.Cleanup(server.Close)
	header := http.Header{"Cookie": []string{auth.AuthCookieName + "=" + makeTestToken(t)}}
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	waitFor(t, "client registration", func() bool { return totalClients(hub) == 1 })
	return hub, conn
}

// A status change written straight to Postgres publishes nothing on
// mul:acct:changed, so only the periodic recheck can cut the socket.
func TestAccountWatchClosesSocketWhenSuspendedBySQL(t *testing.T) {
	store := &fakeAccountStore{found: true, status: accountguard.Active}
	_, conn := watchedConn(t, store)

	store.mu.Lock()
	store.status = accountguard.Suspended
	store.mu.Unlock()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("read succeeded; want the socket closed after the account was suspended")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("socket still open 3s after the account was suspended")
	}
}

func TestAccountWatchStopsWhenConnectionCloses(t *testing.T) {
	store := &fakeAccountStore{found: true, status: accountguard.Active}
	hub, conn := watchedConn(t, store)

	waitFor(t, "first recheck", func() bool { return store.lookups() >= 2 })
	conn.Close()
	waitFor(t, "client removal", func() bool { return totalClients(hub) == 0 })

	// Let a recheck that was already in flight finish, then expect silence.
	time.Sleep(3 * watchTestInterval)
	before := store.lookups()
	time.Sleep(10 * watchTestInterval)
	if after := store.lookups(); after != before {
		t.Fatalf("store reads grew from %d to %d after the connection closed", before, after)
	}
}
