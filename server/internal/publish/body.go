package publish

import (
	"time"
)

// PostRequest is what one channel's post becomes on the wire. Field names are
// Ayrshare's (see POST /api/post): post, platforms, mediaUrls, scheduleDate and
// idempotencyKey.
type PostRequest struct {
	Text        string
	Hashtags    []string
	Platform    string
	MediaURL    string
	ScheduledAt *time.Time
	// IdempotencyKey is the attempt id on a real send. The dry run leaves it empty.
	IdempotencyKey string
}

// Body builds the Ayrshare request body for a post. Optional fields are left out
// when empty, so the body is the exact request that would be sent.
func (r PostRequest) Body() (map[string]any, error) {
	name, err := ayrshareName(r.Platform)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"post":      ComposeText(r.Text, r.Hashtags),
		"platforms": []string{name},
	}
	if r.MediaURL != "" {
		body["mediaUrls"] = []string{r.MediaURL}
	}
	if r.ScheduledAt != nil {
		body["scheduleDate"] = r.ScheduledAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if r.IdempotencyKey != "" {
		body["idempotencyKey"] = r.IdempotencyKey
	}
	return body, nil
}
