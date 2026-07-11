package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// This file builds the `vcs` command tree (plan §2 Stage 2). The legacy
// hand-rolled parseVcsArgs is a single verb-gated flag loop; here each
// verb becomes its own cobra Command whose RunE assembles a cliArgs and
// hands it to the unchanged runVcsCmd* dispatcher. All key/predicate
// validation, exit codes and most error wording therefore stay in
// vcs_cmd.go untouched.
//
// Help routing rule (matching the legacy parser): a bare `vcs <verb>`
// with no positional args and no flags set shows the per-verb help on
// stdout (exit 0). Any token after the verb (a flag OR a positional)
// routes to the dispatcher, which emits the per-verb usage error
// (exit 2) when a required positional is missing.

// bareVerb reports whether the command was invoked with no positional
// args and no flags — the "show per-verb help" trigger that mirrors the
// legacy `len(argv) == N` bare-verb check.
func bareVerb(cmd *cobra.Command, args []string) bool {
	return len(args) == 0 && cmd.Flags().NFlag() == 0
}

// validateVcsOverride re-runs the legacy parse-time validation of the
// --vcs override value (parseVcsArgs tail). The dispatchers swallow the
// parseVcsOverride error (they default to auto), so the validation has
// to happen here to keep `--vcs hg` an exit-2 usage error.
func validateVcsOverride(stderr io.Writer, args cliArgs) error {
	if args.vcsBase.Override == nil {
		return nil
	}
	if _, err := parseVcsOverride(*args.vcsBase.Override); err != nil {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		return &exitErr{code: exitCodeUsage, msg: err.Error()}
	}
	return nil
}

// newVcsCmd builds the `vcs` parent and the full child tree. Everything
// is rebuilt per invocation (see newRootCmd) so flag state never leaks
// across run() calls.
func newVcsCmd(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd, _ := buildVcsCmd(stdin, stdout, stderr)
	return cmd
}

// buildVcsCmd constructs the `vcs` parent and full child tree, returning
// the shared cliArgs the child flags / RunEs populate. Splitting the
// returned args out lets same-package tests parse a vcs argv and inspect
// the assembled cliArgs (e.g. the --glob-* slots) without running the
// dispatcher, the same seam buildBumpCmd / buildCompareCmd provide.
func buildVcsCmd(stdin io.Reader, stdout, stderr io.Writer) (*cobra.Command, *cliArgs) {
	// Shared cliArgs the child RunEs populate. The persistent vcs flags
	// (--vcs / -q / -qq / --no-hint) write into the common sub-structs;
	// each verb adds its own local flags writing into the verb sub-struct.
	args := cliArgs{kind: "vcs"}

	vcsCmd := &cobra.Command{
		Use:           "vcs",
		Short:         "version-control helpers",
		SilenceErrors: true,
		SilenceUsage:  true,
		// The parent RunE handles `vcs` (no verb) → parent help, and
		// `vcs <unknown-verb>` → the dispatcher's "unknown vcs verb"
		// usage error (cobra calls this when no child matches).
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if len(posArgs) == 0 {
				return cmd.Help()
			}
			args.vcsVerb = posArgs[0]
			return runVcsCmd(args, stdin, stdout, stderr)
		},
	}
	addVcsPersistentFlags(vcsCmd, &args)
	vcsCmd.SetFlagErrorFunc(flagErrorFunc)
	applyVcsHelp(vcsCmd)

	vcsCmd.AddCommand(
		newVcsGetCmd(&args, stdout, stderr),
		newVcsIsCmd(&args, stdout, stderr),
		newVcsDiffCmd(&args, stdout, stderr),
		newVcsCommitCmd(&args, stdout, stderr),
		newVcsFetchCmd(&args, stdout, stderr),
		newVcsPushCmd(&args, stdout, stderr),
		newVcsTagCmd(&args, stdout, stderr),
		newVcsOutdatedCmd(&args, stdout, stderr),
		newVcsPromoteCmd(&args, stdout, stderr),
		newVcsSyncCmd(&args, stdout, stderr),
		newVcsBookmarkCmd(&args, stdout, stderr),
	)
	return vcsCmd, &args
}

// addVcsPersistentFlags registers the flags shared by every vcs verb:
// --vcs override, -q/-qq/--no-hint verbosity. They are persistent so
// each child inherits them. -qq cannot be a pflag shorthand (it would be
// tokenised as `-q -q`); it is normalised to --quiet-all before cobra
// parses (see normalizeQuietAll / runCobra).
func addVcsPersistentFlags(cmd *cobra.Command, args *cliArgs) {
	pf := cmd.PersistentFlags()
	pf.Var(newOnceString("--vcs", &args.vcsBase.Override), "vcs", "force backend (`jj|git|auto`, default auto)")

	addVerbosityFlags(pf, &args.output.Verbosity)
}

// --- vcs get ---------------------------------------------------------------

func newVcsGetCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "get",
		Short:         "read a single VCS fact",
		SilenceErrors: true,
		SilenceUsage:  true,
		ValidArgs:     vcsGetKeys,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "get"
			args.vcsArgs = posArgs
			return runVcsCmdGet(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	f.Var(newOnceString("--repository", &args.vcsGet.LatestRepository), "repository", "external `REPO` (owner/repo or URL) for latest-tag / latest-release")
	f.BoolVar(&args.vcsGet.LatestIncludePre, "include-prerelease", false, "include prereleases (latest-tag / latest-release)")
	f.Var(newOnceString("--rev", &args.vcsGet.Rev), "rev", "target `REV` for commit-id (default: latest fixed commit)")
	f.Var(newOnceString("--remote", &args.vcsGet.Remote), "remote", "source `NAME` for repository / repository-url (default: origin, or the sole configured remote)")
	f.BoolVar(&args.output.JSON, "json", false, "structured JSON output (latest-tag / latest-release)")
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs is ----------------------------------------------------------------

func newVcsIsCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "is",
		Short:         "test a VCS predicate (exit code is the answer)",
		SilenceErrors: true,
		SilenceUsage:  true,
		ValidArgs:     vcsIsPreds,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "is"
			args.vcsArgs = posArgs
			return runVcsCmdIs(*args, stdout, stderr)
		},
	}
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs diff --------------------------------------------------------------

func newVcsDiffCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "diff",
		Short:         "show changes since a revision",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "diff"
			args.vcsArgs = posArgs
			return runVcsCmdDiff(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&args.vcsDiff.NameStatus, "name-status", "s", false, "emit <CODE>\\t<path> lines instead of a patch")
	f.Var(&excludesValue{slot: &args.vcsDiff.Excludes}, "excludes", "exclude `PATTERN` (literal / glob: / file:); repeatable")
	addGlobFlags(cmd, &args.glob)
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs commit ------------------------------------------------------------

func newVcsCommitCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "commit",
		Short:         "create a commit",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "commit"
			args.vcsArgs = posArgs
			return runVcsCmdCommit(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	f.VarP(newOnceString("-m", &args.vcsCommit.Message), "message", "m", "commit `MSG` (required unless --amend)")
	f.BoolVar(&args.vcsCommit.Staged, "staged", false, "commit all staged/dirty changes at once")
	f.BoolVar(&args.vcsCommit.Amend, "amend", false, "fold the change into the previous commit")
	f.BoolVarP(&args.vcsCommit.DashA, "all", "a", false, "rejected by design: use --staged")
	f.BoolVar(&args.vcsCommit.AllowNonexistentPath, "allow-nonexistent-path", false, "silently drop PATH args that don't exist on the filesystem (legacy bump declarative-convergence behaviour)")
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs fetch -------------------------------------------------------------

func newVcsFetchCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "fetch",
		Short:         "fetch from a remote",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "fetch"
			args.vcsArgs = posArgs
			return runVcsCmdFetch(*args, stdout, stderr)
		},
	}
	cmd.Flags().Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote `NAME` (default: origin)")
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs push --------------------------------------------------------------

func newVcsPushCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "push",
		Short:         "push the current branch/bookmark",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "push"
			args.vcsArgs = posArgs
			return runVcsCmdPush(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	// --branch and --bookmark are aliases of one slot (DR-0020): a single
	// onceStringValue is shared so "both spellings supplied" is the same
	// "specified twice" error.
	name := newOnceString("--branch/--bookmark", &args.vcsPush.Name)
	f.Var(name, "branch", "branch `NAME` to push (required; jj: bookmark)")
	f.Var(name, "bookmark", "bookmark `NAME` to push (jj spelling of --branch)")
	f.Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote `NAME` (default: origin)")
	f.BoolVar(&args.vcsPush.JjBookmarkAutoAdvance, "jj-bookmark-auto-advance", false, "jj-only: advance bookmark before push")
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs tag (two-tier: push / delete) -------------------------------------

func newVcsTagCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "tag",
		Short:         "manage tags (push / delete)",
		SilenceErrors: true,
		SilenceUsage:  true,
		// `vcs tag` (no sub-verb) → tag help; `vcs tag <unknown>` →
		// dispatcher (emits the DR-0032 migration hint for `latest`,
		// the "unknown sub-verb" usage error otherwise).
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if len(posArgs) == 0 {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "tag"
			args.vcsTag.SubVerb = posArgs[0]
			args.vcsArgs = posArgs[1:]
			return runVcsCmdTag(*args, stdout, stderr)
		},
	}
	setHelp(cmd, vcsTagLong, vcsTagExitCodes, "")
	cmd.AddCommand(
		newVcsTagPushCmd(args, stdout, stderr),
		newVcsTagDeleteCmd(args, stdout, stderr),
	)
	return cmd
}

func newVcsTagPushCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "push",
		Short:         "create/move a tag and push it",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "tag"
			args.vcsTag.SubVerb = "push"
			args.vcsArgs = posArgs
			return runVcsCmdTagPush(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	f.Var(newOnceString("--rev", &args.vcsTag.Rev), "rev", "target `REV` (required)")
	f.Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote `NAME` (default: origin)")
	f.BoolVar(&args.vcsTag.AllowMove, "allow-move", false, "permit moving an existing tag to a different REV")
	setHelp(cmd, vcsTagPushLong, vcsTagPushExitCodes, vcsTagPushExamples)
	return cmd
}

func newVcsTagDeleteCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "delete",
		Short:         "delete a tag locally and on the remote",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "tag"
			args.vcsTag.SubVerb = "delete"
			args.vcsArgs = posArgs
			return runVcsCmdTagDelete(*args, stdout, stderr)
		},
	}
	cmd.Flags().Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote `NAME` (default: origin)")
	setHelp(cmd, vcsTagDeleteLong, vcsTagDeleteExitCodes, vcsTagDeleteExamples)
	return cmd
}

// --- vcs promote -----------------------------------------------------------

func newVcsPromoteCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "promote",
		Short:         "move the default branch/bookmark forward to the current commit",
		SilenceErrors: true,
		SilenceUsage:  true,
		// promote takes no positional args and no flags by design — a bare
		// invocation is the normal mode (= "advance default to current"),
		// so we do NOT short-circuit to help on bareVerb. The dispatcher
		// rejects stray positional args with exit 2.
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "promote"
			args.vcsArgs = posArgs
			return runVcsCmdPromote(*args, stdout, stderr)
		},
	}
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs sync --------------------------------------------------------------

func newVcsSyncCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "rebase the current worktree onto a reference",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "sync"
			args.vcsArgs = posArgs
			return runVcsCmdSync(*args, stdout, stderr)
		},
	}
	cmd.Flags().Var(newOnceString("--onto", &args.vcsSync.Onto), "onto", "target `REF` to rebase onto (required)")
	applyVcsVerbHelp(cmd)
	return cmd
}

// --- vcs bookmark (two-tier: set; future: list / delete) -------------------

func newVcsBookmarkCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bookmark",
		Short:         "manage branches/bookmarks (set)",
		SilenceErrors: true,
		SilenceUsage:  true,
		// `vcs bookmark` (no sub-verb) → bookmark help; `vcs bookmark
		// <unknown>` → dispatcher emits the "unknown sub-verb" usage error.
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if len(posArgs) == 0 {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "bookmark"
			args.vcsBookmark.SubVerb = posArgs[0]
			args.vcsArgs = posArgs[1:]
			return runVcsCmdBookmark(*args, stdout, stderr)
		},
	}
	setHelp(cmd, vcsBookmarkLong, vcsBookmarkExitCodes, "")
	cmd.AddCommand(newVcsBookmarkSetCmd(args, stdout, stderr))
	return cmd
}

func newVcsBookmarkSetCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "set",
		Short:         "create or move a branch/bookmark to a revision",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			if bareVerb(cmd, posArgs) {
				return cmd.Help()
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "bookmark"
			args.vcsBookmark.SubVerb = "set"
			args.vcsArgs = posArgs
			return runVcsCmdBookmarkSet(*args, stdout, stderr)
		},
	}
	f := cmd.Flags()
	// -r shorthand mirrors jj's own `jj bookmark set NAME -r REV`; the
	// justfile's push-wip path calls `bump-semver vcs bookmark set "$ws" -r @`.
	f.VarP(newOnceString("--rev", &args.vcsBookmark.Rev), "rev", "r", "target `REV` (default: @ / HEAD)")
	f.BoolVar(&args.vcsBookmark.AllowBackwards, "allow-backwards", false, "permit a non-fast-forward move (default: FF-only)")
	setHelp(cmd, vcsBookmarkSetLong, vcsBookmarkSetExitCodes, vcsBookmarkSetExamples)
	return cmd
}

// --- vcs outdated (DisableFlagParsing special case) ------------------------

// newVcsOutdatedCmd builds `vcs outdated` with DisableFlagParsing set.
//
// Design rationale: `vcs outdated` uses `--` as a *pair separator*
// (`vcs outdated -- F1 T1 -- F2 T2 ...`), which is incompatible with
// cobra/pflag's "-- means the rest is positional" convention. So cobra
// is told not to parse flags at all; the raw tokens are re-tokenised by
// parseOutdatedTokens, which is a transcription of the legacy
// parseVcsArgs outdated branch (keeping `--` as a literal in vcsArgs so
// splitOutdatedPairs keeps working unchanged).
func newVcsOutdatedCmd(args *cliArgs, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "outdated",
		Short:              "check whether derived files are stale vs a source",
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, rawArgs []string) error {
			// rawArgs already excludes the "vcs outdated" prefix; it is
			// every token after the verb, flags included.
			oargs, err := parseOutdatedTokens(*args, rawArgs)
			if err != nil {
				// Parse-time errors (empty --excludes value, glob bool
				// polarity, unknown flag) use the legacy run() prefix +
				// exit 2 shape.
				fmt.Fprintln(stderr, "bump-semver: "+err.Error())
				return &exitErr{code: exitCodeUsage, msg: err.Error()}
			}
			if oargs == nil {
				// help short-circuit (bare verb / --help)
				return cmd.Help()
			}
			return runVcsCmdOutdated(*oargs, stdout, stderr)
		},
	}
	applyVcsVerbHelp(cmd)
	return cmd
}

// parseOutdatedTokens re-tokenises the raw `vcs outdated` token stream
// (DisableFlagParsing). It is a transcription of the legacy parseVcsArgs
// outdated branch: --explain / --strict / --glob-* / --vcs / -q / -qq /
// --no-hint are recognised, `--` is kept as a literal in vcsArgs (the
// pair separator splitOutdatedPairs scans for), and everything else is a
// positional. A bare verb or a leading --help / -h returns (nil, nil) =
// "show outdated help".
//
// `base` carries the persistent vcs flag slots already populated for the
// parent (none, since outdated disables flag parsing — but the shared
// cliArgs kind/etc are inherited). The returned cliArgs is a fresh copy
// so concurrent run() calls don't share mutated state.
func parseOutdatedTokens(base cliArgs, raw []string) (*cliArgs, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == "--help" || raw[0] == "-h" {
		return nil, nil
	}

	out := base
	out.kind = "vcs"
	out.vcsVerb = "outdated"
	out.vcsArgs = nil

	for i := 0; i < len(raw); i++ {
		a := raw[i]
		switch {
		case a == "--vcs":
			if out.vcsBase.Override != nil {
				return nil, fmt.Errorf("--vcs specified twice")
			}
			if i+1 >= len(raw) {
				return nil, fmt.Errorf("--vcs requires a value (jj, git, or auto)")
			}
			out.vcsBase.Override = ptr(raw[i+1])
			i++
		case strings.HasPrefix(a, "--vcs="):
			if out.vcsBase.Override != nil {
				return nil, fmt.Errorf("--vcs specified twice")
			}
			out.vcsBase.Override = ptr(strings.TrimPrefix(a, "--vcs="))
		case a == "-q" || a == "--quiet":
			out.output.Verbosity.raise(outputQuiet)
		case a == "-qq" || a == "--quiet-all":
			out.output.Verbosity.raise(outputQuietAll)
		case a == "--no-hint":
			out.output.Verbosity.raise(outputNoHint)
		case a == "--explain":
			out.vcsOutdated.Explain = true
		case a == "--strict":
			out.vcsOutdated.Strict = true
		case strings.HasPrefix(a, "--glob-"):
			matched, ferr := parseGlobFlag(a, &out)
			if ferr != nil {
				return nil, ferr
			}
			if !matched {
				return nil, fmt.Errorf("unknown flag for 'vcs outdated': %s", a)
			}
		case a == "--":
			// DR-0027: keep `--` literal so splitOutdatedPairs can split
			// pair groups.
			out.vcsArgs = append(out.vcsArgs, a)
		case strings.HasPrefix(a, "-") && a != "-":
			return nil, fmt.Errorf("unknown flag for 'vcs outdated': %s", a)
		default:
			out.vcsArgs = append(out.vcsArgs, a)
		}
	}
	if out.vcsBase.Override != nil {
		if _, err := parseVcsOverride(*out.vcsBase.Override); err != nil {
			return nil, err
		}
	}
	return &out, nil
}
