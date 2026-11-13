package issueworkflow

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckVarValue(t *testing.T) {
	one, ten := 1, 10
	cases := []struct {
		name  string
		v     Var
		in    any
		want  any
		fails string
	}{
		{"bool", Var{Key: "k", Type: VarTypeBool}, true, true, ""},
		{"bool given text", Var{Key: "k", Type: VarTypeBool}, "true", nil, "true or false"},
		{"enum", Var{Key: "k", Type: VarTypeEnum, Values: []string{"a", "b"}}, "b", "b", ""},
		{"enum outside", Var{Key: "k", Type: VarTypeEnum, Values: []string{"a", "b"}}, "c", nil, "one of: a, b"},
		{"int from json", Var{Key: "k", Type: VarTypeInt, Min: &one, Max: &ten}, float64(4), 4, ""},
		{"int below", Var{Key: "k", Type: VarTypeInt, Min: &one, Max: &ten}, float64(0), nil, "at least 1"},
		{"int above", Var{Key: "k", Type: VarTypeInt, Min: &one, Max: &ten}, float64(11), nil, "at most 10"},
		{"int fraction", Var{Key: "k", Type: VarTypeInt}, 1.5, nil, "whole number"},
		{"string too long", Var{Key: "k", Type: VarTypeString, MaxLen: 3}, "abcd", nil, "at most 3 characters"},
		{"list", Var{Key: "k", Type: VarTypeListString, MaxItems: 2}, []any{"a", "b"}, []string{"a", "b"}, ""},
		{"list too long", Var{Key: "k", Type: VarTypeListString, MaxItems: 2}, []any{"a", "b", "c"}, nil, "at most 2 items"},
		{"list of numbers", Var{Key: "k", Type: VarTypeListString}, []any{1}, nil, "list of texts"},
		{"list item too long", Var{Key: "k", Type: VarTypeListString, MaxLen: 2}, []any{"abc"}, nil, "item of k"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckVarValue(tc.v, tc.in)
			if tc.fails != "" {
				if err == nil || !strings.Contains(err.Error(), tc.fails) {
					t.Fatalf("CheckVarValue = %v, %v; want an error with %q", got, err, tc.fails)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CheckVarValue = %v (%T), %v; want %v", got, got, err, tc.want)
			}
		})
	}
}

func TestCountSlotFiles(t *testing.T) {
	files := []AttachedFile{
		{"a.md", "text/markdown"}, {"B.MD", "text/markdown; charset=utf-8"}, {"c.csv", "text/csv"}, {"d.csv", "application/octet-stream"},
	}
	cases := []struct {
		name string
		slot FileReq
		want int
	}{
		{"extension, case blind", FileReq{Slot: "r", Min: 1, Ext: []string{".md"}}, 2},
		{"content type, parameters ignored", FileReq{Slot: "r", Min: 1, ContentTypes: []string{"text/markdown"}}, 2},
		{"extension and content type", FileReq{Slot: "r", Min: 1, Ext: []string{".csv"}, ContentTypes: []string{"text/csv"}}, 1},
		{"wildcard type", FileReq{Slot: "r", Min: 1, ContentTypes: []string{"text/*"}}, 3},
		{"no filter takes all", FileReq{Slot: "r", Min: 1}, 4},
	}
	for _, tc := range cases {
		if got := CountSlotFiles(tc.slot, files); got != tc.want {
			t.Errorf("%s: %d files, want %d", tc.name, got, tc.want)
		}
	}
	slot, count, ok := SlotShortfall([]FileReq{{Slot: "r", Min: 1, Max: 1, Ext: []string{".md"}}}, files)
	if ok || slot.Slot != "r" || count != 2 {
		t.Fatalf("SlotShortfall = %v %d %v; want slot r over its max with 2", slot, count, ok)
	}
}

func TestStateCloneSharesNothing(t *testing.T) {
	st := State{Step: "a", Loops: map[string]int{"x": 1}, Vars: map[string]any{"k": 1}, Stop: &StopRecord{Code: "c"}, Trail: []TrailEntry{{Step: "a"}}}
	c := st.Clone()
	c.Loops["x"] = 9
	c.Vars["k"] = 2
	c.Stop.Code = "d"
	c.Trail[0].Step = "b"
	if st.Loops["x"] != 1 || st.Vars["k"] != 1 || st.Stop.Code != "c" || st.Trail[0].Step != "a" {
		t.Fatalf("a change of the clone reached the state: %+v", st)
	}
}

// The text of a task of a schema 2 flow lists what the graph says: the keys of its
// verdict set, the files per slot, the variables it sets and the skills it uses.
func TestGraphTaskTexts(t *testing.T) {
	doc := FlowDoc{
		Version: FlowDocVersion, Start: "plan",
		Roles:       []Role{{Key: "dev", Kind: RoleKindAgent}},
		Vars:        []Var{{Key: "score", Type: VarTypeInt, Default: 0}},
		VerdictSets: []VerdictSet{{Key: "made", Verdicts: []Verdict{{Key: "made", Class: ClassPass}, {Key: "partial", Class: ClassNeutral}}}},
	}
	g := NewGraph(doc)
	task := DocTask{Key: "make", Title: "Make it", Kind: KindWork, Role: "dev", Verdicts: "made", Skills: []string{"pdf-reader"}, Sets: []string{"score"},
		Requires: &Requires{Verdict: boolPtr(true), Files: []FileReq{{Slot: "report", Min: 1, Ext: []string{".md"}}}, Vars: []string{"score"}}}
	in := GraphTaskText{RequestIdentifier: "MUL-1", TaskIdentifier: "MUL-2", StepName: "Build", Task: task, Row: OpenTask{Key: "make~2", Task: task, Index: 2, Count: 3, Item: "alpha `x`"}}
	for name, text := range map[string]string{"description": GraphTaskDescription(g, in), "brief": GraphTaskBrief(g, in)} {
		for _, want := range []string{"--outcome made", "--outcome partial", "slot `report`: at least 1, extension .md", "multica issue flow set MUL-2 score", "pdf-reader", "alpha 'x'"} {
			if !strings.Contains(text, want) {
				t.Errorf("the %s lacks %q:\n%s", name, want, text)
			}
		}
		if strings.Contains(text, "--outcome approved") {
			t.Errorf("the %s names a verdict the task does not have:\n%s", name, text)
		}
	}
}
