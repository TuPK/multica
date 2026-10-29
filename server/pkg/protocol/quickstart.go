package protocol

// QuickstartBundle is the set of Claude Code pieces a machine needs to match
// the team setup. The server owns the list and sends it with each pending
// quickstart request, so a daemon never installs from a stale copy.
type QuickstartBundle struct {
	ClaudeCode   QuickstartClaudeCode    `json:"claude_code"`
	Marketplaces []QuickstartMarketplace `json:"marketplaces"`
	Plugins      []string                `json:"plugins"`
	Skills       []string                `json:"skills"`
}

type QuickstartClaudeCode struct {
	NPMPackage string `json:"npm_package"`
}

type QuickstartMarketplace struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// QuickstartStep reports one install item. Status is one of
// QuickstartStepInstalled, QuickstartStepAlreadyInstalled or
// QuickstartStepFailed.
type QuickstartStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

const (
	QuickstartStepInstalled        = "installed"
	QuickstartStepAlreadyInstalled = "already_installed"
	QuickstartStepFailed           = "failed"
)

// QuickstartSkill is a workspace skill ready to be written to disk.
type QuickstartSkill struct {
	Name    string                `json:"name"`
	Content string                `json:"content"`
	Files   []QuickstartSkillFile `json:"files,omitempty"`
}

type QuickstartSkillFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// QuickstartSkillBundles is the daemon's answer for a set of skill names.
// Names the workspace does not have are listed in Missing.
type QuickstartSkillBundles struct {
	Skills  []QuickstartSkill `json:"skills"`
	Missing []string          `json:"missing,omitempty"`
}
