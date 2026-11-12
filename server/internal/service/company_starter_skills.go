package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The starter company's skills (spec 13.6.6) and the Secretary desk (spec 13.3.2).
// The skills are the catalog/skills/starter folders, shipped in
// the generated company data (catalog build). Setup and the starter apply install them as
// workspace skills and attach them to the agents that need them. A rerun never
// touches a skill the boss has, and it reuses one the starter company made.

const (
	StarterSkillSquadReporting     = "squad-reporting"
	StarterSkillDirectiveExecution = "directive-execution"

	// The Secretary's built-in skills. The router is the entry point every run
	// reads first; the others are the procedures it routes to.
	StarterSkillSecretaryRouter          = "secretary-router"
	StarterSkillSecretaryFacts           = "secretary-facts"
	StarterSkillSecretaryAsk             = "secretary-ask"
	StarterSkillSecretaryAnalysis        = "secretary-analysis"
	StarterSkillSecretaryBrief           = "secretary-brief"
	StarterSkillSecretaryDirectives      = "secretary-directives"
	StarterSkillSecretaryLeaderQuestions = "secretary-leader-questions"

	// SecretaryDeskProjectTitle names the project the Secretary files its request
	// issues in. Setup finds it by this title.
	SecretaryDeskProjectTitle       = "Secretary desk"
	secretaryDeskProjectDescription = "Requests the Secretary files for the squads and floors, with their child issues and the directives that come from them."

	starterSkillSource = "starter_company"
)

// SecretarySkills are the Secretary's built-in skills. Every Secretary gets all of
// them on setup, so its procedures never depend on the boss's choice.
var SecretarySkills = []string{
	StarterSkillSecretaryRouter,
	StarterSkillSecretaryFacts,
	StarterSkillSecretaryAsk,
	StarterSkillSecretaryAnalysis,
	StarterSkillSecretaryBrief,
	StarterSkillSecretaryDirectives,
	StarterSkillSecretaryLeaderQuestions,
}

// contentSHA is the fingerprint of a skill's SKILL.md. The starter config records
// the fingerprint of what it installed, so a later bundle can tell a skill the
// boss has not edited from one the boss has.
func contentSHA(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// starterSkillConfig is the config a starter skill is created with. The marker
// tells a rerun a skill the starter company made from one the boss made under
// the same name; the fingerprint records the content it installed.
func starterSkillConfig(content string) []byte {
	b, _ := json.Marshal(map[string]string{"source": starterSkillSource, "content_sha256": contentSHA(content)})
	return b
}

// starterSkillInstalledSHA returns the fingerprint a starter config recorded, or
// "" for a row made before fingerprints were recorded.
func starterSkillInstalledSHA(config []byte) string {
	var c struct {
		ContentSHA string `json:"content_sha256"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return ""
	}
	return c.ContentSHA
}

// starterSkillUpdateDue reports whether a starter-owned skill takes the bundle's
// newer content. Only a skill the boss has not edited is updated: the content on
// the row is still what the starter installed. A row from before fingerprints were
// recorded has no proof either way and is treated as unedited.
func starterSkillUpdateDue(existing db.Skill, next companyStarterSkill) bool {
	if !isStarterSkillConfig(existing.Config) {
		return false
	}
	if contentSHA(existing.Content) == contentSHA(next.Content) {
		return false
	}
	installed := starterSkillInstalledSHA(existing.Config)
	return installed == "" || installed == contentSHA(existing.Content)
}

// isStarterSkillConfig reports whether a skill's config marks it as the starter
// company's.
func isStarterSkillConfig(config []byte) bool {
	var c struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return false
	}
	return c.Source == starterSkillSource
}

// starterSkillsForRole returns the starter skills an agent of a role gets. The
// Secretary gets its built-in bundle. A squad leader gets the reporting and the
// directive skills. Other roles get none.
func starterSkillsForRole(card companySquad, role companyRole) []string {
	if role.Runtime == "secretary" {
		return SecretarySkills
	}
	if role.Key == card.Leader {
		return []string{StarterSkillSquadReporting, StarterSkillDirectiveExecution}
	}
	return nil
}

// floorChiefStarterSkills are the skills a floor chief gets. The starter company
// does not create chiefs; the chief a floor is given gets these on the apply.
var floorChiefStarterSkills = []string{StarterSkillSquadReporting, StarterSkillDirectiveExecution}

// ensureStarterSkill makes the starter skill in the workspace unless a skill
// with its name exists. created is true when this call made it. owned is false
// when the name belongs to a skill the boss made, which is then left as it is.
// An owned starter skill the boss has not edited takes the bundle's newer content,
// so an existing company receives a corrected procedure on its next setup.
func ensureStarterSkill(ctx context.Context, q *db.Queries, workspaceID, actorUserID pgtype.UUID, sk companyStarterSkill) (skill db.Skill, created, owned bool, err error) {
	existing, err := q.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: workspaceID, Name: sk.Name})
	if err == nil {
		if !starterSkillUpdateDue(existing, sk) {
			return existing, false, isStarterSkillConfig(existing.Config), nil
		}
		skill, err = q.UpdateSkill(ctx, db.UpdateSkillParams{
			ID:          existing.ID,
			Description: pgtype.Text{String: sk.Description, Valid: true},
			Content:     pgtype.Text{String: sk.Content, Valid: true},
			Config:      starterSkillConfig(sk.Content),
		})
		if err != nil {
			return db.Skill{}, false, false, err
		}
		if err := upsertStarterFiles(ctx, q, skill.ID, sk.Files); err != nil {
			return db.Skill{}, false, false, err
		}
		return skill, false, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Skill{}, false, false, err
	}
	skill, err = q.CreateSkill(ctx, db.CreateSkillParams{
		WorkspaceID: workspaceID,
		Name:        sk.Name,
		Description: sk.Description,
		Content:     sk.Content,
		Config:      starterSkillConfig(sk.Content),
		CreatedBy:   actorUserID,
	})
	if err != nil {
		return db.Skill{}, false, false, err
	}
	if err := upsertStarterFiles(ctx, q, skill.ID, sk.Files); err != nil {
		return db.Skill{}, false, false, err
	}
	return skill, true, true, nil
}

func upsertStarterFiles(ctx context.Context, q *db.Queries, skillID pgtype.UUID, files []companyStarterFile) error {
	for _, f := range files {
		if _, err := q.UpsertSkillFile(ctx, db.UpsertSkillFileParams{SkillID: skillID, Path: f.Path, Content: f.Content}); err != nil {
			return err
		}
	}
	return nil
}

// attachStarterSkills attaches the named starter skills to an agent that has no
// row for them yet, and returns the names it attached. A row already there,
// enabled or disabled, is left as it is. So is a skill the boss owns under that
// name. A skill that is not installed yet is skipped; the install runs first.
func attachStarterSkills(ctx context.Context, q *db.Queries, workspaceID, agentID pgtype.UUID, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := q.ListAgentSkillSummaries(ctx, agentID)
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(rows))
	for _, row := range rows {
		have[row.Name] = true
	}
	var attached []string
	for _, name := range names {
		if have[name] {
			continue
		}
		skill, err := q.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: workspaceID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !isStarterSkillConfig(skill.Config) {
			continue
		}
		if err := q.AddAgentSkill(ctx, db.AddAgentSkillParams{AgentID: agentID, SkillID: skill.ID}); err != nil {
			return nil, err
		}
		attached = append(attached, name)
	}
	return attached, nil
}

// ensureSecretaryDesk returns the workspace's Secretary desk project, and makes
// it when no project has its title. created is nil when the project already
// existed.
func ensureSecretaryDesk(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID) (db.Project, *db.Project, error) {
	projects, err := q.ListProjects(ctx, db.ListProjectsParams{WorkspaceID: workspaceID})
	if err != nil {
		return db.Project{}, nil, err
	}
	for _, p := range projects {
		if p.Title == SecretaryDeskProjectTitle {
			return p, nil, nil
		}
	}
	project, err := q.CreateProject(ctx, db.CreateProjectParams{
		WorkspaceID: workspaceID,
		Title:       SecretaryDeskProjectTitle,
		Description: pgtype.Text{String: secretaryDeskProjectDescription, Valid: true},
		Status:      "in_progress",
		Priority:    "none",
	})
	if err != nil {
		return db.Project{}, nil, err
	}
	return project, &project, nil
}

// findSecretaryDesk returns the Secretary desk project's id, or an invalid id
// when the workspace has none. It changes nothing.
func findSecretaryDesk(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID) (pgtype.UUID, error) {
	projects, err := q.ListProjects(ctx, db.ListProjectsParams{WorkspaceID: workspaceID})
	if err != nil {
		return pgtype.UUID{}, err
	}
	for _, p := range projects {
		if p.Title == SecretaryDeskProjectTitle {
			return p.ID, nil
		}
	}
	return pgtype.UUID{}, nil
}

// installStarterSkills installs the starter skills and attaches them to the
// agents the chosen squads hold: the Secretary, each squad leader, and each
// floor chief. Each install and each attachment is a step of the apply, and a
// failed step stops it. It returns false when a step failed.
func (r *companyRun) installStarterSkills(ctx context.Context) bool {
	for _, sk := range r.data.StarterSkills {
		if !r.run("skill:"+sk.Name, func() (string, string, error) { return r.installStarterSkill(ctx, sk) }) {
			return false
		}
	}
	var order []string
	roles := map[string]companyRole{}
	wanted := map[string][]string{}
	for _, card := range r.cards {
		for _, role := range card.Roles {
			names := starterSkillsForRole(card, role)
			if len(names) == 0 {
				continue
			}
			if _, ok := roles[role.Name]; !ok {
				roles[role.Name] = role
				order = append(order, role.Name)
			}
			for _, name := range names {
				if !slices.Contains(wanted[role.Name], name) {
					wanted[role.Name] = append(wanted[role.Name], name)
				}
			}
		}
	}
	for _, name := range order {
		role, names := roles[name], wanted[name]
		if !r.run("skills:"+name, func() (string, string, error) { return r.attachSkills(ctx, role, names) }) {
			return false
		}
	}
	for _, key := range r.floorKeys() {
		if !r.run("skills:chief:"+key, func() (string, string, error) { return r.attachChiefSkills(ctx, key) }) {
			return false
		}
	}
	return true
}

func (r *companyRun) installStarterSkill(ctx context.Context, sk companyStarterSkill) (string, string, error) {
	var created, owned bool
	err := r.inTx(ctx, func(q *db.Queries) error {
		var err error
		_, created, owned, err = ensureStarterSkill(ctx, q, r.in.WorkspaceID, r.in.ActorUserID, sk)
		return err
	})
	if err != nil {
		return "", "", err
	}
	switch {
	case !owned:
		return CompanyStepReused, "the boss has a skill named " + sk.Name + "; it is left as it is", nil
	case created:
		return CompanyStepCreated, "installed the skill " + sk.Name, nil
	default:
		return CompanyStepReused, "the skill " + sk.Name + " is installed", nil
	}
}

func (r *companyRun) attachSkills(ctx context.Context, role companyRole, names []string) (string, string, error) {
	agent, found, err := r.liveAgentFor(ctx, r.svc.Queries, role)
	if err != nil {
		return "", "", err
	}
	if !found || agent.ArchivedAt.Valid {
		return "", "", companyErr("the agent %q is not in the workspace", role.Name)
	}
	var attached []string
	err = r.inTx(ctx, func(q *db.Queries) error {
		var err error
		attached, err = attachStarterSkills(ctx, q, r.in.WorkspaceID, agent.ID, names)
		return err
	})
	if err != nil {
		return "", "", err
	}
	if len(attached) > 0 {
		return CompanyStepCreated, "attached " + strings.Join(attached, ", ") + " to " + role.Name, nil
	}
	return CompanyStepReused, "the skills are on " + role.Name, nil
}

func (r *companyRun) attachChiefSkills(ctx context.Context, floorKey string) (string, string, error) {
	floor, err := r.floorNamed(ctx, r.svc.Queries, floorKey)
	if err != nil {
		return "", "", err
	}
	chief, err := r.svc.Queries.GetFloorChief(ctx, db.GetFloorChiefParams{WorkspaceID: r.in.WorkspaceID, FloorID: floor.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CompanyStepReused, "no floor chief on " + floor.Name + " yet", nil
	}
	if err != nil {
		return "", "", err
	}
	var attached []string
	err = r.inTx(ctx, func(q *db.Queries) error {
		var err error
		attached, err = attachStarterSkills(ctx, q, r.in.WorkspaceID, chief.AgentID, floorChiefStarterSkills)
		return err
	})
	if err != nil {
		return "", "", err
	}
	if len(attached) > 0 {
		return CompanyStepCreated, "attached " + strings.Join(attached, ", ") + " to the chief of " + floor.Name, nil
	}
	return CompanyStepReused, "the skills are on the chief of " + floor.Name, nil
}
