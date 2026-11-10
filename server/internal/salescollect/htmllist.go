package salescollect

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// htmlListAdapter reads a list of items from a page. parse.items is a selector that picks each
// item; each field is a selector relative to the item, with an optional "@attr" suffix to read
// an attribute instead of the text. An empty field selector reads the item itself.
func htmlListAdapter(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error) {
	if strings.TrimSpace(r.Parse.Items) == "" {
		return fetched{}, errors.New("html_list needs parse.items: a selector for each item")
	}
	items, err := parseSelector(r.Parse.Items)
	if err != nil {
		return fetched{}, fmt.Errorf("parse.items: %w", err)
	}
	fields := map[string]fieldSpec{}
	for name, raw := range map[string]string{
		"entity": r.Parse.Fields.Entity, "url": r.Parse.Fields.URL,
		"observed_at": r.Parse.Fields.ObservedAt, "text": r.Parse.Fields.Text, "website": r.Parse.Fields.Website,
	} {
		spec, err := parseFieldSpec(raw)
		if err != nil {
			return fetched{}, fmt.Errorf("parse.fields.%s: %w", name, err)
		}
		fields[name] = spec
	}

	resp, err := f.get(ctx, r.Fetch.URL)
	if err != nil {
		return fetched{}, err
	}
	out := fetched{status: resp.Status}
	body, err := readHTML(resp)
	if err != nil {
		return out, err
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("parse html: %w", err)
	}
	for _, node := range items.findAll(doc) {
		rec := record{
			Entity:     fields["entity"].read(node, r.Fetch.URL),
			URL:        fields["url"].read(node, r.Fetch.URL),
			ObservedAt: fields["observed_at"].read(node, r.Fetch.URL),
			Text:       fields["text"].read(node, r.Fetch.URL),
			Website:    fields["website"].read(node, r.Fetch.URL),
		}
		if rec.Entity == "" && rec.URL == "" && rec.Text == "" {
			continue
		}
		rec.RawRef = rec.URL
		out.records = append(out.records, rec)
	}
	if len(out.records) == 0 {
		out.warnings = append(out.warnings, "the selector for parse.items matched no item on the page")
	}
	return out, nil
}

// fieldSpec is a selector relative to an item, and the attribute it reads, when it reads one.
type fieldSpec struct {
	sel  selector // nil reads the item itself
	attr string
}

func parseFieldSpec(raw string) (fieldSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fieldSpec{}, nil
	}
	spec := fieldSpec{}
	if i := strings.LastIndex(raw, "@"); i >= 0 && !strings.ContainsAny(raw[i:], " []") {
		spec.attr = raw[i+1:]
		raw = strings.TrimSpace(raw[:i])
		if spec.attr == "" {
			return fieldSpec{}, errors.New("an @ must name an attribute")
		}
	}
	if raw == "" {
		return spec, nil
	}
	sel, err := parseSelector(raw)
	if err != nil {
		return fieldSpec{}, err
	}
	spec.sel = sel
	return spec, nil
}

// read returns the field's value for one item: the text of the first match of its selector
// inside the item, or that element's attribute. An unset selector reads the item itself.
func (s fieldSpec) read(item *html.Node, base string) string {
	target := item
	if s.sel != nil {
		target = s.sel.firstWithin(item)
		if target == nil {
			return ""
		}
	}
	if s.attr != "" {
		for _, a := range target.Attr {
			if a.Key == s.attr {
				if s.attr == "href" || s.attr == "src" {
					return absolute(base, a.Val)
				}
				return fieldText(a.Val)
			}
		}
		return ""
	}
	var b strings.Builder
	collectText(target, &b)
	return fieldText(b.String())
}

// collectText writes the text under a node, skipping scripts and styles.
func collectText(n *html.Node, b *strings.Builder) {
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
		return
	}
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		b.WriteByte(' ')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, b)
	}
}

// findAll returns every element under root that the selector matches, in document order.
func (s selector) findAll(root *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && s.matches(n) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// firstWithin returns the first element under item (not item itself) that the selector matches.
func (s selector) firstWithin(item *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && s.matches(c) {
				found = c
				return true
			}
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(item)
	return found
}

// compound is one element test: a tag, an id, classes, and attributes.
type compound struct {
	tag     string
	id      string
	classes []string
	attrs   []attrCond
}

type attrCond struct {
	name  string
	value *string
}

// step is a compound and how it relates to the step before it: a space (any ancestor) or ">"
// (the direct parent).
type step struct {
	child bool
	comp  compound
}

// selector is a chain of compound tests, the subset of CSS the collector reads.
type selector []step

// parseSelector reads the subset: tag, #id, .class, [attr], [attr=value], a descendant space and
// a child ">". Anything else is refused with the token it cannot read.
func parseSelector(src string) (selector, error) {
	tokens := tokenizeSelector(src)
	var sel selector
	nextChild := false
	for _, tok := range tokens {
		if tok == ">" {
			if len(sel) == 0 || nextChild {
				return nil, errors.New("a > needs an element before it")
			}
			nextChild = true
			continue
		}
		comp, err := parseCompound(tok)
		if err != nil {
			return nil, err
		}
		sel = append(sel, step{child: nextChild, comp: comp})
		nextChild = false
	}
	if len(sel) == 0 {
		return nil, errors.New("the selector is empty")
	}
	if nextChild {
		return nil, errors.New("a > needs an element after it")
	}
	sel[0].child = false
	return sel, nil
}

// tokenizeSelector splits on spaces and ">", but not inside brackets.
func tokenizeSelector(src string) []string {
	var tokens []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range src {
		switch {
		case r == '[':
			depth++
			cur.WriteRune(r)
		case r == ']':
			depth--
			cur.WriteRune(r)
		case depth == 0 && (r == ' ' || r == '\t' || r == '\n'):
			flush()
		case depth == 0 && r == '>':
			flush()
			tokens = append(tokens, ">")
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func parseCompound(tok string) (compound, error) {
	var c compound
	i := 0
	readName := func() (string, error) {
		start := i
		for i < len(tok) && (isNameByte(tok[i])) {
			i++
		}
		if start == i {
			return "", fmt.Errorf("cannot read %q in selector", tok)
		}
		return tok[start:i], nil
	}
	if i < len(tok) && tok[i] != '#' && tok[i] != '.' && tok[i] != '[' {
		name, err := readName()
		if err != nil {
			return c, err
		}
		c.tag = strings.ToLower(name)
	}
	for i < len(tok) {
		switch tok[i] {
		case '#':
			i++
			name, err := readName()
			if err != nil {
				return c, err
			}
			c.id = name
		case '.':
			i++
			name, err := readName()
			if err != nil {
				return c, err
			}
			c.classes = append(c.classes, name)
		case '[':
			end := strings.IndexByte(tok[i:], ']')
			if end < 0 {
				return c, fmt.Errorf("unclosed [ in %q", tok)
			}
			body := tok[i+1 : i+end]
			i += end + 1
			name, value, hasValue := strings.Cut(body, "=")
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				return c, fmt.Errorf("an attribute test needs a name in %q", tok)
			}
			cond := attrCond{name: name}
			if hasValue {
				v := strings.Trim(strings.TrimSpace(value), `"'`)
				cond.value = &v
			}
			c.attrs = append(c.attrs, cond)
		default:
			return c, fmt.Errorf("selector %q uses %q, which this collector does not read", tok, tok[i:i+1])
		}
	}
	return c, nil
}

func isNameByte(b byte) bool {
	return b == '-' || b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func (c compound) matches(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if c.tag != "" && n.Data != c.tag {
		return false
	}
	if c.id != "" && attr(n, "id") != c.id {
		return false
	}
	if len(c.classes) > 0 {
		have := strings.Fields(attr(n, "class"))
		for _, want := range c.classes {
			if !contains(have, want) {
				return false
			}
		}
	}
	for _, cond := range c.attrs {
		v, ok := lookup(n, cond.name)
		if !ok {
			return false
		}
		if cond.value != nil && v != *cond.value {
			return false
		}
	}
	return true
}

func attr(n *html.Node, name string) string {
	v, _ := lookup(n, name)
	return v
}

func lookup(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// matches reports whether the node satisfies the whole chain, with the ancestors of the node
// standing for the steps before the last.
func (s selector) matches(n *html.Node) bool {
	return s.matchFrom(len(s)-1, n)
}

func (s selector) matchFrom(i int, n *html.Node) bool {
	if !s[i].comp.matches(n) {
		return false
	}
	if i == 0 {
		return true
	}
	if s[i].child {
		p := n.Parent
		return p != nil && s.matchFrom(i-1, p)
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if s.matchFrom(i-1, p) {
			return true
		}
	}
	return false
}
