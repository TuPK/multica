package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The task rows of a schema 2 flow (spec 2.5): who holds each row, the text of
// the issue, and the rows' identity (key~i for the i-th instance).

// flowRowHolder is the person who holds a task row, as "type:id", or "" for a row
// nobody holds. Of the rows with the key it reads the one of the highest round.
func flowRowHolder(rows []flowTaskRow, step, rowKey string) (typ, id string) {
	best := -1
	for _, r := range rows {
		if r.state.Step == step && r.state.Key == rowKey && r.state.Round > best && r.issue.AssigneeID.Valid {
			best = r.state.Round
			typ, id = r.issue.AssigneeType.String, uuidToString(r.issue.AssigneeID)
		}
	}
	return typ, id
}

// flowOpenAssignment is who an opened row is given to.
type flowOpenAssignment struct {
	open issueworkflow.OpenTask
	typ  string
	id   pgtype.UUID
}

// assignGraphRows picks the person for every row to open. A person who cannot take
// work, or who did the work a gate judges, is passed over for the next one of the
// pool; a pool with nobody left stops the request. stop is non-nil when the request
// was stopped instead, with the outcome of the stop.
func (h *Handler) assignGraphRows(ctx context.Context, run *flowRun, rows []flowTaskRow, open []issueworkflow.OpenTask) (plan []flowOpenAssignment, stop *advanceOutcome, err error) {
	g := run.def.Graph
	tasks := taskStates(rows)
	round := run.round()
	holder := func(step, rowKey string) string {
		_, id := flowRowHolder(rows, step, rowKey)
		return id
	}
	taken := map[string]map[string]bool{}
	for _, r := range rows {
		if r.state.Step == run.state.Step && r.state.Round == round && r.issue.AssigneeID.Valid {
			base := issueworkflow.TaskBaseKey(r.state.Key)
			if taken[base] == nil {
				taken[base] = map[string]bool{}
			}
			taken[base][uuidToString(r.issue.AssigneeID)] = true
		}
	}
	for _, o := range open {
		role, found := g.Role(o.Task.Role)
		a := flowOpenAssignment{open: o}
		kind := role.Kind
		if !found {
			kind = issueworkflow.RoleKindManagers
		}
		switch kind {
		case issueworkflow.RoleKindAgent, issueworkflow.RoleKindMember:
			var pool []string
			if role.Bind != nil {
				pool = role.Bind.AgentIDs
				if kind == issueworkflow.RoleKindMember {
					pool = role.Bind.MemberIDs
				}
			}
			req := issueworkflow.AssignRequest{
				Pool: pool, Instance: o.Index,
				Taken:       taken[o.Task.Key],
				Unavailable: map[string]bool{},
				Forbidden:   issueworkflow.JudgedWorkers(g, run.state, tasks, o.Task, holder),
				Distinct:    o.Task.Instances != nil && o.Task.Instances.IsDistinct(),
			}
			_, req.Prior = flowRowHolder(rows, run.state.Step, o.Key)
			reasons := map[string]string{}
			if kind == issueworkflow.RoleKindAgent {
				for _, p := range pool {
					why, err := h.flowAgentUnavailable(ctx, run, p)
					if err != nil {
						return nil, nil, err
					}
					if why != "" {
						req.Unavailable[p] = true
						reasons[p] = why
					}
				}
			}
			person, ok := issueworkflow.AssignPool(req)
			if !ok {
				out, err := h.stopForNoOneToTake(ctx, run, o, pool, reasons)
				if err != nil {
					return nil, nil, err
				}
				return nil, &out, nil
			}
			a.typ = "agent"
			if kind == issueworkflow.RoleKindMember {
				a.typ = "member"
			}
			a.id, _ = util.ParseUUID(person)
			if taken[o.Task.Key] == nil {
				taken[o.Task.Key] = map[string]bool{}
			}
			taken[o.Task.Key][person] = true
		case issueworkflow.RoleKindRequester:
			if run.issue.CreatorType == "member" {
				a.typ, a.id = "member", run.issue.CreatorID
			}
		}
		plan = append(plan, a)
	}
	return plan, nil, nil
}

// stopForNoOneToTake stops the request when nobody in the pool of the role can
// take a row: every agent of it is unavailable (the same stop as a single agent
// that is gone), or the pool is too small for what the step needs of it.
func (h *Handler) stopForNoOneToTake(ctx context.Context, run *flowRun, o issueworkflow.OpenTask, pool []string, reasons map[string]string) (advanceOutcome, error) {
	if len(pool) > 0 && len(reasons) == len(pool) {
		first := pool[0]
		return h.stopForUnavailableAgent(ctx, run, o.Key, o.Task.Title, first, reasons[first])
	}
	reason := fmt.Sprintf("No one in the pool of the role %q can take the task %q (%s): the pool has %d, and the ones left did the work it judges or hold the other parts.", o.Task.Role, o.Key, o.Task.Title, len(pool))
	out, err := h.stallFlow(ctx, run, issueworkflow.ReasonPoolTooSmall, reason, "")
	if err != nil {
		return advanceOutcome{}, err
	}
	out.action.Tasks = []string{o.Key}
	return out, nil
}

// flowRowIdentifiers turns the rows a text names ("step/key" with their round)
// into issue identifiers.
func flowRowIdentifiers(prefix string, rows []flowTaskRow, in []issueworkflow.TaskInput) []issueworkflow.TaskInput {
	out := make([]issueworkflow.TaskInput, 0, len(in))
	for _, t := range in {
		item := t
		for _, r := range rows {
			if issueworkflow.RowRef(r.state.Step, r.state.Key) == t.Identifier && r.state.Round == t.Round {
				item.Identifier = fmt.Sprintf("%s-%d", prefix, r.issue.Number)
				break
			}
		}
		out = append(out, item)
	}
	return out
}

// graphTaskText is the data the texts of a row of the current step are rendered from.
func (h *Handler) graphTaskText(run *flowRun, rows []flowTaskRow, o issueworkflow.OpenTask, prefix, stepName string) issueworkflow.GraphTaskText {
	inputs, feedback := issueworkflow.TaskLinks(run.def.Graph, run.state, taskStates(rows), o.Task)
	return issueworkflow.GraphTaskText{
		WorkflowName:      run.def.Name,
		RequestIdentifier: prefix + "-" + fmt.Sprint(run.issue.Number),
		StepName:          stepName,
		Task:              o.Task,
		Row:               o,
		Round:             run.round(),
		Inputs:            flowRowIdentifiers(prefix, rows, inputs),
		Feedback:          flowRowIdentifiers(prefix, rows, feedback),
	}
}

// openGraphTasks opens the rows a decision names for a step of a schema 2 flow.
func (h *Handler) openGraphTasks(ctx context.Context, run *flowRun, rows []flowTaskRow, open []issueworkflow.OpenTask) (advanceOutcome, error) {
	g := run.def.Graph
	step, _ := g.Step(run.state.Step)
	plan, stop, err := h.assignGraphRows(ctx, run, rows, open)
	if err != nil {
		return advanceOutcome{}, err
	}
	if stop != nil {
		return *stop, nil
	}
	prefix := h.getIssuePrefix(ctx, run.issue.WorkspaceID)
	stepName := step.Title
	if stepName == "" {
		stepName = issuestatus.NewResolver(run.issue.WorkspaceID).Name(ctx, h.Queries, step.Status.Key)
	}
	stage := int32(1)
	if i, ok := g.StepIndex(run.state.Step); ok {
		stage = int32(i + 1)
	}
	round := run.round()
	var opened []string
	for _, a := range plan {
		o := a.open
		title := fmt.Sprintf("%s · %s", stepName, o.Task.Title)
		if o.Count > 0 {
			title += fmt.Sprintf(" · %d/%d", o.Index, o.Count)
		}
		title += fmt.Sprintf(" · v%d", round+1)
		created, err := h.createFlowTask(ctx, run, prefix, flowTaskCreate{
			Title:       title,
			Description: issueworkflow.GraphTaskDescription(g, h.graphTaskText(run, rows, o, prefix, stepName)),
			Stage:       stage, Step: run.state.Step, Key: o.Key, Round: round,
			AssigneeTyp: a.typ, AssigneeID: a.id,
		})
		if err != nil {
			return advanceOutcome{}, err
		}
		if created {
			opened = append(opened, o.Key)
		}
	}
	if len(opened) == 0 {
		return advanceOutcome{action: FlowAction{Kind: "noop", Step: run.state.Step, Tasks: []string{}}}, nil
	}
	h.publishFlowUpdated(run.issue)
	return advanceOutcome{action: FlowAction{Kind: "opened_tasks", Step: run.state.Step, Tasks: opened}, progressed: true}, nil
}

// graphRowOf rebuilds the row of a task issue the engine opened: its task, the
// instance it is, and the item of the list it works on.
func graphRowOf(run *flowRun, task db.Issue) (issueworkflow.OpenTask, bool) {
	doc, ok := flowGraphTask(run.def.Graph, task.FlowStep.String, task.FlowTask.String)
	if !ok {
		return issueworkflow.OpenTask{}, false
	}
	row := issueworkflow.OpenTask{Key: task.FlowTask.String, Task: doc}
	if doc.Instances == nil {
		return row, true
	}
	if _, suffix, found := strings.Cut(row.Key, "~"); found {
		fmt.Sscan(suffix, &row.Index)
	}
	count := doc.Instances.Count
	var list []string
	if doc.Instances.FromVar != "" {
		if v, ok := run.state.Vars[doc.Instances.FromVar]; ok {
			list = flowStringList(v)
		}
		count = len(list)
		if doc.Instances.Max > 0 && count > doc.Instances.Max {
			count = doc.Instances.Max
		}
	}
	row.Count = count
	if row.Index >= 1 && row.Index <= len(list) {
		row.Item = list[row.Index-1]
	}
	return row, true
}

func flowStringList(v any) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// graphTaskBrief renders the brief of a run on a task row of a schema 2 flow.
func (h *Handler) graphTaskBrief(ctx context.Context, run *flowRun, rows []flowTaskRow, parent, issue db.Issue, prefix string) *TaskProjectWorkflowData {
	row, ok := graphRowOf(run, issue)
	if !ok {
		return nil
	}
	stepName := h.flowStepStatusKey(run, issue.FlowStep.String)
	if step, ok := run.def.Graph.Step(issue.FlowStep.String); ok && step.Title != "" {
		stepName = step.Title
	}
	text := h.graphTaskText(run, rows, row, prefix, stepName)
	text.TaskIdentifier = fmt.Sprintf("%s-%d", prefix, issue.Number)
	text.RequestIdentifier = fmt.Sprintf("%s-%d", prefix, parent.Number)
	text.Round = int(issue.FlowRound.Int32)
	return &TaskProjectWorkflowData{Brief: issueworkflow.GraphTaskBrief(run.def.Graph, text)}
}
