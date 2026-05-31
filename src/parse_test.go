package main

import (
	"reflect"
	"testing"
)

func TestParseArgs_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want cliArgs
	}{
		{"bump-file", []string{"patch", "Cargo.toml"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}}},
		{"bump-file-write", []string{"patch", "Cargo.toml", "--write"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}, write: true}},
		{"write-before-input", []string{"patch", "--write", "Cargo.toml"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}, write: true}},
		{"get-file", []string{"get", "VERSION"}, cliArgs{kind: "bump", action: "get", inputs: []string{"VERSION"}}},
		{"bump-ver", []string{"minor", "1.2.3"}, cliArgs{kind: "bump", action: "minor", inputs: []string{"1.2.3"}}},
		{"version-flag", []string{"--version"}, cliArgs{kind: "version"}},
		{"version-short", []string{"-V"}, cliArgs{kind: "version"}},
		{"version-json", []string{"--version", "--json"}, cliArgs{kind: "version", output: outputOpts{JSON: true}}},
		{"version-json-short", []string{"-V", "--json"}, cliArgs{kind: "version", output: outputOpts{JSON: true}}},
		{"help-flag", []string{"--help"}, cliArgs{kind: "help"}},
		{"help-short", []string{"-h"}, cliArgs{kind: "help"}},
		{"help-full", []string{"--help-full"}, cliArgs{kind: "helpFull"}},
		{"empty", []string{}, cliArgs{kind: "help"}},
		// v0.13.0 subcommand --help dispatch (DR-0017 周辺)
		{"action-help-major", []string{"major", "--help"}, cliArgs{kind: "helpAction", action: "major"}},
		{"action-help-minor", []string{"minor", "--help"}, cliArgs{kind: "helpAction", action: "minor"}},
		{"action-help-patch", []string{"patch", "--help"}, cliArgs{kind: "helpAction", action: "patch"}},
		{"action-help-patch-short", []string{"patch", "-h"}, cliArgs{kind: "helpAction", action: "patch"}},
		{"action-help-pre", []string{"pre", "--help"}, cliArgs{kind: "helpAction", action: "pre"}},
		{"action-help-get", []string{"get", "--help"}, cliArgs{kind: "helpAction", action: "get"}},
		{"action-help-compare-no-op", []string{"compare", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		{"action-help-compare-op-then-help", []string{"compare", "eq", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		{"action-help-compare-precision-then-help", []string{"compare", "eq-major", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		// --vcs auto (DR-0016) happy path
		{"vcs-flag-auto", []string{"patch", "1.2.3", "--vcs", "auto"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcsBase: vcsBaseOpts{Override: ptr("auto")}}},
		{"dash-dash-passthrough", []string{"patch", "--", "--weird-file.json"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"--weird-file.json"}}},
		{"multi-file", []string{"get", "package.json", "package-lock.json"}, cliArgs{kind: "bump", action: "get", inputs: []string{"package.json", "package-lock.json"}}},
		{"multi-file-write", []string{"patch", "a.json", "b.json", "c.json", "--write"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"a.json", "b.json", "c.json"}, write: true}},
		// pre action with cross-cutting flags
		{"pre-with-pre", []string{"pre", "1.2.3", "--pre", "rc.0"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3"}, bump: bumpOpts{Pre: ptr("rc.0")}}},
		{"pre-with-pre-eq", []string{"pre", "1.2.3", "--pre=rc.0"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3"}, bump: bumpOpts{Pre: ptr("rc.0")}}},
		{"pre-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3-rc.0"}, bump: bumpOpts{NoPre: true}}},
		{"patch-build-meta", []string{"patch", "1.2.3", "--build-metadata", "sha.abc"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, bump: bumpOpts{BuildMetadata: ptr("sha.abc")}}},
		{"patch-build-meta-eq", []string{"patch", "1.2.3", "--build-metadata=sha.abc"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, bump: bumpOpts{BuildMetadata: ptr("sha.abc")}}},
		{"patch-no-build-meta", []string{"patch", "1.2.3+x", "--no-build-metadata"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3+x"}, bump: bumpOpts{NoBuildMetadata: true}}},
		// stdin marker
		{"stdin-marker", []string{"patch", "-"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"-"}}},
		// compare
		{"compare-eq", []string{"compare", "eq", "1.2.3", "1.2.3"}, cliArgs{kind: "compare", compareOp: "eq", inputs: []string{"1.2.3", "1.2.3"}}},
		{"compare-lt-files", []string{"compare", "lt", "a.json", "b.json"}, cliArgs{kind: "compare", compareOp: "lt", inputs: []string{"a.json", "b.json"}}},
		{"compare-ge-stdin", []string{"compare", "ge", "1.2.3", "-"}, cliArgs{kind: "compare", compareOp: "ge", inputs: []string{"1.2.3", "-"}}},
		// DR-0017: compare precision suffix split into base + precision
		{"compare-eq-major", []string{"compare", "eq-major", "1.2.3", "1.9.7"}, cliArgs{kind: "compare", compareOp: "eq", comparePrecision: "major", inputs: []string{"1.2.3", "1.9.7"}}},
		{"compare-lt-minor", []string{"compare", "lt-minor", "1.2.9", "1.3.0"}, cliArgs{kind: "compare", compareOp: "lt", comparePrecision: "minor", inputs: []string{"1.2.9", "1.3.0"}}},
		{"compare-ge-patch", []string{"compare", "ge-patch", "1.2.3", "1.2.3-rc.0"}, cliArgs{kind: "compare", compareOp: "ge", comparePrecision: "patch", inputs: []string{"1.2.3", "1.2.3-rc.0"}}},
		// DR-0008: vcs flag and vcs: inputs survive parseArgs intact
		{"vcs-flag-jj", []string{"patch", "1.2.3", "--vcs", "jj"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcsBase: vcsBaseOpts{Override: ptr("jj")}}},
		{"vcs-flag-git-eq", []string{"patch", "1.2.3", "--vcs=git"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcsBase: vcsBaseOpts{Override: ptr("git")}}},
		{"vcs-input-bump", []string{"patch", "vcs:HEAD"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"vcs:HEAD"}}},
		{"vcs-input-compare", []string{"compare", "gt", "Cargo.toml", "vcs:latest-tag()"}, cliArgs{kind: "compare", compareOp: "gt", inputs: []string{"Cargo.toml", "vcs:latest-tag()"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(tc.argv)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", tc.argv, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseArgs(%v)\n  got = %+v\n  want= %+v", tc.argv, got, tc.want)
			}
		})
	}
}

func TestParseArgs_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
	}{
		{"unknown-action", []string{"foo", "Cargo.toml"}},
		{"version-with-other-flag", []string{"--version", "--quiet"}},
		{"version-short-with-other-flag", []string{"-V", "--no-hint"}},
		{"version-with-positional", []string{"--version", "Cargo.toml"}},
		{"missing-input", []string{"patch"}},
		{"unknown-flag", []string{"patch", "Cargo.toml", "--unknown"}},
		{"double-write", []string{"patch", "Cargo.toml", "--write", "--write"}},
		{"get-with-write", []string{"get", "VERSION", "--write"}},
		{"get-with-pre", []string{"get", "VERSION", "--pre", "rc.0"}},
		{"get-with-build-metadata", []string{"get", "VERSION", "--build-metadata", "sha.x"}},
		{"compare-with-write", []string{"compare", "eq", "1.2.3", "1.2.3", "--write"}},
		{"compare-with-pre", []string{"compare", "eq", "1.2.3", "1.2.3", "--pre", "rc.0"}},
		{"compare-with-build-meta", []string{"compare", "eq", "1.2.3", "1.2.3", "--build-metadata", "sha"}},
		{"compare-too-few", []string{"compare", "eq", "1.2.3"}},
		// DR-0023: `compare OP F1 OTHERS...` accepts N>=1 OTHERS, so
		// `compare eq A B C` is no longer an arity error (it's the N=2
		// case). The legacy "too many" test row is intentionally
		// removed.
		{"compare-no-op", []string{"compare"}},
		{"compare-bad-op", []string{"compare", "neq", "1.2.3", "1.2.3"}},
		// DR-0017: precision suffix validation
		{"compare-bad-precision", []string{"compare", "eq-foo", "1.2.3", "1.2.3"}},
		{"compare-bad-base-with-precision", []string{"compare", "neq-major", "1.2.3", "1.2.3"}},
		{"compare-empty-precision", []string{"compare", "eq-", "1.2.3", "1.2.3"}},
		{"compare-double-precision", []string{"compare", "eq-major-minor", "1.2.3", "1.2.3"}},
		{"pre-and-no-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--no-pre"}},
		{"build-and-no-build", []string{"patch", "1.2.3", "--build-metadata", "x", "--no-build-metadata"}},
		{"empty-pre", []string{"pre", "1.2.3", "--pre", ""}},
		{"empty-build-metadata", []string{"patch", "1.2.3", "--build-metadata", ""}},
		{"pre-missing-arg", []string{"pre", "1.2.3", "--pre"}},
		{"build-missing-arg", []string{"patch", "1.2.3", "--build-metadata"}},
		{"double-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--pre", "rc.1"}},
		{"double-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre", "--no-pre"}},
		// DR-0008: --vcs validation
		{"vcs-bad-value", []string{"patch", "1.2.3", "--vcs", "hg"}},
		{"vcs-missing-arg", []string{"patch", "1.2.3", "--vcs"}},
		{"vcs-double", []string{"patch", "1.2.3", "--vcs", "git", "--vcs", "jj"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseArgs(tc.argv); err == nil {
				t.Errorf("parseArgs(%v) expected error, got nil", tc.argv)
			}
		})
	}
}

// parseArgs recognizes all the new quiet/no-hint flag spellings.
func TestParseArgs_QuietFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want cliArgs
	}{
		{
			"no-hint",
			[]string{"patch", "1.2.3", "--no-hint"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputNoHint}},
		},
		{
			"quiet-short",
			[]string{"patch", "1.2.3", "-q"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuiet}},
		},
		{
			"quiet-long",
			[]string{"patch", "1.2.3", "--quiet"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuiet}},
		},
		{
			"quiet-all-short",
			[]string{"patch", "1.2.3", "-qq"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuietAll}},
		},
		{
			"quiet-all-long",
			[]string{"patch", "1.2.3", "--quiet-all"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuietAll}},
		},
		{
			"compare-with-quiet",
			[]string{"compare", "eq", "1.2.3", "1.2.3", "-qq"},
			cliArgs{kind: "compare", compareOp: "eq", inputs: []string{"1.2.3", "1.2.3"}, output: outputOpts{Verbosity: outputQuietAll}},
		},
		{
			"get-with-quiet",
			[]string{"get", "VERSION", "-q"},
			cliArgs{kind: "bump", action: "get", inputs: []string{"VERSION"}, output: outputOpts{Verbosity: outputQuiet}},
		},
		{
			// `-q -qq` should collapse to the stronger -qq (max wins).
			// Also covers the descending case `-qq -q` via the raise()
			// helper — both orderings settle on outputQuietAll.
			"q-and-qq-coexist",
			[]string{"patch", "1.2.3", "-q", "-qq"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuietAll}},
		},
		{
			"qq-then-q-stays-at-qq",
			[]string{"patch", "1.2.3", "-qq", "-q"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuietAll}},
		},
		{
			"no-hint-and-quiet-coexist",
			[]string{"patch", "1.2.3", "--no-hint", "-q"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, output: outputOpts{Verbosity: outputQuiet}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(tc.argv)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", tc.argv, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseArgs(%v)\n  got = %+v\n  want= %+v", tc.argv, got, tc.want)
			}
		})
	}
}

// --- DR-0008: vcs: input mode ----------------------------------------------
//
// These tests exercise the CLI from end to end against a real git
// fixture. They cannot run with t.Parallel() because they chdir(2) the
// process. jj-flavoured CLI tests would need ssh-agent / signing
// disabled to run hermetically; we cover jj at the unit-test layer
// (vcs_test.go) and stick with git here for CLI round-tripping.
