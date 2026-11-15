// Package jobs runs named jobs on a schedule. A Postgres advisory lock per job
// name lets several service instances share a schedule: one instance runs a
// job at a time and the others skip that tick.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// lockKeyPrefix keeps the platform's lock keys apart from other advisory-lock
// users in the same database.
const lockKeyPrefix = "platform-management/job/"

// Locker grants a named lock to one holder at a time without blocking.
type Locker interface {
	// TryLock returns ok=false when another holder has the name. When ok is
	// true, release must be called once the job is done.
	TryLock(ctx context.Context, name string) (release func(), ok bool, err error)
}

type job struct {
	name  string
	every time.Duration
	fn    func(context.Context) error
}

// Runner holds the registered jobs and runs them.
type Runner struct {
	locker Locker

	mu      sync.Mutex
	started bool
	jobs    []job
	names   map[string]bool
}

func New(locker Locker) *Runner {
	return &Runner{locker: locker, names: map[string]bool{}}
}

// Register adds a job that runs every interval. The name is lowercase letters,
// digits, '.', '_' and '-', starts with a letter, and is at most 63 bytes. It
// must be unique in the runner. Register fails once Run has started.
func (r *Runner) Register(name string, every time.Duration, fn func(context.Context) error) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("job name %q: must match %s", name, nameRE)
	}
	if every <= 0 {
		return fmt.Errorf("job %q: interval must be positive", name)
	}
	if fn == nil {
		return fmt.Errorf("job %q: function is nil", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("job %q: Register called after Run started", name)
	}
	if r.names[name] {
		return fmt.Errorf("job %q: already registered", name)
	}
	r.names[name] = true
	r.jobs = append(r.jobs, job{name: name, every: every, fn: fn})
	return nil
}

// Run starts every registered job, each once at once and then on its interval,
// and returns nil when ctx ends and the running jobs have returned. With no job
// registered it waits for ctx.
func (r *Runner) Run(ctx context.Context) error {
	r.mu.Lock()
	r.started = true
	jobs := append([]job(nil), r.jobs...)
	r.mu.Unlock()

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.runOnce(ctx, j)
			t := time.NewTicker(j.every)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					r.runOnce(ctx, j)
				}
			}
		}()
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}

// runOnce runs j if it can take the job's lock. A failed or panicking job is
// logged and does not stop the schedule.
func (r *Runner) runOnce(ctx context.Context, j job) {
	release, ok, err := r.locker.TryLock(ctx, j.name)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("job lock failed", "job", j.name, "error", err)
		}
		return
	}
	if !ok {
		slog.Debug("job skipped: another instance holds the lock", "job", j.name)
		return
	}
	defer release()

	defer func() {
		if p := recover(); p != nil {
			slog.Error("job panicked", "job", j.name, "panic", p)
		}
	}()
	start := time.Now()
	if err := j.fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("job failed", "job", j.name, "error", err, "took", time.Since(start))
		return
	}
	slog.Debug("job finished", "job", j.name, "took", time.Since(start))
}

type poolLocker struct{ pool *pgxpool.Pool }

// NewPoolLocker returns a Locker backed by session-level advisory locks. Each
// held lock keeps one pool connection until it is released.
func NewPoolLocker(pool *pgxpool.Pool) Locker { return poolLocker{pool: pool} }

const (
	tryLockSQL = `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`
	unlockSQL  = `SELECT pg_advisory_unlock(hashtextextended($1, 0))`
)

func (l poolLocker) TryLock(ctx context.Context, name string) (func(), bool, error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := lockKeyPrefix + name
	var got bool
	if err := conn.QueryRow(ctx, tryLockSQL, key).Scan(&got); err != nil {
		// The lock state is unknown; closing the session drops any lock it holds.
		closeConn(conn)
		return nil, false, err
	}
	if !got {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		// ctx may already be cancelled, so the unlock gets its own deadline.
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(uctx, unlockSQL, key).Scan(&unlocked); err != nil || !unlocked {
			closeConn(conn)
			return
		}
		conn.Release()
	}, true, nil
}

func closeConn(c *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Hijack().Close(ctx)
}
