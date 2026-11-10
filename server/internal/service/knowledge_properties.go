package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Typed properties of a knowledge document (spec 7.1). A kind declares its fields; a document
// carries their values in properties, so a list, a board and a filter read values without reading
// the document. Changing a value does not re-index the document.

// KindField is one field a kind declares (spec 6.2).
type KindField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
	// Ordered marks a select whose options run from the first value to the last, such as a
	// pipeline. A person's value cannot be moved back by an agent.
	Ordered bool `json:"ordered,omitempty"`
}

// The field types a kind may declare.
const (
	KindFieldText   = "text"
	KindFieldSelect = "select"
	KindFieldNumber = "number"
	KindFieldDate   = "date"
	KindFieldBool   = "bool"
	KindFieldURL    = "url"
)

// maxPropertyText bounds a text property. A property is a short value, not a document.
const maxPropertyText = 2000

var (
	fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	isoDayPattern   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	recordLine      = regexp.MustCompile(`^([a-z][a-z0-9_]*):\s*(.*)$`)
)

// ValidateKindFields checks the fields a kind declares: a key, a label, a known type, options
// for a select, and no key twice.
func ValidateKindFields(fields []KindField) error {
	seen := map[string]bool{}
	for _, f := range fields {
		if !fieldKeyPattern.MatchString(f.Key) {
			return &KnowledgeInvalidError{fmt.Sprintf("field key %q must be lower-case letters, digits and underscores", f.Key)}
		}
		if seen[f.Key] {
			return &KnowledgeInvalidError{"field " + f.Key + " is declared twice"}
		}
		seen[f.Key] = true
		if strings.TrimSpace(f.Label) == "" {
			return &KnowledgeInvalidError{"field " + f.Key + " needs a label"}
		}
		switch f.Type {
		case KindFieldText, KindFieldNumber, KindFieldDate, KindFieldBool, KindFieldURL:
		case KindFieldSelect:
			if len(f.Options) == 0 {
				return &KnowledgeInvalidError{"select field " + f.Key + " needs options"}
			}
		default:
			return &KnowledgeInvalidError{fmt.Sprintf("field %s has type %q, which is not one of text, select, number, date, bool, url", f.Key, f.Type)}
		}
		if f.Ordered && f.Type != KindFieldSelect {
			return &KnowledgeInvalidError{"only a select field can be ordered: " + f.Key}
		}
	}
	return nil
}

// encodeKindFields is the stored form of a kind's fields. No fields is an empty list.
func encodeKindFields(fields []KindField) ([]byte, error) {
	if fields == nil {
		fields = []KindField{}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode knowledge kind fields: %w", err)
	}
	return raw, nil
}

// decodeKindFields reads a kind's stored fields. An empty or null value is no fields.
func decodeKindFields(raw []byte) []KindField {
	var fields []KindField
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}

// CheckProperties validates a complete set of properties against a kind's fields: no key the
// kind does not declare, and each value of its declared type. It returns the values as they
// are stored: numbers as float64, and the rest as text or bool.
func CheckProperties(fields []KindField, props map[string]any) (map[string]any, error) {
	byKey := map[string]KindField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	out := make(map[string]any, len(props))
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f, ok := byKey[k]
		if !ok {
			return nil, &KnowledgeInvalidError{"this kind has no field " + k}
		}
		v, err := coerceProperty(f, props[k])
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// coerceProperty checks one value against its field and returns it in its stored form.
func coerceProperty(f KindField, v any) (any, error) {
	switch f.Type {
	case KindFieldNumber:
		switch t := v.(type) {
		case float64:
			if math.IsNaN(t) || math.IsInf(t, 0) {
				break
			}
			return t, nil
		case int:
			return float64(t), nil
		case int64:
			return float64(t), nil
		case string:
			if n, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				return n, nil
			}
		}
	case KindFieldBool:
		switch t := v.(type) {
		case bool:
			return t, nil
		case string:
			if b, err := strconv.ParseBool(strings.TrimSpace(t)); err == nil {
				return b, nil
			}
		}
	case KindFieldDate:
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if _, err := parseISODay(s); err == nil {
				return s, nil
			}
		}
	case KindFieldURL:
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				return s, nil
			}
		}
	case KindFieldSelect:
		if s, ok := v.(string); ok {
			for _, o := range f.Options {
				if s == o {
					return s, nil
				}
			}
			return nil, &KnowledgeInvalidError{fmt.Sprintf("%s must be one of %s", f.Key, strings.Join(f.Options, ", "))}
		}
	case KindFieldText:
		if s, ok := v.(string); ok {
			if len([]rune(s)) > maxPropertyText {
				return nil, &KnowledgeInvalidError{fmt.Sprintf("%s is longer than %d characters", f.Key, maxPropertyText)}
			}
			return s, nil
		}
	}
	return nil, &KnowledgeInvalidError{fmt.Sprintf("%s must be a %s", f.Key, f.Type)}
}

// PropertiesFromRecord reads the Record section of a document: one "key: value" line for each
// key the kind declares. A line whose value does not fit its field is skipped, not guessed.
func PropertiesFromRecord(content string, fields []KindField) map[string]any {
	declared := map[string]KindField{}
	for _, f := range fields {
		declared[f.Key] = f
	}
	out := map[string]any{}
	inRecord := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			inRecord = normalizeHeading(strings.TrimLeft(line, "# ")) == "record"
			continue
		}
		if !inRecord {
			continue
		}
		m := recordLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		f, ok := declared[m[1]]
		if !ok || strings.TrimSpace(m[2]) == "" {
			continue
		}
		if v, err := coerceProperty(f, strings.TrimSpace(m[2])); err == nil {
			out[m[1]] = v
		}
	}
	return out
}

// mergeProperties applies a patch: a value replaces, and a null removes the key.
func mergeProperties(current, patch map[string]any) map[string]any {
	out := make(map[string]any, len(current)+len(patch))
	for k, v := range current {
		out[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

// propertiesOf decodes a document's stored properties. A value that does not decode is empty.
func propertiesOf(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// stageRank is a value's place in an ordered field, or -1 when it is not one of the options.
func stageRank(f KindField, v any) int {
	s, ok := v.(string)
	if !ok {
		return -1
	}
	for i, o := range f.Options {
		if o == s {
			return i
		}
	}
	return -1
}

// kindFieldsFor loads a kind's declared fields. A kind that is not in the workspace has none.
func (s *KnowledgeService) kindFieldsFor(ctx context.Context, workspaceID pgtype.UUID, key string) ([]KindField, error) {
	if key == "" {
		return nil, nil
	}
	k, err := s.Queries.GetKnowledgeKind(ctx, db.GetKnowledgeKindParams{WorkspaceID: workspaceID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load knowledge kind: %w", err)
	}
	return decodeKindFields(k.Fields), nil
}

// UpdateDocumentProperties changes some properties of a document. It checks each value against its
// kind's fields, refuses an agent that would move an ordered value back after a person set it,
// and records who changed which keys. It never touches the document's content or index.
func (s *KnowledgeService) UpdateDocumentProperties(ctx context.Context, workspaceID pgtype.UUID, actor KnowledgeActor, linkID pgtype.UUID, patch map[string]any) (KnowledgeDocumentView, error) {
	link, err := s.loadLink(ctx, workspaceID, linkID)
	if err != nil {
		return KnowledgeDocumentView{}, err
	}
	fields, err := s.kindFieldsFor(ctx, workspaceID, link.Kind)
	if err != nil {
		return KnowledgeDocumentView{}, err
	}
	if len(fields) == 0 {
		return KnowledgeDocumentView{}, &KnowledgeInvalidError{"this document's kind has no fields"}
	}
	current := propertiesOf(link.Properties)
	// A null in the patch removes a key, so only the values that remain are checked.
	present := map[string]any{}
	for k, v := range patch {
		if v != nil {
			present[k] = v
		}
	}
	checked, err := CheckProperties(fields, present)
	if err != nil {
		return KnowledgeDocumentView{}, err
	}
	for k, v := range patch {
		if v == nil {
			if err := knownField(fields, k); err != nil {
				return KnowledgeDocumentView{}, err
			}
		}
	}
	if actor.Type == KnowledgeActorAgent {
		for _, f := range fields {
			if !f.Ordered {
				continue
			}
			next, moving := checked[f.Key]
			if !moving {
				continue
			}
			if stageRank(f, next) < stageRank(f, current[f.Key]) {
				last, err := s.Queries.LastKnowledgePropertyActorType(ctx, db.LastKnowledgePropertyActorTypeParams{
					WorkspaceID: workspaceID, LinkID: link.ID, Key: f.Key,
				})
				if err == nil && last == KnowledgeActorMember {
					return KnowledgeDocumentView{}, &KnowledgeInvalidError{fmt.Sprintf("a person set %s; an agent cannot move it back", f.Key)}
				}
			}
		}
	}

	next := mergeProperties(current, checked)
	for k, v := range patch {
		if v == nil {
			delete(next, k)
		}
	}
	// mergeProperties drops removed keys already; the loop above keeps the intent explicit for a reader.
	changed := changedKeys(current, next)
	raw, err := json.Marshal(next)
	if err != nil {
		return KnowledgeDocumentView{}, fmt.Errorf("encode properties: %w", err)
	}
	updated, err := s.Queries.SetKnowledgeLinkProperties(ctx, db.SetKnowledgeLinkPropertiesParams{
		ID: link.ID, WorkspaceID: workspaceID, Properties: raw,
	})
	if err != nil {
		return KnowledgeDocumentView{}, fmt.Errorf("save properties: %w", err)
	}
	if len(changed) > 0 {
		changes, _ := json.Marshal(map[string]any{"from": pick(current, changed), "to": pick(next, changed)})
		if err := s.Queries.InsertKnowledgePropertyHistory(ctx, db.InsertKnowledgePropertyHistoryParams{
			WorkspaceID: workspaceID, LinkID: link.ID, ActorType: actor.Type, ActorID: actor.ID,
			ChangedKeys: changed, Changes: changes,
		}); err != nil {
			return KnowledgeDocumentView{}, fmt.Errorf("record property change: %w", err)
		}
	}
	return s.linkView(ctx, workspaceID, updated)
}

func knownField(fields []KindField, key string) error {
	for _, f := range fields {
		if f.Key == key {
			return nil
		}
	}
	return &KnowledgeInvalidError{"this kind has no field " + key}
}

// changedKeys lists the keys whose values differ between two property sets, sorted.
func changedKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	var out []string
	for k := range seen {
		av, aok := a[k]
		bv, bok := b[k]
		if aok != bok || fmt.Sprint(av) != fmt.Sprint(bv) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func pick(m map[string]any, keys []string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// CollectionQuery filters the documents of the Collections view. Match holds the values as the
// caller sent them; each is read as its field's type. DueKey names a date field that must be on or
// before DueOn (a YYYY-MM-DD day).
type CollectionQuery struct {
	Kind   string
	KB     string
	Match  map[string]string
	DueKey string
	DueOn  string
	Limit  int
}

// KnowledgeCollectionView is the Collections view: the kind's fields and the documents that match.
type KnowledgeCollectionView struct {
	Kind      string                  `json:"kind"`
	Fields    []KindField             `json:"fields"`
	Documents []KnowledgeDocumentView `json:"documents"`
}

// ListCollection returns the published documents of a kind that match a query, with their
// properties. A kind without fields has no collection.
func (s *KnowledgeService) ListCollection(ctx context.Context, workspaceID pgtype.UUID, q CollectionQuery) (KnowledgeCollectionView, error) {
	fields, err := s.kindFieldsFor(ctx, workspaceID, q.Kind)
	if err != nil {
		return KnowledgeCollectionView{}, err
	}
	if len(fields) == 0 {
		return KnowledgeCollectionView{}, &KnowledgeInvalidError{"kind " + q.Kind + " has no fields to show"}
	}
	match := map[string]any{}
	if len(q.Match) > 0 {
		byKey := map[string]KindField{}
		for _, f := range fields {
			byKey[f.Key] = f
		}
		raw := map[string]any{}
		for k, v := range q.Match {
			f, ok := byKey[k]
			if !ok {
				return KnowledgeCollectionView{}, &KnowledgeInvalidError{"this kind has no field " + k}
			}
			val, err := coerceProperty(f, v)
			if err != nil {
				return KnowledgeCollectionView{}, err
			}
			raw[k] = val
		}
		match = raw
	}
	params := db.ListKnowledgeCollectionParams{WorkspaceID: workspaceID, MaxRows: 500}
	if q.Limit > 0 && q.Limit < 500 {
		params.MaxRows = int32(q.Limit)
	}
	params.Kind = pgtype.Text{String: q.Kind, Valid: q.Kind != ""}
	if len(match) > 0 {
		b, _ := json.Marshal(match)
		params.Match = b
	}
	if q.KB != "" {
		base, err := s.Queries.GetKnowledgeBaseByKey(ctx, db.GetKnowledgeBaseByKeyParams{WorkspaceID: workspaceID, Key: q.KB})
		if errors.Is(err, pgx.ErrNoRows) {
			return KnowledgeCollectionView{}, ErrKnowledgeBaseNotFound
		}
		if err != nil {
			return KnowledgeCollectionView{}, fmt.Errorf("load knowledge base: %w", err)
		}
		params.KnowledgeBaseID = base.ID
	}
	if q.DueKey != "" {
		if err := knownField(fields, q.DueKey); err != nil {
			return KnowledgeCollectionView{}, err
		}
		if _, err := parseISODay(q.DueOn); err != nil {
			return KnowledgeCollectionView{}, &KnowledgeInvalidError{"due_on must be a YYYY-MM-DD day"}
		}
		params.DueKey = pgtype.Text{String: q.DueKey, Valid: true}
		params.DueOn = pgtype.Text{String: q.DueOn, Valid: true}
	}
	links, err := s.Queries.ListKnowledgeCollection(ctx, params)
	if err != nil {
		return KnowledgeCollectionView{}, fmt.Errorf("list collection: %w", err)
	}
	bases, err := s.baseKeys(ctx, workspaceID)
	if err != nil {
		return KnowledgeCollectionView{}, err
	}
	out := KnowledgeCollectionView{Kind: q.Kind, Fields: fields, Documents: []KnowledgeDocumentView{}}
	for _, l := range links {
		out.Documents = append(out.Documents, linkViewWith(bases, l))
	}
	return out, nil
}

// parseISODay parses a YYYY-MM-DD day.
func parseISODay(s string) (time.Time, error) {
	if !isoDayPattern.MatchString(s) {
		return time.Time{}, errors.New("not a day")
	}
	return time.Parse("2006-01-02", s)
}
