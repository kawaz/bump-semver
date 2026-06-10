package main

import (
	"fmt"
	"io"
)

// runCompare implements the `compare {eq|lt|le|gt|ge}` subcommand.
//
// Inputs are resolved with the same FILE | VER | `-` rules as `bump`.
// When an input contributes multiple version fields (e.g. a
// package-lock.json with $.version and $.packages[""].version), the
// fields are checked for equality and collapsed to a single value
// before comparison.
//
// DR-0023: compare takes `BASE OTHERS...` (one base + one or more
// OTHERS). The predicate `BASE OP OTHER` is evaluated for **every**
// OTHER independently — there is no short-circuit, so a single run
// surfaces every failing relation at once. The legacy 2-input form
// (`compare OP A B`) is the N=1 case.
//
// Borrow semantics: file-omitted `vcs:REV` OTHERS borrow F1's path
// (peerExpand=false in resolveInputs). The same `vcs:main` after
// `compare gt VERSION` therefore resolves to `vcs:main:VERSION`.
//
// Exit codes (DR-0006 確定論点 A, extended by DR-0023):
//   - 0  every predicate true
//   - 1  at least one predicate false
//   - 2  any error (parse failure, missing input, ...)
//
// Quiet flags (Phase 5 + DR-0010 + DR-0023):
//   - -q / --quiet:   suppresses the DR-0010 fallback / unsupported-file
//     hints (predicate-false stderr is preserved).
//   - -qq / --quiet-all: also suppresses the stderr "bump-semver: ..."
//     diagnostic emitted on exit-2 errors, **and** the per-OTHER
//     failure list emitted on exit-1.
//   - --no-hint:      suppresses the DR-0010 fallback / unsupported-file
//     hints. compare has no "files not modified" hint to suppress.
func runCompare(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	_ = stdout // compare prints nothing on success; consumers read the exit code.
	if len(args.inputs) < 2 {
		return emitErr(stderr, args, fmt.Errorf("compare requires at least two inputs (BASE OTHERS...), got %d", len(args.inputs)))
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // already validated in applySharedTail
	// PeerExpand=false: compare's borrow has always been "use F1's
	// path", and that's exactly what DR-0023 requires for N OTHERS
	// too — every file-omitted `vcs:REV` OTHER borrows F1's path.
	resolved, err := resolveInputs(args.inputs, stdin, resolveInputsOpts{
		Write:      false,
		VCSKind:    vcsOverride,
		PeerExpand: false,
		Glob:       args.glob,
		RuleBlocks: args.ruleBlocks,
	})
	if err != nil {
		return emitErr(stderr, args, err)
	}
	// DR-0024: glob: selectors may expand (peer expansion) or contract
	// (0-match), so the resolved-count check now allows the shape "F1
	// + at least one OTHER" rather than pinning to the literal input
	// count.
	if len(resolved) < 2 {
		return emitErr(stderr, args, fmt.Errorf("compare requires at least two inputs after glob expansion, got %d", len(resolved)))
	}

	// DR-0010: surface confidence-1 fallback matches for compare too —
	// the hint reflects "you used an unknown filename", not a property
	// of the bump action. Suppression flags handled inside the helper.
	emitFallbackHints(stderr, args, resolved)

	base, err := collapseToOneVersion(resolved[0])
	if err != nil {
		return emitErr(stderr, args, err)
	}

	// DR-0023: full-evaluation. Walk every OTHER, collect failures,
	// emit a per-OTHER stderr line for each. Position-aware label
	// (O1, O2, ...) lets the user line up failures with their argv
	// position even when the literal labels are long (e.g. URLs).
	var failures []string
	for i := 1; i < len(resolved); i++ {
		other, oerr := collapseToOneVersion(resolved[i])
		if oerr != nil {
			return emitErr(stderr, args, oerr)
		}
		cmp := base.CompareAt(other, args.comparePrecision)
		if evalCompareOp(args.compareOp, cmp) {
			continue
		}
		failures = append(failures, formatCompareFailure(args, i, resolved[0], base, resolved[i], other))
	}
	if len(failures) == 0 {
		return nil
	}
	// Predicate false: exit 1. -qq suppresses the per-OTHER listing
	// (consistent with "quiet-all suppresses diagnostics"); -q alone
	// leaves it intact so users still see why the assertion failed.
	if !args.output.Verbosity.ShouldSuppressError() {
		for _, line := range failures {
			fmt.Fprintln(stderr, line)
		}
	}
	return &exitErr{code: exitCodeFalse}
}

// formatCompareFailure renders one "base OP other failed" line for
// stderr. The phrase is operator- and precision-aware so the user
// sees an English description ("not greater than", "not equal at
// MAJOR.MINOR", ...) rather than just the raw OP token.
//
// otherIdx is 1-based among the OTHERS (O1, O2, ...) and is included
// in the label so multi-OTHER runs are scannable.
func formatCompareFailure(args cliArgs, otherIdx int, baseRI resolvedInput, base Version, otherRI resolvedInput, other Version) string {
	opPhrase := compareOpPhrase(args.compareOp, args.comparePrecision)
	op := args.compareOp
	if args.comparePrecision != "" {
		op = args.compareOp + "-" + args.comparePrecision
	}
	return fmt.Sprintf("compare %s: %s (%s) is %s O%d=%s (%s)",
		op,
		baseRI.originFile, base.String(),
		opPhrase,
		otherIdx, otherRI.originFile, other.String(),
	)
}

// compareOpPhrase maps an operator + precision to a negated English
// phrase used in the per-OTHER stderr listing ("not greater than",
// "not less than or equal to (major)", ...).
func compareOpPhrase(op, precision string) string {
	base := "not " + map[string]string{
		"eq": "equal to",
		"lt": "less than",
		"le": "less than or equal to",
		"gt": "greater than",
		"ge": "greater than or equal to",
	}[op]
	if precision == "" {
		return base
	}
	return base + " (" + precision + ")"
}

// collapseToOneVersion checks that all detected version fields agree,
// then returns the parsed Version. Mismatches become "version mismatch:"
// errors via formatMismatchError, exactly like the bump path.
func collapseToOneVersion(ri resolvedInput) (Version, error) {
	val, ok := allSameValue(ri.fields)
	if !ok {
		return Version{}, formatMismatchError("version", ri.fields)
	}
	v, err := ParseVersion(val)
	if err != nil {
		// Use the first field as the origin label for context.
		label := ri.originFile
		if len(ri.fields) > 0 {
			label = ri.fields[0].label()
		}
		return Version{}, wrapOriginErr(label, val, err)
	}
	return v, nil
}

func evalCompareOp(op string, cmp int) bool {
	switch op {
	case "eq":
		return cmp == 0
	case "lt":
		return cmp < 0
	case "le":
		return cmp <= 0
	case "gt":
		return cmp > 0
	case "ge":
		return cmp >= 0
	}
	return false
}
