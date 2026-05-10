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
func runCompare(args cliArgs, stdin io.Reader, stdout io.Writer) error {
	if len(args.inputs) != 2 {
		return fmt.Errorf("compare requires exactly two inputs, got %d", len(args.inputs))
	}
	resolved, err := resolveInputs(args.inputs, stdin, false)
	if err != nil {
		return err
	}
	if len(resolved) != 2 {
		return fmt.Errorf("compare: internal: expected 2 resolved inputs, got %d", len(resolved))
	}

	left, err := collapseToOneVersion(resolved[0])
	if err != nil {
		return err
	}
	right, err := collapseToOneVersion(resolved[1])
	if err != nil {
		return err
	}

	cmp := left.Compare(right)
	if evalCompareOp(args.compareOp, cmp) {
		// stdout is intentionally empty for compare; the caller reads
		// the exit code. No newline avoids polluting pipelines.
		_ = stdout
		return nil
	}
	// Predicate false: exit 1, no message.
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
