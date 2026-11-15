package accountcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeClient struct {
	channels []string
	payloads []any
	err      error
}

func (f *fakeClient) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
	f.channels = append(f.channels, channel)
	f.payloads = append(f.payloads, message)
	cmd := redis.NewIntCmd(ctx)
	if f.err != nil {
		cmd.SetErr(f.err)
	}
	return cmd
}

func TestChannelAndKindsAreTheContractValues(t *testing.T) {
	if Channel != "mul:acct:changed" || KindStatus != "status" || KindSessions != "sessions" {
		t.Fatalf("constants = %q %q %q", Channel, KindStatus, KindSessions)
	}
}

func TestPublishWritesTheContractMessage(t *testing.T) {
	for _, kind := range []string{KindStatus, KindSessions} {
		c := &fakeClient{}
		err := NewPublisher(c).Publish(context.Background(), "11111111-2222-3333-4444-555555555555", kind)
		if err != nil {
			t.Fatalf("Publish(%s): %v", kind, err)
		}
		if len(c.channels) != 1 || c.channels[0] != "mul:acct:changed" {
			t.Fatalf("channels = %v", c.channels)
		}
		want := `{"user_id":"11111111-2222-3333-4444-555555555555","kind":"` + kind + `"}`
		got, ok := c.payloads[0].([]byte)
		if !ok || string(got) != want {
			t.Fatalf("payload = %#v, want %s", c.payloads[0], want)
		}
	}
}

func TestPublishRefusesABadMessageWithoutSendingIt(t *testing.T) {
	c := &fakeClient{}
	p := NewPublisher(c)
	if err := p.Publish(context.Background(), "", KindStatus); err == nil {
		t.Error("empty user id was published")
	}
	if err := p.Publish(context.Background(), "u", "other"); err == nil {
		t.Error("unknown kind was published")
	}
	if len(c.channels) != 0 {
		t.Fatalf("sent %d messages", len(c.channels))
	}
}

func TestPublishReturnsTheRedisError(t *testing.T) {
	boom := errors.New("redis down")
	err := NewPublisher(&fakeClient{err: boom}).Publish(context.Background(), "u", KindSessions)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

type fakeCommands struct {
	value   string
	getErr  error
	setErr  error
	delErr  error
	setKey  string
	setTTL  time.Duration
	deleted []string
}

func (f *fakeCommands) Get(_ context.Context, _ string) *redis.StringCmd {
	return redis.NewStringResult(f.value, f.getErr)
}

func (f *fakeCommands) Set(_ context.Context, key string, _ any, ttl time.Duration) *redis.StatusCmd {
	f.setKey, f.setTTL = key, ttl
	return redis.NewStatusResult("OK", f.setErr)
}

func (f *fakeCommands) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.deleted = append(f.deleted, keys...)
	return redis.NewIntResult(1, f.delErr)
}

func TestCacheMapsRedisToTheGuardContract(t *testing.T) {
	ctx := context.Background()

	f := &fakeCommands{value: "v"}
	b, ok, err := NewCache(f).Get(ctx, "k")
	if err != nil || !ok || string(b) != "v" {
		t.Fatalf("hit = %q %v %v", b, ok, err)
	}

	// A missing key is a miss, not an error.
	if _, ok, err := NewCache(&fakeCommands{getErr: redis.Nil}).Get(ctx, "k"); ok || err != nil {
		t.Fatalf("miss = %v %v", ok, err)
	}
	boom := errors.New("redis down")
	if _, _, err := NewCache(&fakeCommands{getErr: boom}).Get(ctx, "k"); !errors.Is(err, boom) {
		t.Fatalf("get error = %v", err)
	}

	f = &fakeCommands{}
	c := NewCache(f)
	if err := c.Set(ctx, "k", []byte("x"), time.Minute); err != nil || f.setKey != "k" || f.setTTL != time.Minute {
		t.Fatalf("Set: %v key=%q ttl=%v", err, f.setKey, f.setTTL)
	}
	if err := c.Del(ctx, "k"); err != nil || len(f.deleted) != 1 || f.deleted[0] != "k" {
		t.Fatalf("Del: %v %v", err, f.deleted)
	}
	if err := NewCache(&fakeCommands{delErr: boom}).Del(ctx, "k"); !errors.Is(err, boom) {
		t.Fatalf("del error = %v", err)
	}
}
