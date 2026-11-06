package service

import (
	"context"
	"errors"
	"testing"
)

func TestParseChatReplyKind(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    string
		wantErr bool
	}{
		"empty is a clarification": {"", ChatReplyKindClarify, false},
		"ack":                      {"ack", ChatReplyKindAck, false},
		"fact":                     {" fact ", ChatReplyKindFact, false},
		"assessment":               {"assessment", ChatReplyKindAssessment, false},
		"unknown kind is refused":  {"opinion", "", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseChatReplyKind(tc.in)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("ParseChatReplyKind(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// The verifier names the claims no answer supports, and returns none when every
// claim is supported.
func TestVerifyAssessmentNamesUnsupportedClaims(t *testing.T) {
	answers := map[string][]string{"child-1": {"[comment c1] The tax rules are the risk."}}

	unsupported, err := verifyAssessment(context.Background(),
		fakeJSONModel{enabled: true, reply: `{"unsupported": ["the launch is 90 percent done"]}`}, answers, "assessment")
	if err != nil || len(unsupported) != 1 || unsupported[0] != "the launch is 90 percent done" {
		t.Fatalf("unsupported = %v, err = %v; want the one claim", unsupported, err)
	}

	clean, err := verifyAssessment(context.Background(), fakeJSONModel{enabled: true, reply: `{"unsupported": []}`}, answers, "assessment")
	if err != nil || len(clean) != 0 {
		t.Fatalf("clean = %v, err = %v; want none", clean, err)
	}

	if _, err := verifyAssessment(context.Background(), fakeJSONModel{enabled: true, err: errors.New("down")}, answers, "assessment"); err == nil {
		t.Fatal("a failed verifier call must return an error, so the caller can decide to allow the reply")
	}
	if _, err := verifyAssessment(context.Background(), fakeJSONModel{enabled: true, reply: `not json`}, answers, "assessment"); err == nil {
		t.Fatal("a non-JSON verifier reply must be an error")
	}
}
