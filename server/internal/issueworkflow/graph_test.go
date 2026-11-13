package issueworkflow

import (
	"reflect"
	"testing"
)

// gstep builds a step with the edges given; an edge is "id>target" or
// "id>target@loop".
func gstep(key string, edges ...DocEdge) DocStep {
	return DocStep{Key: key, Kind: StepKindTasks, Edges: edges}
}

func fwd(id, to string) DocEdge        { return DocEdge{ID: id, To: to} }
func back(id, to, loop string) DocEdge { return DocEdge{ID: id, To: to, Loop: loop} }
func park(id, stop string) DocEdge     { return DocEdge{ID: id, Stop: stop} }
func graphOf(steps ...DocStep) Graph {
	return NewGraph(FlowDoc{Version: 2, Start: steps[0].Key, Steps: steps, Terminals: []Terminal{{Key: "done", Status: "done"}}})
}

func withLoops(g Graph, loops ...Loop) Graph {
	doc := g.Doc
	doc.Loops = loops
	return NewGraph(doc)
}

func TestGraph_IndexesAndBackEdges(t *testing.T) {
	doc := parseSample(t)
	g := NewGraph(doc)
	if g.Compat != CompatNone {
		t.Fatalf("Compat = %q, want none", g.Compat)
	}

	for i, s := range doc.Steps {
		got, ok := g.Step(s.Key)
		idx, _ := g.StepIndex(s.Key)
		if !ok || got.Key != s.Key || idx != i {
			t.Fatalf("Step(%q) = %v at %d, want index %d", s.Key, ok, idx, i)
		}
	}
	if _, ok := g.Step("nowhere"); ok {
		t.Fatal("Step found a step that does not exist")
	}
	if term, ok := g.Terminal("done"); !ok || !term.LeavesWorkspace {
		t.Fatalf("Terminal(done) = %+v, %v", term, ok)
	}
	if _, ok := g.Terminal("plan"); ok {
		t.Fatal("a step is not a terminal")
	}
	if l, ok := g.Loop("fix"); !ok || l.Header != "draft" {
		t.Fatalf("Loop(fix) = %+v, %v", l, ok)
	}
	if _, ok := g.Loop("none"); ok {
		t.Fatal("Loop found a loop that does not exist")
	}

	edges := g.EdgesFrom("check")
	var ids []string
	for i, e := range edges {
		ids = append(ids, e.Edge.ID)
		if e.From != "check" || e.Index != i {
			t.Fatalf("edge %d = %+v", i, e)
		}
	}
	if want := []string{"check_fix", "check_ok", "check_end"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("EdgesFrom(check) = %v, want %v (the order edges are tried in)", ids, want)
	}
	if e, ok := g.Edge("check", "check_ok"); !ok || e.Edge.To != "release" || e.Back || e.Index != 1 {
		t.Fatalf("Edge(check, check_ok) = %+v, %v", e, ok)
	}
	if _, ok := g.Edge("draft", "check_ok"); ok {
		t.Fatal("an edge id is only unique within its step")
	}

	backs := g.BackEdges()
	if len(backs) != 1 || backs[0].Edge.ID != "check_fix" || !backs[0].Back {
		t.Fatalf("BackEdges = %+v, want only check_fix", backs)
	}
	if got := g.BackEdgesOf("fix"); len(got) != 1 || got[0].Edge.ID != "check_fix" {
		t.Fatalf("BackEdgesOf(fix) = %+v", got)
	}
	if got := g.BackEdgesOf("retry"); len(got) != 0 {
		t.Fatalf("BackEdgesOf(retry) = %+v, want none", got)
	}
	if e, _ := g.Edge("check", "check_end"); e.Back || e.Edge.Stop != "loop_exhausted" {
		t.Fatalf("a stop edge is not a back-edge: %+v", e)
	}
	if got := g.EdgesFrom("nowhere"); len(got) != 0 {
		t.Fatalf("EdgesFrom(nowhere) = %+v", got)
	}
}

func TestGraph_LoopBodies(t *testing.T) {
	loop := func(key, header string) Loop { return Loop{Key: key, Header: header, Max: LoopMax{N: 3}} }
	body := func(g Graph, key string) []string { return g.LoopBody(key) }

	t.Run("a simple loop includes both ends and the steps between", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("a", fwd("a1", "b")),
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "l1"), fwd("c1", "d")),
			gstep("d", fwd("d1", "done")),
		), loop("l1", "b"))
		if got, want := body(g, "l1"), []string{"b", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("body = %v, want %v", got, want)
		}
	})

	t.Run("only steps on a path to the back-edge source count", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("b", fwd("b_x", "x"), fwd("b_c", "c"), fwd("b_y", "y")),
			gstep("x", fwd("x_c", "c")),
			gstep("y", fwd("y_end", "done")),
			gstep("c", back("c_back", "b", "l1"), fwd("c_end", "done")),
		), loop("l1", "b"))
		if got, want := body(g, "l1"), []string{"b", "x", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("body = %v, want %v (y leaves the loop and never reaches c)", got, want)
		}
	})

	t.Run("a loop of one step", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("a", fwd("a1", "b")),
			gstep("b", back("b_back", "b", "l1"), fwd("b1", "done")),
		), loop("l1", "b"))
		if got, want := body(g, "l1"), []string{"b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("body = %v, want %v", got, want)
		}
	})

	t.Run("two loops that do not nest", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("a", fwd("a1", "b")),
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "first"), fwd("c1", "d")),
			gstep("d", fwd("d1", "e")),
			gstep("e", fwd("e1", "f")),
			gstep("f", back("f_back", "e", "second"), fwd("f1", "done")),
		), loop("first", "b"), loop("second", "e"))
		if got, want := body(g, "first"), []string{"b", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("first = %v, want %v", got, want)
		}
		if got, want := body(g, "second"), []string{"e", "f"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("second = %v, want %v", got, want)
		}
	})

	t.Run("nested loops", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("a", fwd("a1", "b")),
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "inner"), fwd("c1", "d")),
			gstep("d", back("d_back", "a", "outer"), fwd("d1", "done")),
		), loop("outer", "a"), loop("inner", "b"))
		if got, want := body(g, "outer"), []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("outer = %v, want %v", got, want)
		}
		if got, want := body(g, "inner"), []string{"b", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("inner = %v, want %v", got, want)
		}
	})

	t.Run("two loops with the same header are independent", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "short"), fwd("c1", "d")),
			gstep("d", back("d_back", "b", "long"), fwd("d1", "done")),
		), loop("short", "b"), loop("long", "b"))
		if got, want := body(g, "short"), []string{"b", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("short = %v, want %v", got, want)
		}
		if got, want := body(g, "long"), []string{"b", "c", "d"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("long = %v, want %v", got, want)
		}
	})

	t.Run("a loop with two back-edges covers both", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "l1"), fwd("c1", "d")),
			gstep("d", back("d_back", "b", "l1"), fwd("d1", "done")),
		), loop("l1", "b"))
		if got, want := body(g, "l1"), []string{"b", "c", "d"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("body = %v, want %v", got, want)
		}
	})

	t.Run("stop edges and terminals lead nowhere", func(t *testing.T) {
		g := withLoops(graphOf(
			gstep("b", fwd("b1", "c")),
			gstep("c", back("c_back", "b", "l1"), park("c_stop", "stalled"), fwd("c1", "done")),
		), loop("l1", "b"))
		if got, want := body(g, "l1"), []string{"b", "c"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("body = %v, want %v", got, want)
		}
	})

	t.Run("a loop that does not exist or has no back-edge has an empty body", func(t *testing.T) {
		g := withLoops(graphOf(gstep("b", fwd("b1", "done"))), loop("l1", "b"), loop("bad_header", "nowhere"))
		if got := body(g, "l1"); len(got) != 0 {
			t.Fatalf("no back-edge: body = %v", got)
		}
		if got := body(g, "bad_header"); len(got) != 0 {
			t.Fatalf("header is not a step: body = %v", got)
		}
		if got := body(g, "missing"); len(got) != 0 {
			t.Fatalf("no such loop: body = %v", got)
		}
	})

	t.Run("the sample document", func(t *testing.T) {
		g := NewGraph(parseSample(t))
		if got, want := body(g, "fix"), []string{"draft", "check"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("fix = %v, want %v", got, want)
		}
		if got := body(g, "retry"); len(got) != 0 {
			t.Fatalf("retry has no back-edge: %v", got)
		}
	})
}
