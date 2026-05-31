package main

// Exit code constants (DR-0020).
//
// The `vcs` subcommand family needs a richer exit-code vocabulary than
// the legacy bump/compare codes (0/1/2). We standardise the full set
// here so all callers — including future PRs (`vcs is`, `vcs commit`,
// `vcs push`, `vcs tag`) — pick from the same vocabulary.
//
// Backwards compatibility: the legacy codes 0 / 1 / 2 keep their
// previous meanings (success, compare-false, usage/parse error). The
// new codes 3-5 are only emitted by the `vcs` subcommands.
const (
	// exitCodeOK signals success across every subcommand.
	exitCodeOK = 0

	// exitCodeFalse is the predicate-false code used by `compare` and
	// (in future PRs) by `vcs is`. Distinct from exitCodeUsage so a
	// shell can branch on `if cmd; then ... fi`.
	exitCodeFalse = 1

	// exitCodeUsage covers argv parse errors, unknown subcommands /
	// verbs / keys, and any "the user typed something wrong" failure.
	exitCodeUsage = 2

	// exitCodeVCSExec is returned when the underlying VCS subprocess
	// (git / jj) fails — including "no repo found" probes. Distinct
	// from usage so callers can tell apart "you spelled it wrong" from
	// "the VCS is unhappy".
	exitCodeVCSExec = 3

	// exitCodeAmbiguous is returned when a query has no single answer
	// (e.g. `current-branch` on a detached HEAD, or zero / multiple
	// bookmarks at @ in jj). Also used by future PRs for tag/integrity
	// violations (`tag push` to a different rev without --allow-move).
	exitCodeAmbiguous = 4

	// exitCodeNonFastForward is reserved for `vcs push` non-ff rejects
	// in a future PR. Defined here so the constant lives in one place.
	exitCodeNonFastForward = 5
)

// exitErr carries an explicit exit code through the call stack so we
// can distinguish "compare predicate is false" (exit 1) from "an error
// occurred" (exit 2). Plain errors propagate as exit 2 in main.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }
func (e *exitErr) ExitCode() int { return e.code }
