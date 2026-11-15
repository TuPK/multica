package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// AccountChangesChannel is the pub/sub channel on which the platform service
// announces that a user's account state or sessions changed. The platform
// publishes it from platform-management/internal/accountcache; that package is
// internal to its module, so the name is declared again here and the two must
// stay equal.
const AccountChangesChannel = "mul:acct:changed"

// accountChangeRetryDelay is the pause before a lost subscription is opened
// again.
const accountChangeRetryDelay = time.Second

// accountChange is the message on AccountChangesChannel. Kind is "status" or
// "sessions"; both close the user's sockets, so Kind is only logged.
type accountChange struct {
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
}

// UserDisconnector closes a user's open connections. *Hub is one.
type UserDisconnector interface {
	DisconnectUser(userID string)
}

// AccountChangeStream is one open subscription.
type AccountChangeStream interface {
	// Receive blocks for the next payload.
	Receive(ctx context.Context) (string, error)
	Close() error
}

// AccountChangeSource opens subscriptions to AccountChangesChannel.
type AccountChangeSource interface {
	Subscribe(ctx context.Context) (AccountChangeStream, error)
}

// AccountChangeListener closes the sockets of every user the platform reports
// as changed. A suspended or signed-out user must not keep a socket that was
// authenticated before the change.
type AccountChangeListener struct {
	Source       AccountChangeSource
	Disconnector UserDisconnector
	Log          *slog.Logger
	// RetryDelay is the pause before a failed subscription is retried. Zero
	// means one second.
	RetryDelay time.Duration
}

// Run reads messages until ctx ends. A message that cannot be read is logged
// and skipped. A lost subscription is opened again after RetryDelay; messages
// published while it was down are not replayed, as Redis pub/sub keeps none.
func (l *AccountChangeListener) Run(ctx context.Context) {
	log := l.Log
	if log == nil {
		log = slog.Default()
	}
	delay := l.RetryDelay
	if delay <= 0 {
		delay = accountChangeRetryDelay
	}

	for ctx.Err() == nil {
		stream, err := l.Source.Subscribe(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("account changes: subscribe failed; retrying", "error", err, "retry_in", delay.String())
			if !sleep(ctx, delay) {
				return
			}
			continue
		}
		stopped := l.read(ctx, stream, log)
		_ = stream.Close()
		if stopped || ctx.Err() != nil {
			return
		}
		if !sleep(ctx, delay) {
			return
		}
	}
}

// read handles messages until the stream fails. It reports true when the
// listener must stop for good, which is when the Redis client was closed.
func (l *AccountChangeListener) read(ctx context.Context, stream AccountChangeStream, log *slog.Logger) (stop bool) {
	for {
		payload, err := stream.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, redis.ErrClosed) {
				return true
			}
			log.Warn("account changes: receive failed; resubscribing", "error", err)
			return false
		}
		var m accountChange
		if err := json.Unmarshal([]byte(payload), &m); err != nil || m.UserID == "" {
			log.Warn("account changes: malformed message skipped", "error", err, "bytes", len(payload))
			continue
		}
		log.Info("account changes: disconnecting user", "user_id", m.UserID, "kind", m.Kind)
		l.Disconnector.DisconnectUser(m.UserID)
	}
}

// sleep waits for d and reports false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// RedisAccountChangeSource subscribes on a Redis client. Use a client that is
// not shared with request-path commands: a subscription holds its connection.
type RedisAccountChangeSource struct{ client redis.UniversalClient }

// NewRedisAccountChangeSource returns a source on client.
func NewRedisAccountChangeSource(client redis.UniversalClient) *RedisAccountChangeSource {
	return &RedisAccountChangeSource{client: client}
}

// Subscribe implements AccountChangeSource. go-redis reconnects a dropped
// subscription on its own; the listener opens a new one only after an error
// that Receive returns.
func (s *RedisAccountChangeSource) Subscribe(ctx context.Context) (AccountChangeStream, error) {
	return &redisAccountChangeStream{ps: s.client.Subscribe(ctx, AccountChangesChannel)}, nil
}

type redisAccountChangeStream struct{ ps *redis.PubSub }

// Receive does not rely on the client to notice a cancelled ctx: a blocked read
// is ended by closing the subscription.
func (s *redisAccountChangeStream) Receive(ctx context.Context) (string, error) {
	stop := context.AfterFunc(ctx, func() { _ = s.ps.Close() })
	defer stop()
	msg, err := s.ps.ReceiveMessage(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	return msg.Payload, nil
}

func (s *redisAccountChangeStream) Close() error { return s.ps.Close() }
