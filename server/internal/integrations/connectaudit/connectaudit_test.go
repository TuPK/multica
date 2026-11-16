package connectaudit

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/util"
)

func TestEntryNamesTheInstallerAndTheInstallation(t *testing.T) {
	ws, inst, user := util.MustParseUUID("11111111-1111-1111-1111-111111111111"),
		util.MustParseUUID("22222222-2222-2222-2222-222222222222"),
		util.MustParseUUID("33333333-3333-3333-3333-333333333333")
	e := Entry(Connected{Provider: "slack", WorkspaceID: ws, InstallationID: inst, InstallerUserID: user}, "owner@example.com")

	if e.Action != audit.ActionIntegrationConnected {
		t.Errorf("action = %q, want %q", e.Action, audit.ActionIntegrationConnected)
	}
	if e.ActorType != audit.ActorMember || e.ActorID != util.UUIDToString(user) || e.ActorLabel != "owner@example.com" {
		t.Errorf("actor = %s/%s/%s, want the installer", e.ActorType, e.ActorID, e.ActorLabel)
	}
	if e.WorkspaceID != util.UUIDToString(ws) || e.TargetType != "integration" || e.TargetID != util.UUIDToString(inst) {
		t.Errorf("workspace/target = %s %s/%s", e.WorkspaceID, e.TargetType, e.TargetID)
	}
	if len(e.Detail) != 1 || e.Detail["provider"] != "slack" {
		t.Errorf("detail = %v, want only the provider", e.Detail)
	}
}

func TestEntryWithoutAnInstallerIsASystemRow(t *testing.T) {
	e := Entry(Connected{Provider: "wecom"}, "")
	if e.ActorType != audit.ActorSystem || e.ActorID != "" {
		t.Errorf("actor = %s/%q, want system with no id", e.ActorType, e.ActorID)
	}
}

// The entry is built from ids and the provider only, so no field of the
// installation's config can reach it.
func TestEntryCarriesNoCredential(t *testing.T) {
	for _, provider := range []string{"slack", "lark", "telegram", "wecom", "dingtalk"} {
		raw, err := json.Marshal(audit.Redact(Entry(Connected{Provider: provider}, "a@example.com").Detail))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"xoxb-", "app_secret", "token", "secret"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("%s: detail %s mentions %q", provider, raw, secret)
			}
		}
	}
}

// Source check: each of the five install paths writes the audit row inside its
// install transaction, before the commit. A path that skipped it, or wrote it
// after the commit, would save an installation with no row, or a row for an
// installation that was rolled back.
func TestEveryInstallPathAuditsBeforeItCommits(t *testing.T) {
	paths := []struct{ file, fn string }{
		{"../slack/install.go", "persistInstall"},
		{"../telegram/install.go", "persistInstall"},
		{"../dingtalk/install.go", "persistInstall"},
		{"../lark/registration_service.go", "finishSuccess"},
		{"../wecom/installation.go", "Upsert"},
	}
	for _, p := range paths {
		src, err := os.ReadFile(p.file)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p.file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		var body string
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == p.fn && fn.Body != nil {
				body = string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			}
		}
		if body == "" {
			t.Errorf("%s: function %s not found", p.file, p.fn)
			continue
		}
		record := strings.Index(body, "RecordConnected(")
		if record < 0 {
			record = strings.Index(body, "connectaudit.Record(")
		}
		commit := strings.Index(body, "Commit(ctx)")
		if record < 0 || commit < 0 || record > commit {
			t.Errorf("%s: %s must write the audit row before Commit (record at %d, commit at %d)", p.file, p.fn, record, commit)
		}
	}
}
