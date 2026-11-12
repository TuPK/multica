package publish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// maxErrorLen bounds the error text stored on an attempt.
const maxErrorLen = 500

// ProcessNext sends one queued attempt and records the result. It reports false
// when nothing is queued, or when no workspace has an Ayrshare key.
//
// Each run first queues again any attempt left sending for longer than StaleAfter.
// That is how a crash in the middle of a send is recovered: the attempt is resent
// with its attempt id as the idempotency key. Ayrshare's answer to a repeated key
// is not documented, so a duplicate refusal (code 110) fails the attempt with a
// message to check the channel. It is never recorded as posted.
//
// The claim is one statement that commits the sending status before the HTTP
// call starts. No database transaction is held during the call. The result is
// written by a second statement that only applies while the attempt is sending.
// SKIP LOCKED in the claim keeps two server instances from sending one attempt.
func (s *Service) ProcessNext(ctx context.Context) (bool, error) {
	if s.Queries == nil {
		return false, errors.New("publish: service has no database")
	}
	if s.Box == nil {
		return false, nil
	}
	if _, err := s.Queries.RequeueStalePublishAttempts(ctx, pgtype.Timestamptz{
		Time: s.Now().Add(-s.staleAfter()), Valid: true,
	}); err != nil {
		return false, fmt.Errorf("requeue stale publish attempts: %w", err)
	}
	// Only workspaces with a key are claimed. An attempt from a workspace without
	// one stays queued and does not hold up the others.
	workspaces, err := s.Queries.ListPublishConfiguredWorkspaces(ctx)
	if err != nil {
		return false, fmt.Errorf("list configured workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return false, nil
	}
	attempt, err := s.Queries.ClaimQueuedPublishAttemptInWorkspaces(ctx, workspaces)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim publish attempt: %w", err)
	}

	res, sendErr := s.send(ctx, attempt)
	if ctx.Err() != nil {
		// Shutting down mid-send. Queue the attempt now, so the next run sends it
		// without waiting out the stale timeout. Its write uses a context that
		// shutdown does not cancel.
		if err := s.Queries.RequeuePublishAttempt(context.WithoutCancel(ctx), attempt.ID); err != nil {
			return false, fmt.Errorf("requeue interrupted publish attempt: %w", err)
		}
		return false, ctx.Err()
	}
	if errors.Is(sendErr, ErrNotConfigured) {
		// The keys were removed after the claim. Queue the attempt again: it waits
		// for a key rather than failing.
		if err := s.Queries.RequeuePublishAttempt(ctx, attempt.ID); err != nil {
			return false, fmt.Errorf("requeue publish attempt: %w", err)
		}
		return false, nil
	}
	if sendErr != nil {
		if err := s.Queries.MarkPublishAttemptFailed(ctx, db.MarkPublishAttemptFailedParams{
			ID: attempt.ID, Error: truncate(sendErr.Error(), maxErrorLen),
		}); err != nil {
			return false, fmt.Errorf("mark publish attempt failed: %w", err)
		}
		return true, nil
	}
	if err := s.Queries.MarkPublishAttemptPosted(ctx, db.MarkPublishAttemptPostedParams{
		ID: attempt.ID, ExternalPostID: res.ExternalID, ExternalUrl: res.ExternalURL,
	}); err != nil {
		return false, fmt.Errorf("mark publish attempt posted: %w", err)
	}
	return true, nil
}

func (s *Service) staleAfter() time.Duration {
	if s.StaleAfter <= 0 {
		return 10 * time.Minute
	}
	return s.StaleAfter
}

// send publishes one attempt's channel content from the approved proposal and
// returns Ayrshare's answer. It retries only a network error or a 5xx answer,
// up to len(s.Backoff) times. A duplicate refusal (code 110) is final.
func (s *Service) send(ctx context.Context, attempt db.PublishAttempt) (PostResult, error) {
	creds, err := s.credentials(ctx, attempt.WorkspaceID)
	if err != nil {
		return PostResult{}, err
	}
	if creds.AyrshareKey == "" {
		return PostResult{}, ErrNotConfigured
	}
	proposal, err := s.Queries.GetProposalInWorkspace(ctx, db.GetProposalInWorkspaceParams{
		ID: attempt.ProposalID, WorkspaceID: attempt.WorkspaceID,
	})
	if err != nil {
		return PostResult{}, fmt.Errorf("load approved proposal: %w", err)
	}
	payload, err := DecodePayload(proposal.Payload)
	if err != nil {
		return PostResult{}, err
	}
	channelID := util.UUIDToString(attempt.ChannelID)
	var entry *PayloadChannel
	for i := range payload.Channels {
		if payload.Channels[i].ChannelID == channelID {
			entry = &payload.Channels[i]
			break
		}
	}
	if entry == nil {
		return PostResult{}, errors.New("channel is not in the approved proposal")
	}
	channel, err := s.channel(ctx, attempt.WorkspaceID, attempt.ChannelID)
	if err != nil {
		return PostResult{}, fmt.Errorf("channel: %w", err)
	}
	// A channel re-created on another platform since approval is not the one the
	// content was written for, so it is not posted to.
	if channel.Platform != entry.Platform || channel.Market != payload.Market {
		return PostResult{}, errors.New("channel no longer matches the approved platform or market")
	}
	scheduledAt, err := ParseScheduledAt(payload.ScheduledAt)
	if err != nil {
		return PostResult{}, err
	}
	profileKey, err := s.profileKey(ctx, attempt.WorkspaceID)
	if errors.Is(err, errNoProfile) {
		return PostResult{}, errors.New("no Ayrshare profile: connect a channel first")
	}
	if err != nil {
		return PostResult{}, err
	}
	req := PostRequest{
		Text:           entry.Text,
		Hashtags:       entry.Hashtags,
		Platform:       entry.Platform,
		ScheduledAt:    scheduledAt,
		IdempotencyKey: util.UUIDToString(attempt.ID),
	}
	if entry.MediaURL != nil {
		req.MediaURL = *entry.MediaURL
	}
	body, err := req.Body()
	if err != nil {
		return PostResult{}, err
	}
	client := s.clientFor(creds)
	return retryPost(ctx, s.Backoff, func() (PostResult, error) {
		return client.Post(ctx, profileKey, body, entry.Platform)
	})
}

// retryPost calls post once and again after each wait in backoff, while the
// failure is retryable (a network error or a 5xx answer). A final failure, such
// as a 4xx answer, returns at once. Its length is the number of retries.
func retryPost(ctx context.Context, backoff []time.Duration, post func() (PostResult, error)) (PostResult, error) {
	var lastErr error
	for try := 0; try <= len(backoff); try++ {
		if try > 0 {
			if err := sleepCtx(ctx, backoff[try-1]); err != nil {
				return PostResult{}, err
			}
		}
		res, err := post()
		if err == nil {
			return res, nil
		}
		lastErr = err
		var re *RequestError
		if !errors.As(err, &re) || !re.Retryable {
			return PostResult{}, err
		}
	}
	return PostResult{}, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
