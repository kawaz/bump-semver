package main

import (
	"fmt"
	"testing"
)

// buildArgsForTest parses argv through the real cobra command tree and
// returns the cliArgs the matching RunE would dispatch, without running
// the dispatcher. It is the replacement for the now-removed parseArgs in
// same-package tests that verbatim-compare the assembled cliArgs or
// assert the build-stage error wording (plan §2 Stage 4 item 3, option
// (a)).
//
// It only handles the verbs whose cliArgs the migrated tests inspect:
// the bump family (major/minor/patch/pre/get) and compare. version /
// help / vcs are exercised through run() behavior tests instead (their
// cobra paths short-circuit before building a cliArgs).
func buildArgsForTest(t *testing.T, argv []string) (cliArgs, error) {
	t.Helper()
	if len(argv) == 0 {
		t.Fatalf("buildArgsForTest: empty argv")
	}
	verb := argv[0]

	switch {
	case verb == "vcs":
		// Build the full vcs tree, locate the target leaf command + its
		// remaining args via cobra's Find, parse the flags (which writes
		// the --glob-* / --vcs / verbosity slots into the shared args via
		// the custom Values), then return the shared cliArgs. The
		// dispatcher (runVcsCmd*) is never invoked.
		root, args := buildVcsCmd(nil, nil, nil)
		leaf, rest, ferr := root.Find(argv[1:])
		if ferr != nil {
			return cliArgs{}, ferr
		}
		if err := leaf.ParseFlags(rest); err != nil {
			return cliArgs{}, err
		}
		args.vcsVerb = leaf.Name()
		return *args, nil
	case verb == "compare":
		cmd, args, shared := buildCompareCmd()
		if err := cmd.ParseFlags(argv[1:]); err != nil {
			return cliArgs{}, err
		}
		built, err := buildCompareArgs(args, shared, cmd.Flags().Args())
		if err != nil {
			return cliArgs{}, err
		}
		return *built, nil
	case verb == "major" || verb == "minor" || verb == "patch" || verb == "pre" || verb == "get":
		cmd, args, shared := buildBumpCmd(verb)
		if err := cmd.ParseFlags(argv[1:]); err != nil {
			return cliArgs{}, err
		}
		built, err := buildBumpArgs(args, shared, cmd.Flags().Args())
		if err != nil {
			return cliArgs{}, err
		}
		return *built, nil
	default:
		return cliArgs{}, fmt.Errorf("buildArgsForTest: unsupported verb %q", verb)
	}
}
