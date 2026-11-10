package hub

import (
	"errors"
	"testing"
)

const testPluginManifest = `{
  "manifest_version": 1,
  "key": "com.example.hello",
  "name": "Hello Panel",
  "description": "A greeting panel.",
  "version": "1.2.0",
  "author": { "name": "example", "url": "https://example.com" },
  "scopes": ["issues:read"],
  "contributes": {
    "surfaces": [{
      "key": "hello", "type": "issue_panel", "name": "Hello",
      "entry": "ui/main.js", "platforms": ["web"]
    }]
  }
}`

func TestDetect(t *testing.T) {
	const skillMD = "---\nname: deploy-helper\ndescription: Ships the app safely.\n---\nBody.\n"
	cases := []struct {
		name    string
		files   map[string]string
		want    Detected
		wantErr error
	}{
		{
			name:  "skill at root",
			files: map[string]string{"SKILL.md": skillMD, "scripts/a.sh": "echo"},
			want:  Detected{Kind: KindSkill, Name: "deploy-helper", Description: "Ships the app safely.", SkillMDPath: "SKILL.md"},
		},
		{
			name:  "skill SKILL.md matched case-insensitively",
			files: map[string]string{"skill.md": skillMD},
			want:  Detected{Kind: KindSkill, Name: "deploy-helper", Description: "Ships the app safely.", SkillMDPath: "skill.md"},
		},
		{
			name:  "skill one directory down",
			files: map[string]string{"deploy/SKILL.md": skillMD, "deploy/notes.md": "x"},
			want:  Detected{Kind: KindSkill, Name: "deploy-helper", Description: "Ships the app safely.", SkillMDPath: "deploy/SKILL.md"},
		},
		{
			name:  "skill without frontmatter name falls back to directory name",
			files: map[string]string{"pr-review/SKILL.md": "no frontmatter here"},
			want:  Detected{Kind: KindSkill, Name: "pr-review", SkillMDPath: "pr-review/SKILL.md"},
		},
		{
			name:    "root skill without frontmatter name is refused",
			files:   map[string]string{"SKILL.md": "no frontmatter"},
			wantErr: ErrInvalidSkill,
		},
		{
			name:    "two skills at the same nested level are ambiguous",
			files:   map[string]string{"a/SKILL.md": skillMD, "b/SKILL.md": skillMD},
			wantErr: ErrInvalidArchive,
		},
		{
			name:  "plugin at root",
			files: map[string]string{"multica.plugin.json": testPluginManifest, "ui/main.js": "x"},
			want:  Detected{Kind: KindPlugin, Name: "com.example.hello", Description: "A greeting panel.", Version: "1.2.0"},
		},
		{
			name:  "plugin one directory down",
			files: map[string]string{"hello/multica.plugin.json": testPluginManifest},
			want:  Detected{Kind: KindPlugin, Name: "com.example.hello", Description: "A greeting panel.", Version: "1.2.0"},
		},
		{
			name:    "plugin manifest that fails strict parsing",
			files:   map[string]string{"multica.plugin.json": `{"manifest_version":1,"surprise":true}`},
			wantErr: ErrInvalidManifest,
		},
		{
			name:    "neither skill nor plugin",
			files:   map[string]string{"README.md": "hi", "src/main.go": "package main"},
			wantErr: ErrNotDetected,
		},
		{
			name:    "empty bundle",
			files:   map[string]string{},
			wantErr: ErrNotDetected,
		},
		{
			name:    "both skill and plugin",
			files:   map[string]string{"SKILL.md": skillMD, "multica.plugin.json": testPluginManifest},
			wantErr: ErrAmbiguousKind,
		},
		{
			name:    "SKILL.md nested deeper than one level is not a skill",
			files:   map[string]string{"a/b/SKILL.md": skillMD},
			wantErr: ErrNotDetected,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Detect(bundleOf(tc.files))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Detect = %+v, want %+v", got, tc.want)
			}
		})
	}
}
