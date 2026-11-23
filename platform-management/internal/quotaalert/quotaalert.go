// Package quotaalert sends the warnings at 80 and 100 percent of a counted
// limit (spec R6 section 3c.5, rule 1). The job runs hourly. It sends each
// (workspace, limit, threshold, month) once: the notice is recorded before the
// email goes out, and removed again when the send fails, as the lifecycle
// notices are.
package quotaalert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// JobName and Interval register the job with the platform runner.
const (
	JobName  = "quota_alert"
	Interval = time.Hour
)

// maxRecipients bounds the addresses of one email; Resend takes at most 50.
const maxRecipients = 50

// Gates are the counted limits the platform alerts on: agents, runtimes, issues,
// storage and the autopilot runs of the period (spec R6 section 3c.5, rule 1).
// The API rate is not alerted; it has no count in the database, only a Redis
// counter of the current minute.
var Gates = []string{plan.GateAgents, plan.GateRuntimes, plan.GateIssueCount, plan.GateStorageBytes, plan.GateAutopilotRuns}

// Source reads the workspaces to check, the current count of each gate of one
// workspace, and its effective limits at an instant.
type Source interface {
	Workspaces(ctx context.Context) ([]string, error)
	Counts(ctx context.Context, workspaceID string, now time.Time) (map[string]int64, error)
	Limits(ctx context.Context, workspaceID string, now time.Time) (map[string]*int64, error)
}

// Notices records each warning and reads the workspace's addresses. It is the
// lifecycle notice store.
type Notices interface {
	Contact(ctx context.Context, workspaceID string) (lifecycle.Contact, error)
	InsertNotice(ctx context.Context, n lifecycle.Notice) (bool, error)
	DeleteNotice(ctx context.Context, workspaceID, key string) error
}

// Job checks every workspace. Mail may be nil: then the warnings are recorded
// but no email goes out, the same as the usage notice.
type Job struct {
	Source  Source
	Notices Notices
	Mail    lifecycle.Sender
	AppURL  string
}

// thresholds returns the warnings a use has reached: 80 from 80 percent of the
// limit, and 100 from the limit itself. A zero limit has none.
func thresholds(used, limit int64) []int64 {
	if limit <= 0 {
		return nil
	}
	switch {
	case used*100 >= 100*limit:
		return []int64{80, 100}
	case used*100 >= 80*limit:
		return []int64{80}
	}
	return nil
}

// Run checks every workspace at now and returns how many emails it sent.
func (j Job) Run(ctx context.Context, now time.Time) (int, error) {
	ids, err := j.Source.Workspaces(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	sent := 0
	var errs []error
	for _, ws := range ids {
		n, err := j.checkWorkspace(ctx, ws, now)
		sent += n
		if err != nil {
			errs = append(errs, fmt.Errorf("workspace %s: %w", ws, err))
		}
	}
	return sent, errors.Join(errs...)
}

// Job adapts Run to the runner's job signature.
func (j Job) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := j.Run(ctx, now())
		return err
	}
}

func (j Job) checkWorkspace(ctx context.Context, ws string, now time.Time) (int, error) {
	counts, err := j.Source.Counts(ctx, ws, now)
	if err != nil {
		return 0, err
	}
	limits, err := j.Source.Limits(ctx, ws, now)
	if err != nil {
		return 0, err
	}
	period := now.UTC().Format("2006-01")
	sent := 0
	var errs []error
	for _, gate := range Gates {
		limit := limits[gate]
		if limit == nil {
			continue
		}
		for _, pct := range thresholds(counts[gate], *limit) {
			kind := billing.NoticeKindQuota80
			if pct == 100 {
				kind = billing.NoticeKindQuota100
			}
			ok, err := j.notify(ctx, ws, kind, gate, period, pct, counts[gate], *limit)
			if err != nil {
				errs = append(errs, err)
			}
			if ok {
				sent++
			}
		}
	}
	return sent, errors.Join(errs...)
}

// notify records the warning and emails the workspace's owners and admins. It
// reports whether an email was sent.
func (j Job) notify(ctx context.Context, ws, kind, gate, period string, pct, used, limit int64) (bool, error) {
	n := lifecycle.Notice{WorkspaceID: ws, Key: kind + ":" + gate + ":" + period, Kind: kind}
	inserted, err := j.Notices.InsertNotice(ctx, n)
	if err != nil || !inserted {
		return false, err
	}
	c, err := j.Notices.Contact(ctx, ws)
	if err == nil && len(c.Emails) > 0 && j.Mail != nil {
		subject, body := text(c, j.AppURL, gate, pct, used, limit)
		err = j.Mail.Send(ctx, lifecycle.Message{To: c.Emails[:min(len(c.Emails), maxRecipients)], Subject: subject, Body: body})
		if err == nil {
			return true, nil
		}
	}
	// No address, or a failed send: remove the record so a later run can retry.
	// The caller's context may be what ended, so the delete has its own.
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return false, errors.Join(err, j.Notices.DeleteNotice(dctx, ws, n.Key))
}

// text is a first draft of the warning, for the owner to review.
func text(c lifecycle.Contact, appURL, gate string, pct, used, limit int64) (subject, body string) {
	subject = fmt.Sprintf("Delegora: %s reached %d%% of its %s limit", c.Slug, pct, gate)
	body = fmt.Sprintf("Workspace %s has used %d of %d (%s), %d%% of its limit.\n\nManage the plan: %s\n",
		c.Slug, used, limit, gate, pct, appURL)
	return subject, body
}
