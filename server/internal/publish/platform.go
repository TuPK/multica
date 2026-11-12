// Package publish connects workspace publishing channels to Ayrshare: the
// channel records, the Ayrshare client, the proposal payload, and the publisher
// that sends approved channel versions exactly once.
package publish

import (
	"fmt"
	"strings"
)

// Platforms a channel can be on. The values are the names the API and the
// database use; Ayrshare's own name differs for X (see ayrshareName).
const (
	PlatformX        = "x"
	PlatformLinkedIn = "linkedin"
	PlatformFacebook = "facebook"
	PlatformTikTok   = "tiktok"
	PlatformYouTube  = "youtube"
	PlatformThreads  = "threads"
	PlatformBluesky  = "bluesky"
)

// Markets a channel and a content version are written for.
const (
	MarketVN   = "vn"
	MarketIntl = "intl"
)

// Channel statuses, as stored and returned.
const (
	StatusNotConnected = "not_connected"
	StatusReady        = "ready"
	StatusError        = "error"
	StatusExpired      = "expired"
)

// Attempt statuses.
const (
	AttemptQueued = "queued"
	AttemptPosted = "posted"
	AttemptFailed = "failed"
)

// Limits are what a channel accepts. Max characters are the platform's published
// post limits; they were not re-checked against Ayrshare in this change. Video
// limits are unset because video is out of phase 1 scope.
type Limits struct {
	MaxChars        int     `json:"max_chars"`
	MaxVideoSeconds *int    `json:"max_video_seconds"`
	Aspect          *string `json:"aspect"`
}

type platformSpec struct {
	ayrshareName string
	limits       Limits
}

var platforms = map[string]platformSpec{
	PlatformX:        {ayrshareName: "twitter", limits: Limits{MaxChars: 280}},
	PlatformLinkedIn: {ayrshareName: "linkedin", limits: Limits{MaxChars: 3000}},
	PlatformFacebook: {ayrshareName: "facebook", limits: Limits{MaxChars: 63206}},
	PlatformTikTok:   {ayrshareName: "tiktok", limits: Limits{MaxChars: 2200}},
	PlatformYouTube:  {ayrshareName: "youtube", limits: Limits{MaxChars: 5000}},
	PlatformThreads:  {ayrshareName: "threads", limits: Limits{MaxChars: 500}},
	PlatformBluesky:  {ayrshareName: "bluesky", limits: Limits{MaxChars: 300}},
}

// ValidPlatform reports whether p is a platform a channel can be on.
func ValidPlatform(p string) bool {
	_, ok := platforms[p]
	return ok
}

// ValidMarket reports whether m is a market a channel can be on.
func ValidMarket(m string) bool {
	return m == MarketVN || m == MarketIntl
}

// LimitsFor returns the limits of a platform and whether the platform is known.
func LimitsFor(platform string) (Limits, bool) {
	spec, ok := platforms[platform]
	if !ok {
		return Limits{}, false
	}
	return spec.limits, true
}

// ayrshareName is the platform name the Ayrshare post body uses. Ayrshare
// documents "twitter" for X/Twitter; the other names match ours.
func ayrshareName(platform string) (string, error) {
	spec, ok := platforms[platform]
	if !ok {
		return "", fmt.Errorf("publish: unknown platform %q", platform)
	}
	return spec.ayrshareName, nil
}

// ComposeText is the post text sent to Ayrshare: the text, then the hashtags on
// their own line. Hashtags are given without the leading '#', which is added.
func ComposeText(text string, hashtags []string) string {
	text = strings.TrimSpace(text)
	tags := make([]string, 0, len(hashtags))
	for _, h := range hashtags {
		h = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(h), "#"))
		if h != "" {
			tags = append(tags, "#"+h)
		}
	}
	if len(tags) == 0 {
		return text
	}
	return text + "\n\n" + strings.Join(tags, " ")
}
