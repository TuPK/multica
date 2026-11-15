package platformauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// openRedis skips the test unless PLATFORM_TEST_REDIS_URL names a Redis the
// test may write to. Every key the tests use carries a random prefix.
func openRedis(t *testing.T) redis.Cmdable {
	t.Helper()
	url := os.Getenv("PLATFORM_TEST_REDIS_URL")
	if url == "" {
		t.Skip("PLATFORM_TEST_REDIS_URL is not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("PLATFORM_TEST_REDIS_URL cannot be parsed")
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func randomName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestRedisCodesSaveAttemptDelete(t *testing.T) {
	codes := NewRedisCodes(openRedis(t))
	ctx := context.Background()
	name := randomName(t)
	t.Cleanup(func() { codes.Delete(ctx, name) })

	if _, _, found, err := codes.Attempt(ctx, name); err != nil || found {
		t.Fatalf("Attempt without a code: found=%v err=%v", found, err)
	}
	if err := codes.Save(ctx, name, []byte("mac-one"), time.Minute); err != nil {
		t.Fatal(err)
	}
	for want := 1; want <= 3; want++ {
		mac, n, found, err := codes.Attempt(ctx, name)
		if err != nil || !found || n != want || string(mac) != "mac-one" {
			t.Fatalf("attempt %d: mac=%q n=%d found=%v err=%v", want, mac, n, found, err)
		}
	}
	// Saving again replaces the code and resets the counter.
	if err := codes.Save(ctx, name, []byte("mac-two"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if mac, n, _, _ := codes.Attempt(ctx, name); n != 1 || string(mac) != "mac-two" {
		t.Fatalf("after a second Save: mac=%q n=%d", mac, n)
	}
	if err := codes.Delete(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, _, found, _ := codes.Attempt(ctx, name); found {
		t.Fatal("code survived Delete")
	}
}

func TestRedisCodesExpire(t *testing.T) {
	codes := NewRedisCodes(openRedis(t))
	ctx := context.Background()
	name := randomName(t)
	t.Cleanup(func() { codes.Delete(ctx, name) })
	if err := codes.Save(ctx, name, []byte("m"), 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, _, found, _ := codes.Attempt(ctx, name); found {
		t.Fatal("code outlived its TTL")
	}
}

func TestRedisLimiter(t *testing.T) {
	rdb := openRedis(t)
	l := NewRedisLimiter(rdb)
	ctx := context.Background()
	key := "mpa:test:" + randomName(t)
	t.Cleanup(func() { rdb.Del(ctx, key) })
	for i := 1; i <= 3; i++ {
		ok, err := l.Allow(ctx, key, 3, time.Minute)
		if err != nil || !ok {
			t.Fatalf("request %d: ok=%v err=%v", i, ok, err)
		}
	}
	if ok, _ := l.Allow(ctx, key, 3, time.Minute); ok {
		t.Fatal("fourth request was allowed")
	}
	if ttl := rdb.TTL(ctx, key).Val(); ttl <= 0 {
		t.Fatalf("counter has no expiry (ttl %v)", ttl)
	}
}
