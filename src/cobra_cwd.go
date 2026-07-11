package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// This file implements the global -C/--cwd option (DR-0043): "run as if
// bump-semver had been started in PATH", mirroring git's -C.
//
// -C does NOT ride through cobra's normal flag parsing. It must work at
// *any* argv position (before the subcommand, mixed into its own flags,
// or after them), and the chdir has to land before anything else reads
// the filesystem (file argument resolution, vcs backend cwd probing,
// rule glob expansion). Both are satisfied by extracting the flag from
// argv and calling os.Chdir in the same pre-cobra-parse pass that
// already rewrites -qq (see runCobra / normalizeQuietAll in
// cobra_root.go) — extractCwdOption runs first, before that rewrite.
//
// A visible (non-hidden) persistent flag is still registered on the
// root command purely so `--help` / completion can describe -C/--cwd
// under "Global Options" for every subcommand (renderFlagBlock walks
// the live FlagSet). In the normal case its Value is never actually
// Set() by cobra: extractCwdOption has already removed the token from
// argv by the time root.Execute runs. It is a defence-in-depth guard
// (not a dummy sink) for the case where some -C/--cwd spelling slips
// past the pre-scan unrecognised — see cwdLeakGuardValue.

// cwdOnceErrName is the display name used in the "-C/--cwd specified
// twice" / "-C/--cwd requires a value" wording, matching the
// dual-spelling convention used elsewhere for aliased flags (e.g.
// newOnceString("--branch/--bookmark", ...) in cobra_vcs.go).
const cwdOnceErrName = "-C/--cwd"

// cwdLeakGuardValue backs the -C/--cwd persistent flag registered by
// registerCwdFlag. Its Set() always fails: reaching cobra at all means
// extractCwdOption failed to recognise the token's spelling and the
// chdir it names was never applied. A silent no-op here would be worse
// than an error — the process would exit 0 having quietly run in the
// wrong directory (2026-07-11 audit finding: the attached shorthand
// forms -C<path> / -C=<path>, which pflag itself accepts for any
// value-taking shorthand, originally slipped through this way before
// extractCwdOption below was taught to recognise them too). This guard
// is what catches the *next* unrecognised spelling.
type cwdLeakGuardValue struct{}

func (cwdLeakGuardValue) Set(string) error {
	return fmt.Errorf("%s could not be applied; pass it as a separate token (-C PATH) or --cwd=PATH", cwdOnceErrName)
}
func (cwdLeakGuardValue) Type() string   { return "string" }
func (cwdLeakGuardValue) String() string { return "" }

// registerCwdFlag adds the -C/--cwd persistent flag to root for help
// display and as the cwdLeakGuardValue defence-in-depth backstop (see
// file doc comment and cwdLeakGuardValue).
func registerCwdFlag(root *cobra.Command) {
	root.PersistentFlags().VarP(cwdLeakGuardValue{}, "cwd", "C",
		"change to `PATH` before running (global; may appear anywhere in argv)")
}

// extractCwdOption scans argv for -C/--cwd (DR-0043), removing it and
// returning the remaining argv plus the requested path. found is false
// when the option was not given.
//
// Every spelling pflag itself would accept for a value-taking shorthand
// is recognised here, not just the separate-token / long-flag forms:
// `-C PATH`, `--cwd PATH`, `--cwd=PATH`, the attached shorthand
// `-CPATH`, and its explicit-separator variant `-C=PATH`. Leaving any of
// these unhandled lets the token slip past this pre-scan and reach the
// never-really-Set() persistent flag (registerCwdFlag) — silently
// running with the original cwd instead of chdir'ing (the 2026-07-11
// audit finding this function was extended to close).
//
// This mirrors normalizeQuietAll's `--` boundary: anything at or after
// a literal `--` separator is left untouched (post-separator tokens are
// always positional, never reinterpreted as flags).
func extractCwdOption(argv []string) (rest []string, path string, found bool, err error) {
	rest = make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			rest = append(rest, argv[i:]...)
			break
		}

		var val string
		switch {
		case a == "-C" || a == "--cwd":
			if i+1 >= len(argv) {
				return nil, "", false, fmt.Errorf("%s requires a value (a directory path)", cwdOnceErrName)
			}
			val = argv[i+1]
			i++
		case strings.HasPrefix(a, "--cwd="):
			val = strings.TrimPrefix(a, "--cwd=")
		case strings.HasPrefix(a, "-C"):
			// Attached shorthand: -CPATH, or -C=PATH with the explicit
			// '=' separator stripped (git accepts both -C<path> and
			// -C=<path>; a leading '=' is not part of the path).
			val = strings.TrimPrefix(a[len("-C"):], "=")
		default:
			rest = append(rest, a)
			continue
		}

		if val == "" {
			// Reached by an explicit-but-empty value in any spelling
			// (-C '' / --cwd '' / --cwd= / -C=). os.Chdir("") is not a
			// meaningful request — its outcome is platform-dependent
			// rather than a clear error — so this folds into the same
			// "requires a value" usage error instead of ever reaching
			// applyCwdOption (audit finding item 4).
			return nil, "", false, fmt.Errorf("%s requires a value (a directory path)", cwdOnceErrName)
		}
		if found {
			return nil, "", false, fmt.Errorf("%s specified twice", cwdOnceErrName)
		}
		found = true
		path = val
	}
	return rest, path, found, nil
}

// applyCwdOption performs the os.Chdir requested by -C/--cwd. It runs
// once per runCobra invocation, before any cobra parsing, so every
// downstream cwd-relative operation (file inputs, vcs backend probing,
// glob expansion, rule resolution) observes the new directory.
func applyCwdOption(path string) error {
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("%s: cannot change directory to %q: %w", cwdOnceErrName, path, err)
	}
	return nil
}
