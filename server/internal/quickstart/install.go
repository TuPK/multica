package quickstart

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// SkillSource returns the workspace copies of the named skills. Names the
// workspace does not have come back in Missing, not as an error.
type SkillSource func(ctx context.Context, names []string) (protocol.QuickstartSkillBundles, error)

// Env is everything that touches the host. Tests supply fakes.
type Env struct {
	LookPath func(file string) (string, error)
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	// ConfigDir is the Claude Code config directory: $CLAUDE_CONFIG_DIR, or
	// ~/.claude by default.
	ConfigDir string
}

// DefaultEnv resolves the real host environment.
func DefaultEnv() (Env, error) {
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Env{}, fmt.Errorf("resolve home directory: %w", err)
		}
		configDir = filepath.Join(home, ".claude")
	}
	return Env{
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		ConfigDir: configDir,
	}, nil
}

// Run installs whatever in the bundle is missing and reports every item.
// A failed item does not stop the others, so one problem does not hide the
// rest of the setup. Items that are already present are not touched.
func Run(ctx context.Context, env Env, bundle protocol.QuickstartBundle, skills SkillSource) []protocol.QuickstartStep {
	var steps []protocol.QuickstartStep

	claude := ensureClaudeCode(ctx, env, bundle.ClaudeCode.NPMPackage)
	steps = append(steps, claude)
	claudeReady := claude.Status != protocol.QuickstartStepFailed

	steps = append(steps, ensureMarketplaces(ctx, env, bundle.Marketplaces, claudeReady)...)
	steps = append(steps, ensurePlugins(ctx, env, bundle.Plugins, claudeReady)...)
	steps = append(steps, ensureSkills(ctx, env, bundle.Skills, skills)...)
	return steps
}

// Failed reports whether any step failed.
func Failed(steps []protocol.QuickstartStep) bool {
	for _, step := range steps {
		if step.Status == protocol.QuickstartStepFailed {
			return true
		}
	}
	return false
}

func ensureClaudeCode(ctx context.Context, env Env, npmPackage string) protocol.QuickstartStep {
	const name = "claude code"
	if path, err := env.LookPath("claude"); err == nil {
		return protocol.QuickstartStep{Name: name, Status: protocol.QuickstartStepAlreadyInstalled, Detail: path}
	}
	if _, err := env.LookPath("npm"); err != nil {
		return protocol.QuickstartStep{Name: name, Status: protocol.QuickstartStepFailed, Detail: "npm not found; install Node.js or install Claude Code manually"}
	}
	if out, err := env.Run(ctx, "npm", "install", "-g", npmPackage); err != nil {
		return protocol.QuickstartStep{Name: name, Status: protocol.QuickstartStepFailed, Detail: firstLine(out, err)}
	}
	if _, err := env.LookPath("claude"); err != nil {
		return protocol.QuickstartStep{Name: name, Status: protocol.QuickstartStepFailed, Detail: "installed but claude is not on PATH; check your npm global bin directory"}
	}
	return protocol.QuickstartStep{Name: name, Status: protocol.QuickstartStepInstalled, Detail: "installed " + npmPackage}
}

func ensureMarketplaces(ctx context.Context, env Env, marketplaces []protocol.QuickstartMarketplace, claudeReady bool) []protocol.QuickstartStep {
	known := readKeys(filepath.Join(env.ConfigDir, "plugins", "known_marketplaces.json"), false)
	steps := make([]protocol.QuickstartStep, 0, len(marketplaces))
	for _, m := range marketplaces {
		step := protocol.QuickstartStep{Name: "marketplace " + m.Name}
		switch {
		case known[m.Name]:
			step.Status = protocol.QuickstartStepAlreadyInstalled
		case !claudeReady:
			step.Status = protocol.QuickstartStepFailed
			step.Detail = "Claude Code is not available"
		default:
			if out, err := env.Run(ctx, "claude", "plugin", "marketplace", "add", m.Source); err != nil {
				step.Status = protocol.QuickstartStepFailed
				step.Detail = firstLine(out, err)
			} else {
				step.Status = protocol.QuickstartStepInstalled
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func ensurePlugins(ctx context.Context, env Env, plugins []string, claudeReady bool) []protocol.QuickstartStep {
	installed := readKeys(filepath.Join(env.ConfigDir, "plugins", "installed_plugins.json"), true)
	steps := make([]protocol.QuickstartStep, 0, len(plugins))
	for _, p := range plugins {
		step := protocol.QuickstartStep{Name: "plugin " + p}
		switch {
		case installed[p]:
			step.Status = protocol.QuickstartStepAlreadyInstalled
		case !claudeReady:
			step.Status = protocol.QuickstartStepFailed
			step.Detail = "Claude Code is not available"
		default:
			if out, err := env.Run(ctx, "claude", "plugin", "install", p, "--scope", "user"); err != nil {
				step.Status = protocol.QuickstartStepFailed
				step.Detail = firstLine(out, err)
			} else {
				step.Status = protocol.QuickstartStepInstalled
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func ensureSkills(ctx context.Context, env Env, wanted []string, source SkillSource) []protocol.QuickstartStep {
	skillsRoot := filepath.Join(env.ConfigDir, "skills")
	steps := make(map[string]protocol.QuickstartStep, len(wanted))

	var missingOnDisk []string
	for _, name := range wanted {
		if _, err := os.Stat(filepath.Join(skillsRoot, name, "SKILL.md")); err == nil {
			steps[name] = protocol.QuickstartStep{Name: "skill " + name, Status: protocol.QuickstartStepAlreadyInstalled}
			continue
		}
		missingOnDisk = append(missingOnDisk, name)
	}

	if len(missingOnDisk) > 0 {
		bundles, err := source(ctx, missingOnDisk)
		if err != nil {
			for _, name := range missingOnDisk {
				steps[name] = protocol.QuickstartStep{Name: "skill " + name, Status: protocol.QuickstartStepFailed, Detail: err.Error()}
			}
		} else {
			missing := make(map[string]bool, len(bundles.Missing))
			for _, name := range bundles.Missing {
				missing[name] = true
			}
			for _, skill := range bundles.Skills {
				step := protocol.QuickstartStep{Name: "skill " + skill.Name, Status: protocol.QuickstartStepInstalled}
				if err := writeSkill(skillsRoot, skill); err != nil {
					step.Status = protocol.QuickstartStepFailed
					step.Detail = err.Error()
				}
				steps[skill.Name] = step
			}
			for _, name := range missingOnDisk {
				if _, done := steps[name]; !done {
					steps[name] = protocol.QuickstartStep{Name: "skill " + name, Status: protocol.QuickstartStepFailed, Detail: "not found in the workspace"}
				}
			}
		}
	}

	out := make([]protocol.QuickstartStep, 0, len(wanted))
	for _, name := range wanted {
		out = append(out, steps[name])
	}
	return out
}

// writeSkill writes a skill into root/<name>. Names and paths come from the
// server, so each must stay inside its directory.
func writeSkill(root string, skill protocol.QuickstartSkill) error {
	if skill.Name == "" || filepath.Base(skill.Name) != skill.Name || skill.Name == "." || skill.Name == ".." {
		return fmt.Errorf("skill name %q is not a plain directory name", skill.Name)
	}
	dir := filepath.Join(root, skill.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill.Content), 0o644); err != nil {
		return err
	}
	for _, file := range skill.Files {
		rel := filepath.Clean(file.Path)
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("skill file path %q escapes the skill directory", file.Path)
		}
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// readKeys returns the top-level keys of a Claude Code registry file. For
// installed_plugins.json the entries sit under "plugins", so nested is true.
// A missing or unreadable file counts as empty.
func readKeys(path string, nested bool) map[string]bool {
	keys := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return keys
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return keys
	}
	entries := doc
	if nested {
		var inner map[string]json.RawMessage
		if json.Unmarshal(doc["plugins"], &inner) != nil {
			return keys
		}
		entries = inner
	}
	for k := range entries {
		keys[k] = true
	}
	return keys
}

func firstLine(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return text
}
