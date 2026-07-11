package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// useCobra reports whether argv should be handled by the cobra entry
// point (runCobra) instead of the legacy hand-rolled parser.
//
// This is the co-existence router (plan §2). It grows one stage at a
// time: every migrated first-token is added here, and once every verb
// is on cobra the router is removed entirely and run() always delegates
// to runCobra.
//
// Stage 4 scope: every verb is now on cobra — the global short-circuit
// forms (--version / -V / --help / -h / --help-full), the no-argument
// case (short help), the `vcs` subtree, `compare`, and the bump family
// (major/minor/patch/pre/get). Any other leading token is routed to
// cobra too, where the root RunE reports it as an unknown action. The
// legacy parseArgs path is no longer reachable and is removed.
func useCobra(argv []string) bool {
	return true
}

// runCobra builds a fresh root command and executes it against argv.
//
// A new command tree is constructed on every call (no package-level
// singleton) so that the per-flag state cobra/pflag keeps does not leak
// across the many parallel run() invocations the tests make.
//
// RunE handlers (and the version short-circuit) always return either nil
// on success or an *exitErr on failure, so main only has to translate
// the carried code into a process exit status. Any bare cobra/pflag
// error that reaches here is defensively wrapped into an exit-2 *exitErr.
func runCobra(argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	// -C/--cwd (DR-0043) is extracted and applied first, ahead of every
	// other pre-processing step: it must land before anything else
	// (including the --version short-circuit below and cobra's own flag
	// parsing) reads the filesystem or resolves a relative path.
	argv, cwdPath, hasCwd, err := extractCwdOption(argv)
	if err != nil {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		return &exitErr{code: exitCodeUsage, msg: err.Error()}
	}
	if hasCwd {
		if err := applyCwdOption(cwdPath); err != nil {
			fmt.Fprintln(stderr, "bump-semver: "+err.Error())
			return &exitErr{code: exitCodeUsage, msg: err.Error()}
		}
	}

	// --version / -V must take effect before cobra resolves any
	// subcommand and only when it is the leading token (matching the
	// legacy argv[0] semantics). Handle it up-front.
	if len(argv) > 0 && (argv[0] == "--version" || argv[0] == "-V") {
		return handleVersion(argv[1:], stdout, stderr)
	}

	// pflag cannot represent `-qq` as a single shorthand (it tokenises
	// it as `-q -q` = quiet, not quiet-all). Rewrite the literal token
	// to --quiet-all before cobra parses, but only in a flag position:
	// the `--` guard skips post-separator positionals and the
	// value-taking-flag set skips a `-qq` that is the value of a flag
	// like --pre / -m (so `--pre -qq` stays literal). The set is derived
	// from the command tree's pflag FlagSets so new flags auto-follow.
	argv = normalizeQuietAll(argv, valueTakingFlags())

	root := newRootCmd(stdin, stdout, stderr)
	root.SetArgs(argv)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err = root.Execute()
	if err == nil {
		return nil
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee
	}
	// Unexpected: a bare cobra/pflag error slipped past the
	// per-command FlagErrorFunc. Treat it as a usage error so the
	// process still exits non-zero with the project's prefix.
	fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	return &exitErr{code: exitCodeUsage, msg: err.Error()}
}

// newRootCmd constructs the cobra root command.
//
// The command is intentionally rebuilt on every runCobra call (see the
// note there). It owns the two project-specific persistent flags that
// cobra has no native concept of (--version/-V and --help-full) and the
// help wiring that turns `bump-semver` / `bump-semver --help` /
// `bump-semver --help-full` into the existing short / full help text.
func newRootCmd(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var helpFull bool

	root := &cobra.Command{
		Use:           "bump-semver",
		Short:         "focused semver bump CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		// ArbitraryArgs overrides cobra's default legacyArgs validator,
		// which would emit `unknown command "x"` for an unmatched leading
		// token. Instead the token reaches the root RunE, which reports it
		// with the legacy `unknown action: x (expected ...)` wording.
		Args: cobra.ArbitraryArgs,
		// RunE is reached for the no-argument case (short help) and for an
		// unmatched leading token (an unknown action).
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpFull {
				fmt.Fprint(stdout, fullHelpText)
				return nil
			}
			if len(args) == 0 {
				fmt.Fprint(stdout, shortHelpText)
				return nil
			}
			// A non-empty args slice here means args[0] resolved to no
			// child command: an unknown action. Match the legacy
			// parseBumpArgs wording (exit 2).
			msg := fmt.Sprintf("unknown action: %s (expected one of major|minor|patch|pre|get|compare)", args[0])
			fmt.Fprintln(stderr, "bump-semver: "+msg)
			return &exitErr{code: exitCodeUsage, msg: msg}
		},
	}

	// --help-full: there is no native cobra concept for it, so it is a
	// persistent bool flag that the root RunE / help wiring inspects. It is
	// a root-only concept (the short/full help selector), so it is hidden
	// from the auto-generated subcommand Global Options block — the root's
	// own help (shortHelpText / fullHelpText) documents it by hand.
	root.PersistentFlags().BoolVar(&helpFull, "help-full", false, "show the complete reference and exit")
	root.PersistentFlags().Lookup("help-full").Hidden = true
	// -V / --version: registered so the root help can mention it; the
	// actual handling happens in runCobra before cobra parsing (the flag
	// value here is never read). Hidden from subcommand help for the same
	// reason as --help-full (it only fires as a leading token).
	root.PersistentFlags().BoolP("version", "V", false, "print version and exit")
	root.PersistentFlags().Lookup("version").Hidden = true
	// -C / --cwd (DR-0043): registered visible (unlike --help-full /
	// --version above) so every subcommand's Global Options lists it —
	// unlike those two, -C is meant to be usable at any position with any
	// subcommand. The actual chdir happens in runCobra before cobra
	// parsing (see cobra_cwd.go); this registration never has its Set()
	// called.
	registerCwdFlag(root)

	root.SetFlagErrorFunc(flagErrorFunc)

	// Migrated subcommand trees (plan §2). Stage 2: vcs. Stage 3: compare.
	// Stage 4: bump family (major/minor/patch/pre/get).
	root.AddCommand(newVcsCmd(stdin, stdout, stderr))
	root.AddCommand(newCompareCmd(stdin, stdout, stderr))
	for _, c := range newBumpCmds(stdin, stdout, stderr) {
		root.AddCommand(c)
	}

	// Help wiring. The root keeps its bespoke short / full help (selected
	// by --help-full); every subcommand renders through renderCommandHelp,
	// whose Options / Global Options sections come from the live FlagSet
	// (the cobra-migration single-source-of-truth goal). installHelp sets a
	// single HelpFunc on the root that children inherit.
	installHelp(root, stdout, func() {
		if helpFull {
			fmt.Fprint(stdout, fullHelpText)
			return
		}
		fmt.Fprint(stdout, shortHelpText)
	})

	return root
}

// handleVersion implements the --version / -V short-circuit. It mirrors
// the legacy parser (cli_parse.go) + dispatcher (cli_dispatch.go)
// behavior: --json is the only flag accepted alongside --version; any
// other token is a usage error.
func handleVersion(rest []string, stdout, stderr io.Writer) error {
	jsonOut := false
	for _, a := range rest {
		if a == "--json" {
			jsonOut = true
			continue
		}
		// Match the legacy exit-2 usage error verbatim.
		fmt.Fprintln(stderr, "bump-semver: --version only accepts --json")
		return &exitErr{code: exitCodeUsage, msg: "--version only accepts --json"}
	}

	if jsonOut {
		v, perr := ParseVersion(version)
		if perr != nil {
			msg := fmt.Sprintf("parse own version %q: %v", version, perr)
			fmt.Fprintln(stderr, "bump-semver: "+msg)
			return &exitErr{code: exitCodeUsage, msg: msg}
		}
		data, mErr := marshalJSONOutput(v.ToJSON(nil))
		if mErr != nil {
			msg := fmt.Sprintf("marshal json: %v", mErr)
			fmt.Fprintln(stderr, "bump-semver: "+msg)
			return &exitErr{code: exitCodeUsage, msg: msg}
		}
		_, _ = stdout.Write(data)
		return nil
	}
	fmt.Fprintln(stdout, version)
	return nil
}
