package issueworkflow

import (
	"fmt"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// entries builds a script list: n copies of the entry.
func entries(n int, entry map[string]string) []map[string]string {
	out := make([]map[string]string, n)
	for i := range out {
		out[i] = entry
	}
	return out
}

func wantTerminal(t *testing.T, tr Trace, terminal string) {
	t.Helper()
	if tr.Outcome != TraceTerminal || tr.Code != terminal {
		t.Fatalf("outcome %s %q, want terminal %q\npath %v", tr.Outcome, tr.Code, terminal, tr.Path())
	}
}

func wantTraceStop(t *testing.T, tr Trace, code string) {
	t.Helper()
	if tr.Outcome != TraceStop || tr.Code != code {
		t.Fatalf("outcome %s %q, want stop %q\npath %v", tr.Outcome, tr.Code, code, tr.Path())
	}
}

func wantPath(t *testing.T, tr Trace, path ...string) {
	t.Helper()
	if got := tr.Path(); !reflect.DeepEqual(got, path) {
		t.Fatalf("path = %v, want %v", got, path)
	}
}

func TestSimulate_Trade(t *testing.T) {
	g := loadGraph(t, "trade.json")
	main := []string{"plan", "collect", "draft", "gate", "handover", "release"}

	t.Run("success", func(t *testing.T) {
		tr := Simulate(g, nil, nil, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, main...)
		if got := tr.Steps[1].Opened; !reflect.DeepEqual(got, []string{"docs"}) {
			t.Errorf("collect opened %v: customs is off", got)
		}
		if tr.Steps[3].JoinResult != "pass" || tr.Steps[3].Edge != "gate_ok" {
			t.Errorf("gate = %+v", tr.Steps[3])
		}
		if tr.Final.Unreviewed || len(tr.Final.Checkpoints) != 1 || tr.Final.ClosedAt == "" {
			t.Errorf("the release is a checkpoint: %+v", tr.Final)
		}
		if tr.Final.Seq != 6 || len(tr.Final.Trail) != 6 {
			t.Errorf("seq %d, trail %d, want 6 and 6", tr.Final.Seq, len(tr.Final.Trail))
		}
	})
	t.Run("the optional tasks open on the switches", func(t *testing.T) {
		tr := Simulate(g, map[string]any{"customs_lookup": true, "export": true}, nil, 0)
		wantTerminal(t, tr, "done")
		if got := tr.Steps[1].Opened; !reflect.DeepEqual(got, []string{"docs", "customs"}) {
			t.Errorf("collect opened %v", got)
		}
		if got := tr.Steps[5].Opened; !reflect.DeepEqual(got, []string{"export"}) {
			t.Errorf("release opened %v", got)
		}
	})
	t.Run("rejected once, then passes", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"gate": {{"docs_gate": "rejected"}}}, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, "plan", "collect", "draft", "gate", "draft", "gate", "handover", "release")
		if tr.Final.Loops["fix"] != 1 {
			t.Errorf("loops = %v", tr.Final.Loops)
		}
		if tr.Steps[3].Edge != "gate_fix" || tr.Steps[3].JoinResult != "fail" || tr.Steps[3].To != "draft" {
			t.Errorf("first gate = %+v", tr.Steps[3])
		}
	})
	t.Run("rework limit reached", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"gate": entries(5, map[string]string{"customs_gate": "rejected"})}, 0)
		wantTraceStop(t, tr, "loop_exhausted")
		if tr.Visits("draft") != 3 || tr.Visits("gate") != 3 {
			t.Errorf("draft %d gate %d, want 3 and 3", tr.Visits("draft"), tr.Visits("gate"))
		}
		if tr.Final.Stop == nil || tr.Final.Stop.From != "gate" || tr.Final.BlockedFrom != "gate" || tr.Final.ClosedAt != "" {
			t.Errorf("parked state = %+v", tr.Final)
		}
	})
	t.Run("stop 1 at the plan", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"plan": {{ScriptLead: "blocked"}}}, 0)
		wantTraceStop(t, tr, "grounds_missing")
		wantPath(t, tr, "plan")
	})
	t.Run("stop 1 from a task", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"collect": {{"docs": "blocked"}}}, 0)
		wantTraceStop(t, tr, "grounds_missing")
		wantPath(t, tr, "plan", "collect")
		if tr.Steps[1].Edge != "collect_blocked" || tr.Steps[1].JoinResult != "block" {
			t.Errorf("collect = %+v", tr.Steps[1])
		}
	})
	t.Run("a verdict outside the set fails the gate", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"gate": {{"docs_gate": "maybe"}}}, 0)
		if tr.Steps[3].JoinResult != "fail" {
			t.Errorf("gate = %+v", tr.Steps[3])
		}
	})
}

func TestSimulate_Research(t *testing.T) {
	g := loadGraph(t, "research.json")
	// The decision step asks for another round every time it can.
	refine := map[string][]map[string]string{"decide": entries(10, map[string]string{"decide": "refine_query"})}
	for _, tt := range []struct {
		name string
		vars map[string]any
		want int
	}{
		{"two rounds", map[string]any{"max_rounds": 2}, 3},
		{"five rounds", map[string]any{"max_rounds": 5}, 6},
		{"nine is clamped at five", map[string]any{"max_rounds": 9}, 6},
		{"the default of three", nil, 4},
		{"none", map[string]any{"max_rounds": 0}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := Simulate(g, tt.vars, refine, 0)
			wantTerminal(t, tr, "done")
			if got := tr.Visits("search"); got != tt.want {
				t.Fatalf("search entered %d times, want %d\npath %v", got, tt.want, tr.Path())
			}
			if tr.Visits("synthesize") != 1 || tr.Visits("review") != 1 {
				t.Errorf("after the rounds the request goes on once: %v", tr.Path())
			}
		})
	}

	t.Run("the auditor sends the synthesis back twice, then the request stops", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"audit": entries(5, map[string]string{"auditor": "request_changes"})}, 0)
		wantTraceStop(t, tr, "loop_exhausted")
		if tr.Visits("synthesize") != 3 || tr.Visits("audit") != 3 || tr.Visits("search") != 1 {
			t.Errorf("path %v", tr.Path())
		}
	})
	t.Run("a conditional pass is a pass", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"audit": {{"auditor": "pass_with_conditions"}}}, 0)
		wantTerminal(t, tr, "done")
	})
	t.Run("the review has both reviewers", func(t *testing.T) {
		tr := Simulate(g, nil, nil, 0)
		if got := tr.Steps[tr.indexOf("review")].Opened; !reflect.DeepEqual(got, []string{"review_b", "review_a"}) {
			t.Errorf("review opened %v", got)
		}
	})
}

func (t Trace) indexOf(step string) int {
	for i, s := range t.Steps {
		if s.Step == step {
			return i
		}
	}
	return -1
}

func TestSimulate_Market(t *testing.T) {
	g := loadGraph(t, "market.json")
	sources := map[string]any{"sources": []any{"a", "b", "c"}}

	tr := Simulate(g, sources, nil, 0)
	wantTerminal(t, tr, "done")
	wantPath(t, tr, "plan", "collect", "verify", "synthesize", "critique", "edit")
	if got := tr.Steps[1].Opened; !reflect.DeepEqual(got, []string{"collect_source~1", "collect_source~2", "collect_source~3"}) {
		t.Errorf("collect opened %v, want one task per source", got)
	}
	if got := tr.Steps[2].Opened; !reflect.DeepEqual(got, []string{"source_check~1", "source_check~2", "source_check~3"}) {
		t.Errorf("verify opened %v", got)
	}

	t.Run("an unverified source sends the request back to collect", func(t *testing.T) {
		tr := Simulate(g, sources, map[string][]map[string]string{"verify": {{"source_check~2": "unverified"}}}, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, "plan", "collect", "verify", "collect", "verify", "synthesize", "critique", "edit")
		if tr.Final.Loops["recheck"] != 1 {
			t.Errorf("loops = %v", tr.Final.Loops)
		}
	})
	t.Run("verification is required: nothing but a pass reaches the synthesis", func(t *testing.T) {
		tr := Simulate(g, sources, map[string][]map[string]string{"verify": entries(5, map[string]string{"source_check": "unverified"})}, 0)
		wantTraceStop(t, tr, "loop_exhausted")
		if tr.Visits("synthesize") != 0 || tr.Visits("collect") != 3 {
			t.Errorf("path %v", tr.Path())
		}
	})
	t.Run("a list with no sources stalls", func(t *testing.T) {
		tr := Simulate(g, map[string]any{"sources": []any{}}, nil, 0)
		if tr.Outcome != TraceStalled || tr.Code != ReasonInstancesTooFew {
			t.Fatalf("outcome %s %q", tr.Outcome, tr.Code)
		}
	})
	t.Run("more sources than the maximum are cut", func(t *testing.T) {
		many := []any{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
		tr := Simulate(g, map[string]any{"sources": many}, nil, 0)
		if got := len(tr.Steps[1].Opened); got != 8 {
			t.Errorf("collect opened %d tasks, want 8", got)
		}
	})
}

func TestSimulate_Legal(t *testing.T) {
	g := loadGraph(t, "legal.json")
	reviewers := func(verdicts ...string) map[string]string {
		entry := map[string]string{}
		for i, v := range verdicts {
			entry[fmt.Sprintf("reviewer~%d", i+1)] = v
		}
		return entry
	}

	t.Run("three approvals of five finish the review", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"review": {reviewers("approve", "approve", "approve", "open", "open")}}, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, "plan", "intake", "redraft_clauses", "review", "counsel_signoff")
		review := tr.Steps[3]
		if len(review.Opened) != 5 || review.JoinResult != "pass" || review.Edge != "review_ok" {
			t.Errorf("review = %+v", review)
		}
		if want := []string{"review/reviewer~4", "review/reviewer~5"}; !reflect.DeepEqual(review.Cancelled, want) {
			t.Errorf("cancelled = %v, want the two reviewers left over %v", review.Cancelled, want)
		}
	})
	t.Run("three rejections of five end it early and go back to the clauses", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"review": {reviewers("reject", "reject", "reject", "open", "open")}}, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, "plan", "intake", "redraft_clauses", "review", "redraft_clauses", "review", "counsel_signoff")
		if tr.Steps[3].Edge != "review_fix" || tr.Steps[3].JoinResult != "fail" || len(tr.Steps[3].Cancelled) != 2 {
			t.Errorf("review = %+v", tr.Steps[3])
		}
		if tr.Final.Loops["clauses"] != 1 || tr.Final.Loops["cutback"] != 0 {
			t.Errorf("loops = %v", tr.Final.Loops)
		}
	})
	t.Run("a critical case goes back to the intake", func(t *testing.T) {
		tr := Simulate(g, map[string]any{"severity": "critical"}, map[string][]map[string]string{"review": {reviewers("reject", "reject", "reject", "open", "open")}}, 0)
		wantTerminal(t, tr, "done")
		wantPath(t, tr, "plan", "intake", "redraft_clauses", "review", "intake", "redraft_clauses", "review", "counsel_signoff")
		if tr.Steps[3].Edge != "review_cut" || tr.Final.Loops["cutback"] != 1 {
			t.Errorf("review = %+v loops %v", tr.Steps[3], tr.Final.Loops)
		}
	})
	t.Run("a critical case goes back to the intake once, then to the clauses", func(t *testing.T) {
		tr := Simulate(g, map[string]any{"severity": "critical"}, map[string][]map[string]string{"review": entries(2, reviewers("reject", "reject", "reject", "open", "open"))}, 0)
		wantPath(t, tr, "plan", "intake", "redraft_clauses", "review", "intake", "redraft_clauses", "review", "redraft_clauses", "review", "counsel_signoff")
	})
	t.Run("abstentions do not make a quorum", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"review": {reviewers("approve", "approve", "abstain", "abstain", "abstain")}}, 0)
		if tr.Steps[3].JoinResult != "fail" {
			t.Errorf("review = %+v", tr.Steps[3])
		}
	})
	t.Run("the clauses loop runs out", func(t *testing.T) {
		tr := Simulate(g, nil, map[string][]map[string]string{"review": entries(8, reviewers("reject", "reject", "reject", "reject", "reject"))}, 0)
		wantTraceStop(t, tr, "loop_exhausted")
		if tr.Visits("review") != 4 || tr.Visits("redraft_clauses") != 4 {
			t.Errorf("path %v", tr.Path())
		}
	})
}

// The files that implement the pure layer must not reach a database, a clock, a
// network or a process: the simulation has to run anywhere, and the engine must
// get the same answers from the same inputs.
func TestSimulate_HasNoDatabaseDependency(t *testing.T) {
	banned := []string{"context", "database/sql", "github.com/jackc/pgx", "net/http", "os", "time", "math/rand", "os/exec",
		"github.com/multica-ai/multica/server/pkg/db", "github.com/multica-ai/multica/server/internal/handler", "github.com/multica-ai/multica/server/internal/service"}
	for _, file := range []string{"decide.go", "links.go", "assign.go", "simulate.go", "graph.go", "facts.go", "expr.go", "state.go", "lift.go", "adapt.go"} {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, b := range banned {
				if path == b || strings.HasPrefix(path, b+"/") {
					t.Errorf("%s imports %s", file, path)
				}
			}
		}
	}
	// The package itself still imports pgtype for the issue columns it reads
	// (flow.go); that is the only reason pgx is among its dependencies.
}

func TestSimulate_Truncates(t *testing.T) {
	g := loadGraph(t, "trade.json")
	tr := Simulate(g, nil, nil, 3)
	if !tr.Truncated || tr.Outcome != TraceTruncated || len(tr.Steps) != 3 {
		t.Fatalf("outcome %s, truncated %v, %d steps; want 3 steps cut off", tr.Outcome, tr.Truncated, len(tr.Steps))
	}
	wantPath(t, tr, "plan", "collect", "draft")

	// Never more than 120 hops, however many are asked for.
	doc := FlowDoc{Version: 2, Start: "s0", Terminals: []Terminal{{Key: "done", Status: "done"}}, Policy: Policy{MaxActivations: 500}}
	for i := 0; i < 130; i++ {
		to := fmt.Sprintf("s%d", i+1)
		if i == 129 {
			to = "done"
		}
		doc.Steps = append(doc.Steps, DocStep{Key: fmt.Sprintf("s%d", i), Kind: StepKindTasks, Edges: []DocEdge{{ID: "next", To: to}}})
	}
	long := Simulate(NewGraph(doc), nil, nil, 1000)
	if !long.Truncated || len(long.Steps) != MaxSimulateHops {
		t.Fatalf("a 130 step chain: %d steps, truncated %v, want %d and true", len(long.Steps), long.Truncated, MaxSimulateHops)
	}
	if exact := Simulate(NewGraph(doc), nil, nil, 130); !exact.Truncated || len(exact.Steps) != MaxSimulateHops {
		t.Fatalf("130 hops asked for must still be cut at %d: %d steps", MaxSimulateHops, len(exact.Steps))
	}
	if short := Simulate(NewGraph(doc), nil, nil, 50); !short.Truncated || len(short.Steps) != 50 {
		t.Fatalf("50 hops: %d steps", len(short.Steps))
	}
}

func TestSimulate_AnalysisOfTheSampleFlows(t *testing.T) {
	trade := Analyze(loadFlowDoc(t, "trade.json"))
	if trade.WorstCaseActivations != 10 || trade.WorstCaseTaskIssues != 13 {
		t.Errorf("trade: activations %d tasks %d, want 10 and 13", trade.WorstCaseActivations, trade.WorstCaseTaskIssues)
	}
	if len(trade.Loops) != 1 || trade.Loops[0].Key != "fix" || trade.Loops[0].Max != 2 || !reflect.DeepEqual(trade.Loops[0].Body, []string{"draft", "gate"}) {
		t.Errorf("trade loops = %+v", trade.Loops)
	}
	var codes []string
	for _, s := range trade.Stops {
		codes = append(codes, fmt.Sprintf("%s:%d", s.Code, len(s.Edges)))
	}
	if want := []string{"grounds_missing:5", "loop_exhausted:2"}; !reflect.DeepEqual(codes, want) {
		t.Errorf("trade stops = %v, want %v", codes, want)
	}
	if len(trade.ReleasePathsWithoutHuman) != 0 {
		t.Errorf("the release is a manual exit: %+v", trade.ReleasePathsWithoutHuman)
	}

	nested := Analyze(loadFlowDoc(t, "patterns/p09_nested_cycles.json"))
	if nested.WorstCaseActivations != 17 {
		t.Errorf("nested cycles: activations %d, want plan 1 + a 2 + b 6 + c 6 + d 2 = 17", nested.WorstCaseActivations)
	}

	research := Analyze(loadFlowDoc(t, "research.json"))
	if research.WorstCaseActivations != 27 {
		t.Errorf("research: activations %d, want 27 (rounds are clamped at their cap of 5)", research.WorstCaseActivations)
	}

	t.Run("a release that no person sees", func(t *testing.T) {
		a := Analyze(releaseDoc(false, "checker"))
		if len(a.ReleasePathsWithoutHuman) == 0 {
			t.Fatal("an agent gate lets the work leave unseen")
		}
		p := a.ReleasePathsWithoutHuman[0]
		if p.From != "plan" || p.Terminal != "done" || !reflect.DeepEqual(p.Path, []string{"plan", "work", "check", "done"}) {
			t.Errorf("path = %+v", p)
		}
		if human := Analyze(releaseDoc(false, "person")); len(human.ReleasePathsWithoutHuman) != 0 {
			t.Errorf("a gate for a person is a checkpoint: %+v", human.ReleasePathsWithoutHuman)
		}
	})
	t.Run("a loop with no header bounds every step", func(t *testing.T) {
		doc := FlowDoc{Version: 2, Start: "a", Terminals: []Terminal{{Key: "done", Status: "done"}},
			Loops: []Loop{{Key: "r", Max: LoopMax{N: 3}, Reset: LoopResetNever}},
			Steps: []DocStep{
				{Key: "a", Kind: StepKindTasks, Edges: []DocEdge{{ID: "x", To: "b"}}},
				{Key: "b", Kind: StepKindTasks, Edges: []DocEdge{{ID: "y", To: "a", Loop: "r"}, {ID: "z", To: "done"}}},
			}}
		if got := Analyze(doc).WorstCaseActivations; got != 8 {
			t.Errorf("activations = %d, want 4 + 4", got)
		}
	})
}

// adversarialScript makes every gate fail, or fail and block at random, and
// every work task finish or block at random.
func adversarialScript(doc FlowDoc, rng *rand.Rand, chaos bool) map[string][]map[string]string {
	script := map[string][]map[string]string{}
	for _, s := range doc.Steps {
		for n := 0; n < 40; n++ {
			entry := map[string]string{}
			for _, task := range s.Tasks {
				switch {
				case chaos && rng.Intn(12) == 0:
					entry[task.Key] = OutcomeBlocked
				case task.Kind == KindGate:
					entry[task.Key] = firstOfClass(doc, task, ClassFail)
					if chaos && rng.Intn(3) == 0 {
						entry[task.Key] = firstOfClass(doc, task, ClassPass)
					}
				}
			}
			script[s.Key] = append(script[s.Key], entry)
		}
	}
	return script
}

func firstOfClass(doc FlowDoc, task DocTask, class string) string {
	for _, v := range verdictsOf(doc, task.Kind, task.Verdicts) {
		if v.Class == class {
			return v.Key
		}
	}
	return "nothing"
}

// sampleVars gives the flows that need a list something to fan out over.
func sampleVars(name string) map[string]any {
	switch filepath.Base(name) {
	case "market.json":
		return map[string]any{"sources": []any{"a", "b", "c"}}
	case "research.json":
		return map[string]any{"max_rounds": 9}
	case "p06_instances_from_var.json":
		return map[string]any{"items": []any{"a", "b", "c", "d"}}
	}
	return nil
}

func TestAnalyze_BoundCoversAdversarialSimulation(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "flows", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	patterns, _ := filepath.Glob(filepath.Join("testdata", "flows", "patterns", "*.json"))
	files = append(files, patterns...)
	if len(files) < 18 {
		t.Fatalf("found %d sample flows", len(files))
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			doc := loadFlowDoc(t, strings.TrimPrefix(file, filepath.Join("testdata", "flows")+string(filepath.Separator)))
			a := Analyze(doc)
			g := NewGraph(doc)
			bounds := map[string]int{}
			for _, s := range a.Steps {
				bounds[s.Step] = s.MaxEntries
			}
			for seed := int64(0); seed < 25; seed++ {
				rng := rand.New(rand.NewSource(seed))
				tr := Simulate(g, sampleVars(file), adversarialScript(doc, rng, seed > 0), 0)
				if tr.Truncated {
					t.Fatalf("seed %d: the simulation was cut off at %d hops; the bound is %d", seed, len(tr.Steps), a.WorstCaseActivations)
				}
				if len(tr.Steps) > a.WorstCaseActivations {
					t.Errorf("seed %d: %d entries, the bound is %d", seed, len(tr.Steps), a.WorstCaseActivations)
				}
				opened := 0
				for _, s := range tr.Steps {
					opened += len(s.Opened)
				}
				if opened > a.WorstCaseTaskIssues {
					t.Errorf("seed %d: %d task issues, the bound is %d", seed, opened, a.WorstCaseTaskIssues)
				}
				for step, bound := range bounds {
					if got := tr.Visits(step); got > bound {
						t.Errorf("seed %d: step %s entered %d times, the bound is %d", seed, step, got, bound)
					}
				}
			}
		})
	}
}

// randomDoc builds a valid graph: forward edges lead to later steps, every back
// edge has a loop with a bound, and the last edge of every step has no condition.
func randomDoc(rng *rand.Rand) FlowDoc {
	n := 2 + rng.Intn(5)
	doc := FlowDoc{Version: 2, Start: "plan",
		Roles:     []Role{{Key: "worker", Kind: RoleKindAgent}, {Key: "checker", Kind: RoleKindAgent}},
		Stops:     []StopKind{{Code: "loop_exhausted", Class: "loop_exhausted", LegacyNo: 3}},
		Terminals: []Terminal{{Key: "done", Status: "done"}},
		Steps:     []DocStep{{Key: "plan", Kind: StepKindLead, Edges: []DocEdge{{ID: "plan_ok", To: "s1"}}}},
	}
	loops := 0
	for i := 1; i <= n; i++ {
		next := "done"
		if i < n {
			next = fmt.Sprintf("s%d", i+1)
		}
		s := DocStep{Key: fmt.Sprintf("s%d", i), Kind: StepKindTasks}
		if rng.Intn(2) == 0 {
			s.Tasks = []DocTask{{Key: "work", Title: "w", Kind: KindWork, Role: "worker"}}
			s.Edges = []DocEdge{{ID: "next", To: next}}
		} else {
			count := 1 + rng.Intn(3)
			gate := DocTask{Key: "gate", Title: "g", Kind: KindGate, Role: "checker"}
			if count > 1 {
				gate.Instances = &Instances{Count: count}
			}
			s.Tasks = []DocTask{gate}
			s.Join = &Join{Over: JoinOver{Kind: JoinOverGates}, Mode: []string{JoinModeAll, JoinModeAny, JoinModeQuorum}[rng.Intn(3)], K: 1 + rng.Intn(count)}
			s.Edges = []DocEdge{{ID: "pass", To: next, When: &Condition{Field: "join.result", Op: OpEq, Value: "pass"}}}
			for b := rng.Intn(3); b > 0 && loops < 3; b-- {
				loops++
				key := fmt.Sprintf("l%d", loops)
				max := 1 + rng.Intn(2)
				doc.Loops = append(doc.Loops, Loop{Key: key, Header: fmt.Sprintf("s%d", 1+rng.Intn(i)), Max: LoopMax{N: max}})
				s.Edges = append(s.Edges, DocEdge{ID: "back_" + key, To: doc.Loops[len(doc.Loops)-1].Header, Loop: key,
					When: &Condition{All: []Condition{{Field: "join.result", Op: OpEq, Value: "fail"}, {Field: "loop." + key + ".count", Op: OpLt, Value: max}}}})
			}
			if rng.Intn(2) == 0 {
				s.Edges = append(s.Edges, DocEdge{ID: "end", Stop: "loop_exhausted"})
			} else {
				s.Edges = append(s.Edges, DocEdge{ID: "end", To: next})
			}
		}
		doc.Steps = append(doc.Steps, s)
	}
	return doc
}

func TestDecide_AlwaysTerminatesWithinBound(t *testing.T) {
	rng := rand.New(rand.NewSource(20261113))
	checked, cut := 0, 0
	for n := 0; n < 300; n++ {
		doc := randomDoc(rng)
		a := Analyze(doc)
		if a.WorstCaseActivations > MaxSimulateHops {
			cut++
			continue
		}
		checked++
		g := NewGraph(doc)
		for seed := int64(0); seed < 6; seed++ {
			tr := Simulate(g, nil, adversarialScript(doc, rand.New(rand.NewSource(seed)), seed%2 == 1), 0)
			if tr.Truncated || tr.Outcome == TraceWaiting {
				t.Fatalf("graph %d seed %d: %s after %d entries (bound %d)\n%s", n, seed, tr.Outcome, len(tr.Steps), a.WorstCaseActivations, describeDoc(doc))
			}
			if len(tr.Steps) > a.WorstCaseActivations {
				t.Fatalf("graph %d seed %d: %d entries, bound %d\n%s", n, seed, len(tr.Steps), a.WorstCaseActivations, describeDoc(doc))
			}
			for _, s := range a.Steps {
				if got := tr.Visits(s.Step); got > s.MaxEntries {
					t.Fatalf("graph %d seed %d: %s entered %d times, bound %d\n%s", n, seed, s.Step, got, s.MaxEntries, describeDoc(doc))
				}
			}
		}
	}
	if checked < 150 {
		t.Fatalf("only %d of 300 graphs fit the simulation (%d were over %d hops)", checked, cut, MaxSimulateHops)
	}
}

func describeDoc(doc FlowDoc) string {
	var b strings.Builder
	for _, s := range doc.Steps {
		fmt.Fprintf(&b, "%s", s.Key)
		if s.Join != nil {
			fmt.Fprintf(&b, " [%s k=%d]", s.Join.Mode, s.Join.K)
		}
		for _, e := range s.Edges {
			fmt.Fprintf(&b, " %s->%s%s%s", e.ID, e.To, e.Stop, map[bool]string{true: "(" + e.Loop + ")", false: ""}[e.Loop != ""])
		}
		b.WriteString("\n")
	}
	for _, l := range doc.Loops {
		fmt.Fprintf(&b, "loop %s header %s max %d\n", l.Key, l.Header, l.Max.N)
	}
	return b.String()
}

// patternCase is a run of a pattern document and where it must end.
type patternCase struct {
	file    string
	name    string
	vars    map[string]any
	script  map[string][]map[string]string
	outcome string
	code    string
	path    []string
	opened  map[int][]string // entry index -> rows opened
	cancel  map[int][]string // entry index -> rows superseded
	visits  map[string]int
}

func TestPatterns_AllParseAndSimulate(t *testing.T) {
	cases := []patternCase{
		{file: "p01_sequential", outcome: TraceTerminal, code: "done", path: []string{"plan", "first", "second"}, opened: map[int][]string{1: {"one", "two"}, 2: {"three"}}},
		{file: "p02_parallel_join_all", outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"ta", "tb", "tc"}}},
		{file: "p03_exclusive_choice_merge", name: "route a", vars: map[string]any{"route": "a"}, outcome: TraceTerminal, code: "done", path: []string{"plan", "branch_a", "merge"}},
		{file: "p03_exclusive_choice_merge", name: "route b", vars: map[string]any{"route": "b"}, outcome: TraceTerminal, code: "done", path: []string{"plan", "branch_b", "merge"}},
		{file: "p03_exclusive_choice_merge", name: "the default", outcome: TraceTerminal, code: "done", path: []string{"plan", "branch_c", "merge"}},
		{file: "p04_multi_choice", name: "a only", vars: map[string]any{"want_a": true}, outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"task_a", "task_c"}}},
		{file: "p04_multi_choice", name: "a and b", vars: map[string]any{"want_a": true, "want_b": true}, outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"task_a", "task_b", "task_c"}}},
		{file: "p04_multi_choice", name: "neither", outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"task_c"}}},
		{file: "p05_instances_count", outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"item~1", "item~2", "item~3"}}},
		{file: "p06_instances_from_var", vars: map[string]any{"items": []any{"x", "y"}}, outcome: TraceTerminal, code: "done", path: []string{"plan", "work"}, opened: map[int][]string{1: {"item~1", "item~2"}}},
		{file: "p07_discriminator_any", script: map[string][]map[string]string{"race": {{"g1": "open", "g2": "approved", "g3": "open"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "race"}, cancel: map[int][]string{1: {"race/g1", "race/g3"}}},
		{file: "p08_n_of_m_quorum", script: map[string][]map[string]string{"vote": {{"v1": "approved", "v2": "approved", "v3": "open"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "vote"}, cancel: map[int][]string{1: {"vote/v3"}}},
		{file: "p08_n_of_m_quorum", name: "no majority", script: map[string][]map[string]string{"vote": {{"v1": "rejected", "v2": "rejected", "v3": "open"}}},
			outcome: TraceStop, code: "loop_exhausted", path: []string{"plan", "vote"}},
		{file: "p09_nested_cycles", name: "approved", outcome: TraceTerminal, code: "done", path: []string{"plan", "a", "b", "c", "d"}},
		{file: "p09_nested_cycles", name: "everything rejected", script: map[string][]map[string]string{
			"c": entries(8, map[string]string{"gate_c": "rejected"}), "d": entries(8, map[string]string{"gate_d": "rejected"})},
			outcome: TraceStop, code: "loop_exhausted", visits: map[string]int{"a": 2, "b": 6, "c": 6, "d": 2}},
		{file: "p10_deferred_choice", name: "the first exit", outcome: TraceTerminal, code: "done", path: []string{"plan", "work", "choose"}},
		{file: "p10_deferred_choice", name: "the other exit", script: map[string][]map[string]string{"choose": {{ScriptExit: "choose_cancel"}}}, outcome: TraceTerminal, code: "cancelled", path: []string{"plan", "work", "choose"}},
		{file: "p11_cancel", name: "cancel with a task open", script: map[string][]map[string]string{"hold": {{"export": "open", ScriptExit: "hold_cancel"}}},
			outcome: TraceTerminal, code: "cancelled", path: []string{"plan", "hold"}, cancel: map[int][]string{1: {"hold/export"}}},
		{file: "p11_cancel", name: "go on", outcome: TraceTerminal, code: "done", path: []string{"plan", "hold"}},
		{file: "p12_redo_failed_branch", script: map[string][]map[string]string{"review": {{"fe_review": "approved", "be_review": "rejected"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "build", "review", "build", "review"},
			opened: map[int][]string{1: {"fe", "be"}, 2: {"fe_review", "be_review"}, 3: {"be"}, 4: {"be_review"}}},
		{file: "p12_redo_failed_branch", name: "both fail twice", script: map[string][]map[string]string{"review": {
			{"fe_review": "rejected", "be_review": "rejected"}, {"fe_review": "rejected", "be_review": "approved"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "build", "review", "build", "review", "build", "review"},
			opened: map[int][]string{3: {"fe", "be"}, 4: {"fe_review", "be_review"}, 5: {"fe"}, 6: {"fe_review"}}},
		{file: "crossreview", name: "both approve", outcome: TraceTerminal, code: "done", path: []string{"plan", "build", "review"}, opened: map[int][]string{2: {"ana_checks_ben", "ben_checks_ana"}}},
		{file: "crossreview", name: "one rejects", script: map[string][]map[string]string{"review": {{"ben_checks_ana": "rejected"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "build", "review", "build", "review"}},
		{file: "planner_fanout", name: "the planner names three topics", script: map[string][]map[string]string{"planning": {{ScriptVarPrefix + "topics": "alpha, beta, gamma"}}},
			outcome: TraceTerminal, code: "done", path: []string{"plan", "planning", "fanout", "check"}, opened: map[int][]string{2: {"research_topic~1", "research_topic~2", "research_topic~3"}}},
		{file: "planner_fanout", name: "the planner names none", outcome: TraceStalled, code: ReasonInstancesTooFew, path: []string{"plan", "planning", "fanout"}},
	}

	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.file] = true
		name := c.file
		if c.name != "" {
			name += "/" + c.name
		}
		t.Run(name, func(t *testing.T) {
			dir := ""
			if strings.HasPrefix(c.file, "p") && c.file != "planner_fanout" {
				dir = "patterns/"
			}
			g := loadGraph(t, dir+c.file+".json")
			tr := Simulate(g, c.vars, c.script, 0)
			if tr.Outcome != c.outcome || tr.Code != c.code {
				t.Fatalf("outcome %s %q, want %s %q\npath %v", tr.Outcome, tr.Code, c.outcome, c.code, tr.Path())
			}
			if c.path != nil && !reflect.DeepEqual(tr.Path(), c.path) {
				t.Fatalf("path = %v, want %v", tr.Path(), c.path)
			}
			for i, want := range c.opened {
				if got := tr.Steps[i].Opened; !reflect.DeepEqual(got, want) {
					t.Errorf("entry %d (%s) opened %v, want %v", i, tr.Steps[i].Step, got, want)
				}
			}
			for i, want := range c.cancel {
				if got := tr.Steps[i].Cancelled; !reflect.DeepEqual(got, want) {
					t.Errorf("entry %d (%s) cancelled %v, want %v", i, tr.Steps[i].Step, got, want)
				}
			}
			for step, want := range c.visits {
				if got := tr.Visits(step); got != want {
					t.Errorf("step %s entered %d times, want %d", step, got, want)
				}
			}
		})
	}

	t.Run("every pattern document has a case", func(t *testing.T) {
		files, _ := filepath.Glob(filepath.Join("testdata", "flows", "patterns", "*.json"))
		if len(files) != 12 {
			t.Fatalf("%d pattern documents, want 12 (patterns 1 to 12; 15 is TestPatterns_Pattern15_LateTaskOpensWhenVariableChanges)", len(files))
		}
		var missing []string
		for _, f := range files {
			if base := strings.TrimSuffix(filepath.Base(f), ".json"); !covered[base] {
				missing = append(missing, base)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Fatalf("no case for %v", missing)
		}
	})
	t.Run("every sample flow parses cleanly", func(t *testing.T) {
		files, _ := filepath.Glob(filepath.Join("testdata", "flows", "*.json"))
		files2, _ := filepath.Glob(filepath.Join("testdata", "flows", "patterns", "*.json"))
		for _, f := range append(files, files2...) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, issues := ParseFlowDoc(raw); len(issues) > 0 {
				t.Errorf("%s: %+v", f, issues)
			}
		}
	})
}
