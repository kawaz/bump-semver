package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// This file builds the `compare` command (plan §2 Stage 3). compare is a
// predicate-only command (DR-0006): the first positional is the operator
// (eq / lt / le / gt / ge, optionally with a -major / -minor / -patch
// precision suffix, DR-0017), the rest are the BASE + OTHERS inputs.
//
// The operator stays a positional (not a sub-command) so the existing
// parseCompareOp grammar and error wording carry over unchanged. compare
// accepts the full bump shared flag group (addSharedBumpFlags) and rejects
// the bump-only ones (--write / --pre / --build-metadata / --json) in its
// RunE with the legacy per-flag wording, matching the order of the legacy
// parseCompareArgs validity tail.

// newCompareCmd builds the `compare` command. Rebuilt per invocation (see
// newRootCmd) so flag state never leaks across run() calls.
func newCompareCmd(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd, args, shared := buildCompareCmd()

	// DisableFlagParsing is left off: the operator is an ordinary leading
	// positional, and inputs starting with `-` are handled by the `--`
	// separator (cobra's standard end-of-flags convention).
	cmd.RunE = func(cmd *cobra.Command, posArgs []string) error {
		built, err := buildCompareArgs(args, shared, posArgs)
		if err != nil {
			// Build-stage (parse) errors precede any quiet flag taking
			// effect, so they are always printed (legacy run() §3.1).
			fmt.Fprintln(stderr, "bump-semver: "+err.Error())
			return &exitErr{code: exitCodeUsage, msg: err.Error()}
		}
		return runCompare(*built, stdin, stdout, stderr)
	}

	cmd.SetFlagErrorFunc(flagErrorFunc)

	// Help prose (Long / Exit codes / Examples). The `compare <op> --help`
	// form also lands here: the operator positional is ignored, and the
	// root's HelpFunc renders the same screen. Options come from the
	// FlagSet (renderCommandHelp).
	applyCompareHelp(cmd)

	return cmd
}

// buildCompareCmd constructs the cobra `compare` command together with
// the per-invocation cliArgs and shared rule-recorder state it captures.
// Splitting construction from the RunE wiring lets same-package tests
// parse an argv and inspect the assembled cliArgs without running the
// dispatcher (plan §0.3 recommendation (a)).
func buildCompareCmd() (*cobra.Command, *cliArgs, *sharedBumpFlags) {
	args := &cliArgs{kind: "compare"}
	cmd := &cobra.Command{
		Use:           "compare",
		Short:         "compare a base value to one or more others (exit-code-driven)",
		SilenceErrors: true,
		SilenceUsage:  true,
		// Complete the operator only at the first positional; the
		// remaining inputs (BASE / OTHERS) are files or raw versions, so
		// fall back to default file completion there.
		ValidArgsFunction: func(cmd *cobra.Command, posArgs []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(posArgs) == 0 {
				return compareOpList, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveDefault
		},
	}
	shared := addSharedBumpFlags(cmd, args)
	return cmd, args, shared
}

// compareOpList is the completion-facing ordering of the 20 compare
// operators (5 bases × {full, -major, -minor, -patch}). It drives
// `compare <TAB>`; the authoritative grammar stays in parseCompareOp.
var compareOpList = func() []string {
	bases := []string{"eq", "lt", "le", "gt", "ge"}
	precisions := []string{"", "-major", "-minor", "-patch"}
	ops := make([]string, 0, len(bases)*len(precisions))
	for _, b := range bases {
		for _, p := range precisions {
			ops = append(ops, b+p)
		}
	}
	return ops
}()

// buildCompareArgs assembles the compare cliArgs from the parsed flags and
// positional args, reproducing the legacy parseCompareArgs sequencing:
//
//  1. operator presence + validity (parseCompareOp);
//  2. shared-flag exclusivity / empty-value / --vcs validation tail;
//  3. compare-only rejections (--write / --pre / --build-metadata / --json);
//  4. the "at least two inputs" check.
//
// The DR-0029 rule blocks are replayed here in argv order from the shared
// recorder. Inputs are whatever positional tokens cobra left after the
// operator.
func buildCompareArgs(args *cliArgs, shared *sharedBumpFlags, posArgs []string) (*cliArgs, error) {
	if len(posArgs) == 0 {
		return nil, fmt.Errorf("compare requires an operator (eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)")
	}
	op := posArgs[0]
	base, precision, ok := parseCompareOp(op)
	if !ok {
		return nil, fmt.Errorf("unknown compare operator: %s (expected eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)", op)
	}
	args.compareOp = base
	args.comparePrecision = precision
	args.inputs = posArgs[1:]

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

	// compare-only rejections (legacy parseCompareArgs tail, same order).
	if args.write {
		return nil, fmt.Errorf("--write is not valid with compare")
	}
	if args.bump.Pre != nil {
		return nil, fmt.Errorf("--pre is not valid with compare")
	}
	if args.bump.BuildMetadata != nil {
		return nil, fmt.Errorf("--build-metadata is not valid with compare")
	}
	if args.output.JSON {
		return nil, fmt.Errorf("compare does not support --json")
	}
	if len(args.inputs) < 2 {
		return nil, fmt.Errorf("compare requires at least two inputs (BASE OTHERS...), got %d", len(args.inputs))
	}
	return args, nil
}
