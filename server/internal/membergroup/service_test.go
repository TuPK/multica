package membergroup

import (
	"context"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Database tests of the service. They run against the database named by
// DATABASE_URL, which must be a dedicated *_test database; without the
// variable they are skipped, so the pure tests of this package still run alone.

var (
	poolOnce sync.Once
	testPool *pgxpool.Pool
	poolErr  error
)

func requireDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL is not set")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("DATABASE_URL is not a URL: %v", err)
	}
	if name := strings.TrimPrefix(u.Path, "/"); name == "multica" || !strings.HasSuffix(name, "_test") {
		t.Fatalf("database %q is not a test database", name)
	}
	poolOnce.Do(func() {
		testPool, poolErr = pgxpool.New(context.Background(), raw)
		if poolErr == nil {
			poolErr = testPool.Ping(context.Background())
		}
	})
	if poolErr != nil {
		t.Fatalf("connect to the test database: %v", poolErr)
	}
	return testPool
}

type rig struct {
	t    *testing.T
	pool *pgxpool.Pool
	f    *testutil.Fixture
	svc  *Service
	ws   pgtype.UUID
	wsID string
	// owner is a member of the workspace with the owner role.
	owner pgtype.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := requireDB(t)
	f := testutil.New(pool, "", "")
	r := &rig{t: t, pool: pool, f: f, svc: New(db.New(pool))}
	r.wsID = f.Workspace(t, "Member groups", "member-groups-"+uuid.NewString()[:8])
	r.ws = util.MustParseUUID(r.wsID)
	// Registered after the workspace, so it runs before the workspace is
	// deleted. The group tables have no foreign key to clean them up.
	f.Cleanup(t, `DELETE FROM member_group_member WHERE workspace_id = $1`, r.wsID)
	f.Cleanup(t, `DELETE FROM member_group WHERE workspace_id = $1`, r.wsID)
	r.owner = r.addMember("owner")
	return r
}

// addMember creates a user and joins them to the workspace with role.
func (r *rig) addMember(role string) pgtype.UUID {
	r.t.Helper()
	user := r.f.User(r.t, "Group test user", testutil.UniqueEmail("mg"))
	r.f.Member(r.t, r.wsID, user, role)
	return util.MustParseUUID(user)
}

func (r *rig) create(name string) db.MemberGroup {
	r.t.Helper()
	g, err := r.svc.Create(context.Background(), r.ws, r.owner, name, "")
	if err != nil {
		r.t.Fatalf("Create(%q): %v", name, err)
	}
	return g
}

func (r *rig) memberRows(group pgtype.UUID) int {
	r.t.Helper()
	return r.f.Count(r.t, `SELECT count(*) FROM member_group_member WHERE workspace_id = $1 AND group_id = $2`,
		r.wsID, util.UUIDToString(group))
}

func TestMemberGroupServiceCreateStoresTheGroup(t *testing.T) {
	r := newRig(t)
	g, err := r.svc.Create(context.Background(), r.ws, r.owner, "Accounting", "the accountants")
	if err != nil {
		t.Fatal(err)
	}
	if g.WorkspaceID != r.ws || g.Name != "Accounting" || g.Description != "the accountants" || g.CreatedBy != r.owner {
		t.Fatalf("group = %+v", g)
	}
	if got := StoredPermissions(g); got == nil || len(got) != 0 {
		t.Fatalf("a new group's permissions = %#v, want an empty set", got)
	}
	back, err := r.svc.Get(context.Background(), r.ws, g.ID)
	if err != nil || back.ID != g.ID {
		t.Fatalf("Get = %+v, %v", back, err)
	}
}

func TestMemberGroupServiceRejectsInvalidNamesAndDescriptions(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.svc.Create(ctx, r.ws, r.owner, "  ", ""); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("blank name: %v", err)
	}
	if _, err := r.svc.Create(ctx, r.ws, r.owner, strings.Repeat("n", 65), ""); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("long name: %v", err)
	}
	if _, err := r.svc.Create(ctx, r.ws, r.owner, "ok", strings.Repeat("d", 281)); !errors.Is(err, ErrDescriptionInvalid) {
		t.Fatalf("long description: %v", err)
	}
	if n := r.f.Count(t, `SELECT count(*) FROM member_group WHERE workspace_id = $1`, r.wsID); n != 0 {
		t.Fatalf("%d groups after refused creations", n)
	}
}

// Rule 1: the name is unique per workspace, ignoring case.
func TestMemberGroupServiceNamesAreUniqueIgnoringCase(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	first := r.create("Finance")
	for _, name := range []string{"Finance", "finance", "FINANCE", "  fInAnCe "} {
		if _, err := r.svc.Create(ctx, r.ws, r.owner, name, ""); !errors.Is(err, ErrNameTaken) {
			t.Errorf("Create(%q): err = %v, want ErrNameTaken", name, err)
		}
	}
	// Another workspace may use the name.
	other := newRig(t)
	if _, err := other.svc.Create(ctx, other.ws, other.owner, "Finance", ""); err != nil {
		t.Fatalf("the same name in another workspace: %v", err)
	}
	// Renaming into a taken name is refused, renaming to its own name in
	// another case is not.
	second := r.create("Support")
	taken := "finance"
	if _, _, err := r.svc.Update(ctx, r.ws, second.ID, UpdateInput{Name: &taken}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("rename into a taken name: %v", err)
	}
	recased := "FINANCE"
	got, changed, err := r.svc.Update(ctx, r.ws, first.ID, UpdateInput{Name: &recased})
	if err != nil || got.Name != "FINANCE" || !reflect.DeepEqual(changed, []string{"name"}) {
		t.Fatalf("recasing its own name: %+v %v %v", got, changed, err)
	}
}

func TestMemberGroupServiceUpdateReportsOnlyChangedFields(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g, err := r.svc.Create(ctx, r.ws, r.owner, "Ops", "operations")
	if err != nil {
		t.Fatal(err)
	}
	same := "Ops"
	desc := "on call"
	got, changed, err := r.svc.Update(ctx, r.ws, g.ID, UpdateInput{Name: &same, Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(changed, []string{"description"}) || got.Description != "on call" || got.Name != "Ops" {
		t.Fatalf("got %+v changed %v, want only description", got, changed)
	}
	// Nothing differs: nothing is written and nothing is reported.
	_, changed, err = r.svc.Update(ctx, r.ws, g.ID, UpdateInput{Description: &desc})
	if err != nil || len(changed) != 0 {
		t.Fatalf("an unchanged update: changed %v, err %v", changed, err)
	}
	if _, _, err := r.svc.Update(ctx, r.ws, g.ID, UpdateInput{Description: ptr(strings.Repeat("x", 281))}); !errors.Is(err, ErrDescriptionInvalid) {
		t.Fatalf("long description: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// Rule 2: only members of the workspace join a group.
func TestMemberGroupServiceOnlyWorkspaceMembersJoin(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g := r.create("Team")
	insider := r.addMember("member")

	outsider := util.MustParseUUID(r.f.User(t, "Outsider", testutil.UniqueEmail("out")))
	otherWS := newRig(t)
	elsewhere := otherWS.addMember("member")

	for name, user := range map[string]pgtype.UUID{"a user with no membership": outsider, "a member of another workspace": elsewhere} {
		if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{insider, user}); !errors.Is(err, ErrNotAMember) {
			t.Errorf("%s: err = %v, want ErrNotAMember", name, err)
		}
	}
	if n := r.memberRows(g.ID); n != 0 {
		t.Fatalf("%d rows after a refused batch; a batch is all or nothing", n)
	}

	added, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{insider})
	if err != nil || len(added) != 1 || added[0] != insider {
		t.Fatalf("AddMembers = %v, %v", added, err)
	}
	// Adding again changes nothing and reports nobody added.
	added, err = r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{insider})
	if err != nil || len(added) != 0 {
		t.Fatalf("a repeated add = %v, %v", added, err)
	}
	if n := r.memberRows(g.ID); n != 1 {
		t.Fatalf("%d rows, want 1", n)
	}
	members, err := r.svc.ListMembers(ctx, r.ws, g.ID)
	if err != nil || len(members) != 1 || members[0].UserID != insider || members[0].AddedBy != r.owner {
		t.Fatalf("ListMembers = %+v, %v", members, err)
	}
}

// A person may be in many groups.
func TestMemberGroupServiceAPersonJoinsManyGroups(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	person := r.addMember("member")
	for _, name := range []string{"One", "Two", "Three"} {
		g := r.create(name)
		if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{person}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// Rule 2: a group holds at most 1000 members.
func TestMemberGroupServiceMemberLimit(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g := r.create("Big")
	fill := func(n int) {
		r.f.Exec(t, `INSERT INTO member_group_member (workspace_id, group_id, user_id)
			SELECT $1, $2, gen_random_uuid() FROM generate_series(1, $3)`,
			r.wsID, util.UUIDToString(g.ID), n)
	}
	fill(MaxMembers - 1)
	one, two := r.addMember("member"), r.addMember("member")

	_, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{one, two})
	var limit *MemberLimitError
	if !errors.As(err, &limit) || limit.Limit != MaxMembers {
		t.Fatalf("two over the last slot: err = %v, want *MemberLimitError with limit %d", err, MaxMembers)
	}
	if n := r.memberRows(g.ID); n != MaxMembers-1 {
		t.Fatalf("%d rows after a refused batch, want %d", n, MaxMembers-1)
	}
	if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{one}); err != nil {
		t.Fatalf("the 1000th member: %v", err)
	}
	if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{two}); !errors.As(err, &limit) {
		t.Fatalf("the 1001st member: err = %v, want *MemberLimitError", err)
	}
	// Someone already in the full group is not a new member.
	if added, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{one}); err != nil || len(added) != 0 {
		t.Fatalf("re-adding a member of a full group = %v, %v", added, err)
	}
}

func TestMemberGroupServiceRemoveMember(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g := r.create("Team")
	person := r.addMember("member")
	if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{person}); err != nil {
		t.Fatal(err)
	}
	removed, _, err := r.svc.RemoveMember(ctx, r.ws, g.ID, person)
	if err != nil || !removed {
		t.Fatalf("RemoveMember = %v, %v", removed, err)
	}
	removed, _, err = r.svc.RemoveMember(ctx, r.ws, g.ID, person)
	if err != nil || removed {
		t.Fatalf("removing someone who is not in the group = %v, %v; want false, nil", removed, err)
	}
	if _, _, err := r.svc.RemoveMember(ctx, r.ws, uuidOf(t), person); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("an unknown group: %v", err)
	}
}

func uuidOf(t *testing.T) pgtype.UUID {
	t.Helper()
	return util.MustParseUUID(uuid.NewString())
}

// Rule 3: the permissions of a group are a subset of the catalogue.
func TestMemberGroupServiceSetPermissions(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g := r.create("Billing")

	updated, old, err := r.svc.SetPermissions(ctx, r.ws, g.ID, []string{"billing.manage", "audit.read", "audit.read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 0 || !reflect.DeepEqual(StoredPermissions(updated), []string{"audit.read", "billing.manage"}) {
		t.Fatalf("old %v new %v", old, StoredPermissions(updated))
	}
	updated, old, err = r.svc.SetPermissions(ctx, r.ws, g.ID, []string{"members.invite"})
	if err != nil || !reflect.DeepEqual(old, []string{"audit.read", "billing.manage"}) ||
		!reflect.DeepEqual(StoredPermissions(updated), []string{"members.invite"}) {
		t.Fatalf("old %v new %v err %v", old, StoredPermissions(updated), err)
	}

	_, _, err = r.svc.SetPermissions(ctx, r.ws, g.ID, []string{"members.invite", "root"})
	var unknown *UnknownPermissionError
	if !errors.As(err, &unknown) || unknown.Key != "root" {
		t.Fatalf("unknown key: err = %v", err)
	}
	back, _ := r.svc.Get(ctx, r.ws, g.ID)
	if !reflect.DeepEqual(StoredPermissions(back), []string{"members.invite"}) {
		t.Fatalf("a refused change was stored: %v", StoredPermissions(back))
	}
}

// Rule 4 and 11: the effective permissions are read from the database on
// every call, so a change takes effect on the next one.
func TestMemberGroupServiceAccessReadsTheDatabaseEveryTime(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	person := r.addMember("member")
	g1, g2 := r.create("Billing"), r.create("Audit")
	if _, _, err := r.svc.SetPermissions(ctx, r.ws, g1.ID, []string{"billing.manage"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.svc.SetPermissions(ctx, r.ws, g2.ID, []string{"audit.read"}); err != nil {
		t.Fatal(err)
	}

	access, err := r.svc.Access(ctx, r.ws, person)
	if err != nil || access.Role != "member" || len(access.Permissions) != 0 {
		t.Fatalf("before joining: %+v, %v", access, err)
	}
	if _, err := r.svc.AddMembers(ctx, r.ws, g1.ID, r.owner, []pgtype.UUID{person}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddMembers(ctx, r.ws, g2.ID, r.owner, []pgtype.UUID{person}); err != nil {
		t.Fatal(err)
	}
	access, err = r.svc.Access(ctx, r.ws, person)
	if want := []string{"audit.read", "billing.manage", "billing.view"}; err != nil || !reflect.DeepEqual(access.Permissions, want) {
		t.Fatalf("in two groups: %+v, %v; want %v", access, err, want)
	}
	if _, _, err := r.svc.RemoveMember(ctx, r.ws, g1.ID, person); err != nil {
		t.Fatal(err)
	}
	access, _ = r.svc.Access(ctx, r.ws, person)
	if !reflect.DeepEqual(access.Permissions, []string{"audit.read"}) {
		t.Fatalf("after leaving the billing group: %v", access.Permissions)
	}
	if _, _, err := r.svc.SetPermissions(ctx, r.ws, g2.ID, nil); err != nil {
		t.Fatal(err)
	}
	access, _ = r.svc.Access(ctx, r.ws, person)
	if len(access.Permissions) != 0 {
		t.Fatalf("after emptying the audit group: %v", access.Permissions)
	}
}

func TestMemberGroupServiceAccessOfTheRoles(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, role := range []string{"owner", "admin"} {
		user := r.addMember(role)
		access, err := r.svc.Access(ctx, r.ws, user)
		if err != nil || access.Role != role || len(access.Permissions) != 6 {
			t.Errorf("%s: %+v, %v; want all six keys", role, access, err)
		}
	}
	viewer := r.addMember("viewer")
	g := r.create("Readers")
	if _, _, err := r.svc.SetPermissions(ctx, r.ws, g.ID, []string{"audit.read"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{viewer}); err != nil {
		t.Fatal(err)
	}
	access, err := r.svc.Access(ctx, r.ws, viewer)
	if err != nil || access.Role != "viewer" || !reflect.DeepEqual(access.Permissions, []string{"audit.read"}) {
		t.Fatalf("viewer: %+v, %v", access, err)
	}

	stranger := util.MustParseUUID(r.f.User(t, "Stranger", testutil.UniqueEmail("str")))
	if _, err := r.svc.Access(ctx, r.ws, stranger); !errors.Is(err, ErrNotAMember) {
		t.Fatalf("a non-member: err = %v, want ErrNotAMember", err)
	}
}

// Rule 3: a key another build stored grants nothing, and a group of another
// workspace never counts.
func TestMemberGroupServiceAccessIgnoresStoredUnknownKeysAndOtherWorkspaces(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	person := r.addMember("member")
	g := r.create("Future")
	r.f.Exec(t, `UPDATE member_group SET permissions = '["members.invite","groups.manage","from.the.future"]'::jsonb WHERE id = $1`,
		util.UUIDToString(g.ID))
	if _, err := r.svc.AddMembers(ctx, r.ws, g.ID, r.owner, []pgtype.UUID{person}); err != nil {
		t.Fatal(err)
	}
	access, err := r.svc.Access(ctx, r.ws, person)
	if err != nil || !reflect.DeepEqual(access.Permissions, []string{"members.invite"}) {
		t.Fatalf("access = %+v, %v; want only members.invite", access, err)
	}

	// A membership row naming a group of another workspace grants nothing here.
	other := newRig(t)
	foreign := other.create("Foreign")
	if _, _, err := other.svc.SetPermissions(ctx, other.ws, foreign.ID, []string{"audit.read"}); err != nil {
		t.Fatal(err)
	}
	r.f.Exec(t, `INSERT INTO member_group_member (workspace_id, group_id, user_id) VALUES ($1, $2, $3)`,
		r.wsID, util.UUIDToString(foreign.ID), util.UUIDToString(person))
	access, _ = r.svc.Access(ctx, r.ws, person)
	if !reflect.DeepEqual(access.Permissions, []string{"members.invite"}) {
		t.Fatalf("a foreign group leaked: %v", access.Permissions)
	}
}

// Rule 10, second sentence: deleting a group deletes its member rows.
func TestMemberGroupServiceDeleteRemovesTheMemberRows(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	g, keep := r.create("Doomed"), r.create("Kept")
	a, b := r.addMember("member"), r.addMember("member")
	for _, group := range []db.MemberGroup{g, keep} {
		if _, err := r.svc.AddMembers(ctx, r.ws, group.ID, r.owner, []pgtype.UUID{a, b}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, members, err := r.svc.Delete(ctx, r.ws, g.ID)
	if err != nil || deleted.ID != g.ID || members != 2 {
		t.Fatalf("Delete = %+v, %d, %v", deleted, members, err)
	}
	if _, err := r.svc.Get(ctx, r.ws, g.ID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
	if n := r.memberRows(g.ID); n != 0 {
		t.Fatalf("%d member rows of a deleted group", n)
	}
	if n := r.memberRows(keep.ID); n != 2 {
		t.Fatalf("%d rows of the other group, want 2", n)
	}
	if _, _, err := r.svc.Delete(ctx, r.ws, g.ID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("a second Delete: %v", err)
	}
}

// Every query filters by workspace: the id of another workspace's group is not found.
func TestMemberGroupServiceGroupsOfAnotherWorkspaceAreNotFound(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	other := newRig(t)
	foreign := other.create("Theirs")
	person := r.addMember("member")
	name := "x"
	perms := []string{"audit.read"}

	checks := map[string]func() error{
		"Get":    func() error { _, err := r.svc.Get(ctx, r.ws, foreign.ID); return err },
		"Update": func() error { _, _, err := r.svc.Update(ctx, r.ws, foreign.ID, UpdateInput{Name: &name}); return err },
		"SetPermissions": func() error {
			_, _, err := r.svc.SetPermissions(ctx, r.ws, foreign.ID, perms)
			return err
		},
		"Delete": func() error { _, _, err := r.svc.Delete(ctx, r.ws, foreign.ID); return err },
		"AddMembers": func() error {
			_, err := r.svc.AddMembers(ctx, r.ws, foreign.ID, r.owner, []pgtype.UUID{person})
			return err
		},
		"RemoveMember": func() error { _, _, err := r.svc.RemoveMember(ctx, r.ws, foreign.ID, person); return err },
		"ListMembers":  func() error { _, err := r.svc.ListMembers(ctx, r.ws, foreign.ID); return err },
	}
	for name, call := range checks {
		if err := call(); !errors.Is(err, ErrGroupNotFound) {
			t.Errorf("%s: err = %v, want ErrGroupNotFound", name, err)
		}
	}
	back, err := other.svc.Get(ctx, other.ws, foreign.ID)
	if err != nil || back.Name != "Theirs" || len(StoredPermissions(back)) != 0 {
		t.Fatalf("the foreign group changed: %+v, %v", back, err)
	}
}

func TestMemberGroupServiceListAndCount(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if n, err := r.svc.CountGroups(ctx, r.ws); err != nil || n != 0 {
		t.Fatalf("CountGroups = %d, %v", n, err)
	}
	a, _ := r.create("Alpha"), r.create("Beta")
	person := r.addMember("member")
	if _, err := r.svc.AddMembers(ctx, r.ws, a.ID, r.owner, []pgtype.UUID{person}); err != nil {
		t.Fatal(err)
	}
	other := newRig(t)
	other.create("Elsewhere")

	list, err := r.svc.List(ctx, r.ws)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %d rows, %v", len(list), err)
	}
	counts := map[string]int64{}
	for _, row := range list {
		counts[row.Name] = row.MemberCount
	}
	if counts["Alpha"] != 1 || counts["Beta"] != 0 {
		t.Fatalf("member counts = %v", counts)
	}
	if n, err := r.svc.CountGroups(ctx, r.ws); err != nil || n != 2 {
		t.Fatalf("CountGroups = %d, %v", n, err)
	}
}
