package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagErrorFunc translates cobra/pflag flag-parsing errors into the
// project's "bump-semver: <reason>" stderr line + *exitErr{code: 2}
// shape, reproducing the legacy hand-rolled wording (plan §3.2 / §3.3).
//
// Three classes of error reach here:
//
//  1. Errors authored by this package's custom pflag.Values (onceString
//     "specified twice", glob bool polarity, excludes empty, invalid
//     --vcs value). These already carry the final legacy text, so they
//     are emitted verbatim.
//  2. pflag "unknown flag" — reshaped to the verb-scoped wording
//     ("unknown flag for 'vcs <verb>': <flag>" for the vcs subtree, or
//     "unknown option: <flag>" elsewhere).
//  3. pflag "flag needs an argument" — reshaped via requiresValueMsg to
//     the legacy "<flag> requires a value (...)" wording, scoped by the
//     command path where the same flag means different things.
func flagErrorFunc(cmd *cobra.Command, err error) error {
	stderr := cmd.ErrOrStderr()

	// (1) Unknown flag — pflag *NotExistError.
	var notExist *pflag.NotExistError
	if errors.As(err, &notExist) {
		flag := flagDisplayName(notExist.GetSpecifiedName(), notExist.GetSpecifiedShortnames())
		return emitFlagError(stderr, unknownFlagMsg(cmd, flag))
	}

	// (2) Missing value — pflag *ValueRequiredError.
	var valReq *pflag.ValueRequiredError
	if errors.As(err, &valReq) {
		flag := flagDisplayName(valReq.GetSpecifiedName(), valReq.GetSpecifiedShortnames())
		return emitFlagError(stderr, requiresValueText(cmd, flag))
	}

	// (3) Invalid value — pflag *InvalidValueError. The cause is the
	// error our custom Value returned from Set() (already final legacy
	// text); unwrap and emit it verbatim.
	var invalid *pflag.InvalidValueError
	if errors.As(err, &invalid) {
		if cause := invalid.Unwrap(); cause != nil {
			return emitFlagError(stderr, cause.Error())
		}
		return emitFlagError(stderr, invalid.Error())
	}

	// (4) Anything else (including errors a custom Value returned that
	// pflag surfaced directly) — emit verbatim.
	return emitFlagError(stderr, err.Error())
}

// emitFlagError writes the standard prefixed line and returns the usage
// exit error carrying the same message.
func emitFlagError(stderr interface{ Write([]byte) (int, error) }, msg string) error {
	fmt.Fprintln(stderr, "bump-semver: "+msg)
	return &exitErr{code: exitCodeUsage, msg: msg}
}

// flagDisplayName reconstructs the dash-prefixed flag token from pflag's
// (name, shorthands) pair: a shorthand group yields "-x", a long name
// yields "--name".
func flagDisplayName(name, shorthands string) string {
	if shorthands != "" {
		// The offending char is the first of the group.
		if name != "" {
			return "-" + name
		}
		return "-" + shorthands
	}
	return "--" + name
}

// vcsVerbScope returns the "<verb>" (or "tag <sub>") label for a command
// under the vcs subtree, or "" if cmd is not in the vcs subtree.
func vcsVerbScope(cmd *cobra.Command) string {
	path := cmd.CommandPath() // e.g. "bump-semver vcs get" / "bump-semver vcs tag push"
	const prefix = "bump-semver vcs"
	if path == prefix {
		return "" // the bare `vcs` parent
	}
	if rest, ok := strings.CutPrefix(path, prefix+" "); ok {
		return rest // "get" / "tag push" / ...
	}
	return ""
}

// unknownFlagMsg builds the legacy unknown-flag wording for cmd. The vcs
// subtree names the verb ("unknown flag for 'vcs get': -s"); the bump /
// compare grammar uses "unknown option: --x".
func unknownFlagMsg(cmd *cobra.Command, flag string) string {
	if scope := vcsVerbScope(cmd); scope != "" {
		return fmt.Sprintf("unknown flag for 'vcs %s': %s", scope, flag)
	}
	if isVcsSubtree(cmd) {
		// On the bare `vcs` parent itself.
		return fmt.Sprintf("unknown flag for 'vcs': %s", flag)
	}
	return fmt.Sprintf("unknown option: %s", flag)
}

// isVcsSubtree reports whether cmd is `vcs` or one of its descendants.
func isVcsSubtree(cmd *cobra.Command) bool {
	path := cmd.CommandPath()
	return path == "bump-semver vcs" || strings.HasPrefix(path, "bump-semver vcs ")
}

// requiresValueMsg maps a flag token to the legacy "requires a value"
// wording. Flags whose wording depends on the command (currently only
// --rev) are resolved in requiresValueText.
var requiresValueMsg = map[string]string{
	"--vcs":        "--vcs requires a value (jj, git, or auto)",
	"--excludes":   "--excludes requires a value (literal path / glob: / file:)",
	"-m":           "-m requires a value (commit message)",
	"--message":    "--message requires a value",
	"--repository": "--repository requires a value (owner/repo or URL)",
	"--remote":     "--remote requires a value",
	"--branch":     "--branch requires a value (the branch/bookmark name)",
	"--bookmark":   "--bookmark requires a value (the branch/bookmark name)",
}

// requiresValueText resolves the legacy "requires a value" wording for a
// flag, honouring command-specific phrasing. Falls back to a generic
// "<flag> requires a value" when the flag is not in the table.
func requiresValueText(cmd *cobra.Command, flag string) string {
	if flag == "--rev" {
		// --rev means different things under `vcs get` (commit-id REV)
		// and `vcs tag push` (the target revision).
		if vcsVerbScope(cmd) == "tag push" {
			return "--rev requires a value (the target revision)"
		}
		return "--rev requires a value (REV)"
	}
	if msg, ok := requiresValueMsg[flag]; ok {
		return msg
	}
	return fmt.Sprintf("%s requires a value", flag)
}
