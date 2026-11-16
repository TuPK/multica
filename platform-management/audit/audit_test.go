package audit

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeWriter struct {
	got []Entry
	err error
}

func (f *fakeWriter) InsertAuditEvent(_ context.Context, e Entry) error {
	f.got = append(f.got, e)
	return f.err
}

func TestRedactNestedAndNoMutation(t *testing.T) {
	in := map[string]any{
		"name":     "x",
		"API_KEY":  "k",
		"Password": "p",
		"nested": map[string]any{
			"auth_token": "t",
			"ok":         1,
			"list":       []any{map[string]any{"client_secret": "s", "v": "keep"}},
		},
		"maps":   []map[string]any{{"envVars": "e", "v": 2}},
		"plain":  []any{"a", 1},
		"AWSEnv": map[string]any{"a": "b"},
	}
	out := Redact(in)
	want := map[string]any{
		"name":     "x",
		"API_KEY":  Redacted,
		"Password": Redacted,
		"nested": map[string]any{
			"auth_token": Redacted,
			"ok":         1,
			"list":       []any{map[string]any{"client_secret": Redacted, "v": "keep"}},
		},
		"maps":   []map[string]any{{"envVars": Redacted, "v": 2}},
		"plain":  []any{"a", 1},
		"AWSEnv": Redacted,
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("got %#v\nwant %#v", out, want)
	}
	if in["API_KEY"] != "k" || in["nested"].(map[string]any)["auth_token"] != "t" {
		t.Fatal("input was mutated")
	}
}

func TestRedactNil(t *testing.T) {
	if Redact(nil) != nil {
		t.Fatal("nil in, nil out")
	}
}

func TestRecordRedactsAndWrites(t *testing.T) {
	w := &fakeWriter{}
	err := Record(context.Background(), w, Entry{
		Action: ActionInvitationCreated, ActorType: ActorMember,
		Detail: map[string]any{"token": "abc", "role": "member"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.got) != 1 || w.got[0].Detail["token"] != Redacted || w.got[0].Detail["role"] != "member" {
		t.Fatalf("unexpected: %#v", w.got)
	}
}

func TestRecordValidates(t *testing.T) {
	w := &fakeWriter{}
	if Record(context.Background(), w, Entry{ActorType: ActorMember}) == nil {
		t.Fatal("missing action must fail")
	}
	if Record(context.Background(), w, Entry{Action: ActionMemberLeft}) == nil {
		t.Fatal("missing actor type must fail")
	}
	if len(w.got) != 0 {
		t.Fatal("invalid entry was written")
	}
}

func TestRecordReturnsWriterError(t *testing.T) {
	boom := errors.New("boom")
	w := &fakeWriter{err: boom}
	err := Record(context.Background(), w, Entry{Action: ActionMemberLeft, ActorType: ActorSystem})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestUserMFAResetAction(t *testing.T) {
	if ActionUserMFAReset != "user.mfa_reset" {
		t.Fatalf("ActionUserMFAReset = %q", ActionUserMFAReset)
	}
}
