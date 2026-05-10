// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json /
// package-lock.json / VERSION) and provides five flat actions plus a
// nested `compare` subcommand:
//
//   - bump: major / minor / patch / pre
//   - read: get
//   - cmp:  compare {eq|lt|gt|le|ge}
//
// Inputs are positional: each argument may be a FILE path, a raw
// semver VER (e.g. `1.2.3`), or `-` (read VER from stdin once). When
// multiple inputs are given the values must agree; FILE-origin entries
// can be written back with `--write`, VER/stdin-origin entries are
// reference values only.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

const helpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <ACTION> <INPUT...> [flags]
  bump-semver compare <OP> <INPUT> <INPUT> [flags]
  bump-semver --version
  bump-semver --help

Actions (bump/read):
  major   Bump major (X.0.0); pre-release / build-metadata dropped by default
  minor   Bump minor (x.Y.0); pre-release / build-metadata dropped by default
  patch   Bump patch (x.y.Z); pre-release / build-metadata dropped by default
  pre     Pre-release counter advance / set / remove (see --pre / --no-pre)
  get     Print the current version (with optional --no-pre / --no-build-metadata)

Compare (nested subcommand):
  compare eq  INPUT INPUT     true if equal (SemVer 2.0.0 ordering, build metadata ignored)
  compare lt  INPUT INPUT     true if first <  second
  compare le  INPUT INPUT     true if first <= second
  compare gt  INPUT INPUT     true if first >  second
  compare ge  INPUT INPUT     true if first >= second

Inputs:
  FILE     path to a supported file (auto-detected by basename)
  VER      a raw semver string (e.g. 1.2.3, v1.2.3, 1.2.3-rc.1+build.42)
  -        read VER from stdin (single line, used at most once)

Flags:
  --pre PRE              Set pre-release identifiers (e.g. --pre rc.0)
  --no-pre               Remove pre-release identifiers
  --build-metadata META  Set build metadata identifiers (e.g. --build-metadata sha.abc)
  --no-build-metadata    Remove build metadata identifiers
  --write                Write the new version back to each FILE input (bump only)
  --version, -V          Print the binary version
  --help, -h             Show this help

Supported file formats (auto-detected by basename):
  Cargo.toml         TOML, [package].version (and [package].name for cross-input checks)
  package-lock.json  npm 7+ lockfile, $.version + $.packages[""].version (deps untouched)
  *.json             JSON, $.version (and optional $.name)
  VERSION            plain text

Multiple inputs (FILE / VER / -) may be mixed. All extracted versions must
agree; otherwise a "version mismatch:" error lists each origin and value.
With --write, only FILE-origin inputs are written back.

Exit codes:
  0   success (or compare predicate true)
  1   compare predicate false
  2   error (parse failure, mismatch, missing input, etc.)

Examples:
  bump-semver patch Cargo.toml --write
  bump-semver minor package.json package-lock.json --write
  bump-semver get .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
  bump-semver patch 1.2.3
  bump-semver patch v1.2.3                       # v1.2.4 (prefix preserved)
  bump-semver pre 1.2.3-rc.0                     # 1.2.3-rc.1
  bump-semver pre 1.2.3 --pre rc.0               # 1.2.3-rc.0
  bump-semver patch Cargo.toml --pre rc.0        # 1.2.4-rc.0 (pre re-attached)
  bump-semver compare lt 1.2.3-rc.1 1.2.3        # exit 0
  bump-semver compare eq Cargo.toml package.json # cross-file equality
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		var ee *exitErr
		if errors.As(err, &ee) {
			if ee.msg != "" {
				fmt.Fprintln(os.Stderr, "bump-semver: "+ee.msg)
			}
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "bump-semver: "+err.Error())
		os.Exit(2)
	}
}

// exitErr carries an explicit exit code through the call stack so we
// can distinguish "compare predicate is false" (exit 1) from "an error
// occurred" (exit 2). Plain errors propagate as exit 2 in main.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }
func (e *exitErr) ExitCode() int { return e.code }

// cliArgs is the parsed command-line.
type cliArgs struct {
	kind      string // "bump" | "compare" | "version" | "help"
	action    string // bump 時: "major"/"minor"/"patch"/"pre"/"get"
	compareOp string // compare 時: "eq"/"lt"/"gt"/"le"/"ge"
	inputs    []string
	write     bool

	pre              string
	preSet           bool
	noPre            bool
	buildMetadata    string
	buildMetadataSet bool
	noBuildMetadata  bool
}

var bumpActions = map[string]bool{
	"major": true, "minor": true, "patch": true, "pre": true, "get": true,
}

var compareOps = map[string]bool{
	"eq": true, "lt": true, "le": true, "gt": true, "ge": true,
}

func parseArgs(argv []string) (cliArgs, error) {
	if len(argv) == 0 {
		return cliArgs{kind: "help"}, nil
	}
	switch argv[0] {
	case "--version", "-V":
		return cliArgs{kind: "version"}, nil
	case "--help", "-h":
		return cliArgs{kind: "help"}, nil
	}

	out := cliArgs{}
	var rest []string
	if argv[0] == "compare" {
		out.kind = "compare"
		if len(argv) < 2 {
			return cliArgs{}, fmt.Errorf("compare requires an operator (eq|lt|le|gt|ge)")
		}
		op := argv[1]
		if !compareOps[op] {
			return cliArgs{}, fmt.Errorf("unknown compare operator: %s (expected one of eq|lt|le|gt|ge)", op)
		}
		out.compareOp = op
		rest = argv[2:]
	} else {
		out.kind = "bump"
		if !bumpActions[argv[0]] {
			return cliArgs{}, fmt.Errorf("unknown action: %s (expected one of major|minor|patch|pre|get|compare)", argv[0])
		}
		out.action = argv[0]
		rest = argv[1:]
	}

	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--write":
			if out.write {
				return cliArgs{}, fmt.Errorf("--write specified twice")
			}
			out.write = true
		case a == "--pre":
			if out.preSet {
				return cliArgs{}, fmt.Errorf("--pre specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--pre requires a value")
			}
			out.pre = rest[i+1]
			out.preSet = true
			i++
		case strings.HasPrefix(a, "--pre="):
			if out.preSet {
				return cliArgs{}, fmt.Errorf("--pre specified twice")
			}
			out.pre = strings.TrimPrefix(a, "--pre=")
			out.preSet = true
		case a == "--no-pre":
			if out.noPre {
				return cliArgs{}, fmt.Errorf("--no-pre specified twice")
			}
			out.noPre = true
		case a == "--build-metadata":
			if out.buildMetadataSet {
				return cliArgs{}, fmt.Errorf("--build-metadata specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--build-metadata requires a value")
			}
			out.buildMetadata = rest[i+1]
			out.buildMetadataSet = true
			i++
		case strings.HasPrefix(a, "--build-metadata="):
			if out.buildMetadataSet {
				return cliArgs{}, fmt.Errorf("--build-metadata specified twice")
			}
			out.buildMetadata = strings.TrimPrefix(a, "--build-metadata=")
			out.buildMetadataSet = true
		case a == "--no-build-metadata":
			if out.noBuildMetadata {
				return cliArgs{}, fmt.Errorf("--no-build-metadata specified twice")
			}
			out.noBuildMetadata = true
		case a == "--":
			// Treat all remaining argv as inputs (lets paths starting with `-` through).
			out.inputs = append(out.inputs, rest[i+1:]...)
			i = len(rest)
		case strings.HasPrefix(a, "-") && a != "-":
			return cliArgs{}, fmt.Errorf("unknown option: %s", a)
		default:
			out.inputs = append(out.inputs, a)
		}
	}

	// --- exclusivity / validity checks ---------------------------------

	if out.preSet && out.noPre {
		return cliArgs{}, fmt.Errorf("--pre and --no-pre are mutually exclusive")
	}
	if out.buildMetadataSet && out.noBuildMetadata {
		return cliArgs{}, fmt.Errorf("--build-metadata and --no-build-metadata are mutually exclusive")
	}
	if out.preSet && out.pre == "" {
		return cliArgs{}, fmt.Errorf("--pre value cannot be empty, use --no-pre to remove")
	}
	if out.buildMetadataSet && out.buildMetadata == "" {
		return cliArgs{}, fmt.Errorf("--build-metadata value cannot be empty, use --no-build-metadata to remove")
	}

	if out.kind == "compare" {
		if out.write {
			return cliArgs{}, fmt.Errorf("--write is not valid with compare")
		}
		if out.preSet {
			return cliArgs{}, fmt.Errorf("--pre is not valid with compare")
		}
		if out.buildMetadataSet {
			return cliArgs{}, fmt.Errorf("--build-metadata is not valid with compare")
		}
		if len(out.inputs) != 2 {
			return cliArgs{}, fmt.Errorf("compare requires exactly two inputs, got %d", len(out.inputs))
		}
		return out, nil
	}

	// bump path.
	if out.action == "get" {
		if out.write {
			return cliArgs{}, fmt.Errorf("--write is not valid with get")
		}
		if out.preSet {
			return cliArgs{}, fmt.Errorf("--pre is not valid with get (use --no-pre to strip)")
		}
		if out.buildMetadataSet {
			return cliArgs{}, fmt.Errorf("--build-metadata is not valid with get (use --no-build-metadata to strip)")
		}
	}
	if len(out.inputs) == 0 {
		return cliArgs{}, fmt.Errorf("at least one input (FILE | VER | -) is required")
	}
	return out, nil
}

// locatedField is one detected version-or-name value, annotated with
// the origin label used for display in mismatch errors.
type locatedField struct {
	// File is the origin label. For FILE-origin fields it's the file
	// path; for VER-origin fields it's "<argv>" or "<argv:N>"; for
	// stdin (`-`) origin it's "<stdin>". Path is the in-file location
	// (e.g. "$.version") or empty for VER/stdin origins.
	File, Path, Value string
}

// label returns the human-readable origin label for column-aligned
// mismatch error rendering. FILE: "<file>:<path>"; non-FILE: just File.
func (lf locatedField) label() string {
	if lf.Path == "" {
		return lf.File
	}
	return lf.File + ":" + lf.Path
}

func locatedFromInspection(file string, fields []Field) []locatedField {
	out := make([]locatedField, 0, len(fields))
	for _, f := range fields {
		out = append(out, locatedField{File: file, Path: f.Path, Value: f.Value})
	}
	return out
}

func allSameValue(items []locatedField) (string, bool) {
	if len(items) == 0 {
		return "", true
	}
	first := items[0].Value
	for _, x := range items[1:] {
		if x.Value != first {
			return first, false
		}
	}
	return first, true
}

// formatMismatchError renders a "<kind> mismatch:" error with column-
// aligned origin labels (確定論点 F).
func formatMismatchError(kind string, items []locatedField) error {
	maxW := 0
	for _, x := range items {
		if w := len(x.label()); w > maxW {
			maxW = w
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s mismatch:", kind)
	for _, x := range items {
		fmt.Fprintf(&sb, "\n  %-*s = %s", maxW, x.label(), x.Value)
	}
	return errors.New(sb.String())
}

// resolvedInput is one positional input fully resolved into its origin
// label and the version field(s) it contributes to consistency checks.
type resolvedInput struct {
	originFile string // value used for locatedField.File on every contribution
	fields     []locatedField

	// FILE-origin only — needed for --write.
	file    string
	handler Handler
	content []byte
	insp    Inspection
}

// stdinReadState tracks at-most-once stdin consumption for `-` inputs.
type stdinReadState struct {
	consumed bool
	value    string
	err      error
}

// resolveInput interprets one positional argument according to the
// precedence rules from 確定論点 B:
//  1. `-`             → read VER from stdin (once)
//  2. file exists     → FILE
//  3. ParseVersion OK → VER
//  4. otherwise       → error
//
// argIdx is the 1-based positional index used to disambiguate VER
// labels when there are multiple raw VERs ("<argv:2>" etc).
func resolveInput(arg string, argIdx, totalVERorStdin int, stdin io.Reader, st *stdinReadState) (resolvedInput, error) {
	if arg == "-" {
		if !st.consumed {
			st.consumed = true
			st.value, st.err = readStdinLine(stdin)
		}
		if st.err != nil {
			return resolvedInput{}, st.err
		}
		v, err := ParseVersion(st.value)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("<stdin>: %w", err)
		}
		ri := resolvedInput{originFile: "<stdin>"}
		ri.fields = []locatedField{{File: ri.originFile, Value: v.String()}}
		return ri, nil
	}

	// Try as file first if it exists. Use Stat so we don't masquerade
	// directories or sockets as parseable VERs.
	if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
		return resolveFile(arg)
	}

	// Try as VER.
	if v, err := ParseVersion(arg); err == nil {
		label := "<argv>"
		if totalVERorStdin > 1 {
			label = fmt.Sprintf("<argv:%d>", argIdx)
		}
		ri := resolvedInput{originFile: label}
		ri.fields = []locatedField{{File: ri.originFile, Value: v.String()}}
		// Preserve the input string verbatim so prefix/sep round-trip.
		ri.fields[0].Value = strings.TrimSpace(arg)
		return ri, nil
	}

	return resolvedInput{}, fmt.Errorf("%q is neither a file nor a valid version", arg)
}

func resolveFile(file string) (resolvedInput, error) {
	h, err := detectHandler(file)
	if err != nil {
		return resolvedInput{}, err
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("read %s: %w", file, err)
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, err
	}
	ri := resolvedInput{
		originFile: file,
		fields:     locatedFromInspection(file, insp.Versions),
		file:       file,
		handler:    h,
		content:    content,
		insp:       insp,
	}
	return ri, nil
}

// resolveFileFromStdin handles the legacy "single FILE + stdin pipe"
// shortcut: the path is treated as a name hint and content is read
// from stdin.
func resolveFileFromStdin(file string, stdin io.Reader) (resolvedInput, error) {
	h, err := detectHandler(file)
	if err != nil {
		return resolvedInput{}, err
	}
	content, err := io.ReadAll(stdin)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("read stdin: %w", err)
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, err
	}
	ri := resolvedInput{
		originFile: file,
		fields:     locatedFromInspection(file, insp.Versions),
		file:       "", // stdin pipe: do not allow writeback (already rejected at parse time)
		handler:    h,
		content:    content,
		insp:       insp,
	}
	return ri, nil
}

func readStdinLine(stdin io.Reader) (string, error) {
	br := bufio.NewReader(stdin)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("stdin: empty input")
	}
	return line, nil
}

// wrapOriginErr prefixes a semver-layer error with its origin context
// (確定論点 E). For FILE-origin inputs we include the version path.
func wrapOriginErr(originLabel, value string, err error) error {
	if err == nil {
		return nil
	}
	// VER / stdin origin: keep the message as-is.
	if !strings.Contains(originLabel, ":") && (originLabel == "<argv>" ||
		strings.HasPrefix(originLabel, "<argv:") || originLabel == "<stdin>") {
		if originLabel == "<stdin>" {
			return fmt.Errorf("<stdin> (%s): %w", value, err)
		}
		return err
	}
	// FILE-origin: "<file>:<path>=<value>: <semver-error>" form.
	return fmt.Errorf("%s=%s: %w", originLabel, value, err)
}

func run(argv []string, stdin io.Reader, stdout io.Writer) error {
	args, err := parseArgs(argv)
	if err != nil {
		return err
	}
	switch args.kind {
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help":
		fmt.Fprint(stdout, helpText)
		return nil
	case "compare":
		return runCompare(args, stdin, stdout)
	}

	return runBump(args, stdin, stdout)
}

func runBump(args cliArgs, stdin io.Reader, stdout io.Writer) error {
	resolved, err := resolveInputs(args.inputs, stdin, args.write)
	if err != nil {
		return err
	}

	// Validate --write has at least one FILE-origin input early, before
	// any side effects (printing the bumped version).
	if args.write {
		writable := 0
		for _, ri := range resolved {
			if ri.handler != nil && ri.file != "" {
				writable++
			}
		}
		if writable == 0 {
			return fmt.Errorf("--write requires at least one FILE")
		}
	}

	// Aggregate every detected version field, across all inputs, with
	// origin provenance.
	var allVersions []locatedField
	for _, ri := range resolved {
		allVersions = append(allVersions, ri.fields...)
	}
	cur, ok := allSameValue(allVersions)
	if !ok {
		return formatMismatchError("version", allVersions)
	}

	// Aggregate names across FILE-origin entries (VER/stdin contribute none).
	var allNames []locatedField
	for _, ri := range resolved {
		if ri.file != "" || ri.handler != nil {
			allNames = append(allNames, locatedFromInspection(ri.originFile, ri.insp.Names)...)
		}
	}
	if _, ok := allSameValue(allNames); len(allNames) > 0 && !ok {
		return formatMismatchError("name", allNames)
	}

	// Use the first contributing field as the origin source for parse
	// errors (any one would work; they're all equal by construction).
	origin := allVersions[0]

	v, err := ParseVersion(cur)
	if err != nil {
		return wrapOriginErr(origin.label(), cur, err)
	}
	opts := BumpOptions{
		Pre:              args.pre,
		PreSet:           args.preSet,
		NoPre:            args.noPre,
		BuildMetadata:    args.buildMetadata,
		BuildMetadataSet: args.buildMetadataSet,
		NoBuildMetadata:  args.noBuildMetadata,
	}
	newV, err := v.Bump(args.action, opts)
	if err != nil {
		return wrapOriginErr(origin.label(), cur, err)
	}
	fmt.Fprintln(stdout, newV.String())

	if args.write {
		for _, ri := range resolved {
			if ri.handler == nil || ri.file == "" {
				continue
			}
			out, err := ri.handler.Replace(ri.content, cur, newV.String())
			if err != nil {
				return fmt.Errorf("replace %s: %w", ri.file, err)
			}
			mode := os.FileMode(0644)
			if fi, statErr := os.Stat(ri.file); statErr == nil {
				mode = fi.Mode().Perm()
			}
			if err := os.WriteFile(ri.file, out, mode); err != nil {
				return fmt.Errorf("write %s: %w", ri.file, err)
			}
		}
	}
	return nil
}

// resolveInputs walks the positional inputs and resolves each. It also
// handles the legacy "single FILE + stdin pipe" shortcut: if exactly
// one input is present, that input is a FILE path (not `-`, not a VER
// pattern), and stdin is a pipe, the FILE's content is read from stdin.
func resolveInputs(inputs []string, stdin io.Reader, write bool) ([]resolvedInput, error) {
	// Pre-classify each input as "looks like a file on disk" vs "raw"
	// (VER or `-`). The classification is used both for the
	// legacy-stdin-pipe shortcut and for VER label disambiguation
	// (`<argv>` vs `<argv:N>`).
	isRaw := make([]bool, len(inputs))
	rawCount := 0
	for i, in := range inputs {
		if in == "-" {
			isRaw[i] = true
			rawCount++
			continue
		}
		if fi, err := os.Stat(in); err == nil && !fi.IsDir() {
			continue // exists as a file
		}
		isRaw[i] = true
		rawCount++
	}

	// Legacy stdin pipe shortcut: one FILE input (not `-`, exists or
	// at least matches a known rule), stdin is a pipe → read content
	// from stdin and treat the path as a name hint.
	if len(inputs) == 1 && inputs[0] != "-" && isStdinPipe(stdin) {
		if write {
			return nil, fmt.Errorf("--write is incompatible with stdin pipe input")
		}
		if pathHasAnyRule(inputs[0]) {
			ri, err := resolveFileFromStdin(inputs[0], stdin)
			if err != nil {
				return nil, err
			}
			return []resolvedInput{ri}, nil
		}
	}

	st := stdinReadState{}
	out := make([]resolvedInput, 0, len(inputs))
	rawIdx := 0
	for i, in := range inputs {
		argIdx := 0
		if isRaw[i] {
			rawIdx++
			argIdx = rawIdx
		}
		ri, err := resolveInput(in, argIdx, rawCount, stdin, &st)
		if err != nil {
			return nil, err
		}
		out = append(out, ri)
	}
	return out, nil
}

func isStdinPipe(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}
