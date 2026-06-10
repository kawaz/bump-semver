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

// vcsHelpKey maps a cobra command's path to its actionHelpTexts key.
// "bump-semver vcs tag push" → "vcs tag push". Unknown paths fall back
// to "vcs" (the parent help).
func vcsHelpKey(cmd *cobra.Command) string {
	if key, ok := strings.CutPrefix(cmd.CommandPath(), "bump-semver "); ok {
		if _, found := actionHelpTexts[key]; found {
			return key
		}
	}
	return "vcs"
}

// printActionHelp writes the per-action help const to stdout (exit 0).
// key is an actionHelpTexts key ("vcs get", "vcs tag push", ...).
func printActionHelp(stdout io.Writer, key string) error {
	text, ok := actionHelpTexts[key]
	if !ok {
		fmt.Fprint(stdout, shortHelpText)
		return nil
	}
	fmt.Fprint(stdout, text)
	return nil
}

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
				return printActionHelp(stdout, "vcs")
			}
			args.vcsVerb = posArgs[0]
			return runVcsCmd(args, stdin, stdout, stderr)
		},
	}
	addVcsPersistentFlags(vcsCmd, &args)
	vcsCmd.SetFlagErrorFunc(flagErrorFunc)

	// cobra intercepts --help / -h before RunE, so per-command help has
	// to be wired through SetHelpFunc rather than the RunE bareVerb path.
	// The help func is set on the parent; children inherit it. It maps
	// the command path ("bump-semver vcs tag push") to the matching
	// actionHelpTexts key ("vcs tag push") and prints the existing const.
	vcsCmd.SetHelpFunc(func(cmd *cobra.Command, helpArgs []string) {
		key := vcsHelpKey(cmd)
		// DR-0032: `vcs get latest-tag --help` / `latest-release --help`
		// route to the per-key help. The key is the positional arg cobra
		// left before the --help flag.
		if key == "vcs get" {
			for _, a := range helpArgs {
				if a == "latest-tag" || a == "latest-release" {
					key = "vcs get " + a
					break
				}
			}
		}
		_ = printActionHelp(stdout, key)
	})

	vcsCmd.AddCommand(
		newVcsGetCmd(&args, stdout, stderr),
		newVcsIsCmd(&args, stdout, stderr),
		newVcsDiffCmd(&args, stdout, stderr),
		newVcsCommitCmd(&args, stdout, stderr),
		newVcsFetchCmd(&args, stdout, stderr),
		newVcsPushCmd(&args, stdout, stderr),
		newVcsTagCmd(&args, stdout, stderr),
		newVcsOutdatedCmd(&args, stdout, stderr),
	)
	return vcsCmd
}

// addVcsPersistentFlags registers the flags shared by every vcs verb:
// --vcs override, -q/-qq/--no-hint verbosity. They are persistent so
// each child inherits them. -qq cannot be a pflag shorthand (it would be
// tokenised as `-q -q`); it is normalised to --quiet-all before cobra
// parses (see normalizeQuietAll / runCobra).
func addVcsPersistentFlags(cmd *cobra.Command, args *cliArgs) {
	pf := cmd.PersistentFlags()
	pf.Var(newOnceString("--vcs", &args.vcsBase.Override), "vcs", "force backend: jj | git | auto")

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
				return printActionHelp(stdout, "vcs get")
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
	f.Var(newOnceString("--repository", &args.vcsGet.LatestRepository), "repository", "external owner/repo or URL (latest-tag / latest-release)")
	f.BoolVar(&args.vcsGet.LatestIncludePre, "include-prerelease", false, "include prereleases (latest-tag / latest-release)")
	f.Var(newOnceString("--rev", &args.vcsGet.Rev), "rev", "target revision (commit-id)")
	f.BoolVar(&args.output.JSON, "json", false, "structured JSON output (latest-tag / latest-release)")
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
				return printActionHelp(stdout, "vcs is")
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "is"
			args.vcsArgs = posArgs
			return runVcsCmdIs(*args, stdout, stderr)
		},
	}
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
				return printActionHelp(stdout, "vcs diff")
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
	f.Var(&excludesValue{slot: &args.vcsDiff.Excludes}, "excludes", "exclude paths (literal / glob: / file:); repeatable")
	addGlobFlags(cmd, &args.glob)
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
				return printActionHelp(stdout, "vcs commit")
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
	f.VarP(newOnceString("-m", &args.vcsCommit.Message), "message", "m", "commit message")
	f.BoolVar(&args.vcsCommit.Staged, "staged", false, "commit only staged changes")
	f.BoolVar(&args.vcsCommit.Amend, "amend", false, "amend the current commit")
	f.BoolVarP(&args.vcsCommit.DashA, "all", "a", false, "(rejected: use --staged)")
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
				return printActionHelp(stdout, "vcs fetch")
			}
			if err := validateVcsOverride(stderr, *args); err != nil {
				return err
			}
			args.vcsVerb = "fetch"
			args.vcsArgs = posArgs
			return runVcsCmdFetch(*args, stdout, stderr)
		},
	}
	cmd.Flags().Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote name (default: origin)")
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
				return printActionHelp(stdout, "vcs push")
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
	f.Var(name, "branch", "branch/bookmark to push")
	f.Var(name, "bookmark", "branch/bookmark to push (jj alias of --branch)")
	f.Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote name (default: origin)")
	f.BoolVar(&args.vcsPush.JjBookmarkAutoAdvance, "jj-bookmark-auto-advance", false, "jj-only: advance bookmark before push")
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
				return printActionHelp(stdout, "vcs tag")
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
				return printActionHelp(stdout, "vcs tag push")
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
	f.Var(newOnceString("--rev", &args.vcsTag.Rev), "rev", "target revision")
	f.Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote name (default: origin)")
	f.BoolVar(&args.vcsTag.AllowMove, "allow-move", false, "move an existing tag")
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
				return printActionHelp(stdout, "vcs tag delete")
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
	cmd.Flags().Var(newOnceString("--remote", &args.vcsPush.Remote), "remote", "remote name (default: origin)")
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
	return &cobra.Command{
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
				return printActionHelp(stdout, "vcs outdated")
			}
			return runVcsCmdOutdated(*oargs, stdout, stderr)
		},
	}
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
