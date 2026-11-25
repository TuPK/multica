package connectedapps

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// ExecuteUndo runs the inverse call of an executed effect (durable spec 4.6) with the
// account the member acted with. Who may undo is the caller's check. It returns the
// tool's data.
func (s *Service) ExecuteUndo(ctx context.Context, workspaceID, memberID pgtype.UUID, target string, spec UndoSpec) (map[string]any, error) {
	toolkit := strings.SplitN(target, "/", 2)[0]
	data, _, err := s.executeAs(ctx, workspaceID, memberID, toolkit, spec.Tool, spec.Args)
	if err != nil {
		return nil, fmt.Errorf("undo: %w", err)
	}
	return data, nil
}

// executeAs runs one tool as memberID, through the account the gateway would pick for
// them: a personal account when the app is personal, the team account otherwise. It
// returns the tool's data and the connected account id it used.
func (s *Service) executeAs(ctx context.Context, workspaceID, memberID pgtype.UUID, toolkit, tool string, args map[string]any) (map[string]any, string, error) {
	slug := NormalizeSlug(toolkit)
	app, _ := Curated(slug)
	scope := app.Scope
	if scope == "" {
		scope = ScopePersonal
	}
	acct, err := s.resolveAccount(ctx, workspaceID, memberID, slug, scope)
	if err != nil {
		return nil, "", fmt.Errorf("resolve account: %w", err)
	}
	t, err := s.composio.GetTool(ctx, slug, tool)
	if err != nil {
		return nil, acct.connectedAccountID, fmt.Errorf("load tool: %w", err)
	}
	resp, err := s.composio.ExecuteTool(ctx, acct.connectedAccountID, acct.composioUserID, *t, args)
	s.touch(ctx, workspaceID, acct)
	if err != nil {
		return nil, acct.connectedAccountID, fmt.Errorf("run tool: %w", err)
	}
	if !resp.Successful {
		return nil, acct.connectedAccountID, fmt.Errorf("tool returned an error: %s", truncate(resp.Error, 500))
	}
	return resp.Data, acct.connectedAccountID, nil
}
