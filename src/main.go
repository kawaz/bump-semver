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
// semver VER (e.g. `1.2.3`), `-` (read VER from stdin once), or
// `vcs:REV[:FILE]` / `vcs:<func>(...)` (read version from the VCS;
// DR-0008). When multiple inputs are given the values must agree;
// FILE-origin entries can be written back with `--write`, VER /
// stdin / vcs entries are reference values only.
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

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		var ee *exitErr
		if errors.As(err, &ee) {
			// run() is responsible for writing any user-facing error
			// message to stderr (so it can honor --quiet-all). main only
			// translates the carried code into the process exit status.
			os.Exit(ee.code)
		}
		// Unexpected: run() must always wrap errors as *exitErr before
		// returning. Defensive fallback so we still exit non-zero.
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
	kind             string // "bump" | "compare" | "vcs" | "version" | "help" | "helpFull" | "helpAction"
	action           string // bump 時: "major"/"minor"/"patch"/"pre"/"get"; vcs 時: "get" / ...
	compareOp        string // compare 時 base: "eq"/"lt"/"gt"/"le"/"ge"
	comparePrecision string // compare 時 precision (DR-0017): "" / "major" / "minor" / "patch"
	vcsVerb          string // vcs 時 1st verb (e.g. "get"); "" = parent-level (show help)
	vcsArgs          []string
	inputs           []string
	write            bool

	pre              string
	preSet           bool
	noPre            bool
	buildMetadata    string
	buildMetadataSet bool
	noBuildMetadata  bool

	// Output suppression flags (Phase 5).
	//
	// Precedence: quietAll > quiet > noHint. -qq and -q given together
	// collapse to quietAll silently (-qq is a strict superset of -q);
	// likewise --no-hint with -q/-qq is absorbed by the quiet flag (which
	// already suppresses the hint).
	quiet    bool // -q / --quiet:    suppress stdout + hint
	quietAll bool // -qq / --quiet-all: also suppress error output
	noHint   bool // --no-hint:        suppress only the hint

	// Structured-output flag (DR-0007). When true, runBump emits a
	// single-line JSON rendering of the bumped/get version instead of
	// the bare String(). Rejected for compare (predicate-only output).
	json bool // --json

	// VCS override (DR-0008). When non-empty and not "auto", takes
	// priority over the auto-probe (`.jj` / `.git`). Accepted values:
	// "jj" / "git" / "auto" (auto and "" both fall through to probing).
	vcs    string // --vcs value (validated in parseArgs)
	vcsSet bool   // whether --vcs was supplied at all

	// vcsDiffNameStatus enables `vcs diff -s/--name-status`: instead of a
	// raw patch, emit one `<CODE>\t<path>` line per changed file (git
	// --name-status shape). When combined with -q/--quiet, -q wins (stdout
	// suppressed; exit code still reflects diff presence). Verb-local —
	// parsed in the vcs branch and only consumed by runVcsCmdDiff.
	vcsDiffNameStatus bool

	// vcsCommitMessage / vcsCommitMessageSet / vcsCommitStaged /
	// vcsCommitAmend / vcsCommitDashA: DR-0020 PR-4. Verb-local to
	// `vcs commit`. -a is parsed only to give a tailored exit-2
	// rejection (kawaz CLI safety: --staged is the supported "all"
	// mode; -a's unstaged-grabbing semantic is intentionally absent).
	vcsCommitMessage    string
	vcsCommitMessageSet bool
	vcsCommitStaged     bool
	vcsCommitAmend      bool
	vcsCommitDashA      bool
}

var bumpActions = map[string]bool{
	"major": true, "minor": true, "patch": true, "pre": true, "get": true,
}

var compareOps = map[string]bool{
	"eq": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// comparePrecisions enumerates DR-0017 precision suffixes. Empty
// string (full SemVer comparison) is implicit and not represented
// here.
var comparePrecisions = map[string]bool{
	"major": true, "minor": true, "patch": true,
}

// parseCompareOp splits a CLI compare operator into its base ("eq" /
// "lt" / etc.) and optional precision suffix ("major" / "minor" /
// "patch", DR-0017). An empty precision means SemVer-full comparison.
// Returns ok=false for any unrecognised combination so the caller can
// emit a uniform error.
func parseCompareOp(s string) (base, precision string, ok bool) {
	if compareOps[s] {
		return s, "", true
	}
	if i := strings.LastIndex(s, "-"); i > 0 {
		b, p := s[:i], s[i+1:]
		if compareOps[b] && comparePrecisions[p] {
			return b, p, true
		}
	}
	return "", "", false
}

func parseArgs(argv []string) (cliArgs, error) {
	if len(argv) == 0 {
		return cliArgs{kind: "help"}, nil
	}
	switch argv[0] {
	case "--version", "-V":
		out := cliArgs{kind: "version"}
		// --version は他フラグを基本受け付けないが、--json だけは
		// バイナリ自身のバージョンを構造化 JSON で出力する用に解釈する
		// (CI で `bump-semver --version --json | jq -r .semver` のような使い方)
		for _, a := range argv[1:] {
			if a == "--json" {
				out.json = true
				continue
			}
			return cliArgs{}, fmt.Errorf("--version only accepts --json")
		}
		return out, nil
	case "--help", "-h":
		return cliArgs{kind: "help"}, nil
	case "--help-full":
		return cliArgs{kind: "helpFull"}, nil
	}

	out := cliArgs{}
	var rest []string
	if argv[0] == "vcs" {
		// `vcs` is a two-tier subcommand (vcs <verb> [args...]) — we
		// parse it specially because the existing flat-action grammar
		// doesn't fit. Help routing:
		//
		//   bump-semver vcs            → show vcs parent help
		//   bump-semver vcs --help     → show vcs parent help
		//   bump-semver vcs get        → show vcs get help (no key given)
		//   bump-semver vcs <verb> --help → show vcs <verb> help
		//   bump-semver vcs <verb> <args...> → dispatch to runVcsCmd
		out.kind = "vcs"
		if len(argv) == 1 {
			return cliArgs{kind: "helpAction", action: "vcs"}, nil
		}
		if argv[1] == "--help" || argv[1] == "-h" {
			return cliArgs{kind: "helpAction", action: "vcs"}, nil
		}
		out.vcsVerb = argv[1]
		// Per-verb help only for known verbs — unknown verbs must
		// surface as an exit-2 error, not as a silent help fallthrough.
		// We route them to runVcsCmd which emits the proper usage error.
		isKnownVerb := out.vcsVerb == "get" || out.vcsVerb == "is" || out.vcsVerb == "diff" || out.vcsVerb == "commit"
		if len(argv) == 2 {
			if isKnownVerb {
				return cliArgs{kind: "helpAction", action: "vcs " + out.vcsVerb}, nil
			}
			return out, nil
		}
		if argv[2] == "--help" || argv[2] == "-h" {
			if isKnownVerb {
				return cliArgs{kind: "helpAction", action: "vcs " + out.vcsVerb}, nil
			}
			return out, nil
		}
		// Split flags from positional vcsArgs. The vcs branch supports a
		// curated subset of the global flags: --vcs (override), -q/-qq,
		// --no-hint. `-s/--name-status` is verb-local to `vcs diff`.
		// Anything else is reported as an unknown flag (exit 2, names the
		// verb in the hint so typos like `vcs get -s root` are caught).
		// Unlike the main flat-action grammar, we don't process --pre /
		// --write etc. here — those are bump-only.
		//
		// Design rationale: there is exactly one verb-local flag, so a
		// `(verb == "diff")` guard inline is simpler than a verb→flags
		// table. If verb-local flags grow, refactor to a table keyed by
		// verb (see DR-0020 implementation notes).
		rest := argv[2:]
		for i := 0; i < len(rest); i++ {
			a := rest[i]
			switch {
			case a == "--vcs":
				if out.vcsSet {
					return cliArgs{}, fmt.Errorf("--vcs specified twice")
				}
				if i+1 >= len(rest) {
					return cliArgs{}, fmt.Errorf("--vcs requires a value (jj, git, or auto)")
				}
				out.vcs = rest[i+1]
				out.vcsSet = true
				i++
			case strings.HasPrefix(a, "--vcs="):
				if out.vcsSet {
					return cliArgs{}, fmt.Errorf("--vcs specified twice")
				}
				out.vcs = strings.TrimPrefix(a, "--vcs=")
				out.vcsSet = true
			case a == "-q", a == "--quiet":
				out.quiet = true
			case a == "-qq", a == "--quiet-all":
				out.quietAll = true
			case (a == "-s" || a == "--name-status") && out.vcsVerb == "diff":
				// Verb-local to `vcs diff` only. For other verbs this
				// case is skipped and the generic unknown-flag catch-all
				// below rejects with exit 2.
				out.vcsDiffNameStatus = true
			case a == "-m" && out.vcsVerb == "commit":
				// Verb-local to `vcs commit`. Takes a value.
				if out.vcsCommitMessageSet {
					return cliArgs{}, fmt.Errorf("-m specified twice")
				}
				if i+1 >= len(rest) {
					return cliArgs{}, fmt.Errorf("-m requires a value (commit message)")
				}
				out.vcsCommitMessage = rest[i+1]
				out.vcsCommitMessageSet = true
				i++
			case strings.HasPrefix(a, "-m=") && out.vcsVerb == "commit":
				if out.vcsCommitMessageSet {
					return cliArgs{}, fmt.Errorf("-m specified twice")
				}
				out.vcsCommitMessage = strings.TrimPrefix(a, "-m=")
				out.vcsCommitMessageSet = true
			case a == "--message" && out.vcsVerb == "commit":
				if out.vcsCommitMessageSet {
					return cliArgs{}, fmt.Errorf("--message specified twice")
				}
				if i+1 >= len(rest) {
					return cliArgs{}, fmt.Errorf("--message requires a value")
				}
				out.vcsCommitMessage = rest[i+1]
				out.vcsCommitMessageSet = true
				i++
			case strings.HasPrefix(a, "--message=") && out.vcsVerb == "commit":
				if out.vcsCommitMessageSet {
					return cliArgs{}, fmt.Errorf("--message specified twice")
				}
				out.vcsCommitMessage = strings.TrimPrefix(a, "--message=")
				out.vcsCommitMessageSet = true
			case a == "--staged" && out.vcsVerb == "commit":
				out.vcsCommitStaged = true
			case a == "--amend" && out.vcsVerb == "commit":
				out.vcsCommitAmend = true
			case (a == "-a" || a == "--all") && out.vcsVerb == "commit":
				// Captured here only so we can give a tailored exit-2
				// rejection in runVcsCmdCommit (instead of the generic
				// "unknown flag" message). DR-0020: -a is intentionally
				// non-provided to prevent unstaged-grab accidents in
				// jj's auto-staged world.
				out.vcsCommitDashA = true
			case a == "--no-hint":
				out.noHint = true
			case a == "--":
				out.vcsArgs = append(out.vcsArgs, rest[i+1:]...)
				i = len(rest)
			case strings.HasPrefix(a, "-") && a != "-":
				return cliArgs{}, fmt.Errorf("unknown flag for 'vcs %s': %s", out.vcsVerb, a)
			default:
				out.vcsArgs = append(out.vcsArgs, a)
			}
		}
		if out.vcsSet {
			if _, err := parseVcsOverride(out.vcs); err != nil {
				return cliArgs{}, err
			}
		}
		return out, nil
	}
	if argv[0] == "compare" {
		// `bump-semver compare --help` / `compare -h`: アクション固有 help
		// に短絡 (OP の解釈は始めない)。OP 後に置かれた `--help` は通常の
		// rest 走査で拾う。
		if len(argv) >= 2 && (argv[1] == "--help" || argv[1] == "-h") {
			return cliArgs{kind: "helpAction", action: "compare"}, nil
		}
		out.kind = "compare"
		if len(argv) < 2 {
			return cliArgs{}, fmt.Errorf("compare requires an operator (eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)")
		}
		op := argv[1]
		base, precision, ok := parseCompareOp(op)
		if !ok {
			return cliArgs{}, fmt.Errorf("unknown compare operator: %s (expected eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)", op)
		}
		out.compareOp = base
		out.comparePrecision = precision
		rest = argv[2:]
	} else {
		out.kind = "bump"
		if !bumpActions[argv[0]] {
			return cliArgs{}, fmt.Errorf("unknown action: %s (expected one of major|minor|patch|pre|get|compare)", argv[0])
		}
		out.action = argv[0]
		rest = argv[1:]
	}
	// `bump-semver <action> --help` / `<action> -h` (compare の OP 後も含む):
	// rest 先頭で検出してアクション固有 help に短絡。
	if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
		helpAction := out.action
		if out.kind == "compare" {
			helpAction = "compare"
		}
		return cliArgs{kind: "helpAction", action: helpAction}, nil
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
		case a == "--no-hint":
			// Idempotent: silently absorb duplicates rather than erroring,
			// to match the "no-op flags are silently accepted" policy from
			// Phase 5 (a -qq subsumes --no-hint anyway).
			out.noHint = true
		case a == "-q", a == "--quiet":
			out.quiet = true
		case a == "-qq", a == "--quiet-all":
			out.quietAll = true
		case a == "--json":
			// Idempotent: silently absorb duplicates. Same policy as
			// --no-hint — boolean flags don't benefit from a strict
			// double-set check (no value is being lost).
			out.json = true
		case a == "--vcs":
			if out.vcsSet {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--vcs requires a value (jj, git, or auto)")
			}
			out.vcs = rest[i+1]
			out.vcsSet = true
			i++
		case strings.HasPrefix(a, "--vcs="):
			if out.vcsSet {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			out.vcs = strings.TrimPrefix(a, "--vcs=")
			out.vcsSet = true
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
	if out.vcsSet {
		if _, err := parseVcsOverride(out.vcs); err != nil {
			return cliArgs{}, err
		}
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
		// DR-0007: compare is a predicate-only command — exit code is
		// the answer, stdout is intentionally empty. There is nothing
		// to render as JSON.
		if out.json {
			return cliArgs{}, fmt.Errorf("compare does not support --json")
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
// precedence rules from 確定論点 B (extended for DR-0008's `vcs:` input):
//  1. `-`             → read VER from stdin (once)
//  2. `vcs:...`       → resolve via VCS (DR-0008)
//  3. file exists     → FILE
//  4. ParseVersion OK → VER
//  5. otherwise       → error
//
// argIdx is the 1-based positional index used to disambiguate VER
// labels when there are multiple raw VERs ("<argv:2>" etc).
//
// borrowedFile is the path used when a `vcs:REV` spec omits its FILE
// component (it borrows from a sibling FILE-origin input). Empty
// string means "no sibling to borrow from", which is an error for
// vcs: rev-mode specs.
func resolveInput(arg string, argIdx, totalVERorStdin int, stdin io.Reader, st *stdinReadState, backend vcsBackend, borrowedFile string) (resolvedInput, error) {
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

	if strings.HasPrefix(arg, "vcs:") {
		return resolveVcsInput(arg, borrowedFile, backend)
	}

	if strings.HasPrefix(arg, "cmd:") {
		return resolveCmdInput(arg)
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

// run is the testable entry point. It always returns nil on success or
// an *exitErr on failure (so main only has to translate the carried code
// into a process exit status). User-facing diagnostics — including the
// "bump-semver: <reason>" prefix — are written to stderr from here so
// the --quiet-all flag can suppress them.
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	args, err := parseArgs(argv)
	if err != nil {
		// parse errors precede any quiet flag taking effect (the flag
		// itself may be malformed). Always print to stderr.
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		return &exitErr{code: 2}
	}
	switch args.kind {
	case "version":
		if args.json {
			v, perr := ParseVersion(version)
			if perr != nil {
				return emitErr(stderr, args, fmt.Errorf("parse own version %q: %w", version, perr))
			}
			data, mErr := marshalJSONOutput(v.ToJSON(nil))
			if mErr != nil {
				return emitErr(stderr, args, fmt.Errorf("marshal json: %w", mErr))
			}
			_, _ = stdout.Write(data)
			return nil
		}
		fmt.Fprintln(stdout, version)
		return nil
	case "help":
		fmt.Fprint(stdout, shortHelpText)
		return nil
	case "helpFull":
		fmt.Fprint(stdout, fullHelpText)
		return nil
	case "helpAction":
		text, ok := actionHelpTexts[args.action]
		if !ok {
			// defensive: parseArgs should only set helpAction for
			// known actions; fall back to the short help so the
			// caller still sees something useful.
			fmt.Fprint(stdout, shortHelpText)
			return nil
		}
		fmt.Fprint(stdout, text)
		return nil
	case "compare":
		return runCompare(args, stdin, stdout, stderr)
	case "vcs":
		return runVcsCmd(args, stdin, stdout, stderr)
	}

	return runBump(args, stdin, stdout, stderr)
}

// runVcsCmd is the dispatcher for the `vcs <verb>` family (DR-0020). PR-2
// adds `vcs is` alongside `vcs get`; future verbs (diff / commit / push /
// tag) plug in here as additional cases.
func runVcsCmd(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	switch args.vcsVerb {
	case "get":
		return runVcsCmdGet(args, stdout, stderr)
	case "is":
		return runVcsCmdIs(args, stdout, stderr)
	case "diff":
		return runVcsCmdDiff(args, stdout, stderr)
	case "commit":
		return runVcsCmdCommit(args, stdout, stderr)
	default:
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs verb: %s (expected: get / is / diff / commit)", args.vcsVerb))
	}
}

// vcsGetKeys lists the keys recognised by `vcs get`. Kept as a slice so
// the order is preserved when we surface it in error messages.
var vcsGetKeys = []string{"root", "backend", "current-branch"}

// runVcsCmdGet implements `vcs get <key>`.
//
// Exit codes (DR-0020):
//
//   - 0  on success
//   - 2  when the key is missing / unknown (usage)
//   - 3  when the VCS subprocess fails or the cwd is not a vcs repo
//   - 4  when the answer is ambiguous (detached HEAD, multi-bookmark)
//
// The output is unadorned (no JSON wrapper) — `vcs get` is intentionally
// shell-friendly, like `git rev-parse --show-toplevel`.
func runVcsCmdGet(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get requires a key (one of: %s)", strings.Join(vcsGetKeys, " / ")))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get takes exactly one key, got %d", len(args.vcsArgs)))
	}
	key := args.vcsArgs[0]

	// The `backend` key is the only one that doesn't need to actually
	// build a backend before answering — but for consistency (and so a
	// non-vcs cwd reports exit 3 here too) we resolve the backend up
	// front and let the unknown-key check fall through.
	known := false
	for _, k := range vcsGetKeys {
		if k == key {
			known = true
			break
		}
	}
	if !known {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs get key: %s (expected one of: %s)", key, strings.Join(vcsGetKeys, " / ")))
	}

	vcsOverride, _ := parseVcsOverride(args.vcs) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// -q / -qq both suppress the stdout value (the exit code carries the
	// information the caller actually needs in scripted contexts).
	emit := func(s string) {
		if args.quiet || args.quietAll {
			return
		}
		fmt.Fprintln(stdout, s)
	}
	switch key {
	case "backend":
		emit(b.Kind())
		return nil
	case "root":
		root, err := b.Root()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		emit(root)
		return nil
	case "current-branch":
		name, err := b.CurrentBranch()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		emit(name)
		return nil
	}
	// Unreachable: key was validated against vcsGetKeys above.
	return emitVcsUsage(stderr, args, fmt.Errorf("internal: unhandled vcs get key %q", key))
}

// vcsIsPreds lists the predicates recognised by `vcs is`. Kept as a
// slice so the order is preserved when surfaced in error messages.
//
// DR-0020 scope rule: only predicates that read the same way for git
// and jj users land here. Backend-specific concepts (e.g. jj's
// `empty @`) stay out — they would not be transferable to git users
// reading shared Taskfiles.
var vcsIsPreds = []string{"clean", "dirty", "git", "jj"}

// runVcsCmdIs implements `vcs is <pred>`.
//
// Exit codes (DR-0020):
//
//   - 0  predicate true
//   - 1  predicate false (silent on stderr, mirroring `compare`)
//   - 2  usage error (missing / unknown / too many args)
//   - 3  VCS subprocess error or "not a vcs repo"
//
// `clean` / `dirty` need to consult the backend's worktree state.
// `git` / `jj` need to know which backend the auto-probe (or override)
// selected — for both we build the backend up front and surface its
// exit-3 if we're outside a repo. That distinguishes "not git" from
// "can't tell" (DR-0020: 曖昧・期待外はエラー).
func runVcsCmdIs(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs is requires a predicate (one of: %s)", strings.Join(vcsIsPreds, " / ")))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs is takes exactly one predicate, got %d", len(args.vcsArgs)))
	}
	pred := args.vcsArgs[0]

	known := false
	for _, p := range vcsIsPreds {
		if p == pred {
			known = true
			break
		}
	}
	if !known {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs is predicate: %s (expected one of: %s)", pred, strings.Join(vcsIsPreds, " / ")))
	}

	vcsOverride, _ := parseVcsOverride(args.vcs) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	var result bool
	switch pred {
	case "clean":
		result, err = b.IsClean()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
	case "dirty":
		clean, ierr := b.IsClean()
		if ierr != nil {
			return emitVcsErr(stderr, args, ierr)
		}
		result = !clean
	case "git", "jj":
		result = b.Kind() == pred
	default:
		// Unreachable: pred was validated against vcsIsPreds above.
		return emitVcsUsage(stderr, args, fmt.Errorf("internal: unhandled vcs is predicate %q", pred))
	}

	if result {
		return nil
	}
	// Predicate-false is silent on stderr — matches `compare` semantics
	// so shell `if`/`&&` chains work without filtering output.
	return &exitErr{code: exitCodeFalse}
}

// runVcsCmdDiff implements `vcs diff REV [PATH..]` (DR-0020 PR-3, PR-3.1).
//
// Exit codes (DR-0020):
//
//   - 0  success: with -q, "no diff"; otherwise patch written to stdout
//     (which may legitimately be empty)
//   - 1  with -q only: "diff present" (predicate-false, mirrors
//     `git diff --quiet`'s --exit-code semantic)
//   - 2  usage error (parser surfaces "no REV" as help; reserved for
//     future verb-level usage problems)
//   - 3  VCS subprocess error or "not a vcs repo"
//
// Design rationale (-q overload):
//
//	`-q/--quiet` on `vcs diff` overloads the global "suppress stdout"
//	meaning to ALSO reflect diff presence in the exit code. This is
//	consistency with `git diff --quiet` (which implies --exit-code:
//	0 = clean, 1 = differs), the right mental model for a diff command.
//	Other vcs verbs (`get`/`is`) keep the pure stdout-suppression
//	meaning — diff is the only verb whose "is there anything?" question
//	is well-posed.
//
// `-s/--name-status` switches the output to one `<CODE>\t<path>\n` line
// per changed file (git's --name-status shape, normalized in the jj
// backend). `-s -q` collapses to `-q` (stdout empty, exit reflects
// presence) — one code path feeds both views: name-status output's
// emptiness == diff absence.
//
// Path-handling rule (kawaz's declarative-convergence): nonexistent paths
// are silently ignored. When every supplied path is filtered out we emit
// nothing — `vcs diff REV nope.txt` deliberately does NOT widen back to
// "diff everything" the way `git diff REV --` would. Under -q this yields
// exit 0 (= "no diff to report").
func runVcsCmdDiff(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		// The parseArgs layer normally short-circuits "vcs diff" with no
		// further args to the per-verb help; this branch only fires if a
		// future code path reaches the dispatcher with an empty arg list.
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs diff requires a REV (usage: vcs diff REV [PATH..])"))
	}
	rev := args.vcsArgs[0]
	paths := args.vcsArgs[1:]

	vcsOverride, _ := parseVcsOverride(args.vcs) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// -q (and -qq) trigger the predicate-only path: derive presence from
	// name-status output (cheap; same shape feeds -s display). Doing this
	// before -s keeps `-q` strictly authoritative when both are set.
	if args.quiet || args.quietAll {
		ns, err := b.DiffNameStatus(rev, paths)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		if len(ns) == 0 {
			return nil
		}
		// Silent predicate-false (matches runVcsCmdIs / compare).
		return &exitErr{code: exitCodeFalse}
	}

	// -s: name-status display (no quiet → stdout gets the codes).
	if args.vcsDiffNameStatus {
		out, err := b.DiffNameStatus(rev, paths)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		if len(out) > 0 {
			if _, werr := stdout.Write(out); werr != nil {
				return werr
			}
		}
		return nil
	}

	// Default: raw patch.
	out, err := b.Diff(rev, paths)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	if len(out) > 0 {
		if _, werr := stdout.Write(out); werr != nil {
			return werr
		}
	}
	return nil
}

// runVcsCmdCommit implements `vcs commit` (DR-0020 PR-4).
//
// Three modes:
//
//   - `vcs commit -m MSG PATH..`         — commit listed paths' content
//   - `vcs commit -m MSG --staged`       — commit all staged/dirty changes
//   - `vcs commit --amend [-m MSG]`      — fold current change into prev
//
// `-a` / `--all` is intentionally not provided (DR-0020 safety: kawaz
// CLI design + jj's auto-staged worldview). It's parsed only so we can
// reject it with a tailored exit-2 hint instead of the generic
// "unknown flag" catch-all.
//
// Argument-error ordering (advisor #3):
//
//  1. -a   → exit 2 with hint (backend-independent, before resolve)
//  2. path + --staged → exit 2 (backend-independent, before resolve)
//  3. !amend && !message → exit 2 (backend-independent, before resolve)
//  4. Resolve backend (exit 3 if not a vcs repo)
//  5. no PATH && no --staged && !amend → exit 2 with backend-Kind() hint
//  6. Dispatch to backend.Commit
//
// Exit codes (DR-0020):
//
//   - 0  success (commit created, or no-op if there was nothing to commit)
//   - 2  usage error
//   - 3  VCS subprocess error or "not a vcs repo"
func runVcsCmdCommit(args cliArgs, stdout, stderr io.Writer) error {
	// Step 1: -a explicit reject (before backend resolve so non-repo
	// cwd still gets the tailored hint).
	if args.vcsCommitDashA {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: -a / --all is not supported (use --staged to commit all staged changes, or pass PATH..)"))
	}
	// Step 2: path + --staged exclusivity.
	if args.vcsCommitStaged && len(args.vcsArgs) > 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: --staged and PATH.. are mutually exclusive"))
	}
	// Step 3: -m required for !amend.
	if !args.vcsCommitAmend && !args.vcsCommitMessageSet {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: -m MSG is required (unless --amend)"))
	}
	// Step 4: resolve backend.
	vcsOverride, _ := parseVcsOverride(args.vcs) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// Step 5: no mode (= no path, no --staged, no --amend) → backend-
	// specific hint. By this point we know we're in a vcs repo, so the
	// hint can be specific to git's "you usually want --staged" vs jj's
	// "auto-staged world, name a PATH".
	if !args.vcsCommitAmend && !args.vcsCommitStaged && len(args.vcsArgs) == 0 {
		var hint string
		switch b.Kind() {
		case "git":
			hint = "use --staged to commit staged changes, or specify PATH.."
		case "jj":
			hint = "specify PATH.. (commit -a is not supported by design); or use --staged for the entire @ change"
		default:
			hint = "specify PATH.. or --staged"
		}
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: nothing to commit (%s)", hint))
	}
	// Step 6: dispatch.
	opts := commitOpts{
		paths:   args.vcsArgs,
		message: args.vcsCommitMessage,
		staged:  args.vcsCommitStaged,
		amend:   args.vcsCommitAmend,
		noEdit:  args.vcsCommitAmend && !args.vcsCommitMessageSet,
	}
	if err := b.Commit(opts); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// commit is silent on success (mirrors `git commit -q` philosophy
	// for scripted callers; jj users get jj's own snapshot text in
	// stderr from the subprocess but we don't echo on stdout).
	_ = stdout
	return nil
}

// emitVcsUsage prints a "bump-semver: <msg>" line and returns an
// exitErr with exitCodeUsage. Separate from emitErr because the
// existing emitErr hardcodes exit code 2 (kept as exitCodeUsage), but
// future vcs errors need a different code path (exitCodeAmbiguous /
// exitCodeVCSExec etc.) and we want a focused helper for those.
func emitVcsUsage(stderr io.Writer, args cliArgs, err error) error {
	if !args.quietAll {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	}
	return &exitErr{code: exitCodeUsage, msg: err.Error()}
}

// emitVcsErr surfaces an error from a vcs verb. When the error already
// carries an exit code (= an *exitErr produced by the backend layer),
// we preserve it. Anything else is treated as a VCS-exec failure
// (exit 3), so a stray non-coded error doesn't silently downgrade into
// the generic exit 2.
func emitVcsErr(stderr io.Writer, args cliArgs, err error) error {
	if !args.quietAll {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee
	}
	return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
}

// emitErr writes a "bump-semver: <reason>" line to stderr unless the
// caller requested -qq/--quiet-all, then returns *exitErr{code: 2}
// carrying the same message so callers (especially tests) can still
// inspect err.Error() for substrings. main does NOT re-print the
// message because run() has already done so.
//
// DR-0010: when err is an *unsupportedFileError, we follow the
// "bump-semver:" line with a one-line `hint:` pointing the user at
// the issue tracker. The hint is suppressed by `--no-hint` / `-q` /
// `-qq` to match every other DR-0010 hint (and every other v0.5.0
// stderr-side hint).
func emitErr(stderr io.Writer, args cliArgs, err error) error {
	if !args.quietAll {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		var ufe *unsupportedFileError
		if errors.As(err, &ufe) && !args.quiet && !args.noHint {
			fmt.Fprintln(stderr, "hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.")
		}
	}
	return &exitErr{code: 2, msg: err.Error()}
}

// emitFallbackHints prints the DR-0010 fallback hint and the DR-0013
// suffix-stripping hint for FILE-origin resolved inputs. The hints are
// suppressed by `--no-hint` / `-q` / `-qq` and printed before any
// other stderr hint so they appear in event order (rule-resolution →
// bump action).
//
// `compare` also goes through this helper because the hints reflect
// the file detection, not the action — passing an unrecognised
// filename to `compare` is just as informative as to `get` / bump.
//
// When both hints fire for the same input (e.g. `unknown.json.bak`:
// suffix `.bak` stripped → `*.json` glob fallback), they are emitted
// in **source order** (suffix stripping is the filename-level
// observation; the *.json fallback is the content-level observation).
// Both share the `hint:` prefix so a single grep / `--no-hint` flag
// captures both.
func emitFallbackHints(stderr io.Writer, args cliArgs, resolved []resolvedInput) {
	if args.quiet || args.quietAll || args.noHint {
		return
	}
	for _, ri := range resolved {
		if ri.handler == nil || ri.file == "" {
			continue
		}
		// DR-0013: suffix stripping happened first (filename-level),
		// so emit it before the DR-0010 fallback hint.
		if suffix := ri.insp.MatchedSuffixStripped; suffix != "" {
			stripped := ri.insp.MatchedStrippedBasename
			if stripped == "" {
				stripped = "(unknown)"
			}
			fmt.Fprintf(stderr,
				"hint: %s matched as %s rule (suffix %s stripped); use --no-hint to suppress\n",
				ri.file, stripped, suffix)
		}
		if ri.insp.MatchedConfidence != 1 {
			continue
		}
		glob := ri.insp.MatchedGlob
		if glob == "" {
			// Defensive: if the rule had no Glob recorded for some
			// reason, fall back to a generic phrasing rather than
			// printing "matched as  fallback".
			glob = "fallback"
		}
		fmt.Fprintf(stderr, "hint: %s matched as %s fallback. Open issue if explicit support is needed.\n", ri.file, glob)
	}
}

func runBump(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	// DR-0008: --write + any read-only input (vcs: / cmd:) is rejected up-front.
	// Both schemas resolve to a value without a writable backing file —
	// writing back would require commit/amend semantics for vcs: or
	// process re-execution for cmd:, both far out of scope. Silently
	// dropping the read-only portion of a multi-input --write would
	// surprise users. The cleanest answer is to refuse the combination
	// and let the caller split the invocation.
	if args.write {
		for _, in := range args.inputs {
			if strings.HasPrefix(in, "vcs:") {
				return emitErr(stderr, args, fmt.Errorf("--write cannot be used with vcs: inputs (vcs: is read-only)"))
			}
			if strings.HasPrefix(in, "cmd:") {
				return emitErr(stderr, args, fmt.Errorf("--write cannot be used with cmd: inputs (cmd: is read-only)"))
			}
		}
	}

	vcsOverride, _ := parseVcsOverride(args.vcs) // already validated in parseArgs
	resolved, err := resolveInputs(args.inputs, stdin, args.write, vcsOverride)
	if err != nil {
		return emitErr(stderr, args, err)
	}

	// DR-0010: warn the user about confidence-1 fallback matches before
	// doing the actual bump, so the hint appears in event order
	// (rule-resolution → bump). Suppression flags are honored inside the
	// helper.
	emitFallbackHints(stderr, args, resolved)

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
			return emitErr(stderr, args, fmt.Errorf("--write requires at least one FILE"))
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
		return emitErr(stderr, args, formatMismatchError("version", allVersions))
	}

	// Aggregate names across FILE-origin entries (VER/stdin contribute none).
	var allNames []locatedField
	for _, ri := range resolved {
		if ri.file != "" || ri.handler != nil {
			allNames = append(allNames, locatedFromInspection(ri.originFile, ri.insp.Names)...)
		}
	}
	if _, ok := allSameValue(allNames); len(allNames) > 0 && !ok {
		return emitErr(stderr, args, formatMismatchError("name", allNames))
	}

	// Use the first contributing field as the origin source for parse
	// errors (any one would work; they're all equal by construction).
	origin := allVersions[0]

	v, err := ParseVersion(cur)
	if err != nil {
		return emitErr(stderr, args, wrapOriginErr(origin.label(), cur, err))
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
		return emitErr(stderr, args, wrapOriginErr(origin.label(), cur, err))
	}

	// Hint output (Phase 5): bump-only, when at least one FILE was given
	// but --write was not, and no quiet/no-hint flag suppresses it. The
	// hint reminds users that a successful bump did not touch any file.
	if shouldShowHint(args, resolved) {
		n := countFileInputs(resolved)
		suffix := "files"
		if n == 1 {
			suffix = "file"
		}
		fmt.Fprintf(stderr, "hint: %d %s not modified; use --write to update or --no-hint to suppress\n", n, suffix)
	}

	// stdout output (suppressed by -q/-qq). With --json the bumped/get
	// version is rendered as a single-line JSON object (DR-0007); the
	// `name` field is populated from the cross-input-validated set of
	// FILE-origin names (which DR-0004 already collapses to one value).
	if !args.quiet && !args.quietAll {
		if args.json {
			var name *string
			if len(allNames) > 0 {
				n := allNames[0].Value
				name = &n
			}
			data, mErr := marshalJSONOutput(newV.ToJSON(name))
			if mErr != nil {
				return emitErr(stderr, args, fmt.Errorf("marshal json: %w", mErr))
			}
			_, _ = stdout.Write(data)
		} else {
			fmt.Fprintln(stdout, newV.String())
		}
	}

	if args.write {
		for _, ri := range resolved {
			if ri.handler == nil || ri.file == "" {
				continue
			}
			out, err := ri.handler.Replace(ri.content, cur, newV.String())
			if err != nil {
				return emitErr(stderr, args, fmt.Errorf("replace %s: %w", ri.file, err))
			}
			mode := os.FileMode(0644)
			if fi, statErr := os.Stat(ri.file); statErr == nil {
				mode = fi.Mode().Perm()
			}
			if err := os.WriteFile(ri.file, out, mode); err != nil {
				return emitErr(stderr, args, fmt.Errorf("write %s: %w", ri.file, err))
			}
		}
	}
	return nil
}

// shouldShowHint returns true when runBump should emit the
// "files not modified" hint. The hint is bump-specific (not get) and
// only meaningful when the user passed at least one FILE input but
// omitted --write — it surfaces a successful no-op write.
func shouldShowHint(args cliArgs, resolved []resolvedInput) bool {
	if args.kind != "bump" {
		return false
	}
	switch args.action {
	case "major", "minor", "patch", "pre":
	default:
		return false // get is read-only, never has a "modified" outcome
	}
	if args.write {
		return false
	}
	if args.quiet || args.quietAll || args.noHint {
		return false
	}
	return countFileInputs(resolved) > 0
}

// countFileInputs returns the number of FILE-origin resolved inputs
// (i.e. anything whose Inspect succeeded against an on-disk file). VER
// and stdin (`-`) inputs are not counted because they were never going
// to be "modified" in the first place.
func countFileInputs(resolved []resolvedInput) int {
	n := 0
	for _, ri := range resolved {
		if ri.handler != nil && ri.file != "" {
			n++
		}
	}
	return n
}

// resolveInputs walks the positional inputs and resolves each. It also
// handles the legacy "single FILE + stdin pipe" shortcut: if exactly
// one input is present, that input is a FILE path (not `-`, not a VER
// pattern), and stdin is a pipe, the FILE's content is read from stdin.
//
// vcsOverride is the parsed --vcs flag value (vcsAuto when absent). The
// VCS itself is detected lazily — only when at least one input is
// `vcs:` — so non-vcs invocations don't error out in environments
// without a `.jj` / `.git` directory.
//
// File-borrowing for `vcs:REV` (no explicit FILE) takes the first
// FILE-providing argument in **position order** (left-to-right). The
// borrow source can be either:
//
//   - a real FILE-origin input (`Cargo.toml`)
//   - another `vcs:REV:FILE` input that names its file explicitly
//
// "Position order" was chosen over "highest-confidence parse" because
// it's predictable from the user's perspective: the file that comes
// first in the argv wins. When every vcs: input omits FILE *and*
// there's no real FILE-origin, we error out — there's nothing to
// borrow from.
func resolveInputs(inputs []string, stdin io.Reader, write bool, vcsOverride vcsKind) ([]resolvedInput, error) {
	// Pre-classify each input. We need three buckets:
	//   - "raw" (VER, `-`, or `vcs:`): contributes to <argv:N> indexing
	//   - "file": exists on disk
	//   - "vcs": needs lazy VCS detection
	// "raw" subsumes vcs because vcs: is not a path on disk; a `vcs:`
	// arg should not be counted as a writable FILE either.
	//
	// The borrow target (fileForBorrow) is "first file-providing arg
	// in position order". A `vcs:REV:FILE` qualifies because its FILE
	// is unambiguous; a `vcs:REV` (no file) does not.
	isRaw := make([]bool, len(inputs))
	rawCount := 0
	hasVcs := false
	var fileForBorrow string // first FILE-providing input, if any
	for i, in := range inputs {
		if in == "-" {
			isRaw[i] = true
			rawCount++
			continue
		}
		if strings.HasPrefix(in, "vcs:") {
			isRaw[i] = true
			rawCount++
			hasVcs = true
			// `vcs:REV:FILE` (file-explicit) qualifies as a borrow
			// source for downstream `vcs:REV` (file-omitted) args.
			if _, file, isFunc, _ := vcsParseSpec(in); !isFunc && file != "" {
				if fileForBorrow == "" {
					fileForBorrow = file
				}
			}
			continue
		}
		if fi, err := os.Stat(in); err == nil && !fi.IsDir() {
			if fileForBorrow == "" {
				fileForBorrow = in
			}
			continue // exists as a file
		}
		isRaw[i] = true
		rawCount++
	}

	// Legacy stdin pipe shortcut: one FILE input (not `-`, exists or
	// at least matches a known rule), stdin is a pipe → read content
	// from stdin and treat the path as a name hint. vcs: inputs are
	// not eligible for this shortcut.
	if len(inputs) == 1 && inputs[0] != "-" && !strings.HasPrefix(inputs[0], "vcs:") && isStdinPipe(stdin) {
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

	// Detect VCS lazily — only when at least one input is `vcs:`.
	// Detecting up-front would error out in repos that don't use
	// `vcs:` syntax, even though they're valid bump-semver targets.
	var backend vcsBackend
	if hasVcs {
		b, err := newVcsBackend(vcsOverride)
		if err != nil {
			return nil, err
		}
		backend = b
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
		// vcs: rev-mode specs without a FILE component borrow the
		// path from the first FILE-origin sibling. We pass the
		// borrow source unconditionally; resolveVcsInput uses it
		// only when the spec actually omits the file part.
		ri, err := resolveInput(in, argIdx, rawCount, stdin, &st, backend, fileForBorrow)
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
