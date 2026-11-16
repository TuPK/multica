package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// stubMFADisabledNotice replaces the disable-notice sender for one test. It
// returns a channel that receives the address each call was made for.
func stubMFADisabledNotice(t *testing.T, sendErr error) <-chan string {
	t.Helper()
	sent := make(chan string, 1)
	prev := sendMFADisabledNotice
	sendMFADisabledNotice = func(_ *service.EmailService, to string) error {
		sent <- to
		return sendErr
	}
	t.Cleanup(func() { sendMFADisabledNotice = prev })
	return sent
}

func awaitNotice(t *testing.T, sent <-chan string) string {
	t.Helper()
	select {
	case to := <-sent:
		return to
	case <-time.After(5 * time.Second):
		t.Fatal("no disable notice was sent")
		return ""
	}
}

func TestMFADisableSendsTheNoticeToTheUser(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	sent := stubMFADisabledNotice(t, nil)

	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": totpAt(t, secret, 0)})).
		Want(http.StatusNoContent)

	if to := awaitNotice(t, sent); to != u.Email {
		t.Fatalf("notice sent to %q, want the user's address", to)
	}
}

func TestMFADisableSucceedsWhenTheNoticeFails(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	sent := stubMFADisabledNotice(t, errors.New("smtp unavailable"))

	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": totpAt(t, secret, 0)})).
		Want(http.StatusNoContent)

	awaitNotice(t, sent)
	if n := mfaRowCount(t, u.ID); n != 0 {
		t.Fatalf("user_mfa rows = %d after disable, want 0", n)
	}
}

// Account deletion must remove the second factor whether or not it was
// confirmed: an unconfirmed secret is still a secret.
func TestMFAAccountDeletionRemovesTheSecondFactor(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		name := "unconfirmed"
		if confirmed {
			name = "confirmed"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeletionFixture(t)
			t.Setenv(mfa.EnvKey, mfaTestKey)
			u := mfaUser{ID: f.userID, Email: f.email}
			if confirmed {
				confirmedMFA(t, u)
			} else {
				dbfx.Exec(t, `INSERT INTO user_mfa (user_id, secret_enc) VALUES ($1::uuid, $2)`, u.ID, []byte{0})
			}
			if n := mfaRowCount(t, u.ID); n != 1 {
				t.Fatalf("setup: user_mfa rows = %d, want 1", n)
			}

			makeDue(t, f.userID)
			done, err := testHandler.DeleteAccount(context.Background(), f.userID, time.Now())
			if err != nil || !done {
				t.Fatalf("DeleteAccount = %v, %v; want true, nil", done, err)
			}
			if n := mfaRowCount(t, u.ID); n != 0 {
				t.Fatalf("user_mfa rows = %d after account deletion, want 0", n)
			}
		})
	}
}
