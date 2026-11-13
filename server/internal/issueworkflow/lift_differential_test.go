package issueworkflow

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// The G-L gate of the flow engine generalisation (plan P7, spec 9.3 item 2).
//
// For every catalog flow, every step, every rework count and every switch
// value, over every combination of task states, the frozen schema 1 oracle and
// the schema 2 DecideGraph on the lifted graph must decide the same: the kind,
// the tasks to open, the target or stop, the rework flag, the stop code, the
// human-stop flag and the edge. The oracle is never edited to make this pass,
// and neither is Lift or the comparison.

const (
	differentialFullLimit = 50000
	differentialSamples   = 20000
	// differentialSeed fixes the random samples of the steps too large to
	// enumerate. It is printed with a failure.
	differentialSeed = 20261113
)

// taskStateOptions are the states a task is put in: open, closed with no
// outcome, approved, rejected, blocked, cancelled, and open with a blocked
// report (a blocked status the system wrote).
var taskStateOptions = []string{"open", "closed", "approved", "rejected", "blocked", "cancelled", "open-blocked"}

func taskStateFor(step string, task Task, round int, option string) TaskState {
	ts := TaskState{Step: step, Key: task.Key, Kind: task.Kind, Round: round}
	switch option {
	case "closed":
		ts.Closed = true
	case "approved", "rejected", "blocked":
		ts.Closed, ts.Outcome = true, option
	case "cancelled":
		ts.Closed, ts.Cancelled = true, true
	case "open-blocked":
		ts.Outcome = OutcomeBlocked
	}
	return ts
}

// switchCombinations lists every value of every switch.
func switchCombinations(cfg *FlowConfig) []map[string]any {
	combos := []map[string]any{{}}
	for _, sw := range cfg.Switches {
		var values []any
		if sw.Type == SwitchEnum {
			for _, v := range sw.Values {
				values = append(values, v)
			}
		} else {
			values = []any{false, true}
		}
		var next []map[string]any
		for _, combo := range combos {
			for _, v := range values {
				c := map[string]any{}
				for k, x := range combo {
					c[k] = x
				}
				c[sw.Key] = v
				next = append(next, c)
			}
		}
		combos = next
	}
	return combos
}

// splitEdgeIDs maps the id of an edge that Lift made by splitting a transition to
// the id of the schema 1 transition it comes from. Lift writes the split edge
// right before the edge that keeps the transition's id.
func splitEdgeIDs(def Definition, g Graph) map[string]string {
	out := map[string]string{}
	for _, s := range def.Steps {
		edges := g.EdgesFrom(s.StatusKey)
		k := 0
		for _, t := range s.Transitions {
			if k < len(edges) && edges[k].Edge.ID != t.ID {
				out[edges[k].Edge.ID] = t.ID
				k++
			}
			k++
		}
	}
	return out
}

// compareDecisions returns what differs between the oracle and the graph decision.
func compareDecisions(o Decision, d GraphDecision, ids map[string]string, checkOutcome bool) string {
	var diffs []string
	if o.Kind != d.Kind {
		return fmt.Sprintf("kind: oracle %q, graph %q (reason %q)", o.Kind, d.Kind, d.Reason)
	}
	switch o.Kind {
	case DecisionOpen:
		var want, got []string
		for _, t := range o.Missing {
			want = append(want, t.Key)
		}
		for _, t := range d.Open {
			got = append(got, t.Key)
		}
		if strings.Join(want, ",") != strings.Join(got, ",") {
			diffs = append(diffs, fmt.Sprintf("tasks to open: oracle %v, graph %v", want, got))
		}
	case DecisionRoute:
		target := d.To
		if d.StopCode != "" {
			target = d.Status
		}
		if o.To != target {
			diffs = append(diffs, fmt.Sprintf("target: oracle %q, graph %q (stop code %q)", o.To, target, d.StopCode))
		}
		if o.Rework != d.Rework {
			diffs = append(diffs, fmt.Sprintf("rework: oracle %v, graph %v", o.Rework, d.Rework))
		}
		if o.Stop != d.Stop {
			diffs = append(diffs, fmt.Sprintf("stop: oracle %q, graph %q (code %q)", o.Stop, d.Stop, d.StopCode))
		}
		if o.HumanStop != d.HumanStop {
			diffs = append(diffs, fmt.Sprintf("human stop: oracle %v, graph %v", o.HumanStop, d.HumanStop))
		}
		id := d.TransitionID
		if orig, split := ids[id]; split {
			id = orig
		}
		if o.TransitionID != id {
			diffs = append(diffs, fmt.Sprintf("edge: oracle %q, graph %q", o.TransitionID, d.TransitionID))
		}
		if checkOutcome && o.TasksOutcome != "" {
			if want := joinResultOf[o.TasksOutcome]; want != d.JoinResult {
				diffs = append(diffs, fmt.Sprintf("join result: oracle %q (%s), graph %q", want, o.TasksOutcome, d.JoinResult))
			}
		}
	}
	return strings.Join(diffs, "; ")
}

// differentialCase is one state to compare.
type differentialCase struct {
	flow, step string
	state      FlowState
	options    []string
	noise      bool
}

func (c differentialCase) describe() string {
	return fmt.Sprintf("flow %s, step %s, rework %d, round_base %d, switches %v, task states %v, stale rows %v",
		c.flow, c.step, c.state.Rework, c.state.RoundBase, c.state.Switches, c.options, c.noise)
}

func (c differentialCase) rows(step Step) []TaskState {
	round := c.state.Round()
	var rows []TaskState
	for i, task := range step.Tasks {
		rows = append(rows, taskStateFor(step.StatusKey, task, round, c.options[i]))
	}
	if c.noise {
		// Rows of another round and of another step must not count.
		for _, task := range step.Tasks {
			stale := taskStateFor(step.StatusKey, task, round+1, "approved")
			rows = append(rows, stale)
			other := taskStateFor("elsewhere", task, round, "rejected")
			rows = append(rows, other)
		}
	}
	return rows
}

func TestLift_DecideEquivalent_AllCatalogFlows(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var compared, sampled, steps int64
	t.Cleanup(func() {
		t.Logf("G-L: %d flows, %d steps, %d states compared (%d of them random samples, seed %d)",
			len(ids), atomic.LoadInt64(&steps), atomic.LoadInt64(&compared), atomic.LoadInt64(&sampled), differentialSeed)
	})

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			def, _, err := flows[id].definition()
			if err != nil {
				t.Fatal(err)
			}
			g, err := Lift(def, LiftOpts{Runtime: true})
			if err != nil {
				t.Fatalf("Lift: %v", err)
			}
			edgeIDs := splitEdgeIDs(def, g)
			combos := switchCombinations(def.Flow)
			limit := 0
			if len(g.Doc.Loops) > 0 {
				limit = g.Doc.Loops[0].Max.N
			}

			check := func(c differentialCase, step Step) {
				atomic.AddInt64(&compared, 1)
				rows := c.rows(step)
				state := c.state
				oracle := oracleDecide(def, state, rows)
				v2 := Adapt(state)
				got := DecideGraph(g, v2, rows, BuildGraphFacts(g, v2, rows, RequestFacts{}))
				if diff := compareDecisions(oracle, got, edgeIDs, step.StatusKey != StartStatus); diff != "" {
					t.Fatalf("the lifted graph decides differently from the schema 1 oracle\n  %s\n  oracle: %+v\n  graph:  %+v\n  diff: %s\n  random seed %d",
						c.describe(), oracle, got, diff, differentialSeed)
				}
			}

			for si, step := range def.Steps {
				atomic.AddInt64(&steps, 1)
				nTasks := len(step.Tasks)
				space := len(combos) * (limit + 2) * 2 * 2
				for i := 0; i < nTasks; i++ {
					space *= len(taskStateOptions)
				}
				mk := func(combo map[string]any, rework, base int, noise bool, options []string) differentialCase {
					return differentialCase{flow: id, step: step.StatusKey, options: options, noise: noise,
						state: FlowState{Step: step.StatusKey, Rework: rework, RoundBase: base, Switches: combo}}
				}
				if space <= differentialFullLimit {
					options := make([]string, nTasks)
					var walk func(i int, combo map[string]any, rework, base int, noise bool)
					walk = func(i int, combo map[string]any, rework, base int, noise bool) {
						if i == nTasks {
							check(mk(combo, rework, base, noise, append([]string(nil), options...)), step)
							return
						}
						for _, o := range taskStateOptions {
							options[i] = o
							walk(i+1, combo, rework, base, noise)
						}
					}
					for _, combo := range combos {
						for rework := 0; rework <= limit+1; rework++ {
							for base := 0; base <= 1; base++ {
								for _, noise := range []bool{false, true} {
									walk(0, combo, rework, base, noise)
								}
							}
						}
					}
				} else {
					// Too many to enumerate: every boundary case, then random samples.
					uniform := func(o string) []string {
						out := make([]string, nTasks)
						for i := range out {
							out[i] = o
						}
						return out
					}
					for _, combo := range combos {
						for rework := 0; rework <= limit+1; rework++ {
							for base := 0; base <= 1; base++ {
								var vectors [][]string
								for _, o := range taskStateOptions {
									vectors = append(vectors, uniform(o))
									for i := 0; i < nTasks; i++ {
										for _, rest := range []string{"approved", "open"} {
											v := uniform(rest)
											v[i] = o
											vectors = append(vectors, v)
										}
									}
								}
								for _, v := range vectors {
									check(mk(combo, rework, base, false, v), step)
								}
							}
						}
					}
					rng := rand.New(rand.NewSource(differentialSeed + int64(si)))
					for n := 0; n < differentialSamples; n++ {
						options := make([]string, nTasks)
						for i := range options {
							options[i] = taskStateOptions[rng.Intn(len(taskStateOptions))]
						}
						atomic.AddInt64(&sampled, 1)
						check(mk(combos[rng.Intn(len(combos))], rng.Intn(limit+2), rng.Intn(2), rng.Intn(2) == 1, options), step)
					}
				}
			}

			// The plan step is routed once the lead has planned: RoutePlan against RouteLead.
			for _, combo := range combos {
				for rework := 0; rework <= limit+1; rework++ {
					for _, blocked := range []bool{false, true} {
						atomic.AddInt64(&compared, 1)
						state := FlowState{Step: StartStatus, Rework: rework, Switches: combo}
						oracle := oracleRoutePlan(def, state, blocked)
						verdict := "planned"
						if blocked {
							verdict = OutcomeBlocked
						}
						v2 := Adapt(state)
						got := RouteLead(g, v2, verdict, BuildGraphFacts(g, v2, nil, RequestFacts{}))
						if diff := compareDecisions(oracle, got, edgeIDs, false); diff != "" {
							t.Fatalf("the lifted plan step routes differently from RoutePlan\n  flow %s, rework %d, switches %v, blocked %v\n  oracle: %+v\n  graph:  %+v\n  diff: %s",
								id, rework, combo, blocked, oracle, got, diff)
						}
					}
				}
			}
		})
	}
}
