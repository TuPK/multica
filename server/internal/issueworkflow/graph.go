package issueworkflow

import "sort"

// Compat values of a Graph.
const (
	// CompatNone is a clean schema 2 graph.
	CompatNone = ""
	// CompatV1 marks a graph lifted from a schema 1 workflow that runs requests:
	// it keeps the schema 1 rounds, input links and texts.
	CompatV1 = "v1"
)

// GraphEdge is an edge of a step with the facts the engine looks up often.
type GraphEdge struct {
	From string
	// Index is the edge's position in its step: the order edges are tried in.
	Index int
	Edge  DocEdge
	// Back: the edge closes a loop (it has a loop key).
	Back bool
}

// Graph is a FlowDoc with its indexes. It is derived from the document and
// never stored. A reference that points nowhere is not an error here; the
// validator reports it, and the indexes simply do not contain it.
type Graph struct {
	Doc    FlowDoc
	Compat string

	steps     map[string]int
	terminals map[string]int
	loops     map[string]int
	edges     map[edgeRef]GraphEdge
	out       map[string][]GraphEdge
	back      []GraphEdge
	backOf    map[string][]GraphEdge
	roles     map[string]Role
	stops     map[string]StopKind
	// factSpecs are the facts of the document, computed once.
	factSpecs map[string]FactSpec
}

type edgeRef struct{ step, id string }

// NewGraph indexes a document. Compat is left clean; the lift sets it.
func NewGraph(doc FlowDoc) Graph {
	g := Graph{
		Doc:       doc,
		steps:     make(map[string]int, len(doc.Steps)),
		terminals: make(map[string]int, len(doc.Terminals)),
		loops:     make(map[string]int, len(doc.Loops)),
		edges:     map[edgeRef]GraphEdge{},
		out:       make(map[string][]GraphEdge, len(doc.Steps)),
		backOf:    map[string][]GraphEdge{},
		roles:     make(map[string]Role, len(doc.Roles)),
		stops:     make(map[string]StopKind, len(doc.Stops)),
	}
	for _, r := range doc.Roles {
		if _, dup := g.roles[r.Key]; !dup {
			g.roles[r.Key] = r
		}
	}
	for _, s := range doc.Stops {
		if _, dup := g.stops[s.Code]; !dup {
			g.stops[s.Code] = s
		}
	}
	expanded := ExpandFacts(doc)
	g.factSpecs = make(map[string]FactSpec, len(expanded))
	for _, spec := range expanded {
		g.factSpecs[spec.Key] = spec
	}
	for i, s := range doc.Steps {
		if _, dup := g.steps[s.Key]; !dup {
			g.steps[s.Key] = i
		}
	}
	for i, t := range doc.Terminals {
		if _, dup := g.terminals[t.Key]; !dup {
			g.terminals[t.Key] = i
		}
	}
	for i, l := range doc.Loops {
		if _, dup := g.loops[l.Key]; !dup {
			g.loops[l.Key] = i
		}
	}
	for _, s := range doc.Steps {
		for i, e := range s.Edges {
			ge := GraphEdge{From: s.Key, Index: i, Edge: e, Back: e.IsBack()}
			g.out[s.Key] = append(g.out[s.Key], ge)
			if _, dup := g.edges[edgeRef{s.Key, e.ID}]; !dup {
				g.edges[edgeRef{s.Key, e.ID}] = ge
			}
			if ge.Back {
				g.back = append(g.back, ge)
				g.backOf[e.Loop] = append(g.backOf[e.Loop], ge)
			}
		}
	}
	return g
}

// Step returns the step with the key.
func (g Graph) Step(key string) (DocStep, bool) {
	i, ok := g.steps[key]
	if !ok {
		return DocStep{}, false
	}
	return g.Doc.Steps[i], true
}

// StepIndex is the position of the step in the document.
func (g Graph) StepIndex(key string) (int, bool) {
	i, ok := g.steps[key]
	return i, ok
}

// Terminal returns the terminal with the key.
func (g Graph) Terminal(key string) (Terminal, bool) {
	i, ok := g.terminals[key]
	if !ok {
		return Terminal{}, false
	}
	return g.Doc.Terminals[i], true
}

// Loop returns the loop with the key.
func (g Graph) Loop(key string) (Loop, bool) {
	i, ok := g.loops[key]
	if !ok {
		return Loop{}, false
	}
	return g.Doc.Loops[i], true
}

// Edge returns the edge of the step with the id.
func (g Graph) Edge(step, id string) (GraphEdge, bool) {
	e, ok := g.edges[edgeRef{step, id}]
	return e, ok
}

// EdgesFrom lists the edges of a step in the order they are tried.
func (g Graph) EdgesFrom(step string) []GraphEdge {
	return g.out[step]
}

// RoleKind is the kind of a role: a declared one, or one of the built-in roles
// lead, requester and managers. It is empty for a role that does not exist.
func (g Graph) RoleKind(key string) string {
	if r, ok := g.roles[key]; ok {
		return r.Kind
	}
	switch key {
	case RoleKindLead, RoleKindRequester, RoleKindManagers:
		return key
	}
	return ""
}

// Stop returns the declared stop kind with the code.
func (g Graph) Stop(code string) (StopKind, bool) {
	s, ok := g.stops[code]
	return s, ok
}

// BackEdges lists every edge that closes a loop, in document order.
func (g Graph) BackEdges() []GraphEdge { return g.back }

// BackEdgesOf lists the edges that close the loop.
func (g Graph) BackEdgesOf(loop string) []GraphEdge { return g.backOf[loop] }

// forwardNext lists the steps an edge of the step leads to without closing a
// loop. Edges into a terminal or a stop lead to no step.
func (g Graph) forwardNext(step string) []string {
	var next []string
	for _, e := range g.out[step] {
		if e.Back || e.Edge.To == "" {
			continue
		}
		if _, ok := g.steps[e.Edge.To]; ok {
			next = append(next, e.Edge.To)
		}
	}
	return next
}

// LoopBody lists the steps of a loop in document order: the steps that lie on a
// forward path (no back-edge) from the loop's header to the source of one of the
// loop's back-edges, both ends included (spec section 4.5). A loop that has no
// back-edge, or whose header cannot reach any of their sources, has an empty body.
func (g Graph) LoopBody(loop string) []string {
	l, ok := g.Loop(loop)
	if !ok {
		return nil
	}
	if _, ok := g.steps[l.Header]; !ok {
		return nil
	}
	fromHeader := g.reach(l.Header, g.forwardNext)
	reverse := map[string][]string{}
	for from := range g.steps {
		for _, to := range g.forwardNext(from) {
			reverse[to] = append(reverse[to], from)
		}
	}
	toSource := map[string]bool{}
	for _, be := range g.backOf[loop] {
		for step := range g.reach(be.From, func(s string) []string { return reverse[s] }) {
			toSource[step] = true
		}
	}
	var body []string
	for _, s := range g.Doc.Steps {
		if fromHeader[s.Key] && toSource[s.Key] {
			body = append(body, s.Key)
		}
	}
	sort.SliceStable(body, func(i, j int) bool { return g.steps[body[i]] < g.steps[body[j]] })
	return body
}

// reach is the set of nodes reachable from start (start included).
func (g Graph) reach(start string, next func(string) []string) map[string]bool {
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range next(cur) {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return seen
}
