package issueworkflow

import "strings"

// ProjectLegacySteps is the steps[] an old client reads from a workflow of schema 2
// (spec 7.5): one element for each status of the row, with the tasks and the
// human-only mark of the steps of the graph that sit on that status. The graph
// itself is in flow_config; this keeps the old graph sheet drawing something
// instead of failing. Routing is not projected: the old sheet reads it from the
// transitions, and the graph of schema 2 has edges instead.
func ProjectLegacySteps(def Definition) []Step {
	out := make([]Step, len(def.Steps))
	for i, s := range def.Steps {
		out[i] = s
		for _, gs := range def.Graph.Doc.Steps {
			if gs.Status.Key != s.StatusKey {
				continue
			}
			if gs.Kind == StepKindManual {
				out[i].HumanOnly = true
			}
			for _, t := range gs.Tasks {
				out[i].Tasks = append(out[i].Tasks, legacyTask(def.Graph, t))
			}
		}
	}
	return out
}

func legacyTask(g Graph, t DocTask) Task {
	task := Task{Key: t.Key, Title: t.Title, Kind: t.Kind, Instructions: t.Instructions, Handler: Handler{Type: HandlerNone}}
	if role, ok := g.Role(t.Role); ok {
		switch role.Kind {
		case RoleKindAgent:
			if role.Bind != nil && len(role.Bind.AgentIDs) > 0 {
				task.Handler = Handler{Type: HandlerAgent, ID: role.Bind.AgentIDs[0]}
			}
		case RoleKindMember:
			if role.Bind != nil && len(role.Bind.MemberIDs) > 0 {
				task.Handler = Handler{Type: HandlerMember, ID: role.Bind.MemberIDs[0]}
			}
		}
	}
	if t.NeedsVerdict() {
		task.Requires = append(task.Requires, RequireOutcome)
	}
	if t.NeedsFiles() {
		task.Requires = append(task.Requires, RequireAttachment)
	}
	task.Title = strings.TrimSpace(task.Title)
	return task
}
