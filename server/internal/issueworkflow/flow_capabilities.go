package issueworkflow

import "sort"

// What a server of this version understands of a flow document (spec 2.7). The
// numbers come from the constants the parser and the validator use, so a client
// that reads them never disagrees with the server.

// SupportedSchemas are the schema versions the engine reads.
var SupportedSchemas = []int{1, FlowDocVersion}

// FlowLimits are the caps of a document and of a request that runs on it.
type FlowLimits struct {
	DocBytes            int `json:"doc_bytes"`
	Steps               int `json:"steps"`
	StatusNames         int `json:"status_names"`
	TasksPerStep        int `json:"tasks_per_step"`
	EdgesPerStep        int `json:"edges_per_step"`
	Roles               int `json:"roles"`
	Vars                int `json:"vars"`
	VerdictSets         int `json:"verdict_sets"`
	Loops               int `json:"loops"`
	LoopMax             int `json:"loop_max"`
	Instances           int `json:"instances"`
	ExprDepth           int `json:"expr_depth"`
	AfterDepth          int `json:"after_depth"`
	MessageRunes        int `json:"message_runes"`
	InstructionsRunes   int `json:"instructions_runes"`
	InputsPerTask       int `json:"inputs_per_task"`
	SetsPerTask         int `json:"sets_per_task"`
	SkillsPerTask       int `json:"skills_per_task"`
	DefaultTaskIssues   int `json:"default_max_task_issues"`
	MaxTaskIssues       int `json:"max_task_issues"`
	DefaultActivations  int `json:"default_max_activations"`
	SchemaOneSteps      int `json:"schema1_steps"`
	SchemaOneTasks      int `json:"schema1_tasks_per_step"`
	SchemaOneReworkMax  int `json:"schema1_rework_max"`
	DraftsPerSquad      int `json:"drafts_per_squad"`
	VersionsPerLineage  int `json:"versions_per_lineage"`
	DraftNameRunes      int `json:"draft_name_runes"`
	WorkflowNameRunes   int `json:"workflow_name_runes"`
	TaskTitleRunes      int `json:"task_title_runes"`
	DefaultTaskIdleMins int `json:"default_task_idle_minutes"`
}

// The caps on drafts and versions are the server's choice (spec 2.2).
const (
	MaxDraftsPerSquad     = 20
	MaxVersionsPerLineage = 50
	MaxDraftNameRunes     = 56
)

// Limits returns the caps of this server.
func Limits() FlowLimits {
	return FlowLimits{
		DocBytes: DocMaxBytes, Steps: DocMaxSteps, StatusNames: DocMaxStatusNames, TasksPerStep: DocMaxTasksPerStep,
		EdgesPerStep: DocMaxEdgesPerStep, Roles: DocMaxRoles, Vars: DocMaxVars, VerdictSets: DocMaxVerdictSets,
		Loops: DocMaxLoops, LoopMax: DocMaxLoopMax, Instances: DocMaxInstances, ExprDepth: DocMaxExprDepth,
		AfterDepth: DocMaxAfterDepth, MessageRunes: DocMaxMessageRunes, InstructionsRunes: DocMaxInstructionsRunes,
		InputsPerTask: DocMaxInputsPerTask, SetsPerTask: DocMaxSetsPerTask, SkillsPerTask: DocMaxSkillsPerTask,
		DefaultTaskIssues: DefaultMaxTaskIssues, MaxTaskIssues: DocMaxTaskIssuesHard, DefaultActivations: DefaultMaxActivations,
		SchemaOneSteps: MaxFlowSteps, SchemaOneTasks: MaxTasksPerStep, SchemaOneReworkMax: MaxReworkLimit,
		DraftsPerSquad: MaxDraftsPerSquad, VersionsPerLineage: MaxVersionsPerLineage, DraftNameRunes: MaxDraftNameRunes,
		WorkflowNameRunes: MaxNameRunes, TaskTitleRunes: MaxTaskTitleRunes, DefaultTaskIdleMins: DefaultTaskIdleMinutes,
	}
}

// FactInfo is a fact an expression can read, as a client lists it.
type FactInfo struct {
	Key    string   `json:"key"`
	Type   string   `json:"type"`
	Source string   `json:"source"`
	Ops    []string `json:"ops"`
	Values []string `json:"values,omitempty"`
}

func factInfo(s FactSpec) FactInfo {
	ops := s.Ops
	if ops == nil {
		ops = []string{}
	}
	return FactInfo{Key: s.Key, Type: s.Type, Source: s.Source, Ops: ops, Values: s.Values}
}

// BuiltinFacts are the facts every document has, with the placeholders <key> a
// document fills in (spec 2.6).
func BuiltinFacts() []FactInfo {
	out := make([]FactInfo, len(FactRegistry))
	for i, s := range FactRegistry {
		out[i] = factInfo(s)
	}
	return out
}

// OpsByType lists the operators each variable type takes.
func OpsByType() map[string][]string {
	out := map[string][]string{}
	for _, t := range []string{VarTypeBool, VarTypeEnum, VarTypeInt, VarTypeString, VarTypeListString} {
		out[t] = opsOfVar(t)
	}
	return out
}

// FactCatalog lists, for each place a document has an expression, the facts the
// expression may read there: "edge:<step>" for the conditions of the edges of a
// step and "task_when:<step>" for the conditions of its tasks. A task is opened
// before its step has a result, so what its condition reads excludes the join
// and the lead's verdict, and the tasks of its own step.
func FactCatalog(doc FlowDoc) map[string][]FactInfo {
	all := ExpandFacts(doc)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	out := map[string][]FactInfo{}
	for _, step := range doc.Steps {
		edge := make([]FactInfo, 0, len(all))
		for _, s := range all {
			edge = append(edge, factInfo(s))
		}
		out["edge:"+step.Key] = edge
		own := map[string]bool{}
		for _, t := range step.Tasks {
			own[t.Key] = true
		}
		when := []FactInfo{}
		for _, s := range all {
			if hasPrefixAny(s.Key, "join.", "lead.") {
				continue
			}
			if taskOfFact(s.Key, own) {
				continue
			}
			when = append(when, factInfo(s))
		}
		out["task_when:"+step.Key] = when
	}
	return out
}

func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// taskOfFact reports whether the fact is task.<key>.* of one of the tasks.
func taskOfFact(key string, tasks map[string]bool) bool {
	if !hasPrefixAny(key, "task.") {
		return false
	}
	rest := key[len("task."):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '.' {
			return tasks[rest[:i]]
		}
	}
	return false
}
