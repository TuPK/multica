package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// scriptedStream yields its items in order, then blocks until the context ends.
type scriptedStream struct {
	items  []scriptedItem
	closed bool
}

type scriptedItem struct {
	payload string
	err     error
}

func (s *scriptedStream) Receive(ctx context.Context) (string, error) {
	if len(s.items) == 0 {
		<-ctx.Done()
		return "", ctx.Err()
	}
	it := s.items[0]
	s.items = s.items[1:]
	return it.payload, it.err
}

func (s *scriptedStream) Close() error {
	s.closed = true
	return nil
}

// fakeAccountSource hands out the scripted streams in order. A nil stream is a
// failed Subscribe.
type fakeAccountSource struct {
	mu       sync.Mutex
	streams  []*scriptedStream
	subCalls int
}

func (f *fakeAccountSource) Subscribe(ctx context.Context) (AccountChangeStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subCalls++
	if len(f.streams) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	s := f.streams[0]
	f.streams = f.streams[1:]
	if s == nil {
		return nil, errors.New("redis unavailable")
	}
	return s, nil
}

type recordingDisconnector struct {
	mu    sync.Mutex
	users []string
	seen  chan string
}

func newRecordingDisconnector() *recordingDisconnector {
	return &recordingDisconnector{seen: make(chan string, 16)}
}

func (r *recordingDisconnector) DisconnectUser(id string) {
	r.mu.Lock()
	r.users = append(r.users, id)
	r.mu.Unlock()
	r.seen <- id
}

func runAccountListener(t *testing.T, src AccountChangeSource, d UserDisconnector) (cancel func(), done <-chan struct{}) {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	l := &AccountChangeListener{
		Source:       src,
		Disconnector: d,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		RetryDelay:   time.Millisecond,
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		l.Run(ctx)
	}()
	t.Cleanup(stop)
	return stop, finished
}

func waitForUser(t *testing.T, d *recordingDisconnector, want string) {
	t.Helper()
	select {
	case got := <-d.seen:
		if got != want {
			t.Fatalf("DisconnectUser(%q), want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("DisconnectUser(%q) was not called", want)
	}
}

func waitStopped(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the listener did not stop")
	}
}

func TestAccountChangesChannelIsTheContractName(t *testing.T) {
	if AccountChangesChannel != "mul:acct:changed" {
		t.Fatalf("channel = %q", AccountChangesChannel)
	}
}

// Two messages from the fake pub/sub disconnect two users.
func TestAccountChangesDisconnectsEachNamedUser(t *testing.T) {
	d := newRecordingDisconnector()
	src := &fakeAccountSource{streams: []*scriptedStream{{items: []scriptedItem{
		{payload: `{"user_id":"11111111-1111-1111-1111-111111111111","kind":"status"}`},
		{payload: `{"user_id":"22222222-2222-2222-2222-222222222222","kind":"sessions"}`},
	}}}}
	stop, done := runAccountListener(t, src, d)

	waitForUser(t, d, "11111111-1111-1111-1111-111111111111")
	waitForUser(t, d, "22222222-2222-2222-2222-222222222222")
	stop()
	waitStopped(t, done)
}

// A malformed message is skipped and the next one is still handled.
func TestAccountChangesSkipsMalformedMessagesAndKeepsReading(t *testing.T) {
	d := newRecordingDisconnector()
	src := &fakeAccountSource{streams: []*scriptedStream{{items: []scriptedItem{
		{payload: `not json`},
		{payload: `{"kind":"status"}`},
		{payload: `{"user_id":""}`},
		{payload: `[1,2]`},
		{payload: `{"user_id":"33333333-3333-3333-3333-333333333333","kind":"status"}`},
	}}}}
	stop, done := runAccountListener(t, src, d)

	waitForUser(t, d, "33333333-3333-3333-3333-333333333333")
	d.mu.Lock()
	n := len(d.users)
	d.mu.Unlock()
	if n != 1 {
		t.Fatalf("DisconnectUser was called %d times, want 1", n)
	}
	stop()
	waitStopped(t, done)
}

// A failed Receive opens a new subscription, and so does a failed Subscribe.
func TestAccountChangesResubscribesAfterFailures(t *testing.T) {
	d := newRecordingDisconnector()
	first := &scriptedStream{items: []scriptedItem{{err: errors.New("connection reset")}}}
	second := &scriptedStream{items: []scriptedItem{
		{payload: `{"user_id":"44444444-4444-4444-4444-444444444444","kind":"status"}`},
	}}
	src := &fakeAccountSource{streams: []*scriptedStream{nil, first, second}}
	stop, done := runAccountListener(t, src, d)

	waitForUser(t, d, "44444444-4444-4444-4444-444444444444")
	if !first.closed {
		t.Error("the failed stream was not closed")
	}
	stop()
	waitStopped(t, done)
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.subCalls < 3 {
		t.Fatalf("Subscribe calls = %d, want at least 3", src.subCalls)
	}
}

// Closing the Redis client at shutdown ends the listener without a retry loop.
func TestAccountChangesStopsWhenTheClientIsClosed(t *testing.T) {
	d := newRecordingDisconnector()
	src := &fakeAccountSource{streams: []*scriptedStream{{items: []scriptedItem{{err: redis.ErrClosed}}}}}
	_, done := runAccountListener(t, src, d)
	waitStopped(t, done)
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.subCalls != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", src.subCalls)
	}
}

func TestAccountChangesStopsWhenTheContextEnds(t *testing.T) {
	d := newRecordingDisconnector()
	src := &fakeAccountSource{streams: []*scriptedStream{{}}}
	stop, done := runAccountListener(t, src, d)
	stop()
	waitStopped(t, done)
}

// The listener calls Hub.DisconnectUser itself, so the hub satisfies the
// interface the listener takes.
var _ UserDisconnector = (*Hub)(nil)
