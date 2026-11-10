package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// accountFields are the fields of a sales account in the seed, for the unit tests below.
func accountFields(t *testing.T) []KindField {
	t.Helper()
	fields := salesKindFields["sales_account"]
	if len(fields) == 0 {
		t.Fatal("sales_account declares no fields")
	}
	return fields
}

func TestCheckPropertiesAcceptsEachTypeAndRefusesTheRest(t *testing.T) {
	fields := accountFields(t)
	ok := map[string]any{
		"stage":           "qualified",
		"fit_score":       72.0,
		"do_not_contact":  false,
		"signal_date":     "2026-11-08",
		"signal_url":      "https://example.vn/news",
		"contact_channel": "phone",
		"contact_value":   "+84 28 0000 0000",
		"size_band":       "small",
		"signal_family":   "expansion",
	}
	got, err := CheckProperties(fields, ok)
	if err != nil {
		t.Fatalf("valid properties refused: %v", err)
	}
	if got["fit_score"] != 72.0 || got["stage"] != "qualified" {
		t.Fatalf("canonical values = %+v", got)
	}

	refuse := map[string]any{
		"unknown_key": "x",
	}
	if _, err := CheckProperties(fields, refuse); err == nil || !strings.Contains(err.Error(), "no field unknown_key") {
		t.Errorf("an undeclared key was accepted: %v", err)
	}
	bad := map[string]any{
		"stage":          "sold",
		"signal_date":    "2026-02-30",
		"signal_url":     "javascript:alert(1)",
		"fit_score":      "not a number",
		"do_not_contact": "maybe",
	}
	for key := range bad {
		if _, err := CheckProperties(fields, map[string]any{key: bad[key]}); err == nil {
			t.Errorf("%s = %v was accepted", key, bad[key])
		}
	}
}

func TestPropertiesFromRecordReadsOnlyTheRecordSection(t *testing.T) {
	fields := accountFields(t)
	content := "# Acme\n\n## Why now\n\nstage: refused\n\n## Record\n\nstage: qualified\nfit_score: 64\nsignal_family: hiring\nowner: maria\nnot_a_field: x\nsignal_date: not a date\n\n## Fit\n\nfit_score: 99\n"
	got := PropertiesFromRecord(content, fields)
	want := map[string]any{"stage": "qualified", "fit_score": 64.0, "signal_family": "hiring", "owner": "maria"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v (got %+v)", k, got[k], v, got)
		}
	}
	if _, ok := got["signal_date"]; ok {
		t.Errorf("a value that does not fit its field was kept: %+v", got)
	}
	if _, ok := got["not_a_field"]; ok {
		t.Errorf("an undeclared key was kept: %+v", got)
	}
}

func TestMergeAndChangedKeysSeeRemovalsAndEdits(t *testing.T) {
	current := map[string]any{"stage": "watch", "owner": "a", "fit_score": 10.0}
	next := mergeProperties(current, map[string]any{"stage": "qualified", "owner": nil})
	if next["stage"] != "qualified" {
		t.Fatalf("stage = %v", next["stage"])
	}
	if _, ok := next["owner"]; ok {
		t.Fatalf("a null did not remove the key: %+v", next)
	}
	changed := changedKeys(current, next)
	if strings.Join(changed, ",") != "owner,stage" {
		t.Fatalf("changed = %v", changed)
	}
	if got := changedKeys(next, mergeProperties(next, map[string]any{"stage": "qualified"})); len(got) != 0 {
		t.Fatalf("an unchanged value reported as changed: %v", got)
	}
}

func TestOrderedStagesRankByTheirOptions(t *testing.T) {
	var stage KindField
	for _, f := range accountFields(t) {
		if f.Key == "stage" {
			stage = f
		}
	}
	if !stage.Ordered {
		t.Fatal("the account stage must be ordered, so an agent cannot move it back")
	}
	if stageRank(stage, "watch") >= stageRank(stage, "qualified") {
		t.Fatal("watch must rank before qualified")
	}
	if stageRank(stage, "nonsense") != -1 {
		t.Fatal("an unknown stage must rank below every stage")
	}
}

func TestValidateKindFieldsNamesEachProblem(t *testing.T) {
	cases := map[string][]KindField{
		"key":     {{Key: "Bad-Key", Label: "x", Type: KindFieldText}},
		"label":   {{Key: "ok", Label: " ", Type: KindFieldText}},
		"options": {{Key: "pick", Label: "Pick", Type: KindFieldSelect}},
		"type":    {{Key: "odd", Label: "Odd", Type: "color"}},
		"twice":   {{Key: "a", Label: "A", Type: KindFieldText}, {Key: "a", Label: "A", Type: KindFieldText}},
		"ordered": {{Key: "n", Label: "N", Type: KindFieldNumber, Ordered: true}},
	}
	for want, fields := range cases {
		if err := ValidateKindFields(fields); err == nil {
			t.Errorf("%s: no error", want)
		}
	}
	for _, s := range []string{"sales_account", "sales_source", "sales_lane"} {
		if err := ValidateKindFields(salesKindFields[s]); err != nil {
			t.Errorf("the seed of %s is invalid: %v", s, err)
		}
	}
}

func TestEverySalesKindHasFieldsAndTheyDecode(t *testing.T) {
	for key, fields := range salesKindFields {
		if !strings.HasPrefix(key, "sales_") {
			t.Errorf("fields for a non-sales kind: %s", key)
		}
		raw, err := encodeKindFields(fields)
		if err != nil {
			t.Fatal(err)
		}
		if back := decodeKindFields(raw); len(back) != len(fields) {
			t.Errorf("%s: %d fields encoded, %d decoded", key, len(fields), len(back))
		}
	}
}

// The properties of a document, changed by a person and then an agent: the agent cannot move the
// stage back, and each change is in the history.
func TestUpdateDocumentPropertiesKeepsAPersonsStageAndRecordsChanges(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	ctx := context.Background()
	doc, _, err := e.svc.CreateDocument(ctx, e.ws, KnowledgeActor{Type: KnowledgeActorMember, ID: e.user}, KnowledgeDocumentInput{
		KB: "sales", Kind: "sales_account", Title: "Acme Co", Content: "# Acme Co\n\n## Record\n\nstage: watch\nfit_score: 40\n\n## Why now\n\nHiring.\n\n## Evidence\n\nA post.\n\n## Contact path\n\nNone.\n\n## Fit\n\nScored.\n",
		Publish: true, IdempotencyKey: "sales:account:test:acme",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	linkID := mustUUID(t, doc.ID)

	member := KnowledgeActor{Type: KnowledgeActorMember, ID: e.user}
	if _, err := e.svc.UpdateDocumentProperties(ctx, e.ws, member, linkID, map[string]any{"stage": "qualified"}); err != nil {
		t.Fatalf("a person moves the stage forward: %v", err)
	}
	agent := KnowledgeActor{Type: KnowledgeActorAgent, ID: e.user}
	if _, err := e.svc.UpdateDocumentProperties(ctx, e.ws, agent, linkID, map[string]any{"stage": "watch"}); err == nil || !strings.Contains(err.Error(), "cannot move it back") {
		t.Fatalf("an agent moved a person's stage back: %v", err)
	}
	after, err := e.svc.UpdateDocumentProperties(ctx, e.ws, agent, linkID, map[string]any{"stage": "meeting", "fit_score": 77})
	if err != nil {
		t.Fatalf("an agent moves the stage forward: %v", err)
	}
	if after.Properties["stage"] != "meeting" || after.Properties["fit_score"] != 77.0 {
		t.Fatalf("properties after the agent = %+v", after.Properties)
	}
	if _, err := e.svc.UpdateDocumentProperties(ctx, e.ws, member, linkID, map[string]any{"color": "red"}); err == nil {
		t.Fatal("an undeclared field was accepted")
	}
}

func TestCollectionFiltersByAFieldAndADueDate(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	ctx := context.Background()
	for i, stage := range []string{"qualified", "watch"} {
		body := "# Lead " + stage + "\n\n## Record\n\nstage: " + stage + "\nnext_action_date: 2026-11-0" + string(rune('5'+i)) + "\n\n## Why now\n\nx.\n\n## Evidence\n\nx.\n\n## Contact path\n\nx.\n\n## Fit\n\nx.\n"
		if _, _, err := e.svc.CreateDocument(ctx, e.ws, KnowledgeActor{Type: KnowledgeActorMember, ID: e.user}, KnowledgeDocumentInput{
			KB: "sales", Kind: "sales_account", Title: "Lead " + stage, Content: body, Publish: true,
			IdempotencyKey: "sales:account:test:collection-" + stage,
		}); err != nil {
			t.Fatalf("create %s: %v", stage, err)
		}
	}
	view, err := e.svc.ListCollection(ctx, e.ws, CollectionQuery{Kind: "sales_account", Match: map[string]string{"stage": "qualified"}})
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	if len(view.Documents) != 1 || view.Documents[0].Title != "Lead qualified" {
		t.Fatalf("stage=qualified matched %+v", view.Documents)
	}
	due, err := e.svc.ListCollection(ctx, e.ws, CollectionQuery{Kind: "sales_account", DueKey: "next_action_date", DueOn: "2026-11-05"})
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due.Documents) != 1 {
		t.Fatalf("due on 2026-11-05 matched %d documents, want the one due that day", len(due.Documents))
	}
	if _, err := e.svc.ListCollection(ctx, e.ws, CollectionQuery{Kind: "sales_account", Match: map[string]string{"colour": "red"}}); err == nil {
		t.Fatal("a filter on an undeclared field was accepted")
	}
}

var _ = pgtype.UUID{}
