package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// SquadCriteriaByTemplate maps a squad template id to the criteria its issues get.
type SquadCriteriaByTemplate map[string][]TemplateCriterion

var (
	squadCriteriaOnce sync.Once
	squadCriteriaData SquadCriteriaByTemplate
)

// catalogSquadCriteria reads the criteria the catalog declares, once. A catalog that is not
// built, or a file that does not parse, declares none; the squad then receives its issues
// without criteria, and the build check catches the file.
func catalogSquadCriteria() SquadCriteriaByTemplate {
	squadCriteriaOnce.Do(func() {
		squadCriteriaData, _ = ParseSquadOutcomeCriteria(catalog.SquadOutcomeCriteriaJSON())
	})
	return squadCriteriaData
}

// ParseSquadOutcomeCriteria reads the criteria file the catalog build writes.
func ParseSquadOutcomeCriteria(raw []byte) (SquadCriteriaByTemplate, error) {
	out := SquadCriteriaByTemplate{}
	if len(raw) == 0 {
		return out, nil
	}
	var wire map[string][]struct {
		Role      string         `json:"role"`
		Text      string         `json:"text"`
		CheckKind string         `json:"check_kind"`
		CheckSpec map[string]any `json:"check_spec"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("squad outcome criteria: %w", err)
	}
	for id, list := range wire {
		for _, c := range list {
			out[id] = append(out[id], TemplateCriterion{Role: c.Role, Text: c.Text, CheckKind: c.CheckKind, CheckSpec: c.CheckSpec})
		}
	}
	return out, nil
}

// ApplySquadOutcomeCriteria writes the criteria of the squad's template to an issue the
// squad receives (durable spec 2.5). A squad made without a template, or a template that
// declares no criteria, writes nothing.
func (s *VerificationService) ApplySquadOutcomeCriteria(ctx context.Context, issue db.Issue, squadTemplate pgtype.Text, criteria SquadCriteriaByTemplate) (int, error) {
	if !squadTemplate.Valid {
		return 0, nil
	}
	templates := criteria[squadTemplate.String]
	if len(templates) == 0 {
		return 0, nil
	}
	return s.ApplyTemplateCriteria(ctx, issue.WorkspaceID, issue.ID, templates)
}

// ApplySquadCriteriaOnReceipt is the hook the squad enqueue paths call when a squad receives
// an issue. A failure is logged and does not stop the dispatch: the issue still goes to the
// squad, and its criteria can be added by hand.
func ApplySquadCriteriaOnReceipt(ctx context.Context, queries *db.Queries, tx TxStarter, issue db.Issue, squad db.Squad) {
	if !squad.TemplateKey.Valid {
		return
	}
	svc := &VerificationService{Queries: queries, TxStarter: tx}
	if _, err := svc.ApplySquadOutcomeCriteria(ctx, issue, squad.TemplateKey, catalogSquadCriteria()); err != nil {
		slog.Error("squad criteria: apply to received issue failed", "issue_id", util.UUIDToString(issue.ID), "squad_id", util.UUIDToString(squad.ID), "error", err)
	}
}
