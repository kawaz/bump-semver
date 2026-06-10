package main

import (
	"strings"
	"testing"
)

// --- DR-0029 Phase A: parser tests ---------------------------------------
//
// These tests exercise only the parsing layer (ruleBlocks construction +
// 0a 補強 + 0c dup detection + --format enum validation). Subsequent
// phases add tier scoring, rule resolution, extraction, and write
// atomicity — those tests live alongside their respective implementation
// files.

func TestParse_DefineRule_GlobalOnly(t *testing.T) {
	t.Parallel()
	args, err := buildArgsForTest(t, []string{
		"get", "VERSION",
		"--format", "text",
		"--version-regex", `v(\d+\.\d+\.\d+)`,
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(args.ruleBlocks) != 1 {
		t.Fatalf("ruleBlocks: got %d blocks, want 1 (global only)", len(args.ruleBlocks))
	}
	g := args.ruleBlocks[0]
	if !g.isGlobal() {
		t.Errorf("block[0] should be global, got Pattern=%q", g.Pattern)
	}
	if g.Opts.Format == nil || *g.Opts.Format != "text" {
		t.Errorf("Format: got %v, want text", g.Opts.Format)
	}
	if g.Opts.VersionRegex == nil || *g.Opts.VersionRegex != `v(\d+\.\d+\.\d+)` {
		t.Errorf("VersionRegex: got %v, want v(\\d+\\.\\d+\\.\\d+)", g.Opts.VersionRegex)
	}
	if args.hasDefineRule {
		t.Errorf("hasDefineRule: true, want false (no --define-rule)")
	}
}

func TestParse_DefineRule_OneBlock(t *testing.T) {
	t.Parallel()
	args, err := buildArgsForTest(t, []string{
		"get", "a.json", "b.txt",
		"--define-rule", "b.txt",
		"--format", "text",
		"--version-regex", `version=(.+)`,
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(args.ruleBlocks) != 2 {
		t.Fatalf("ruleBlocks: got %d blocks, want 2 (global + b.txt)", len(args.ruleBlocks))
	}
	g := args.ruleBlocks[0]
	if !g.isGlobal() {
		t.Errorf("block[0] should be global, got Pattern=%q", g.Pattern)
	}
	if g.Opts.hasAny() {
		t.Errorf("block[0] (global) should be empty, got %+v", g.Opts)
	}
	b := args.ruleBlocks[1]
	if b.Pattern != "b.txt" {
		t.Errorf("block[1] Pattern: got %q, want b.txt", b.Pattern)
	}
	if b.Opts.Format == nil || *b.Opts.Format != "text" {
		t.Errorf("block[1].Format: got %v, want text", b.Opts.Format)
	}
	if !args.hasDefineRule {
		t.Errorf("hasDefineRule: false, want true")
	}
}

func TestParse_DefineRule_TwoBlocks(t *testing.T) {
	t.Parallel()
	args, err := buildArgsForTest(t, []string{
		"get", "a.txt", "b.json",
		"--define-rule", "a.txt",
		"--format", "text",
		"--version-regex", "v(.+)",
		"--define-rule", "b.json",
		"--format", "json",
		"--version-path", "$.version",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(args.ruleBlocks) != 3 {
		t.Fatalf("ruleBlocks: got %d, want 3 (global + a.txt + b.json)", len(args.ruleBlocks))
	}
	if args.ruleBlocks[1].Pattern != "a.txt" || *args.ruleBlocks[1].Opts.Format != "text" {
		t.Errorf("block[1]: got %+v", args.ruleBlocks[1])
	}
	if args.ruleBlocks[2].Pattern != "b.json" || *args.ruleBlocks[2].Opts.Format != "json" {
		t.Errorf("block[2]: got %+v", args.ruleBlocks[2])
	}
}

func TestParse_DefineRule_GlobalThenBlock(t *testing.T) {
	t.Parallel()
	// Global --format json sets default; --define-rule X then overrides
	// with text. Valid: global flags BEFORE the first --define-rule.
	args, err := buildArgsForTest(t, []string{
		"get", "a.json", "b.txt",
		"--format", "json",
		"--version-path", "$.version",
		"--define-rule", "b.txt",
		"--format", "text",
		"--version-regex", "v(.+)",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(args.ruleBlocks) != 2 {
		t.Fatalf("ruleBlocks: got %d, want 2", len(args.ruleBlocks))
	}
	g := args.ruleBlocks[0]
	if *g.Opts.Format != "json" || *g.Opts.VersionPath != "$.version" {
		t.Errorf("global: got %+v", g.Opts)
	}
	b := args.ruleBlocks[1]
	if *b.Opts.Format != "text" || *b.Opts.VersionRegex != "v(.+)" {
		t.Errorf("block[1]: got %+v", b.Opts)
	}
}

func TestParse_DefineRule_DupFlagInBlock(t *testing.T) {
	t.Parallel()
	// --format twice in the same block is error (DR-0029 0c).
	_, err := buildArgsForTest(t, []string{
		"get", "a.txt",
		"--define-rule", "a.txt",
		"--format", "text",
		"--format", "json",
		"--version-regex", "v(.+)",
	})
	if err == nil {
		t.Fatalf("parse should have errored on duplicate --format")
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Errorf("error %q should mention twice", err)
	}
	if !strings.Contains(err.Error(), `"a.txt"`) {
		t.Errorf("error %q should mention block PATTERN a.txt", err)
	}
}

func TestParse_DefineRule_DupFlagInGlobal(t *testing.T) {
	t.Parallel()
	_, err := buildArgsForTest(t, []string{
		"get", "a.txt",
		"--format", "text",
		"--format", "json",
	})
	if err == nil {
		t.Fatalf("parse should have errored on duplicate --format in global")
	}
	if !strings.Contains(err.Error(), "twice") || !strings.Contains(err.Error(), "global") {
		t.Errorf("error %q should mention twice + global", err)
	}
}

func TestParse_DefineRule_XMLAccepted(t *testing.T) {
	t.Parallel()
	// --format xml is accepted (dot-path unified, DR-0029 updated).
	args, err := buildArgsForTest(t, []string{
		"get", "a.xml",
		"--define-rule", "a.xml",
		"--format", "xml",
		"--version-path", "$.project.version",
	})
	if err != nil {
		t.Fatalf("--format xml should be accepted: %v", err)
	}
	if len(args.ruleBlocks) != 2 || *args.ruleBlocks[1].Opts.Format != "xml" {
		t.Errorf("ruleBlocks: %+v", args.ruleBlocks)
	}
}

func TestParse_DefineRule_InvalidFormatRandom(t *testing.T) {
	t.Parallel()
	_, err := buildArgsForTest(t, []string{
		"get", "a.txt",
		"--format", "ini",
	})
	if err == nil {
		t.Fatalf("parse should have errored on --format ini")
	}
	if !strings.Contains(err.Error(), "valid format") {
		t.Errorf("error %q should mention valid format", err)
	}
}

func TestParse_DefineRule_EmptyPattern(t *testing.T) {
	t.Parallel()
	_, err := buildArgsForTest(t, []string{
		"get", "a.txt", "--define-rule", "",
	})
	if err == nil {
		t.Fatalf("parse should have errored on empty --define-rule PATTERN")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error %q should mention cannot be empty", err)
	}
}

func TestParse_DefineRule_EmptyFlagValue(t *testing.T) {
	t.Parallel()
	_, err := buildArgsForTest(t, []string{
		"get", "a.txt",
		"--define-rule", "a.txt",
		"--format", "",
	})
	if err == nil {
		t.Fatalf("parse should have errored on empty --format value")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error %q should mention cannot be empty", err)
	}
}

func TestParse_DefineRule_EqualsForm(t *testing.T) {
	t.Parallel()
	// --define-rule=PAT / --format=text / --version-path=$.v should all
	// behave identically to the space form.
	args, err := buildArgsForTest(t, []string{
		"get", "a.json",
		"--define-rule=a.json",
		"--format=json",
		"--version-path=$.version",
		"--name-path=$.name",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(args.ruleBlocks) != 2 {
		t.Fatalf("ruleBlocks: got %d, want 2", len(args.ruleBlocks))
	}
	b := args.ruleBlocks[1]
	if b.Pattern != "a.json" {
		t.Errorf("Pattern: got %q, want a.json", b.Pattern)
	}
	if *b.Opts.Format != "json" || *b.Opts.VersionPath != "$.version" || *b.Opts.NamePath != "$.name" {
		t.Errorf("block flags: got %+v", b.Opts)
	}
}

func TestParse_DefineRule_NameFlags(t *testing.T) {
	t.Parallel()
	args, err := buildArgsForTest(t, []string{
		"get", "a.json",
		"--define-rule", "a.json",
		"--format", "json",
		"--version-path", "$.version",
		"--name-path", "$.name",
		"--name-regex", "([a-z]+)",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	b := args.ruleBlocks[1]
	if b.Opts.NamePath == nil || *b.Opts.NamePath != "$.name" {
		t.Errorf("NamePath: got %v", b.Opts.NamePath)
	}
	if b.Opts.NameRegex == nil || *b.Opts.NameRegex != "([a-z]+)" {
		t.Errorf("NameRegex: got %v", b.Opts.NameRegex)
	}
}

func TestParse_DefineRule_AllFormatsAccepted(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"text", "json", "yaml", "toml"} {
		t.Run(f, func(t *testing.T) {
			t.Parallel()
			extra := []string{"--version-path", "$.v"}
			if f == "text" {
				extra = []string{"--version-regex", "v(.+)"}
			}
			argv := append([]string{
				"get", "a." + f,
				"--define-rule", "a." + f,
				"--format", f,
			}, extra...)
			_, err := buildArgsForTest(t, argv)
			if err != nil {
				t.Errorf("--format %s rejected: %v", f, err)
			}
		})
	}
}
