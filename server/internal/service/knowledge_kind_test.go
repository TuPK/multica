package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/knowledge"
)

// adrBody carries every heading the adr kind requires.
const adrBody = "# Storage\n\n## Context\n\nqueue latency grew.\n\n## Decision\n\nuse partitioned tables.\n\n## Consequences\n\nmigrations get longer.\n"

func TestMissingSectionsReportsEachAbsentHeading(t *testing.T) {
	required := []string{"Context", "Decision", "Consequences"}
	content := "# Title\n\n## context\n\nx\n\n### Decision  \n\ny\n"
	if got := MissingSections(required, content); !reflect.DeepEqual(got, []string{"Consequences"}) {
		t.Fatalf("missing = %v, want [Consequences]", got)
	}
	if got := MissingSections(required, adrBody); len(got) != 0 {
		t.Fatalf("a complete document reported %v", got)
	}
}

func TestMissingSectionsIgnoresHeadingsInsideCodeFences(t *testing.T) {
	content := "## Context\n\n```md\n## Consequences\n```\n\n## Decision\n"
	if got := MissingSections([]string{"Context", "Decision", "Consequences"}, content); !reflect.DeepEqual(got, []string{"Consequences"}) {
		t.Fatalf("a heading in a fence counted: missing = %v", got)
	}
}

func TestSlugifyAndInitiativeFolderName(t *testing.T) {
	cases := map[string]string{
		"Invoicing tool":    "invoicing-tool",
		"  Café & Crème!  ": "caf-cr-me",
		"***":               "initiative",
		"A very long title that keeps going well past forty characters for sure": "a-very-long-title-that-keeps-going-well",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := InitiativeFolderName(3, "Invoicing tool"); got != "INI-3-invoicing-tool" {
		t.Fatalf("folder = %q", got)
	}
	if !initiativeFolderPattern.MatchString("INI-3-invoicing-tool") || initiativeFolderPattern.MatchString("INI-0-x") {
		t.Fatal("the folder pattern does not accept INI-<seq>-<slug> alone")
	}
}

func TestSeedKindsIsIdempotentAndKeepsAdminEdits(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	ctx := context.Background()

	kinds, err := e.svc.ListKnowledgeKinds(ctx, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != len(defaultKnowledgeKinds) {
		t.Fatalf("seeded %d kinds, want %d", len(kinds), len(defaultKnowledgeKinds))
	}
	byKey := map[string]KnowledgeKindView{}
	for _, k := range kinds {
		byKey[k.Key] = k
	}
	if byKey["adr"].DefaultBaseKey != "engineering" || !strings.Contains(byKey["adr"].Template, "## Consequences") {
		t.Fatalf("adr = %+v", byKey["adr"])
	}

	name := "Decision note"
	if _, err := e.svc.UpdateKnowledgeKind(ctx, e.ws, "adr", KnowledgeKindPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SeedKnowledgeKinds(ctx, e.ws); err != nil {
		t.Fatal(err)
	}
	kinds, _ = e.svc.ListKnowledgeKinds(ctx, e.ws)
	for _, k := range kinds {
		if k.Key == "adr" && k.Name != "Decision note" {
			t.Fatalf("a re-seed overwrote the admin's name: %q", k.Name)
		}
	}
}

func TestUpdateKindRefusesATemplateThatLacksARequiredHeading(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	ctx := context.Background()
	bad := "# Storage\n\n## Context\n"
	_, err := e.svc.UpdateKnowledgeKind(ctx, e.ws, "adr", KnowledgeKindPatch{Template: &bad})
	var invalid *KnowledgeInvalidError
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Message, "Decision") {
		t.Fatalf("err = %v, want an invalid error naming the missing heading", err)
	}
	review := -1
	if _, err := e.svc.UpdateKnowledgeKind(ctx, e.ws, "adr", KnowledgeKindPatch{ReviewDays: &review}); err == nil {
		t.Fatal("a negative review_days was accepted")
	}
	if _, err := e.svc.UpdateKnowledgeKind(ctx, e.ws, "no_such_kind", KnowledgeKindPatch{}); !errors.Is(err, ErrKnowledgeKindNotFound) {
		t.Fatalf("unknown kind: err = %v", err)
	}
}

func TestDocumentWithoutItsRequiredHeadingsIsRejectedWithTheMissingList(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	body := "# Storage\n\n## Context\n\nqueue latency grew.\n\n## Decision\n\nuse partitioned tables.\n"

	_, _, err := e.create(agentActor(e), KnowledgeDocumentInput{KB: "engineering", Title: "Storage", Content: body, Kind: "adr"})
	var missing *KnowledgeMissingSectionsError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want KnowledgeMissingSectionsError", err)
	}
	if !reflect.DeepEqual(missing.Missing, []string{"Consequences"}) || missing.Kind != "adr" {
		t.Fatalf("missing = %+v", missing)
	}
	if n := e.linkCount(); n != 0 {
		t.Fatalf("a refused draft left %d records", n)
	}
	if e.fake.Calls(knowledge.OpCreateManual) != 0 {
		t.Fatal("a refused draft reached WeKnora")
	}
}

func TestKindWithDefaultBaseNeedsNoKB(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	v := e.mustCreate(memberActor(e), KnowledgeDocumentInput{Kind: "adr", Title: "Storage", Content: adrBody})
	if v.KnowledgeBase != "engineering" || v.KnowledgeKind != "adr" {
		t.Fatalf("view = %+v", v)
	}
	if _, _, err := e.create(memberActor(e), KnowledgeDocumentInput{Kind: "no_such_kind", Title: "x", Content: "y"}); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
	if _, _, err := e.create(memberActor(e), KnowledgeDocumentInput{Kind: "policy", Title: "x", Content: "## Scope\n\n## Rule\n\n## Owner\n"}); err != nil {
		t.Fatalf("policy has the company base as default: %v", err)
	}
}

func TestInitiativeFolderNeedsAnInitiative(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	_, _, err := e.create(memberActor(e), KnowledgeDocumentInput{KB: "product", Title: "x", Content: "y", InitiativeFolder: "INI-1-x"})
	var invalid *KnowledgeInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want invalid", err)
	}
}

func TestSalesKindsLiveInTheSalesBaseWithARecordSection(t *testing.T) {
	bases := map[string]bool{}
	for _, b := range defaultKnowledgeBases {
		bases[b.Key] = true
	}
	if !bases[KnowledgeSalesBaseKey] {
		t.Fatalf("the default bases lack %q", KnowledgeSalesBaseKey)
	}
	sales := 0
	for _, def := range defaultKnowledgeKinds {
		if !bases[def.DefaultBase] {
			t.Errorf("kind %s defaults to base %q, which is not a default base", def.Key, def.DefaultBase)
		}
		if def.DefaultBase != KnowledgeSalesBaseKey {
			continue
		}
		sales++
		if !strings.HasPrefix(def.Key, "sales_") {
			t.Errorf("kind %s is in the sales base but not named sales_*", def.Key)
		}
		if def.Required[0] != "Record" {
			t.Errorf("kind %s must open its required sections with Record, got %v", def.Key, def.Required)
		}
	}
	if sales != 13 {
		t.Fatalf("the sales base has %d kinds, want 13", sales)
	}
}

func TestDefaultPublishModesAreValidAndNameSalesKinds(t *testing.T) {
	kinds := map[string]bool{}
	for _, def := range defaultKnowledgeKinds {
		kinds[def.Key] = true
	}
	for key, mode := range defaultKindPublishModes {
		if !kinds[key] {
			t.Errorf("publish mode for %q, which is not a default kind", key)
		}
		if !validPublishMode(mode) {
			t.Errorf("publish mode %q for %q is not a policy mode", mode, key)
		}
		if !strings.HasPrefix(key, "sales_") {
			t.Errorf("%q has a default publish mode but is not a sales kind", key)
		}
	}
	if defaultKindPublishModes["sales_icp"] != "" {
		t.Error("sales_icp must stay manual: its default is manual, so it has no seeded mode")
	}
}

func TestSeedDefaultPublishModesKeepsAnAdminsMode(t *testing.T) {
	e := newKnowledgeEnv(t).enabled()
	ctx := context.Background()

	if _, err := e.svc.SetPublishPolicy(ctx, e.ws, e.user, PublishScopeKind, "sales_outreach", PublishModeManual); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.seedDefaultPublishModes(ctx, e.ws, e.user); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.seedDefaultPublishModes(ctx, e.ws, e.user); err != nil {
		t.Fatalf("a second seed must change nothing and not fail: %v", err)
	}
	policies, err := e.svc.ListPublishPolicies(ctx, e.ws)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]string{}
	count := map[string]int{}
	for _, p := range policies {
		if p.Scope == PublishScopeKind {
			byKey[p.Key] = p.Mode
			count[p.Key]++
		}
	}
	if byKey["sales_lane"] != PublishModeAuto {
		t.Errorf("sales_lane = %q, want auto", byKey["sales_lane"])
	}
	if byKey["sales_lesson"] != PublishModeAfterReview {
		t.Errorf("sales_lesson = %q, want auto_after_review", byKey["sales_lesson"])
	}
	if byKey["sales_outreach"] != PublishModeManual {
		t.Errorf("the admin's manual mode on sales_outreach was overwritten: %q", byKey["sales_outreach"])
	}
	if count["sales_lane"] != 1 {
		t.Errorf("sales_lane has %d policy rows, want one", count["sales_lane"])
	}
}
