package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DR-0029 Phase C: E2E (--define-rule wired into dispatcher) -----

func TestDefineRule_TextRegexGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "my.txt")
	if err := os.WriteFile(path, []byte("version: 1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	args, err := buildArgsForTest(t, []string{
		"get", path,
		"--define-rule", path,
		"--format", "text",
		"--version-regex", `version: (\d+\.\d+\.\d+)`,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(args.ruleBlocks) != 2 {
		t.Fatalf("ruleBlocks count: %d, want 2", len(args.ruleBlocks))
	}
	insp, err := inspectViaCliRule(t, path, args.ruleBlocks)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want one 1.2.3", insp.Versions)
	}
}

func TestDefineRule_GlobalOverridesBuiltin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	// builtin would pick $.version (= "0.1.0"). With CLI rule we pick
	// $.metadata.appVersion instead.
	body := `{"version": "0.1.0", "metadata": {"appVersion": "9.9.9"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	args, err := buildArgsForTest(t, []string{
		"get", path,
		"--format", "json",
		"--version-path", "$.metadata.appVersion",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	insp, err := inspectViaCliRule(t, path, args.ruleBlocks)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "9.9.9" {
		t.Errorf("Versions = %+v, want one 9.9.9 (CLI rule should override builtin)", insp.Versions)
	}
}

func TestDefineRule_TextExactlyOneMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "many.txt")
	body := "v1.0.0\nv1.0.0\nv1.0.0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	args, err := buildArgsForTest(t, []string{
		"get", path,
		"--define-rule", path,
		"--format", "text",
		"--version-regex", `v(\d+\.\d+\.\d+)`,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = inspectViaCliRule(t, path, args.ruleBlocks)
	if err == nil {
		t.Fatalf("expected error: --version-regex matched 3 times")
	}
	if !strings.Contains(err.Error(), "exactly one match") {
		t.Errorf("error %q should mention exactly one match", err)
	}
}

func TestDefineRule_TextRequiresVersionRegex(t *testing.T) {
	t.Parallel()
	block := ruleBlock{
		Pattern: "my.txt",
		Opts: ruleOpts{
			Format: strPtr("text"),
		},
	}
	err := validateRuleBlock(block)
	if err == nil {
		t.Fatalf("expected error for --format text without --version-regex")
	}
	if !strings.Contains(err.Error(), "--version-regex") {
		t.Errorf("error %q should mention --version-regex", err)
	}
}

func TestDefineRule_JsonRequiresPath(t *testing.T) {
	t.Parallel()
	block := ruleBlock{
		Pattern: "a.json",
		Opts: ruleOpts{
			Format: strPtr("json"),
		},
	}
	err := validateRuleBlock(block)
	if err == nil {
		t.Fatalf("expected error for --format json with no --version-path")
	}
	if !strings.Contains(err.Error(), "--version-path") {
		t.Errorf("error %q should mention --version-path", err)
	}
}

func TestDefineRule_StructuredRegexOnlyRejected(t *testing.T) {
	t.Parallel()
	// --format json --version-regex (no --version-path) → steer to text.
	block := ruleBlock{
		Pattern: "a.json",
		Opts: ruleOpts{
			Format:       strPtr("json"),
			VersionRegex: strPtr(`v(\d+\.\d+\.\d+)`),
		},
	}
	err := validateRuleBlock(block)
	if err == nil {
		t.Fatalf("expected error for --format json --version-regex without --version-path")
	}
	if !strings.Contains(err.Error(), "--format text") {
		t.Errorf("error %q should steer to --format text", err)
	}
}

func TestDefineRule_DeadBlockErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// --define-rule "ghost.json" matches nothing.
	args, err := buildArgsForTest(t, []string{
		"get", path,
		"--define-rule", "ghost.json",
		"--format", "json",
		"--version-path", "$.version",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolveInputs(args.inputs, nil, resolveInputsOpts{
		Write: false, VCSKind: vcsAuto, PeerExpand: true,
		RuleBlocks: args.ruleBlocks,
	})
	if err == nil {
		t.Fatalf("expected dead block error")
	}
	if !strings.Contains(err.Error(), "dead --define-rule") {
		t.Errorf("error %q should mention dead --define-rule", err)
	}
}

func TestDefineRule_StdinPipe_ExtensionWithoutBuiltin(t *testing.T) {
	t.Parallel()
	// `.env` has no builtin rule, but the user supplies one via
	// --define-rule. resolveFilePipeOrDisk must honour the CLI block
	// instead of rejecting "unsupported file". Regression guard for codex
	// review-gate finding: an earlier stdin path dropped ruleBlocks, so
	// --define-rule was silently ignored on the single-FILE + pipe
	// shortcut.
	args, err := buildArgsForTest(t, []string{
		"get", "myapp.env",
		"--define-rule", "myapp.env",
		"--format", "text",
		"--version-regex", `VERSION=v(\d+\.\d+\.\d+)`,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	stdin := strings.NewReader("VERSION=v3.2.1\n")
	ri, fellThrough, err := resolveFilePipeOrDisk("myapp.env", stdin, args.ruleBlocks)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fellThrough {
		t.Fatalf("fellThrough = true, want false (non-empty pipe must win)")
	}
	if len(ri.fields) != 1 || ri.fields[0].Value != "3.2.1" {
		t.Errorf("Versions = %+v, want 3.2.1", ri.fields)
	}
	// And cliRuleCoversFile (= gate widener) must say "yes" for the
	// same path so the stdin-pipe shortcut admits it.
	if !cliRuleCoversFile("myapp.env", args.ruleBlocks) {
		t.Errorf("cliRuleCoversFile = false, want true for myapp.env with matching block")
	}
	// Negative: a path with no matching block and no builtin rule
	// stays gate-rejected.
	if cliRuleCoversFile("unrelated.env", args.ruleBlocks) {
		t.Errorf("cliRuleCoversFile(unrelated.env) = true, want false (no block matches)")
	}
}

// inspectViaCliRule is a tiny helper that wires the same Handler-
// picking path that resolveFileWithRules uses, for unit-test purposes.
// We don't go through the full resolveInputs (= no stdin / VCS) so the
// tests stay self-contained.
func inspectViaCliRule(t *testing.T, path string, blocks []ruleBlock) (Inspection, error) {
	t.Helper()
	h, err := pickHandlerForFile(path, blocks)
	if err != nil {
		return Inspection{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Inspection{}, err
	}
	return h.Inspect(content)
}
