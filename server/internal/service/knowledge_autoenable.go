package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/knowledge"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Knowledge is on by default. A new workspace enables it as soon as it exists,
// and every project gets a knowledge base of its own, so a project's documents
// never mix with another project's. The hourly pass below catches what the
// inline calls missed: a workspace whose enabling failed (the sidecar was down,
// a quota was reached), and a project created by a service instead of the API.

const (
	// projectBaseKeyPrefix starts the key of every project's knowledge base.
	projectBaseKeyPrefix = "project-"
	// projectBasePosition places project bases after the default ones.
	projectBasePosition = 100
	// autoEnableBatch bounds how many workspaces one pass retries, so a
	// sidecar that is still failing is not hammered.
	autoEnableBatch = 5
	// autoEnableTimeout bounds one inline enable run off the request path.
	autoEnableTimeout = 5 * time.Minute
)

// ProjectBaseKey is the key of a project's knowledge base. It derives from the
// project id alone, so a rename never orphans the base.
func ProjectBaseKey(projectID pgtype.UUID) string {
	return projectBaseKeyPrefix + strings.ReplaceAll(util.UUIDToString(projectID), "-", "")[:12]
}

// projectBaseName is unique per project: WeKnora adopts a base by name, so two
// projects with one title must not share a base.
func projectBaseName(projectID pgtype.UUID, title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if r := []rune(title); len(r) > 80 {
		title = string(r[:80])
	}
	return fmt.Sprintf("Project: %s (%s)", title, util.UUIDToString(projectID)[:8])
}

// AutoEnable enables knowledge for a workspace that has just been created. It
// never fails the caller: a deployment that does not offer knowledge is a
// no-op, and any other failure is logged and left to the hourly pass.
func (s *KnowledgeService) AutoEnable(workspaceID, actorID pgtype.UUID) {
	if s == nil {
		return
	}
	if ok, _ := s.offeredReason(); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), autoEnableTimeout)
	defer cancel()
	if _, err := s.Enable(ctx, workspaceID, actorID); err != nil && !errors.Is(err, ErrKnowledgeBusy) {
		slog.Warn("knowledge: automatic enable failed; the hourly pass will retry", "workspace_id", util.UUIDToString(workspaceID), "error", err)
	}
}

// EnsureProjectBase creates a project's knowledge base when it has none. A
// workspace whose knowledge is not ready yet is skipped; the hourly pass
// covers the project once it is.
func (s *KnowledgeService) EnsureProjectBase(ctx context.Context, workspaceID, projectID pgtype.UUID, title string) error {
	key := ProjectBaseKey(projectID)
	if _, err := s.Queries.GetKnowledgeBaseByKey(ctx, db.GetKnowledgeBaseByKeyParams{WorkspaceID: workspaceID, Key: key}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load project knowledge base: %w", err)
	}
	ws, err := s.workspaceClient(ctx, workspaceID)
	if err != nil {
		var notReady *KnowledgeNotReadyError
		if errors.As(err, &notReady) {
			return nil
		}
		return err
	}
	model, err := ws.EnsureEmbeddingModel(ctx, s.Embedding)
	if err != nil {
		return fmt.Errorf("ensure embedding model: %w", err)
	}
	remote, err := ws.ListKnowledgeBases(ctx)
	if err != nil {
		return fmt.Errorf("list remote knowledge bases: %w", err)
	}
	if err := s.createOrAdoptBase(ctx, workspaceID, ws, model.ID, key, projectBaseName(projectID, title),
		"Documents of the project "+title+".", projectBasePosition, remote); err != nil {
		return err
	}
	return s.syncReadEndpoint(ctx, workspaceID, ws)
}

// syncReadEndpoint makes the agents' read-only endpoint cover every
// workspace-visible base. The endpoint holds a fixed list of bases, so a base
// created after provisioning is invisible to agents until the list is updated.
// The library entry already exists once knowledge is ready, so no actor is
// needed.
func (s *KnowledgeService) syncReadEndpoint(ctx context.Context, workspaceID pgtype.UUID, ws knowledge.Workspace) error {
	if _, err := s.ensureMCPServer(ctx, workspaceID, pgtype.UUID{}, ws); err != nil {
		return fmt.Errorf("sync agent read endpoint: %w", err)
	}
	return nil
}

// EnsureProjectBaseLogged is EnsureProjectBase for the create path: the project
// already exists, so a failure here is logged and left to the hourly pass.
func (s *KnowledgeService) EnsureProjectBaseLogged(workspaceID, projectID pgtype.UUID, title string) {
	if s == nil {
		return
	}
	if ok, _ := s.offeredReason(); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := s.EnsureProjectBase(ctx, workspaceID, projectID, title); err != nil {
		slog.Warn("knowledge: project base not created; the hourly pass will retry", "project_id", util.UUIDToString(projectID), "error", err)
	}
}

// RunAutoEnable is the hourly pass. It retries workspaces whose enabling failed
// and gives every project of a ready workspace its base. It returns how many
// workspaces and bases it set up.
func (s *KnowledgeService) RunAutoEnable(ctx context.Context) (int, error) {
	if s == nil {
		return 0, nil
	}
	if ok, _ := s.offeredReason(); !ok {
		return 0, nil
	}
	done := 0
	var failures []error

	failed, err := s.failedWorkspaces(ctx)
	if err != nil {
		return 0, err
	}
	for _, f := range failed {
		if _, err := s.Enable(ctx, f.workspaceID, f.ownerID); err != nil {
			if errors.Is(err, knowledge.ErrUnavailable) {
				return done, err
			}
			if !errors.Is(err, ErrKnowledgeBusy) {
				failures = append(failures, fmt.Errorf("enable workspace %s: %w", util.UUIDToString(f.workspaceID), err))
			}
			continue
		}
		done++
	}

	ids, err := s.Queries.ListReadyKnowledgeWorkspaceIDs(ctx)
	if err != nil {
		return done, fmt.Errorf("list knowledge workspaces: %w", err)
	}
	for _, id := range ids {
		if ws, err := s.workspaceClient(ctx, id); err == nil {
			if err := s.syncReadEndpoint(ctx, id, ws); err != nil {
				if errors.Is(err, knowledge.ErrUnavailable) {
					return done, err
				}
				failures = append(failures, fmt.Errorf("workspace %s: %w", util.UUIDToString(id), err))
			}
		}
		projects, err := s.Queries.ListProjects(ctx, db.ListProjectsParams{WorkspaceID: id})
		if err != nil {
			failures = append(failures, fmt.Errorf("list projects of %s: %w", util.UUIDToString(id), err))
			continue
		}
		for _, p := range projects {
			if p.Status == "cancelled" {
				continue
			}
			key := ProjectBaseKey(p.ID)
			if _, err := s.Queries.GetKnowledgeBaseByKey(ctx, db.GetKnowledgeBaseByKeyParams{WorkspaceID: id, Key: key}); err == nil {
				continue
			}
			if err := s.EnsureProjectBase(ctx, id, p.ID, p.Title); err != nil {
				if errors.Is(err, knowledge.ErrUnavailable) {
					return done, err
				}
				failures = append(failures, fmt.Errorf("project %s: %w", util.UUIDToString(p.ID), err))
				continue
			}
			done++
		}
	}
	return done, errors.Join(failures...)
}

type failedWorkspace struct {
	workspaceID pgtype.UUID
	ownerID     pgtype.UUID
}

// failedWorkspaces lists up to autoEnableBatch workspaces whose enabling
// failed, each with an owner to act as. A workspace with no owner is skipped.
func (s *KnowledgeService) failedWorkspaces(ctx context.Context) ([]failedWorkspace, error) {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	rows, err := conn.Query(ctx, `
		SELECT k.workspace_id,
		       (SELECT m.user_id FROM member m WHERE m.workspace_id = k.workspace_id AND m.role = 'owner' ORDER BY m.created_at LIMIT 1)
		FROM workspace_knowledge k
		WHERE k.state = 'failed'
		ORDER BY k.created_at
		LIMIT $1`, autoEnableBatch)
	if err != nil {
		return nil, fmt.Errorf("list failed knowledge workspaces: %w", err)
	}
	defer rows.Close()
	var out []failedWorkspace
	for rows.Next() {
		var wsID pgtype.UUID
		var owner pgtype.UUID
		if err := rows.Scan(&wsID, &owner); err != nil {
			return nil, fmt.Errorf("scan failed knowledge workspace: %w", err)
		}
		if owner.Valid {
			out = append(out, failedWorkspace{workspaceID: wsID, ownerID: owner})
		}
	}
	return out, rows.Err()
}
