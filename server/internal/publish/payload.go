package publish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Payload is the publish proposal's payload (the publish kind's fields). Each
// channel entry carries its own text, so the publisher reads a channel's
// content from the approved proposal and the attempt needs no text column.
type Payload struct {
	ItemID      string           `json:"item_id"`
	Version     int32            `json:"version"`
	Market      string           `json:"market"`
	ScheduledAt *string          `json:"scheduled_at"`
	Channels    []PayloadChannel `json:"channels"`
}

// PayloadChannel is one channel's content within a publish payload.
type PayloadChannel struct {
	ChannelID   string   `json:"channel_id"`
	Platform    string   `json:"platform"`
	AccountName string   `json:"account_name"`
	Text        string   `json:"text"`
	MediaURL    *string  `json:"media_url"`
	Hashtags    []string `json:"hashtags"`
}

// DecodePayload parses a stored publish payload strictly. Unknown fields are
// refused, as they are at filing.
func DecodePayload(raw []byte) (Payload, error) {
	var p Payload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Payload{}, fmt.Errorf("publish payload: %w", err)
	}
	return p, nil
}

// ParseScheduledAt reads a payload's scheduled_at. A nil value means post now.
func ParseScheduledAt(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, fmt.Errorf("scheduled_at must be an RFC 3339 time: %w", err)
	}
	u := t.UTC()
	return &u, nil
}
