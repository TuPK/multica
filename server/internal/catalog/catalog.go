// Package catalog exposes the template data the server embeds. The data is generated from the
// repository's catalog/ folder by `pnpm catalog:build` (scripts/catalog) into generated/*.json and is not
// committed. Only generated/.gitkeep is tracked, so the package still compiles before the first build;
// a reader then gets nothing back and its caller reports that the catalog has not been built.
package catalog

import (
	"embed"
	"io/fs"
)

//go:embed all:generated
var dataFS embed.FS

// BuildHint is the instruction every "not built" error should carry.
const BuildHint = "the catalog data is not generated; run `pnpm catalog:build` (or `make catalog-build`)"

func read(name string) []byte {
	b, err := fs.ReadFile(dataFS, "generated/"+name)
	if err != nil {
		return nil
	}
	return b
}

// CompanyJSON is the starter company: floors, squad cards, stage owners, agent instruction templates
// and starter skills (catalog/companies/starter, catalog/agents, catalog/skills/starter).
// Empty when the catalog has not been built.
func CompanyJSON() []byte { return read("company.json") }

// QuickstartBundleJSON lists what a fresh machine should have: the Claude Code CLI, plugin
// marketplaces, plugins and skills (catalog/bundles/quickstart). Empty when the catalog has not been built.
func QuickstartBundleJSON() []byte { return read("quickstart-bundle.json") }

// WorkflowsJSON holds the workflow templates (catalog/workflows). Empty when the catalog has not been built.
func WorkflowsJSON() []byte { return read("workflows.json") }

// DesignerWorkflowsJSON holds the system templates of the workflow designer (catalog/workflows/designer_*),
// in picker order. Empty when the catalog has not been built.
func DesignerWorkflowsJSON() []byte { return read("designer-workflows.json") }
