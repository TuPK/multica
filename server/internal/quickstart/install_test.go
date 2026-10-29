package quickstart

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

func fakeEnv(t *testing.T, claudeInstalled bool) (Env, *[][]string) {
	t.Helper()
	var calls [][]string
	installed := claudeInstalled
	env := Env{
		ConfigDir: t.TempDir(),
		LookPath: func(file string) (string, error) {
			if file == "claude" && !installed {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + file, nil
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, append([]string{name}, args...))
			if name == "npm" {
				installed = true
			}
			return nil, nil
		},
	}
	return env, &calls
}

func noSkills(t *testing.T) SkillSource {
	t.Helper()
	return func(context.Context, []string) (protocol.QuickstartSkillBundles, error) {
		t.Fatal("skill source must not be called")
		return protocol.QuickstartSkillBundles{}, nil
	}
}

func writeRegistry(t *testing.T, env Env, name, content string) {
	t.Helper()
	dir := filepath.Join(env.ConfigDir, "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func statusOf(steps []protocol.QuickstartStep, name string) string {
	for _, s := range steps {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}

func TestLoadBundleDeclaresEveryPluginMarketplace(t *testing.T) {
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, m := range bundle.Marketplaces {
		declared[m.Name] = true
	}
	for _, plugin := range bundle.Plugins {
		_, marketplace, ok := strings.Cut(plugin, "@")
		if !ok || !declared[marketplace] {
			t.Fatalf("plugin %q needs plugin@marketplace with a declared marketplace", plugin)
		}
	}
}

func TestRunSkipsEverythingAlreadyPresent(t *testing.T) {
	env, calls := fakeEnv(t, true)
	writeRegistry(t, env, "known_marketplaces.json", `{"m1": {}}`)
	writeRegistry(t, env, "installed_plugins.json", `{"plugins": {"p@m1": []}}`)
	skillDir := filepath.Join(env.ConfigDir, "skills", "s1")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	bundle := protocol.QuickstartBundle{
		Marketplaces: []protocol.QuickstartMarketplace{{Name: "m1", Source: "o/r"}},
		Plugins:      []string{"p@m1"},
		Skills:       []string{"s1"},
	}
	steps := Run(context.Background(), env, bundle, noSkills(t))

	if len(*calls) != 0 {
		t.Fatalf("expected no installer commands, got %v", *calls)
	}
	if Failed(steps) {
		t.Fatalf("steps = %+v, want no failures", steps)
	}
	for _, name := range []string{"claude code", "marketplace m1", "plugin p@m1", "skill s1"} {
		if got := statusOf(steps, name); got != protocol.QuickstartStepAlreadyInstalled {
			t.Fatalf("%s status = %q, want already_installed", name, got)
		}
	}
}

func TestRunInstallsOnlyMissingItems(t *testing.T) {
	env, calls := fakeEnv(t, false)
	writeRegistry(t, env, "installed_plugins.json", `{"plugins": {"have@m": []}}`)
	bundle := protocol.QuickstartBundle{
		ClaudeCode:   protocol.QuickstartClaudeCode{NPMPackage: "@anthropic-ai/claude-code"},
		Marketplaces: []protocol.QuickstartMarketplace{{Name: "m", Source: "o/r"}},
		Plugins:      []string{"have@m", "new@m"},
	}
	steps := Run(context.Background(), env, bundle, noSkills(t))

	want := [][]string{
		{"npm", "install", "-g", "@anthropic-ai/claude-code"},
		{"claude", "plugin", "marketplace", "add", "o/r"},
		{"claude", "plugin", "install", "new@m", "--scope", "user"},
	}
	if !slices.EqualFunc(*calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
	if got := statusOf(steps, "plugin have@m"); got != protocol.QuickstartStepAlreadyInstalled {
		t.Fatalf("have@m status = %q", got)
	}
	if got := statusOf(steps, "plugin new@m"); got != protocol.QuickstartStepInstalled {
		t.Fatalf("new@m status = %q", got)
	}
}

func TestRunFailsPluginsWhenClaudeCannotBeInstalled(t *testing.T) {
	env := Env{
		ConfigDir: t.TempDir(),
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("no command should run without npm or claude")
			return nil, nil
		},
	}
	bundle := protocol.QuickstartBundle{Plugins: []string{"x@m"}}
	steps := Run(context.Background(), env, bundle, noSkills(t))

	if got := statusOf(steps, "claude code"); got != protocol.QuickstartStepFailed {
		t.Fatalf("claude code status = %q, want failed", got)
	}
	if got := statusOf(steps, "plugin x@m"); got != protocol.QuickstartStepFailed {
		t.Fatalf("plugin status = %q, want failed", got)
	}
}

func TestRunWritesMissingSkillsAndReportsNotFound(t *testing.T) {
	env, _ := fakeEnv(t, true)
	source := func(_ context.Context, names []string) (protocol.QuickstartSkillBundles, error) {
		if !slices.Equal(names, []string{"demo", "absent"}) {
			t.Fatalf("source asked for %v", names)
		}
		return protocol.QuickstartSkillBundles{
			Skills: []protocol.QuickstartSkill{{
				Name:    "demo",
				Content: "# demo",
				Files:   []protocol.QuickstartSkillFile{{Path: "references/notes.md", Content: "notes"}},
			}},
			Missing: []string{"absent"},
		}, nil
	}
	steps := Run(context.Background(), env, protocol.QuickstartBundle{Skills: []string{"demo", "absent"}}, source)

	if got := statusOf(steps, "skill demo"); got != protocol.QuickstartStepInstalled {
		t.Fatalf("demo status = %q", got)
	}
	if got := statusOf(steps, "skill absent"); got != protocol.QuickstartStepFailed {
		t.Fatalf("absent status = %q", got)
	}
	got, err := os.ReadFile(filepath.Join(env.ConfigDir, "skills", "demo", "references", "notes.md"))
	if err != nil || string(got) != "notes" {
		t.Fatalf("supporting file = %q, err = %v", got, err)
	}
}

func TestWriteSkillRejectsEscapingPaths(t *testing.T) {
	root := t.TempDir()
	err := writeSkill(root, protocol.QuickstartSkill{
		Name:    "skill",
		Content: "# x",
		Files:   []protocol.QuickstartSkillFile{{Path: "../escape.md", Content: "bad"}},
	})
	if err == nil {
		t.Fatal("expected error for a path that escapes the skill directory")
	}
	if _, statErr := os.Stat(filepath.Join(root, "escape.md")); statErr == nil {
		t.Fatal("escaping file was written")
	}
}

func TestWriteSkillRejectsNamesWithSeparators(t *testing.T) {
	for _, name := range []string{"", "..", "a/b", "../x"} {
		if err := writeSkill(t.TempDir(), protocol.QuickstartSkill{Name: name, Content: "x"}); err == nil {
			t.Fatalf("name %q was accepted", name)
		}
	}
}
