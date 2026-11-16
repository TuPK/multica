package membergroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	ErrNameTaken     = errors.New("a group with this name already exists")
	ErrGroupNotFound = errors.New("group not found")
	// ErrNotAMember reports a person who is not a member of the workspace.
	ErrNotAMember = errors.New("not a member of the workspace")
)

// MemberLimitError reports an add that would put a group over MaxMembers.
type MemberLimitError struct{ Limit int }

func (e *MemberLimitError) Error() string {
	return fmt.Sprintf("a group holds at most %d members", e.Limit)
}

// nameIndex is the unique index on (workspace_id, lower(name)), migration 958.
const nameIndex = "member_group_name_uidx"

// Service reads and writes groups through q. Bind q to a transaction
// (db.Queries.WithTx) to make a change and its audit row atomic; the service
// never begins one. A change takes the group's row lock first, so concurrent
// changes to one group queue.
type Service struct{ q *db.Queries }

func New(q *db.Queries) *Service { return &Service{q: q} }

// Access is what permissions/me reports and what a permission check reads.
type Access struct {
	Role        string
	Permissions []string
}

// ParsePermissions decodes a stored permissions array. A value that is not a
// JSON array of strings reads as empty, so it grants nothing.
func ParsePermissions(raw []byte) []string {
	keys := []string{}
	if err := json.Unmarshal(raw, &keys); err != nil || keys == nil {
		return []string{}
	}
	return keys
}

// StoredPermissions returns the keys as stored on g, unknown ones included.
func StoredPermissions(g db.MemberGroup) []string { return ParsePermissions(g.Permissions) }

// Create stores a new group with no permissions and no members.
func (s *Service) Create(ctx context.Context, workspaceID, createdBy pgtype.UUID, name, description string) (db.MemberGroup, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return db.MemberGroup{}, err
	}
	description, err = NormalizeDescription(description)
	if err != nil {
		return db.MemberGroup{}, err
	}
	g, err := s.q.CreateMemberGroup(ctx, db.CreateMemberGroupParams{
		WorkspaceID: workspaceID, Name: name, Description: description, CreatedBy: createdBy,
	})
	if err != nil {
		return db.MemberGroup{}, mapNameConflict(err)
	}
	return g, nil
}

// CountGroups returns the number of groups of the workspace, for the plan gate.
func (s *Service) CountGroups(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
	return s.q.CountMemberGroups(ctx, workspaceID)
}

// Get returns one group of the workspace.
func (s *Service) Get(ctx context.Context, workspaceID, id pgtype.UUID) (db.MemberGroup, error) {
	g, err := s.q.GetMemberGroup(ctx, db.GetMemberGroupParams{WorkspaceID: workspaceID, ID: id})
	return g, notFound(err)
}

func (s *Service) lock(ctx context.Context, workspaceID, id pgtype.UUID) (db.MemberGroup, error) {
	g, err := s.q.GetMemberGroupForUpdate(ctx, db.GetMemberGroupForUpdateParams{WorkspaceID: workspaceID, ID: id})
	return g, notFound(err)
}

// List returns the groups of the workspace with their member counts.
func (s *Service) List(ctx context.Context, workspaceID pgtype.UUID) ([]db.ListMemberGroupsRow, error) {
	return s.q.ListMemberGroups(ctx, workspaceID)
}

// ListMembers returns the people in a group, oldest first.
func (s *Service) ListMembers(ctx context.Context, workspaceID, id pgtype.UUID) ([]db.MemberGroupMember, error) {
	if _, err := s.Get(ctx, workspaceID, id); err != nil {
		return nil, err
	}
	return s.q.ListMemberGroupMembers(ctx, db.ListMemberGroupMembersParams{WorkspaceID: workspaceID, GroupID: id})
}

// UpdateInput carries the fields to change; a nil field keeps its value.
type UpdateInput struct {
	Name        *string
	Description *string
}

// Update changes the name and description and returns the group and the names
// of the fields whose value differs ("name", "description"). When none does,
// nothing is written.
func (s *Service) Update(ctx context.Context, workspaceID, id pgtype.UUID, in UpdateInput) (db.MemberGroup, []string, error) {
	name, description := "", ""
	if in.Name != nil {
		var err error
		if name, err = NormalizeName(*in.Name); err != nil {
			return db.MemberGroup{}, nil, err
		}
	}
	if in.Description != nil {
		var err error
		if description, err = NormalizeDescription(*in.Description); err != nil {
			return db.MemberGroup{}, nil, err
		}
	}
	g, err := s.lock(ctx, workspaceID, id)
	if err != nil {
		return db.MemberGroup{}, nil, err
	}
	var changed []string
	next := db.UpdateMemberGroupParams{WorkspaceID: workspaceID, ID: id, Name: g.Name, Description: g.Description}
	if in.Name != nil && name != g.Name {
		next.Name = name
		changed = append(changed, "name")
	}
	if in.Description != nil && description != g.Description {
		next.Description = description
		changed = append(changed, "description")
	}
	if len(changed) == 0 {
		return g, nil, nil
	}
	updated, err := s.q.UpdateMemberGroup(ctx, next)
	if err != nil {
		return db.MemberGroup{}, nil, mapNameConflict(err)
	}
	return updated, changed, nil
}

// SetPermissions replaces the keys of a group. It returns the group as it is
// after the call and the keys it held before, unknown stored ones included.
// Writing the keys already stored writes nothing.
func (s *Service) SetPermissions(ctx context.Context, workspaceID, id pgtype.UUID, keys []string) (db.MemberGroup, []string, error) {
	next, err := NormalizePermissions(keys)
	if err != nil {
		return db.MemberGroup{}, nil, err
	}
	g, err := s.lock(ctx, workspaceID, id)
	if err != nil {
		return db.MemberGroup{}, nil, err
	}
	old := StoredPermissions(g)
	if slices.Equal(old, next) {
		return g, old, nil
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return db.MemberGroup{}, nil, err
	}
	updated, err := s.q.UpdateMemberGroupPermissions(ctx, db.UpdateMemberGroupPermissionsParams{
		WorkspaceID: workspaceID, ID: id, Permissions: raw,
	})
	if err != nil {
		return db.MemberGroup{}, nil, err
	}
	return updated, old, nil
}

// Delete removes a group and its member rows. It returns the group as it was
// and how many member rows it removed. Call it inside a transaction: the two
// deletes must commit together.
func (s *Service) Delete(ctx context.Context, workspaceID, id pgtype.UUID) (db.MemberGroup, int64, error) {
	g, err := s.lock(ctx, workspaceID, id)
	if err != nil {
		return db.MemberGroup{}, 0, err
	}
	members, err := s.q.DeleteMemberGroupMembers(ctx, db.DeleteMemberGroupMembersParams{WorkspaceID: workspaceID, GroupID: id})
	if err != nil {
		return db.MemberGroup{}, 0, err
	}
	if _, err := s.q.DeleteMemberGroup(ctx, db.DeleteMemberGroupParams{WorkspaceID: workspaceID, ID: id}); err != nil {
		return db.MemberGroup{}, 0, err
	}
	return g, members, nil
}

// AddMembers puts people in a group and returns the ones actually added, in the
// order given; someone already in it is skipped. The batch is all or nothing:
// one person who is not a member of the workspace (ErrNotAMember), or a total
// over MaxMembers (*MemberLimitError), adds nobody.
func (s *Service) AddMembers(ctx context.Context, workspaceID, id, addedBy pgtype.UUID, userIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	if _, err := s.lock(ctx, workspaceID, id); err != nil {
		return nil, err
	}
	unique := dedupe(userIDs)
	if len(unique) == 0 {
		return []pgtype.UUID{}, nil
	}
	members, err := s.q.ListWorkspaceMemberUserIDs(ctx, db.ListWorkspaceMemberUserIDsParams{WorkspaceID: workspaceID, UserIds: unique})
	if err != nil {
		return nil, err
	}
	if len(members) != len(unique) {
		return nil, ErrNotAMember
	}
	already, err := s.q.ListMemberGroupUserIDs(ctx, db.ListMemberGroupUserIDsParams{WorkspaceID: workspaceID, GroupID: id, UserIds: unique})
	if err != nil {
		return nil, err
	}
	count, err := s.q.CountMemberGroupMembers(ctx, db.CountMemberGroupMembersParams{WorkspaceID: workspaceID, GroupID: id})
	if err != nil {
		return nil, err
	}
	if count+int64(len(unique)-len(already)) > MaxMembers {
		return nil, &MemberLimitError{Limit: MaxMembers}
	}
	inserted, err := s.q.AddMemberGroupMembers(ctx, db.AddMemberGroupMembersParams{
		WorkspaceID: workspaceID, GroupID: id, AddedBy: addedBy, UserIds: unique,
	})
	if err != nil {
		return nil, err
	}
	added := make([]pgtype.UUID, 0, len(inserted))
	for _, user := range unique {
		if slices.Contains(inserted, user) {
			added = append(added, user)
		}
	}
	return added, nil
}

// RemoveMember takes a person out of a group. It reports whether they were in
// it, and returns the group. Removing someone who is not in it is not an error.
func (s *Service) RemoveMember(ctx context.Context, workspaceID, id, userID pgtype.UUID) (bool, db.MemberGroup, error) {
	g, err := s.lock(ctx, workspaceID, id)
	if err != nil {
		return false, db.MemberGroup{}, err
	}
	n, err := s.q.DeleteMemberGroupMember(ctx, db.DeleteMemberGroupMemberParams{WorkspaceID: workspaceID, GroupID: id, UserID: userID})
	if err != nil {
		return false, db.MemberGroup{}, err
	}
	return n > 0, g, nil
}

// Access returns the role and the effective permissions of a member of the
// workspace, read from the database on every call. A user who is not a member
// gets ErrNotAMember.
func (s *Service) Access(ctx context.Context, workspaceID, userID pgtype.UUID) (Access, error) {
	row, err := s.q.GetMemberPermissionInputs(ctx, db.GetMemberPermissionInputsParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Access{}, ErrNotAMember
	}
	if err != nil {
		return Access{}, err
	}
	groups := make([][]string, len(row.GroupPermissions))
	for i, raw := range row.GroupPermissions {
		groups[i] = ParsePermissions(raw)
	}
	return Access{Role: row.Role, Permissions: Effective(row.Role, groups...)}, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrGroupNotFound
	}
	return err
}

func mapNameConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == nameIndex {
		return ErrNameTaken
	}
	return err
}

func dedupe(ids []pgtype.UUID) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
