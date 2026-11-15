package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source check: every R1-catalog handler must write its audit row. The
// accepted forms are a call to audit.Record or to a handler helper that wraps it.
func TestAuditCatalogHandlersRecordAuditEvents(t *testing.T) {
	want := map[string][]string{
		"workspace.go":             {"CreateWorkspace", "UpdateWorkspace", "DeleteWorkspace", "UpdateMember", "DeleteMember", "LeaveWorkspace"},
		"invitation.go":            {"CreateInvitation", "RevokeInvitation", "AcceptInvitation", "DeclineInvitation"},
		"share_link.go":            {"CreateShareLink", "RevokeShareLink", "JoinByShareLink"},
		"personal_access_token.go": {"CreatePersonalAccessToken", "RevokePersonalAccessToken"},
		"me_sessions.go":           {"RevokeAllSessions"},
	}
	fset := token.NewFileSet()
	for file, names := range want {
		src, err := os.ReadFile(filepath.Join(".", file))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		bodies := map[string]string{}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil {
				bodies[fn.Name.Name] = string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			}
		}
		for _, name := range names {
			body, ok := bodies[name]
			if !ok {
				t.Errorf("%s: handler %s not found", file, name)
				continue
			}
			if !strings.Contains(body, "audit.Record(") && !strings.Contains(body, "h.recordAudit(") {
				t.Errorf("%s: %s has no audit.Record call", file, name)
			}
		}
	}

	// auth.go: three issueJWT call sites, each followed by a login audit.
	src, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatal(err)
	}
	if n, m := strings.Count(string(src), "h.issueJWT("), strings.Count(string(src), "h.recordLogin("); n == 0 || m != n {
		t.Errorf("auth.go: %d issueJWT calls but %d recordLogin calls", n, m)
	}
}
