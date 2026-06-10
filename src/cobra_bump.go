package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// This file builds the bump-family commands (plan §2 Stage 4):
// `major` / `minor` / `patch` / `pre` / `get`. They all share the bump
// flag group (addSharedBumpFlags) and the buildBumpArgs assembler; the
// per-action differences are the `get` read-only rejections and the help
// text key. Each RunE is the thin "build cliArgs → runBump" adapter the
// plan prescribes (§0.3), keeping resolve.go / runBump untouched.

// bumpActionList is the canonical ordering of the bump verbs, used to
// register the commands and (via ValidArgs on the root) drive completion.
var bumpActionList = []string{"major", "minor", "patch", "pre", "get"}

// newBumpCmds builds the five bump-family commands. They are rebuilt per
// invocation (see newRootCmd) so flag state never leaks across run()
// calls.
func newBumpCmds(stdin io.Reader, stdout, stderr io.Writer) []*cobra.Command {
	cmds := make([]*cobra.Command, 0, len(bumpActionList))
	for _, action := range bumpActionList {
		cmds = append(cmds, newBumpCmd(action, stdin, stdout, stderr))
	}
	return cmds
}

// bumpShort returns the one-line Short description for a bump action.
func bumpShort(action string) string {
	switch action {
	case "get":
		return "print the current version (read-only)"
	case "pre":
		return "manage pre-release identifiers"
	default:
		return "bump the " + action + " component"
	}
}

func newBumpCmd(action string, stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd, args, shared := buildBumpCmd(action)

	cmd.RunE = func(cmd *cobra.Command, posArgs []string) error {
		built, err := buildBumpArgs(args, shared, posArgs)
		if err != nil {
			// Build-stage (parse) errors precede any quiet flag taking
			// effect, so they are always printed (legacy run() §3.1).
			fmt.Fprintln(stderr, "bump-semver: "+err.Error())
			return &exitErr{code: exitCodeUsage, msg: err.Error()}
		}
		return runBump(*built, stdin, stdout, stderr)
	}

	cmd.SetFlagErrorFunc(flagErrorFunc)

	// cobra intercepts --help / -h before RunE; route it to the existing
	// per-action help const on stdout (exit 0), matching the legacy
	// `<action> --help` short-circuit.
	helpKey := action
	cmd.SetHelpFunc(func(*cobra.Command, []string) {
		_ = printActionHelp(stdout, helpKey)
	})

	return cmd
}

// buildBumpCmd constructs the cobra command for a bump action together
// with the per-invocation cliArgs and shared rule-recorder state it
// captures. Separating construction from the RunE wiring lets same-
// package tests parse an argv and inspect the assembled cliArgs without
// running the dispatcher (plan §0.3 recommendation (a)).
func buildBumpCmd(action string) (*cobra.Command, *cliArgs, *sharedBumpFlags) {
	args := &cliArgs{kind: "bump", action: action}
	cmd := &cobra.Command{
		Use:           action,
		Short:         bumpShort(action),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	shared := addSharedBumpFlags(cmd, args)
	return cmd, args, shared
}

// buildBumpArgs assembles the bump cliArgs from the parsed flags and
// positional args, reproducing the legacy parseBumpArgs sequencing:
//
//  1. DR-0029 rule blocks replayed in argv order (buildRuleBlocks);
//  2. shared exclusivity / empty-value / --vcs validation (applySharedTail);
//  3. get-only read-only rejections (--write / --pre / --build-metadata);
//  4. the "at least one input" check.
//
// Inputs are whatever positional tokens cobra left.
func buildBumpArgs(args *cliArgs, shared *sharedBumpFlags, posArgs []string) (*cliArgs, error) {
	args.inputs = posArgs

	// DR-0029: replay recorded rule events in argv order.
	blocks, hasDefineRule, err := buildRuleBlocks(&shared.rules)
	if err != nil {
		return nil, err
	}
	args.ruleBlocks = blocks
	args.hasDefineRule = hasDefineRule

	// Shared exclusivity / empty-value / --vcs validation tail.
	if err := applySharedTail(args); err != nil {
		return nil, err
	}

	// get is read-only: reject the write/mutation flags (legacy
	// parseBumpArgs tail, same order and wording).
	if args.action == "get" {
		if args.write {
			return nil, fmt.Errorf("--write is not valid with get")
		}
		if args.bump.Pre != nil {
			return nil, fmt.Errorf("--pre is not valid with get (use --no-pre to strip)")
		}
		if args.bump.BuildMetadata != nil {
			return nil, fmt.Errorf("--build-metadata is not valid with get (use --no-build-metadata to strip)")
		}
	}

	if len(args.inputs) == 0 {
		return nil, fmt.Errorf("at least one input (FILE | VER | -) is required")
	}
	return args, nil
}
