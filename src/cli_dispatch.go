package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// run is the testable entry point. It always returns nil on success or
// an *exitErr on failure (so main only has to translate the carried code
// into a process exit status). User-facing diagnostics — including the
// "bump-semver: <reason>" prefix — are written to stderr from here so
// the --quiet-all flag can suppress them.
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	// Co-existence router (cobra migration, plan §2). Migrated entry
	// points are handled by the cobra command tree; everything else
	// falls through to the legacy hand-rolled parser below. This branch
	// is removed once every verb is on cobra.
	if useCobra(argv) {
		return runCobra(argv, stdin, stdout, stderr)
	}
	args, err := parseArgs(argv)
	if err != nil {
		// parse errors precede any quiet flag taking effect (the flag
		// itself may be malformed). Always print to stderr.
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		return &exitErr{code: 2}
	}
	switch args.kind {
	case "version":
		if args.output.JSON {
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
	if !args.output.Verbosity.ShouldSuppressError() {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		var ufe *unsupportedFileError
		if errors.As(err, &ufe) && !args.output.Verbosity.ShouldSuppressHint() {
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
	if args.output.Verbosity.ShouldSuppressHint() {
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

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // already validated in parseArgs
	// PeerExpand=true: bump/get want N-arg cross-source equality
	// across all sibling FILE paths when a file-omitted vcs:REV is
	// present (DR-0023). Compare uses PeerExpand=false.
	resolved, err := resolveInputs(args.inputs, stdin, resolveInputsOpts{
		Write:      args.write,
		VCSKind:    vcsOverride,
		PeerExpand: true,
		Glob:       args.glob,
		RuleBlocks: args.ruleBlocks,
	})
	if err != nil {
		return emitErr(stderr, args, err)
	}

	// DR-0024: if every input was a `glob:` selector and all of them
	// collapsed to 0 matches, resolved is empty here. Surface that as an
	// exit-2 usage error (the parser-side "at least one input" gate sees
	// the literal `glob:...` strings and accepts them; the actual file
	// expansion happens inside resolveInputs).
	if len(resolved) == 0 {
		return emitErr(stderr, args, fmt.Errorf("no inputs after glob expansion (all glob: selectors matched 0 files)"))
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
		// DR-0023: get treats all sources as equal peers. A
		// disagreement is a predicate-false outcome (exit 1) with the
		// per-source listing on stderr — mirroring compare's
		// false-with-diagnostic shape rather than the bump-time
		// "internal data is wrong, refuse to act" exit 2. Genuine
		// bump actions (major/minor/patch/pre) still flow through
		// emitErr (exit 2) because inconsistent inputs there are an
		// error condition, not a queryable result.
		if args.action == "get" {
			if !args.output.Verbosity.ShouldSuppressError() {
				fmt.Fprintln(stderr, formatMismatchError("version", allVersions).Error())
			}
			return &exitErr{code: exitCodeFalse}
		}
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
		// DR-0023 / follow-up #35: same peer-equality model as
		// the version-mismatch branch above — get treats all sources
		// as equal peers, so a name disagreement is predicate-false
		// (exit 1) with the per-source listing on stderr. Bump verbs
		// still flow through emitErr (exit 2) because writing back
		// inconsistent inputs is destructive.
		if args.action == "get" {
			if !args.output.Verbosity.ShouldSuppressError() {
				fmt.Fprintln(stderr, formatMismatchError("name", allNames).Error())
			}
			return &exitErr{code: exitCodeFalse}
		}
		return emitErr(stderr, args, formatMismatchError("name", allNames))
	}

	// Use the first contributing field as the origin source for parse
	// errors (any one would work; they're all equal by construction).
	origin := allVersions[0]

	v, err := ParseVersion(cur)
	if err != nil {
		return emitErr(stderr, args, wrapOriginErr(origin.label(), cur, err))
	}
	// Bridge cliArgs.bump (*string) → BumpOptions (string + Set bool).
	// BumpOptions stays as-is to avoid expanding the refactor blast
	// radius into semver.go / its tests (PR-Simplify-1 is cliArgs-only).
	opts := BumpOptions{
		Pre:              derefOr(args.bump.Pre, ""),
		PreSet:           args.bump.Pre != nil,
		NoPre:            args.bump.NoPre,
		BuildMetadata:    derefOr(args.bump.BuildMetadata, ""),
		BuildMetadataSet: args.bump.BuildMetadata != nil,
		NoBuildMetadata:  args.bump.NoBuildMetadata,
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
	if !args.output.Verbosity.ShouldSuppressStdout() {
		if args.output.JSON {
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
	if args.output.Verbosity.ShouldSuppressHint() {
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
