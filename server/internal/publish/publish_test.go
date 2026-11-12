package publish

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fakeAyrshare serves one fixed response and counts the calls it receives. Tests
// never reach the real Ayrshare API: this server stands in for it.
func fakeAyrshare(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32, *Client) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	client := &Client{BaseURL: srv.URL, APIKey: "test-api-key", HTTP: srv.Client()}
	return srv, &calls, client
}

func TestComposeTextAddsHashtagsOnTheirOwnLine(t *testing.T) {
	got := ComposeText("  Shipped the thing.  ", []string{"#golang", " sql ", ""})
	want := "Shipped the thing.\n\n#golang #sql"
	if got != want {
		t.Fatalf("ComposeText = %q, want %q", got, want)
	}
	if got := ComposeText("No tags", nil); got != "No tags" {
		t.Fatalf("ComposeText without tags = %q", got)
	}
}

func TestPostRequestBodyUsesAyrshareFieldNames(t *testing.T) {
	when := time.Date(2026, 7, 8, 12, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	body, err := PostRequest{
		Text:           "Hello",
		Platform:       PlatformX,
		MediaURL:       "https://cdn.example.com/a.png",
		ScheduledAt:    &when,
		IdempotencyKey: "attempt-1",
	}.Body()
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if body["post"] != "Hello" {
		t.Fatalf("post = %v", body["post"])
	}
	platforms, _ := body["platforms"].([]string)
	if len(platforms) != 1 || platforms[0] != "twitter" {
		t.Fatalf("X must map to Ayrshare's twitter, got %v", body["platforms"])
	}
	if media, _ := body["mediaUrls"].([]string); len(media) != 1 || media[0] != "https://cdn.example.com/a.png" {
		t.Fatalf("mediaUrls = %v", body["mediaUrls"])
	}
	if body["scheduleDate"] != "2026-07-08T05:30:00Z" {
		t.Fatalf("scheduleDate must be UTC YYYY-MM-DDThh:mm:ssZ, got %v", body["scheduleDate"])
	}
	if body["idempotencyKey"] != "attempt-1" {
		t.Fatalf("idempotencyKey = %v", body["idempotencyKey"])
	}
}

func TestPostRequestBodyLeavesOptionalFieldsOut(t *testing.T) {
	body, err := PostRequest{Text: "Plain", Platform: PlatformLinkedIn}.Body()
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	for _, key := range []string{"mediaUrls", "scheduleDate", "idempotencyKey"} {
		if _, ok := body[key]; ok {
			t.Fatalf("%s must be absent when empty, body = %v", key, body)
		}
	}
	platforms, _ := body["platforms"].([]string)
	if len(platforms) != 1 || platforms[0] != "linkedin" {
		t.Fatalf("platforms = %v", body["platforms"])
	}
}

func TestClientPostSuccessReturnsThePlatformPost(t *testing.T) {
	_, calls, client := fakeAyrshare(t, http.StatusOK,
		`{"status":"success","id":"ayr-1","postIds":[{"status":"success","id":"123","postUrl":"https://x.com/i/status/123","platform":"twitter"}]}`)

	res, err := client.Post(context.Background(), "profile-key", map[string]any{"post": "Hi"}, PlatformX)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if res.ExternalID != "123" || res.ExternalURL != "https://x.com/i/status/123" || res.Scheduled {
		t.Fatalf("result = %+v", res)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestClientPostScheduledKeepsAyrshareID(t *testing.T) {
	_, _, client := fakeAyrshare(t, http.StatusOK, `{"status":"scheduled","id":"ayr-9","scheduleDate":"2026-07-08T05:30:00Z"}`)
	res, err := client.Post(context.Background(), "k", map[string]any{"post": "later"}, PlatformFacebook)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !res.Scheduled || res.ExternalID != "ayr-9" || res.ExternalURL != "" {
		t.Fatalf("scheduled result = %+v", res)
	}
}

func TestClientPostClassifiesFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		retryable bool
	}{
		{"4xx is final", http.StatusUnprocessableEntity, `{"status":"error","errors":[{"message":"Status is a duplicate."}]}`, false},
		{"401 is final", http.StatusUnauthorized, `{"status":"error","message":"Unauthorized"}`, false},
		{"5xx is retryable", http.StatusInternalServerError, `{"status":"error","message":"boom"}`, true},
		{"2xx refusal is final", http.StatusOK, `{"status":"error","errors":[{"message":"not linked"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, client := fakeAyrshare(t, tc.status, tc.body)
			_, err := client.Post(context.Background(), "k", map[string]any{"post": "x"}, PlatformX)
			var re *RequestError
			if !errors.As(err, &re) {
				t.Fatalf("err = %v, want a RequestError", err)
			}
			if re.Retryable != tc.retryable {
				t.Fatalf("Retryable = %v, want %v", re.Retryable, tc.retryable)
			}
		})
	}
}

func TestClientNetworkErrorIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	client := &Client{BaseURL: url, APIKey: "k", HTTP: &http.Client{Timeout: time.Second}}
	_, err := client.Post(context.Background(), "p", map[string]any{"post": "x"}, PlatformX)
	var re *RequestError
	if !errors.As(err, &re) || !re.Retryable {
		t.Fatalf("network error = %v, want a retryable RequestError", err)
	}
}

func TestClientWithoutKeyNeverCallsOut(t *testing.T) {
	_, calls, client := fakeAyrshare(t, http.StatusOK, `{"status":"success"}`)
	client.APIKey = ""
	_, err := client.Post(context.Background(), "p", map[string]any{"post": "x"}, PlatformX)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("a keyless client reached the network %d times", calls.Load())
	}
}

func TestClientErrorTextNeverCarriesTheKey(t *testing.T) {
	_, _, client := fakeAyrshare(t, http.StatusUnauthorized, `{"status":"error","message":"bad key"}`)
	_, err := client.Post(context.Background(), "profile-secret", map[string]any{"post": "x"}, PlatformX)
	if err == nil || strings.Contains(err.Error(), "test-api-key") || strings.Contains(err.Error(), "profile-secret") {
		t.Fatalf("error must not carry a key: %v", err)
	}
}

func TestRetryPostRetriesServerErrorsTwice(t *testing.T) {
	var calls int
	_, err := retryPost(context.Background(), []time.Duration{0, 0}, func() (PostResult, error) {
		calls++
		return PostResult{}, &RequestError{Status: 503, Message: "down", Retryable: true}
	})
	if err == nil {
		t.Fatal("want the final error")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (one try and two retries)", calls)
	}
}

func TestRetryPostDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	_, err := retryPost(context.Background(), []time.Duration{0, 0}, func() (PostResult, error) {
		calls++
		return PostResult{}, &RequestError{Status: 422, Message: "rejected"}
	})
	if err == nil {
		t.Fatal("want the error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: a 4xx is never retried", calls)
	}
}

func TestRetryPostStopsWhenASecondTryWorks(t *testing.T) {
	var calls int
	res, err := retryPost(context.Background(), []time.Duration{0, 0}, func() (PostResult, error) {
		calls++
		if calls == 1 {
			return PostResult{}, &RequestError{Status: 500, Retryable: true}
		}
		return PostResult{ExternalID: "ok"}, nil
	})
	if err != nil || res.ExternalID != "ok" || calls != 2 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, calls)
	}
}

func TestPresentReportsNotConfiguredWithoutTouchingTheRow(t *testing.T) {
	svc := &Service{}
	c := svc.Present(channelRow("ready"), false)
	if c.Status != StatusError || c.StatusDetail != "Ayrshare not configured" {
		t.Fatalf("status = %q detail = %q", c.Status, c.StatusDetail)
	}
}

func TestDecodePayloadRefusesUnknownFields(t *testing.T) {
	_, err := DecodePayload([]byte(`{"item_id":"a","version":1,"market":"vn","channels":[],"surprise":true}`))
	if err == nil {
		t.Fatal("an unknown field must be refused")
	}
	p, err := DecodePayload([]byte(`{"item_id":"a","version":2,"market":"intl","scheduled_at":null,"channels":[]}`))
	if err != nil || p.Version != 2 || p.ScheduledAt != nil {
		t.Fatalf("valid payload: %+v err=%v", p, err)
	}
}

func TestParseScheduledAtNormalizesToUTC(t *testing.T) {
	in := "2026-07-08T12:30:00+07:00"
	got, err := ParseScheduledAt(&in)
	if err != nil || got == nil || got.Format(time.RFC3339) != "2026-07-08T05:30:00Z" {
		t.Fatalf("got %v err %v", got, err)
	}
	if none, err := ParseScheduledAt(nil); err != nil || none != nil {
		t.Fatalf("nil must mean post now: %v %v", none, err)
	}
	bad := "tomorrow"
	if _, err := ParseScheduledAt(&bad); err == nil {
		t.Fatal("a non-RFC 3339 time must be refused")
	}
}

func TestPayloadRoundTripKeepsChannelContent(t *testing.T) {
	raw, _ := json.Marshal(Payload{ItemID: "post-1", Version: 1, Market: MarketIntl,
		Channels: []PayloadChannel{{ChannelID: "c1", Platform: PlatformX, Text: "t", Hashtags: []string{"go"}}}})
	back, err := DecodePayload(raw)
	if err != nil || len(back.Channels) != 1 || back.Channels[0].Text != "t" {
		t.Fatalf("round trip = %+v err=%v", back, err)
	}
}

func channelRow(status string) db.PublishChannel {
	return db.PublishChannel{Platform: PlatformX, Market: MarketIntl, AccountName: "acc", Status: status}
}

// Ayrshare's duplicate code 110 is a final failure, never a success: the docs use
// it for content duplicates, so it does not prove this attempt was accepted.
func TestClientPostDuplicateCodeIsFinalAndNeverPosted(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
		_, calls, client := fakeAyrshare(t, status,
			`{"status":"error","postIds":[],"id":"ayr-x","errors":[{"action":"post","status":"error","code":110,"message":"Status is a duplicate."}]}`)
		res, err := client.Post(context.Background(), "profile-key", map[string]any{"post": "x"}, PlatformX)
		var re *RequestError
		if !errors.As(err, &re) {
			t.Fatalf("status %d: duplicate must be an error, got %v (result %+v)", status, err, res)
		}
		if re.Code != 110 || re.Retryable || !strings.Contains(re.Message, "may already be on the network") {
			t.Fatalf("status %d: error = %+v, want final code 110 with the check-the-channel message", status, re)
		}
		if calls.Load() != 1 {
			t.Fatalf("status %d: duplicate was sent %d times, want 1", status, calls.Load())
		}
	}
}

// A 4xx with another code stays final: no duplicate answer, no retry.
func TestClientPostOtherCodeStaysFinal(t *testing.T) {
	_, calls, client := fakeAyrshare(t, http.StatusBadRequest,
		`{"status":"error","errors":[{"code":156,"message":"Instagram is not linked."}]}`)
	_, err := client.Post(context.Background(), "p", map[string]any{"post": "x"}, PlatformX)
	var re *RequestError
	if !errors.As(err, &re) || re.Retryable || re.Code != 156 {
		t.Fatalf("err = %v, want a final RequestError with code 156", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

// A key Ayrshare echoes back in an error never reaches the error text.
func TestClientErrorTextRedactsEveryKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":"error","message":"bad key ayr-echo-1 x-echo-2 profile-echo-3"}`))
	}))
	t.Cleanup(srv.Close)
	client := &Client{BaseURL: srv.URL, APIKey: "ayr-echo-1", XAPIKey: "x-echo-2", XAPISecret: "x-secret-echo", HTTP: srv.Client()}
	_, err := client.Post(context.Background(), "profile-echo-3", map[string]any{"post": "x"}, PlatformX)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, secret := range []string{"ayr-echo-1", "x-echo-2", "profile-echo-3"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks %q: %v", secret, err)
		}
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error should show the redaction marker: %v", err)
	}
}
