package hub

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Severity of a finding. Only "block" changes the scan status to blocked.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityBlock Severity = "block"
)

// Scan status values.
const (
	ScanClean   = "clean"
	ScanWarn    = "warn"
	ScanBlocked = "blocked"
)

// Finding is one rule hit. Message never contains the matched text; secrets are
// reported by kind and line only.
type Finding struct {
	Rule     string
	Severity Severity
	Path     string
	Message  string
}

// ScanResult is the outcome of scanning a bundle. Status is ScanBlocked if any
// block finding exists, ScanWarn if only warnings exist, and ScanClean otherwise.
type ScanResult struct {
	Status   string
	Findings []Finding
}

// largeFileWarnBytes is the size above which a single file is flagged. Files
// over DefaultLimits.MaxFileBytes are blocked outright.
const largeFileWarnBytes = 256 << 10

// binarySniffBytes is how far into a file the NUL-byte test looks.
const binarySniffBytes = 8000

type textRule struct {
	rule    string
	desc    string
	pattern *regexp.Regexp
}

// secretRules report credential-shaped text. The matched value is never copied
// into a finding.
var secretRules = []textRule{
	{"secret-aws-key", "AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"secret-private-key", "private key block", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY(?: BLOCK)?-----`)},
	{"secret-github-token", "GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"secret-slack-token", "Slack token", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`)},
	{"secret-bearer-token", "bearer token", regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{20,}`)},
	{"secret-credential-assignment", "credential assignment", regexp.MustCompile(`(?i)\b(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token|password|passwd)\s*[:=]\s*["']?[A-Za-z0-9_+/=.-]{16,}`)},
}

var behaviorRules = []textRule{
	{"remote-pipe-to-shell", "remote content piped to a shell",
		regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba|z|da|k)?sh\b|\b(?:ba|z|da|k)?sh\s+(?:-c\s+)?["']?\$\(\s*(?:curl|wget)\b|\b(?:ba|z|da|k)?sh\s+<\(\s*(?:curl|wget)\b`)},
	{"base64-exec", "base64-decoded content executed",
		regexp.MustCompile(`(?i)base64\s+(?:-d|--decode|-D)\b[^\n]*\|\s*(?:sudo\s+)?(?:ba|z|da|k)?sh\b|\b(?:exec|eval)\s*\(\s*(?:base64\.)?b64decode\s*\(|\beval\s*\(\s*atob\s*\(`)},
	{"remote-eval", "remote content evaluated",
		regexp.MustCompile(`(?i)\beval\s*\(?\s*["']?\$\(\s*(?:curl|wget)\b|\b(?:eval|exec)\s*\(\s*(?:requests\.get|urllib\.request\.urlopen|fetch|axios\.get|http\.get)\s*\(`)},
}

var networkURLPattern = regexp.MustCompile(`(?i)\b(?:https?|wss?)://[^\s'"<>)\]]+`)

// Scan applies the import rules to a bundle and returns findings in path order.
//
// Blocking: unsafe paths, executable binaries (ELF, PE, Mach-O by magic bytes),
// files and totals over DefaultLimits, too many files.
// Warning: credential-shaped text, .env files, remote content piped or evaluated,
// base64-decoded code execution, network URLs in scripts, shell scripts outside a
// scripts/ directory, and very large files.
//
// Text rules skip files that look binary. A FileEntry carries no mode bits, so
// "executable" is judged from the shebang and extension, not from the zip mode.
func Scan(b *Bundle) ScanResult {
	var findings []Finding
	add := func(rule string, sev Severity, p, msg string) {
		findings = append(findings, Finding{Rule: rule, Severity: sev, Path: p, Message: msg})
	}

	files := append([]FileEntry(nil), b.Files...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	if len(files) > DefaultLimits.MaxFiles {
		add("limit-file-count", SeverityBlock, "", fmt.Sprintf("bundle has %d files, limit %d", len(files), DefaultLimits.MaxFiles))
	}
	var total int64
	for _, f := range files {
		total += int64(len(f.Content))
	}
	if total > DefaultLimits.MaxTotalBytes {
		add("limit-total-size", SeverityBlock, "", fmt.Sprintf("bundle is %d bytes, limit %d", total, DefaultLimits.MaxTotalBytes))
	}

	for _, f := range files {
		scanFile(f, add)
	}

	status := ScanClean
	for _, f := range findings {
		if f.Severity == SeverityBlock {
			status = ScanBlocked
			break
		}
		if f.Severity == SeverityWarn {
			status = ScanWarn
		}
	}
	return ScanResult{Status: status, Findings: findings}
}

func scanFile(f FileEntry, add func(rule string, sev Severity, p, msg string)) {
	if _, err := normalizeEntryName(f.Path); err != nil {
		add("unsafe-path", SeverityBlock, f.Path, "file path is not allowed")
		return
	}
	size := int64(len(f.Content))
	if size > DefaultLimits.MaxFileBytes {
		add("limit-file-size", SeverityBlock, f.Path, fmt.Sprintf("file is %d bytes, limit %d", size, DefaultLimits.MaxFileBytes))
		return
	}
	if kind := executableKind(f.Content); kind != "" {
		add("executable-binary", SeverityBlock, f.Path, kind+" executable is not allowed")
		return
	}
	if size > largeFileWarnBytes {
		add("large-file", SeverityWarn, f.Path, fmt.Sprintf("file is %d bytes", size))
	}
	if isBinary(f.Content) {
		return
	}

	base := path.Base(f.Path)
	if isEnvFile(base) {
		add("env-file", SeverityWarn, f.Path, "environment file may contain secrets")
	}
	for _, r := range secretRules {
		reportMatches(r, f, add, SeverityWarn)
	}
	for _, r := range behaviorRules {
		reportMatches(r, f, add, SeverityWarn)
	}
	if isScript(f.Path, f.Content) {
		if m := networkURLPattern.FindIndex(f.Content); m != nil {
			add("script-network-host", SeverityWarn, f.Path,
				fmt.Sprintf("script references a network URL at line %d", lineOf(f.Content, m[0])))
		}
		if isShellScript(f.Path, f.Content) && !inScriptsDir(f.Path) {
			add("shell-outside-scripts", SeverityWarn, f.Path, "shell script outside a scripts/ directory")
		}
	}
}

// reportMatches adds one finding per match. The message gives the rule's
// description and the line only; the matched text is not copied.
func reportMatches(r textRule, f FileEntry, add func(rule string, sev Severity, p, msg string), sev Severity) {
	for _, m := range r.pattern.FindAllIndex(f.Content, -1) {
		add(r.rule, sev, f.Path, fmt.Sprintf("possible %s at line %d", r.desc, lineOf(f.Content, m[0])))
	}
}

func lineOf(content []byte, offset int) int {
	return bytes.Count(content[:offset], []byte{'\n'}) + 1
}

// executableKind names the binary format from its magic bytes, or returns "".
func executableKind(c []byte) string {
	switch {
	case bytes.HasPrefix(c, []byte{0x7f, 'E', 'L', 'F'}):
		return "ELF"
	case isPE(c):
		return "PE"
	case bytes.HasPrefix(c, []byte{0xfe, 0xed, 0xfa, 0xce}), bytes.HasPrefix(c, []byte{0xfe, 0xed, 0xfa, 0xcf}),
		bytes.HasPrefix(c, []byte{0xce, 0xfa, 0xed, 0xfe}), bytes.HasPrefix(c, []byte{0xcf, 0xfa, 0xed, 0xfe}):
		return "Mach-O"
	case len(c) >= 8 && bytes.HasPrefix(c, []byte{0xca, 0xfe, 0xba, 0xbe}) && isMachOFat(c):
		// 0xCAFEBABE is also the Java class magic. A Mach-O fat header has a small
		// architecture count where a class file has its version number.
		return "Mach-O universal"
	}
	return ""
}

// isPE checks for "MZ" plus a PE signature at the offset in e_lfanew, so text
// that merely starts with "MZ" is not blocked.
func isPE(c []byte) bool {
	if len(c) < 64 || c[0] != 'M' || c[1] != 'Z' {
		return false
	}
	off := int64(binary.LittleEndian.Uint32(c[0x3c:0x40]))
	return off >= 0 && off+4 <= int64(len(c)) && bytes.Equal(c[off:off+4], []byte{'P', 'E', 0, 0})
}

func isMachOFat(c []byte) bool {
	n := binary.BigEndian.Uint32(c[4:8])
	return n >= 1 && n < 20
}

func isBinary(c []byte) bool {
	n := len(c)
	if n > binarySniffBytes {
		n = binarySniffBytes
	}
	return bytes.IndexByte(c[:n], 0) >= 0
}

func isEnvFile(base string) bool {
	lower := strings.ToLower(base)
	if lower != ".env" && !strings.HasPrefix(lower, ".env.") {
		return false
	}
	for _, safe := range []string{".env.example", ".env.sample", ".env.template", ".env.dist"} {
		if lower == safe {
			return false
		}
	}
	return true
}

var scriptExtensions = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".py": true, ".rb": true,
	".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".ps1": true, ".pl": true,
}

func isScript(p string, c []byte) bool {
	return scriptExtensions[strings.ToLower(path.Ext(p))] || hasShebang(c)
}

func isShellScript(p string, c []byte) bool {
	ext := strings.ToLower(path.Ext(p))
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return true
	}
	if !hasShebang(c) {
		return false
	}
	line := string(c[:min(len(c), 200)])
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return false
	}
	interpreter := path.Base(fields[0])
	if interpreter == "env" && len(fields) > 1 {
		interpreter = path.Base(fields[1])
	}
	switch interpreter {
	case "sh", "bash", "zsh", "dash", "ksh":
		return true
	}
	return false
}

func hasShebang(c []byte) bool {
	return bytes.HasPrefix(c, []byte("#!"))
}

func inScriptsDir(p string) bool {
	dir := path.Dir(p)
	for _, seg := range strings.Split(dir, "/") {
		if seg == "scripts" {
			return true
		}
	}
	return false
}
