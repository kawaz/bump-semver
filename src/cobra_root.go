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
// Stage 2 scope: the global short-circuit forms (--version / -V /
// --help / -h / --help-full), the no-argument case (short help) and the
// whole `vcs` subtree. The remaining real verbs (major, compare, ...)
// still flow through the legacy parseArgs path.
func useCobra(argv []string) bool {
	if len(argv) == 0 {
		return true
	}
	switch argv[0] {
	case "--version", "-V", "--help", "-h", "--help-full", "vcs":
		return true
	}
	return false
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
	// --version / -V must take effect before cobra resolves any
	// subcommand and only when it is the leading token (matching the
	// legacy argv[0] semantics). Handle it up-front.
	if len(argv) > 0 && (argv[0] == "--version" || argv[0] == "-V") {
		return handleVersion(argv[1:], stdout, stderr)
	}

	// pflag cannot represent `-qq` as a single shorthand (it tokenises
	// it as `-q -q` = quiet, not quiet-all). Rewrite the literal token
	// to --quiet-all before cobra parses the vcs subtree. The rewrite is
	// scoped to vcs because the other (legacy) verbs are still parsed by
	// the hand-rolled loop where `-qq` matches as a whole token.
	if len(argv) > 0 && argv[0] == "vcs" {
		argv = normalizeQuietAll(argv)
	}

	root := newRootCmd(stdin, stdout, stderr)
	root.SetArgs(argv)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
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
		// Args/RunE is reached for the no-argument case (short help)
		// and, in later stages, for unknown root-level subcommands.
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpFull {
				fmt.Fprint(stdout, fullHelpText)
				return nil
			}
			if len(args) == 0 {
				fmt.Fprint(stdout, shortHelpText)
				return nil
			}
			// Stage 1 never reaches here with a non-empty args slice:
			// real verbs are routed to the legacy parser by useCobra.
			// Later stages replace this with the unknown-action error.
			fmt.Fprint(stdout, shortHelpText)
			return nil
		},
	}

	// --help-full: there is no native cobra concept for it, so it is a
	// persistent bool flag that the root RunE / help wiring inspects.
	root.PersistentFlags().BoolVar(&helpFull, "help-full", false, "show the complete reference and exit")
	// -V / --version: registered so `bump-semver --help` lists it; the
	// actual handling happens in runCobra before cobra parsing (the
	// flag value here is never read).
	root.PersistentFlags().BoolP("version", "V", false, "print version and exit")

	root.SetFlagErrorFunc(flagErrorFunc)

	// Migrated subcommand trees (plan §2). Stage 2: vcs.
	root.AddCommand(newVcsCmd(stdin, stdout, stderr))

	// Route `--help-full` (when it leads) and `--help` / `-h` /
	// no-argument all to the existing help text. cobra's default help
	// flow prints to cmd's out writer; point it at stdout and emit the
	// short help.
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
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
