package main

import (
	"bytes"
	"regexp"
	"testing"
)

// drRefRe matches a user-facing Decision-Record reference (e.g. DR-0020).
// The Stage 5 help finalisation removes these from every help output;
// only code comments may keep them.
var drRefRe = regexp.MustCompile(`DR-[0-9]`)

// japaneseRe matches any hiragana / katakana / CJK ideograph. Help output
// is English-only after Stage 5.
var japaneseRe = regexp.MustCompile(`[\x{3040}-\x{309F}\x{30A0}-\x{30FF}\x{4E00}-\x{9FFF}]`)

// helpArgvs enumerates every argv that prints a help screen to stdout
// (exit 0). Stage 5 finalises all of these to be English-only and free of
// DR numbers.
var helpArgvs = [][]string{
	{"--help"},
	{"-h"},
	{}, // no-arg short help
	{"--help-full"},
	{"major", "--help"},
	{"minor", "--help"},
	{"patch", "--help"},
	{"pre", "--help"},
	{"get", "--help"},
	{"compare", "--help"},
	{"compare", "eq", "--help"},
	{"vcs", "--help"},
	{"vcs", "get", "--help"},
	{"vcs", "is", "--help"},
	{"vcs", "diff", "--help"},
	{"vcs", "commit", "--help"},
	{"vcs", "fetch", "--help"},
	{"vcs", "push", "--help"},
	{"vcs", "tag", "--help"},
	{"vcs", "tag", "push", "--help"},
	{"vcs", "tag", "delete", "--help"},
	{"vcs", "outdated", "--help"},
	{"vcs", "get", "latest-tag", "--help"},
	{"vcs", "get", "latest-release", "--help"},
}

// TestRun_HelpOutputIsEnglishAndDRFree asserts the Stage 5 finalisation:
// every help screen is printed to stdout, exits 0, and contains neither a
// DR number nor any Japanese character.
func TestRun_HelpOutputIsEnglishAndDRFree(t *testing.T) {
	t.Parallel()
	for _, argv := range helpArgvs {
		argv := argv
		t.Run(joinArgv(argv), func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if err := run(argv, bytes.NewReader(nil), &stdout, &stderr); err != nil {
				t.Fatalf("run(%v) error: %v (stderr: %s)", argv, err, stderr.String())
			}
			out := stdout.String()
			if out == "" {
				t.Fatalf("run(%v) produced no stdout", argv)
			}
			if loc := drRefRe.FindString(out); loc != "" {
				t.Errorf("run(%v) help output contains a DR reference %q\ngot:\n%s", argv, loc, out)
			}
			if loc := japaneseRe.FindString(out); loc != "" {
				t.Errorf("run(%v) help output contains a Japanese character %q\ngot:\n%s", argv, loc, out)
			}
		})
	}
}

// TestRun_CompletionSubcommand asserts the cobra-provided completion
// subcommand is enabled (Stage 5) and emits a non-empty script for each
// supported shell, exiting 0.
func TestRun_CompletionSubcommand(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		shell := shell
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if err := run([]string{"completion", shell}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
				t.Fatalf("run(completion %s) error: %v (stderr: %s)", shell, err, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Errorf("run(completion %s) produced an empty completion script", shell)
			}
		})
	}
}

func joinArgv(argv []string) string {
	if len(argv) == 0 {
		return "(no-args)"
	}
	s := ""
	for i, a := range argv {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
