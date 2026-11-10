package salescollect

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// fetched is what an adapter returns: the records it read, the HTTP status of the last
// request, and warnings about what it could not do.
type fetched struct {
	records  []record
	status   int
	warnings []string
}

// adapter reads one source for a recipe.
type adapter func(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error)

func adapterFor(method string) adapter {
	switch method {
	case "rss":
		return rssAdapter
	case "json_api":
		return jsonAPIAdapter
	case "html_jsonld":
		return jsonLDAdapter
	case "html_list":
		return htmlListAdapter
	case "csv_drop":
		return csvAdapter
	}
	return nil
}

// buildURL adds the recipe's params and, for a page, the page parameter.
func buildURL(raw string, params map[string]string, page string, pageParam string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("url: %w", err)
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	if page != "" {
		if pageParam == "" {
			pageParam = "page"
		}
		q.Set(pageParam, page)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// --- rss: RSS 2.0 and Atom feeds, read as a generic element tree ---

type xnode struct {
	name     string
	attrs    map[string]string
	text     strings.Builder
	children []*xnode
}

// parseXML builds the element tree of a document. Namespaces are dropped: a field named
// "title" matches the element "title" of any namespace.
func parseXML(body []byte) (*xnode, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	// Feeds are XML. HTML's void elements (link, br) must not be read as optional here, and the
	// HTML entity table covers the named entities feeds use.
	dec.Entity = xml.HTMLEntity
	root := &xnode{name: "#document"}
	stack := []*xnode{root}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: strings.ToLower(t.Name.Local), attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			stack[len(stack)-1].text.Write(t)
		}
	}
	return root, nil
}

// find returns every descendant with the given element name, in document order.
func (n *xnode) find(name string, out []*xnode) []*xnode {
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
		out = c.find(name, out)
	}
	return out
}

// child returns the first direct child with the given name.
func (n *xnode) child(name string) *xnode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// value is the element's text, or its href for an Atom link that carries none.
func (n *xnode) value() string {
	if t := strings.TrimSpace(n.text.String()); t != "" {
		return t
	}
	if href := n.attrs["href"]; href != "" {
		return href
	}
	return ""
}

func rssAdapter(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error) {
	resp, err := f.get(ctx, r.Fetch.URL)
	if err != nil {
		return fetched{}, err
	}
	out := fetched{status: resp.Status}
	if resp.Status < 200 || resp.Status > 299 {
		return out, fmt.Errorf("feed answered %d", resp.Status)
	}
	doc, err := parseXML(resp.Body)
	if err != nil {
		return out, err
	}
	items := doc.find("item", nil)
	items = doc.find("entry", items)
	for _, item := range items {
		field := func(name string) string {
			if name == "" {
				return ""
			}
			if c := item.child(strings.ToLower(name)); c != nil {
				return c.value()
			}
			return ""
		}
		rec := record{
			Entity:     field(r.Parse.Fields.Entity),
			URL:        field(r.Parse.Fields.URL),
			ObservedAt: field(r.Parse.Fields.ObservedAt),
			Text:       field(r.Parse.Fields.Text),
			Website:    field(r.Parse.Fields.Website),
		}
		if guid := item.child("guid"); guid != nil {
			rec.RawRef = guid.value()
		}
		if rec.RawRef == "" {
			rec.RawRef = rec.URL
		}
		out.records = append(out.records, rec)
	}
	return out, nil
}

// --- json_api: a JSON document, items at a dot path, fields at dot paths ---

func jsonAPIAdapter(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error) {
	out := fetched{}
	pages := 1
	pageParam := ""
	if p := r.Fetch.Pagination; p != nil && p.Type == "page" && p.MaxPages > 1 {
		pages = p.MaxPages
		pageParam = p.Param
	}
	for page := 1; page <= pages; page++ {
		pageValue := ""
		if pages > 1 {
			pageValue = strconv.Itoa(page)
		}
		target, err := buildURL(r.Fetch.URL, r.Fetch.Params, pageValue, pageParam)
		if err != nil {
			return out, err
		}
		resp, err := f.get(ctx, target)
		if err != nil {
			return out, err
		}
		out.status = resp.Status
		if resp.Status < 200 || resp.Status > 299 {
			return out, fmt.Errorf("API answered %d", resp.Status)
		}
		var doc any
		if err := json.Unmarshal(resp.Body, &doc); err != nil {
			return out, fmt.Errorf("parse json: %w", err)
		}
		items, ok := valueAtPath(doc, r.Parse.Items).([]any)
		if !ok {
			return out, fmt.Errorf("parse.items %q is not an array in the response", r.Parse.Items)
		}
		before := len(out.records)
		for _, item := range items {
			rec := record{
				Entity:     scalar(valueAtPath(item, r.Parse.Fields.Entity)),
				URL:        scalar(valueAtPath(item, r.Parse.Fields.URL)),
				ObservedAt: scalar(valueAtPath(item, r.Parse.Fields.ObservedAt)),
				Text:       scalar(valueAtPath(item, r.Parse.Fields.Text)),
				Website:    scalar(valueAtPath(item, r.Parse.Fields.Website)),
			}
			// Metadata objects (a legal notice, a count) carry none of the fields: they are not items.
			if rec.Entity == "" && rec.URL == "" && rec.Text == "" {
				continue
			}
			rec.RawRef = rec.URL
			out.records = append(out.records, rec)
		}
		if len(out.records) == before {
			break
		}
	}
	return out, nil
}

// valueAtPath walks a dot path through maps and arrays. An empty path is the value itself.
func valueAtPath(v any, path string) any {
	if path == "" {
		return v
	}
	for _, key := range strings.Split(path, ".") {
		switch t := v.(type) {
		case map[string]any:
			v = t[key]
		default:
			return nil
		}
	}
	return v
}

// scalar renders a JSON value as the text of a field. A number is kept as written, so a
// Unix time reads as one.
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	return ""
}

// --- html_jsonld: schema.org objects in the page's JSON-LD blocks ---

var jsonLDPattern = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

func jsonLDAdapter(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error) {
	resp, err := f.page(ctx, r.Fetch.URL, r.usesEngine())
	if err != nil {
		return fetched{}, err
	}
	out := fetched{status: resp.Status}
	if resp.Status < 200 || resp.Status > 299 {
		return out, fmt.Errorf("page answered %d", resp.Status)
	}
	var objects []map[string]any
	for _, m := range jsonLDPattern.FindAllSubmatch(resp.Body, -1) {
		var doc any
		if err := json.Unmarshal(bytes.TrimSpace(m[1]), &doc); err != nil {
			out.warnings = append(out.warnings, "a JSON-LD block did not parse")
			continue
		}
		objects = collectTyped(doc, r.Parse.Items, objects)
	}
	if len(objects) == 0 {
		out.warnings = append(out.warnings, fmt.Sprintf("no JSON-LD object of type %q on the page", r.Parse.Items))
	}
	for _, o := range objects {
		rec := record{
			Entity:     scalar(valueAtPath(o, r.Parse.Fields.Entity)),
			URL:        scalar(valueAtPath(o, r.Parse.Fields.URL)),
			ObservedAt: scalar(valueAtPath(o, r.Parse.Fields.ObservedAt)),
			Text:       scalar(valueAtPath(o, r.Parse.Fields.Text)),
			Website:    scalar(valueAtPath(o, r.Parse.Fields.Website)),
		}
		if rec.Entity == "" && rec.URL == "" && rec.Text == "" {
			continue
		}
		rec.RawRef = rec.URL
		out.records = append(out.records, rec)
	}
	return out, nil
}

// collectTyped walks a JSON-LD document and gathers every object whose @type is typ. A
// list of objects, an @graph, and nested objects are all searched.
func collectTyped(v any, typ string, acc []map[string]any) []map[string]any {
	switch t := v.(type) {
	case []any:
		for _, c := range t {
			acc = collectTyped(c, typ, acc)
		}
	case map[string]any:
		if hasType(t["@type"], typ) {
			acc = append(acc, t)
		}
		for k, c := range t {
			if k == "@type" {
				continue
			}
			acc = collectTyped(c, typ, acc)
		}
	}
	return acc
}

func hasType(v any, typ string) bool {
	switch t := v.(type) {
	case string:
		return t == typ
	case []any:
		for _, c := range t {
			if s, ok := c.(string); ok && s == typ {
				return true
			}
		}
	}
	return false
}

// --- csv_drop: a CSV the person dropped; the file comes from the command line ---

func csvAdapter(ctx context.Context, f *fetcher, r *Recipe, opts Options) (fetched, error) {
	if opts.CSVPath == "" {
		return fetched{}, errors.New("csv_drop needs the file: pass --csv <path>")
	}
	fi, err := os.Stat(opts.CSVPath)
	if err != nil {
		return fetched{}, fmt.Errorf("csv: %w", err)
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}
	if fi.Size() > maxBytes {
		return fetched{}, fmt.Errorf("csv is larger than %d bytes", maxBytes)
	}
	file, err := os.Open(opts.CSVPath)
	if err != nil {
		return fetched{}, fmt.Errorf("csv: %w", err)
	}
	defer file.Close()
	rd := csv.NewReader(file)
	rd.FieldsPerRecord = -1
	header, err := rd.Read()
	if err != nil {
		return fetched{}, fmt.Errorf("csv header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, name string) string {
		i, ok := col[strings.ToLower(strings.TrimSpace(name))]
		if !ok || name == "" || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	out := fetched{status: 200}
	for line := 2; ; line++ {
		row, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.warnings = append(out.warnings, fmt.Sprintf("line %d did not parse", line))
			continue
		}
		rec := record{
			Entity:     get(row, r.Parse.Fields.Entity),
			URL:        get(row, r.Parse.Fields.URL),
			ObservedAt: get(row, r.Parse.Fields.ObservedAt),
			Text:       get(row, r.Parse.Fields.Text),
			Website:    get(row, r.Parse.Fields.Website),
			RawRef:     fmt.Sprintf("%s:%d", opts.CSVPath, line),
		}
		if rec.Entity == "" && rec.URL == "" {
			continue
		}
		out.records = append(out.records, rec)
	}
	return out, nil
}

// fieldText returns the text of a parsed HTML field, trimmed and with its spaces collapsed.
func fieldText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// absolute resolves a link against the page it was read from.
func absolute(base, link string) string {
	if link == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return link
	}
	l, err := url.Parse(link)
	if err != nil {
		return link
	}
	return b.ResolveReference(l).String()
}

// readHTML is the body of an HTML response, for the html_list adapter.
func readHTML(resp *Response) (string, error) {
	if resp.Status < 200 || resp.Status > 299 {
		return "", fmt.Errorf("page answered %d", resp.Status)
	}
	if ct := resp.ContentType; ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text") {
		return "", fmt.Errorf("page is %s, not HTML", ct)
	}
	return string(resp.Body), nil
}
