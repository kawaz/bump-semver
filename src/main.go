// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json /
// package-lock.json / VERSION) and provides five flat actions plus a
// nested `compare` subcommand:
//
//   - bump: major / minor / patch / pre
//   - read: get
//   - cmp:  compare {eq|lt|gt|le|ge}
//
// Inputs are positional: each argument may be a FILE path, a raw
// semver VER (e.g. `1.2.3`), `-` (read VER from stdin once), or
// `vcs:REV[:FILE]` / `vcs:<func>(...)` (read version from the VCS;
// DR-0008). When multiple inputs are given the values must agree;
// FILE-origin entries can be written back with `--write`, VER /
// stdin / vcs entries are reference values only.
//
// File layout (PR-Simplify-2 E):
//
//   - main.go         — process entry point (this file) + the tiny
//     generic helpers `ptr` / `derefOr`
//   - exit.go         — exit-code constants + the `exitErr` carrier
//   - cli_types.go    — CLI grammar data model: cliArgs + opts
//     sub-structs + the shared value helpers
//     (parseCompareOp / parseGlobFlag / parseBoolValue)
//   - cobra_*.go      — the cobra command tree (root / bump / compare /
//     vcs), custom pflag.Values, flag-error shaping
//     and help wiring that assemble cliArgs and call
//     the dispatchers
//   - cli_dispatch.go — `run` entry point (delegates to cobra), `runBump`
//     and the bump-side diagnostic helpers (emitErr /
//     emitFallbackHints / shouldShowHint / countFileInputs)
//   - resolve.go      — input resolution layer shared with compare.go
//     (locatedField, resolveInput*, resolveInputs,
//     allSameValue, formatMismatchError, wrapOrigin
//     Err, stdin-pipe helpers)
//   - vcs_cmd.go      — the `vcs <verb>` family (runVcsCmd +
//     runVcsCmd{Get,Is,Diff,Commit,Fetch,Push,Tag*}
//   - emitVcsUsage / emitVcsErr + validTagName)
//   - compare.go      — `compare` verb (predicate-only)
//   - help.go         — root short / full help text (--help / --help-full)
//   - cobra_help.go   — per-command help renderer; Options sections are
//     generated from the live FlagSet (single source of truth)
//   - cobra_help_text.go — per-command help prose (Long / Exit codes /
//     Examples) wired onto each cobra command
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		var ee *exitErr
		if errors.As(err, &ee) {
			// run() is responsible for writing any user-facing error
			// message to stderr (so it can honor --quiet-all). main only
			// translates the carried code into the process exit status.
			os.Exit(ee.code)
		}
		// Unexpected: run() must always wrap errors as *exitErr before
		// returning. Defensive fallback so we still exit non-zero.
		fmt.Fprintln(os.Stderr, "bump-semver: "+err.Error())
		os.Exit(2)
	}
}

// ptr returns a pointer to v. It exists so verb-local opts that use
// `*T` for "explicitly set" semantics can assign in one line:
//
//	out.vcsCommit.Message = ptr(value)
//
// instead of the two-line `tmp := v; field = &tmp` boilerplate.
func ptr[T any](v T) *T { return &v }

// derefOr returns *p when p is non-nil, otherwise def. Used at the
// few sites that need a bare value from a `*string` opt and want
// "unset → empty/default" semantics (e.g. forwarding the parser's
// `*string` into a downstream struct whose field is still `string`).
func derefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
