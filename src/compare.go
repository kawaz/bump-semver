package main

import (
	"fmt"
	"io"
)

// runCompare implements the `compare {eq|lt|le|gt|ge}` subcommand.
//
// Both inputs are resolved with the same FILE | VER | `-` rules as
// `bump`. When an input contributes multiple version fields (e.g. a
// package-lock.json with $.version and $.packages[""].version), the
// fields are checked for equality and collapsed to a single value
// before comparison.
//
// Exit codes (DR-0006 確定論点 A):
//   - 0  predicate true
//   - 1  predicate false
//   - 2  any error (parse failure, mismatch, missing input, ...)
//
// Quiet flags (Phase 5 + DR-0010):
//   - -q / --quiet:   suppresses the DR-0010 fallback / unsupported-file
//     hints (compare has no other stdout to suppress).
//   - -qq / --quiet-all: also suppresses the stderr "bump-semver: ..."
//     diagnostic emitted on exit-2 errors. Predicate-false exit-1 has
//     no diagnostic to begin with, so quiet flags do not affect it.
//   - --no-hint:      suppresses the DR-0010 fallback / unsupported-file
//     hints. compare has no "files not modified" hint to suppress.
func runCompare(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	_ = stdout // compare prints nothing on success; consumers read the exit code.
	if len(args.inputs) != 2 {
		return emitErr(stderr, args, fmt.Errorf("compare requires exactly two inputs, got %d", len(args.inputs)))
	}
	vcsOverride, _ := parseVcsOverride(args.vcs) // already validated in parseArgs
	resolved, err := resolveInputs(args.inputs, stdin, false, vcsOverride)
	if err != nil {
		return emitErr(stderr, args, err)
	}
	if len(resolved) != 2 {
		return emitErr(stderr, args, fmt.Errorf("compare: internal: expected 2 resolved inputs, got %d", len(resolved)))
	}

	// DR-0010: surface confidence-1 fallback matches for compare too —
	// the hint reflects "you used an unknown filename", not a property
	// of the bump action. Suppression flags handled inside the helper.
	emitFallbackHints(stderr, args, resolved)

	left, err := collapseToOneVersion(resolved[0])
	if err != nil {
		return emitErr(stderr, args, err)
	}
	right, err := collapseToOneVersion(resolved[1])
	if err != nil {
		return emitErr(stderr, args, err)
	}

	cmp := left.CompareAt(right, args.comparePrecision)
	if evalCompareOp(args.compareOp, cmp) {
		return nil
	}
	// Predicate false: exit 1, no diagnostic. quietAll has nothing to
	// suppress here (the documented contract is "false has no message").
	return &exitErr{code: 1}
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
