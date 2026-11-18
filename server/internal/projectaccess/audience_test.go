package projectaccess

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func audienceOf(t *testing.T, e *env, projectID string) Recipients {
	t.Helper()
	r, err := Audience(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, projectID))
	if err != nil {
		t.Fatalf("Audience: %v", err)
	}
	sort.Strings(r.UserIDs)
	return r
}

func (e *env) ids(kinds ...string) []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, e.users[k])
	}
	sort.Strings(out)
	return out
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAudienceOfRestrictedProject(t *testing.T) {
	e := newEnv(t)
	p := e.project("restricted audience", "restricted")

	// Nobody is granted: only owner and admin.
	got := audienceOf(t, e, p)
	if got.Workspace || !sameIDs(got.UserIDs, e.ids("owner", "admin")) {
		t.Fatalf("no grants: got %+v, want owner and admin only", got)
	}

	// A non-guest member and a granted guest join; an ungranted guest and a
	// viewer-role non-member of the project do not.
	e.grant(p, e.users["member"])
	e.grant(p, e.users["guest"])
	got = audienceOf(t, e, p)
	if got.Workspace || !sameIDs(got.UserIDs, e.ids("owner", "admin", "member", "guest")) {
		t.Fatalf("granted member and guest: got %+v", got)
	}
}

func TestAudienceOfWorkspaceProject(t *testing.T) {
	e := newEnv(t)
	p := e.project("open audience", "workspace")

	got := audienceOf(t, e, p)
	if !got.Workspace || len(got.UserIDs) != 0 {
		t.Fatalf("no grants: got %+v, want whole workspace and no extras", got)
	}

	// Only the granted guest needs an explicit entry: guests are not in the
	// workspace room. A granted non-guest is already in it.
	e.grant(p, e.users["guest"])
	e.grant(p, e.users["member"])
	got = audienceOf(t, e, p)
	if !got.Workspace || !sameIDs(got.UserIDs, e.ids("guest")) {
		t.Fatalf("granted guest: got %+v, want workspace plus the guest", got)
	}
}

func TestAudienceGuestNotGrantedIsExcluded(t *testing.T) {
	e := newEnv(t)
	for _, access := range []string{"workspace", "restricted"} {
		p := e.project("ungranted-guest-"+access, access)
		e.grant(p, e.users["viewer"])
		got := audienceOf(t, e, p)
		for _, id := range got.UserIDs {
			if id == e.users["guest"] {
				t.Errorf("%s project: ungranted guest is a recipient: %+v", access, got)
			}
		}
	}
}

func TestAudienceUsersEnumeratesWorkspaceProject(t *testing.T) {
	e := newEnv(t)
	p := e.project("enumerated", "workspace")
	e.grant(p, e.users["guest"])
	got, err := AudienceUsers(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, p))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if want := e.ids("owner", "admin", "member", "viewer", "guest"); !sameIDs(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Audience has no visibility rule of its own: for every access value, grant
// and viewer kind it agrees with the Scope that ForViewer computes.
func TestAudienceAgreesWithScope(t *testing.T) {
	e := newEnv(t)
	for _, access := range []string{"workspace", "restricted", "something-else"} {
		for _, k := range viewerKinds {
			for _, granted := range []bool{false, true} {
				p := e.project("parity-"+access+"-"+k, access)
				if granted {
					e.grant(p, e.users[k])
				}
				want := scopeFor(t, e, k).Allows(pgUUID(t, p))
				users, err := AudienceUsers(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, p))
				if err != nil {
					t.Fatal(err)
				}
				got := false
				for _, u := range users {
					got = got || u == e.users[k]
				}
				if got != want {
					t.Errorf("access=%s kind=%s granted=%v: audience has user=%v, scope allows=%v", access, k, granted, got, want)
				}
			}
		}
	}
}

func TestAudienceOfMissingProject(t *testing.T) {
	e := newEnv(t)
	_, err := Audience(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, "00000000-0000-4000-8000-000000000001"))
	if err != ErrProjectNotFound {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

func TestAudienceOfNoProject(t *testing.T) {
	e := newEnv(t)
	got, err := Audience(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgtype.UUID{})
	if err != nil || !got.Workspace || len(got.UserIDs) != 0 {
		t.Fatalf("got %+v, %v; want whole workspace", got, err)
	}
}
