package proposalkinds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/publish"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// KeyPublish is the publish kind's key, as stored in proposal.kind.
const KeyPublish = "publish"

// The publish kind: a squad asks the boss to publish one content item on
// channels. Approval enqueues one publish_attempt per channel; the publisher sends
// them. Rejection and revision enqueue nothing. Standing approval never decides
// it (see ForbiddenForStandingApproval).

const (
	publishMaxChannels   = 10
	publishMaxItemIDLen  = 200
	publishMaxHashtags   = 30
	publishMaxMediaURLen = 2000
)

type publishKind struct{}

func (publishKind) Key() string         { return KeyPublish }
func (publishKind) FiledByAgents() bool { return true }

// Validate checks the payload against the workspace's channels and returns its
// canonical form. Each channel's platform and account name come from the stored
// channel row, not from the filing, so the boss sees the real account. Text is
// checked against the channel's limit here, so a filing that would be refused by
// the platform fails at filing.
func (publishKind) Validate(ctx context.Context, env Env, payload []byte) ([]byte, error) {
	var p publish.Payload
	if err := decodeObject(payload, &p); err != nil {
		return nil, err
	}
	p.ItemID = strings.TrimSpace(p.ItemID)
	if p.ItemID == "" || len(p.ItemID) > publishMaxItemIDLen {
		return nil, fmt.Errorf("publish: item_id is required and must be at most %d characters", publishMaxItemIDLen)
	}
	if p.Version < 1 {
		return nil, errors.New("publish: version must be 1 or more")
	}
	if !publish.ValidMarket(p.Market) {
		return nil, errors.New("publish: market must be vn or intl")
	}
	scheduled, err := publish.ParseScheduledAt(p.ScheduledAt)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if scheduled != nil {
		canonical := scheduled.Format(time.RFC3339)
		p.ScheduledAt = &canonical
	}
	if len(p.Channels) == 0 || len(p.Channels) > publishMaxChannels {
		return nil, fmt.Errorf("publish: channels must hold 1 to %d entries", publishMaxChannels)
	}

	seen := make(map[string]bool, len(p.Channels))
	for i := range p.Channels {
		ch := &p.Channels[i]
		id, err := util.ParseUUID(ch.ChannelID)
		if err != nil {
			return nil, errors.New("publish: channel_id must name a channel of this workspace")
		}
		key := util.UUIDToString(id)
		if seen[key] {
			return nil, errors.New("publish: each channel may appear once per item")
		}
		seen[key] = true

		row, err := env.Q.GetPublishChannelInWorkspace(ctx, db.GetPublishChannelInWorkspaceParams{ID: id, WorkspaceID: env.WorkspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("publish: channel_id must name a channel of this workspace")
		}
		if err != nil {
			return nil, fmt.Errorf("publish: load channel: %w", err)
		}
		if row.Market != p.Market {
			return nil, fmt.Errorf("publish: channel %s is on market %s, not %s", row.AccountName, row.Market, p.Market)
		}
		ch.ChannelID = key
		ch.Platform = row.Platform
		ch.AccountName = row.AccountName

		ch.Text = strings.TrimSpace(ch.Text)
		if ch.Text == "" {
			return nil, errors.New("publish: text is required for every channel")
		}
		hashtags := make([]string, 0, len(ch.Hashtags))
		for _, h := range ch.Hashtags {
			if h = strings.TrimSpace(h); h != "" {
				hashtags = append(hashtags, h)
			}
		}
		if len(hashtags) > publishMaxHashtags {
			return nil, fmt.Errorf("publish: at most %d hashtags per channel", publishMaxHashtags)
		}
		ch.Hashtags = hashtags
		limits, _ := publish.LimitsFor(row.Platform)
		if n := len([]rune(publish.ComposeText(ch.Text, ch.Hashtags))); n > limits.MaxChars {
			return nil, fmt.Errorf("publish: text for %s is %d characters; the limit is %d", row.Platform, n, limits.MaxChars)
		}
		if ch.MediaURL != nil {
			url := strings.TrimSpace(*ch.MediaURL)
			if url == "" {
				ch.MediaURL = nil
			} else if !strings.HasPrefix(url, "https://") || len(url) > publishMaxMediaURLen {
				return nil, errors.New("publish: media_url must be an https URL")
			} else {
				ch.MediaURL = &url
			}
		}
	}
	return json.Marshal(p)
}

func (publishKind) Summarize(payload []byte) string {
	var p publish.Payload
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	return fmt.Sprintf("Publish %s, version %d, to %d channel(s)", p.ItemID, p.Version, len(p.Channels))
}

// Apply enqueues one attempt per channel inside the approval transaction. The
// insert ignores a row that already exists for the same item, channel and
// version, so an approval of a version that was already sent enqueues nothing.
func (publishKind) Apply(ctx context.Context, q *db.Queries, d Decision) (Effect, error) {
	var p publish.Payload
	if err := decodeObject(d.Proposal.Payload, &p); err != nil {
		return Effect{}, err
	}
	approver := ""
	if d.DeciderID.Valid {
		approver = util.UUIDToString(d.DeciderID)
	}
	for _, ch := range p.Channels {
		channelID, err := util.ParseUUID(ch.ChannelID)
		if err != nil {
			return Effect{}, fmt.Errorf("publish: channel_id: %w", err)
		}
		if _, err := q.InsertPublishAttempt(ctx, db.InsertPublishAttemptParams{
			WorkspaceID: d.WorkspaceID,
			ProposalID:  d.Proposal.ID,
			ItemID:      p.ItemID,
			ChannelID:   channelID,
			Version:     p.Version,
			ApprovedBy:  approver,
		}); err != nil {
			return Effect{}, fmt.Errorf("publish: enqueue attempt: %w", err)
		}
	}
	return Effect{}, nil
}

func (publishKind) Reject(ctx context.Context, q *db.Queries, d Decision) error { return nil }
