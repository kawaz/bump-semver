package main

// `vcs outdated FROM TO[..]` — derived-sync predicate. See DR-0027 / DR-0028
// and `docs/specs/glob-backref-v0.1.0.md` for grammar + matching semantics.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// runVcsCmdOutdated dispatches `vcs outdated`.
//
// Exit codes:
//   - 0  fresh (every derived ≥ source ts) OR --explain mode
//   - 1  stale OR missing-mandatory derived OR --strict + literal-FROM-not-found
//   - 2  usage error
//   - 3  VCS subprocess error
func runVcsCmdOutdated(args cliArgs, stdout, stderr io.Writer) error {
	pairs, err := splitOutdatedPairs(args.vcsArgs)
	if err != nil {
		return emitVcsUsage(stderr, args, err)
	}
	if len(pairs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs outdated: at least one FROM TO[..] pair is required (usage: vcs outdated FROM TO[..] | vcs outdated -- F T[..] -- F T[..] -- ...)"))
	}

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	var rows []outdatedRow
	var literalMisses []string // FROM string for each literal pattern that matched nothing
	for pairIdx, p := range pairs {
		pr, missed, perr := evaluateOutdatedPair(b, args.glob, p)
		if perr != nil {
			if !args.output.Verbosity.ShouldSuppressError() {
				fmt.Fprintf(stderr, "bump-semver: vcs outdated: pair %d: %s\n", pairIdx+1, perr.Error())
			}
			// Preserve coded errors (= exitCodeVCSExec wrapped by errors.As
			// — `fmt.Errorf("... %w", terr)` boxes the *exitErr so a bare
			// type assertion misses it; errors.As walks the wrap chain).
			var ee *exitErr
			if errors.As(perr, &ee) {
				return ee
			}
			return &exitErr{code: exitCodeUsage, msg: perr.Error()}
		}
		rows = append(rows, pr...)
		if missed != "" {
			literalMisses = append(literalMisses, missed)
		}
	}

	// Literal-FROM-not-found handling (= DR-0028 blocker #3).
	// Default: warn to stderr, do not fail (= back-compat with DR-0027).
	// --strict: exit 1 with the misses on stderr.
	if len(literalMisses) > 0 && !args.output.Verbosity.ShouldSuppressError() {
		for _, m := range literalMisses {
			fmt.Fprintf(stderr, "vcs outdated: literal FROM %q matched no file — likely typo (use --strict to fail)\n", m)
		}
	}

	if args.vcsOutdated.Explain {
		emitExplainTable(stdout, rows, args.output.Verbosity.ShouldSuppressStdout())
		return nil
	}

	if args.vcsOutdated.Strict && len(literalMisses) > 0 {
		return &exitErr{code: exitCodeFalse}
	}

	stale := stalenessSummary(rows)
	if len(stale) == 0 {
		return nil
	}
	if !args.output.Verbosity.ShouldSuppressError() {
		for _, r := range stale {
			fmt.Fprintln(stderr, "vcs outdated: "+formatRow(r, rowModePredicate))
		}
	}
	return &exitErr{code: exitCodeFalse}
}

// outdatedPair is one (FROM, TO...) group from the user's argv.
type outdatedPair struct {
	From string
	To   []string
}

// splitOutdatedPairs splits argv at literal `"--"` separators. Each pair
// must have FROM + at least one TO.
func splitOutdatedPairs(argv []string) ([]outdatedPair, error) {
	if len(argv) == 0 {
		return nil, nil
	}
	var groups [][]string
	var cur []string
	for _, a := range argv {
		if a == "--" {
			if len(cur) > 0 {
				groups = append(groups, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, a)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	pairs := make([]outdatedPair, 0, len(groups))
	for i, g := range groups {
		if len(g) < 2 {
			return nil, fmt.Errorf("pair %d needs FROM and at least one TO (got %d arg(s))", i+1, len(g))
		}
		pairs = append(pairs, outdatedPair{From: g[0], To: g[1:]})
	}
	return pairs, nil
}

// outdatedRow is one (source → derived) row after FROM expansion + TO
// substitution + freshness measurement. Status is one of:
//   - "fresh"      derived ts >= source ts
//   - "stale"      derived ts < source ts (derived exists)
//   - "missing"    derived path absent (mandatory only — wildcard absence
//     is silently skipped earlier)
//   - "untracked"  derived exists on disk but never committed (ts=0)
type outdatedRow struct {
	Source        string
	Derived       string
	SourceTS      int64
	DerivedTS     int64
	CommitsBehind int
	Status        string
}

// rowModePredicate / rowModeExplain — formatRow output styles.
type rowMode int

const (
	rowModePredicate rowMode = iota // stderr predicate lines (= "stale" / "missing" / "untracked")
	rowModeExplain                  // --explain table cell (= "[fresh: ...]" / "[stale: ...]" / etc.)
)

// formatRow renders one row in the requested mode. Collapses the prior
// `predicateLine` / `explainStatus` pair so the wording (esp. "commit(s)"
// pluralization) stays in sync (= /simplify cleanup).
func formatRow(r outdatedRow, mode rowMode) string {
	switch mode {
	case rowModePredicate:
		switch r.Status {
		case "stale":
			return fmt.Sprintf("%s → %s [stale: derived %s behind source]",
				r.Source, r.Derived, pluralCommits(r.CommitsBehind))
		case "missing":
			return fmt.Sprintf("%s → %s [missing, will fail]", r.Source, r.Derived)
		case "untracked":
			return fmt.Sprintf("%s → %s [untracked: derived has no commit ts]", r.Source, r.Derived)
		default:
			return fmt.Sprintf("%s → %s [%s]", r.Source, r.Derived, r.Status)
		}
	case rowModeExplain:
		switch r.Status {
		case "fresh":
			return "[fresh: derived ts >= source ts]"
		case "missing":
			return "[missing, will fail]"
		case "stale":
			return fmt.Sprintf("[stale: derived %s behind source]", pluralCommits(r.CommitsBehind))
		case "untracked":
			return "[untracked: derived has no commit ts]"
		default:
			return "[" + r.Status + "]"
		}
	}
	return "[" + r.Status + "]"
}

// evaluateOutdatedPair runs one pair end-to-end and returns rows +
// "literalMissed" (= the FROM string when a LITERAL FROM matched zero
// files; "" when FROM is glob:, or when literal FROM matched). The
// literal/glob distinction is the linchpin of the --strict semantics
// (= DR-0028 blocker #3).
func evaluateOutdatedPair(b vcsBackend, gOpts globOpts, p outdatedPair) ([]outdatedRow, string, error) {
	fromPattern, fromIsLiteral, ferr := stripGlobPrefix(p.From)
	if ferr != nil {
		return nil, "", ferr
	}
	sources, err := MatchCollect(fromPattern, ".", gOpts, defaultHomeFn)
	if err != nil {
		return nil, "", fmt.Errorf("FROM %q: %w", p.From, err)
	}
	var literalMissed string
	if len(sources) == 0 {
		if fromIsLiteral {
			literalMissed = p.From
		}
		return nil, literalMissed, nil
	}

	var rows []outdatedRow
	for _, src := range sources {
		srcTS, terr := b.FileTimestamp(src.Path)
		if terr != nil {
			// Wrap with %w so the *exitErr code survives the chain; the
			// caller uses errors.As to recover it.
			return nil, "", fmt.Errorf("file timestamp %s: %w", src.Path, terr)
		}
		for _, toPat := range p.To {
			pairRows, perr := derivedRowsFor(b, gOpts, src.Path, srcTS, src.Captures, toPat)
			if perr != nil {
				return nil, "", perr
			}
			rows = append(rows, pairRows...)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Derived < rows[j].Derived
	})
	return rows, "", nil
}

// stripGlobPrefix removes an optional `glob:` prefix; returns the body +
// "was it literal" + parse error. Centralizes the literal/glob branch so
// every call site (FROM / TO) handles the prefix identically (= /simplify
// cleanup, replaces the duplicated `hasGlobPrefix` + `parseGlobSpec`
// pattern at cmd_vcs_outdated.go:203 + :263).
func stripGlobPrefix(spec string) (body string, wasLiteral bool, err error) {
	if !hasGlobPrefix(spec) {
		return spec, true, nil
	}
	body, err = parseGlobSpec(spec)
	return body, false, err
}

// derivedRowsFor expands one TO pattern for a single source's captures,
// runs the mandatory/optional split, and packages the results.
//
// Mandatory branch (= non-`glob:` TO): per spec §3.4 the substituted
// string is treated as a LITERAL path, even if a captured value happens
// to contain glob meta — value-side `*/?/[/]` are NOT re-glob-interpreted.
// Pathological filenames are user responsibility per spec §9.
//
// Optional branch (= `glob:` TO): spec §3.4.2 char-class-wrap escape is
// applied to captured values so glob meta in $N values stays literal at
// the 2nd-stage walk; the template's own glob meta survives and drives
// fs discovery.
func derivedRowsFor(b vcsBackend, gOpts globOpts, sourcePath string, sourceTS int64, captures []string, toPattern string) ([]outdatedRow, error) {
	toStripped, toWasLiteral, err := stripGlobPrefix(toPattern)
	if err != nil {
		return nil, err
	}
	toIsGlob := !toWasLiteral
	// TO-side `{}` brace expansion. `parsePattern` runs the spec §2.1 reject
	// (= `?` / nested `{}` / `[^...]`) for the TO too. We then walk the AST
	// directly via braceLiteralExpansions (no FROM-side ExpandPairs dummy
	// argument needed — TO has no separate "match" axis here, just literal
	// branch expansion).
	toAST, err := parsePattern(toStripped)
	if err != nil {
		return nil, fmt.Errorf("TO %q: %w", toPattern, err)
	}
	toBranches := braceLiteralExpansions(toAST)
	var rows []outdatedRow
	for _, branch := range toBranches {
		// `substituteBody` runs the spec §3.4.2 escape policy via the
		// explicit `escapeGlob` argument (= true only when TO is `glob:`),
		// so the value-side meta never re-globs. No path.Clean here for
		// the glob: branch — `expandGlob` does its own normalization.
		// The literal branch applies path.Clean ourselves (= preserves
		// the blocker #1 leading-slash fix).
		cand, serr := substituteBody(branch, captures, toIsGlob)
		if serr != nil {
			return nil, fmt.Errorf("TO %q: %w", toPattern, serr)
		}
		if !toIsGlob {
			cand = path.Clean(cand)
		}
		// Hard invariant: if TO was `glob:`, the captured values must NOT
		// re-introduce brace meta into the post-substitute string (§3.4.2).
		// The escape on `{`/`}` covers this; assert defensively (DR-0028
		// runtime invariant requested by task review).
		if toIsGlob && strings.ContainsAny(cand, "{}") {
			panic(fmt.Sprintf("internal: brace leaked into TO glob walk input %q (template=%q, captures=%v)", cand, toPattern, captures))
		}
		if cand == sourcePath {
			continue // per-source auto-exclusion (spec §6).
		}
		if !toIsGlob {
			// Spec §3.4: literal embed. Even if `cand` happens to contain
			// `*/?/[/]` (= captured from a pathological filename),
			// existence check is performed against the LITERAL path; no
			// fs-meta re-interpretation. Status=missing when absent.
			rows = append(rows, freshnessRow(b, sourcePath, sourceTS, cand))
			continue
		}
		// `glob:` TO: optional fs discovery. Wildcards in the TEMPLATE
		// drive the walk; the captured values have been escape-wrapped
		// so they remain literal in the 2nd-stage walk.
		matches, gerr := expandGlob(cand, gOpts, defaultHomeFn)
		if gerr != nil {
			return nil, fmt.Errorf("TO %q: glob expansion of %q: %w", toPattern, cand, gerr)
		}
		for _, m := range matches {
			if m == sourcePath {
				continue
			}
			rows = append(rows, freshnessRow(b, sourcePath, sourceTS, m))
		}
	}
	return rows, nil
}

// freshnessRow measures the derived path's status. Missing-on-disk is
// distinguished from "exists but untracked" via os.Stat.
func freshnessRow(b vcsBackend, sourcePath string, sourceTS int64, derivedPath string) outdatedRow {
	row := outdatedRow{
		Source:   sourcePath,
		Derived:  derivedPath,
		SourceTS: sourceTS,
	}
	if _, statErr := os.Stat(derivedPath); statErr != nil {
		row.Status = "missing"
		return row
	}
	dts, err := b.FileTimestamp(derivedPath)
	if err != nil {
		row.Status = "untracked"
		return row
	}
	row.DerivedTS = dts
	if dts == 0 {
		row.Status = "untracked"
		row.CommitsBehind, _ = b.CountCommitsSince(sourcePath, 0)
		return row
	}
	if dts < sourceTS {
		row.Status = "stale"
		row.CommitsBehind, _ = b.CountCommitsSince(sourcePath, dts)
		return row
	}
	row.Status = "fresh"
	return row
}

// stalenessSummary returns the rows that count as failing for the
// predicate (= NOT fresh).
func stalenessSummary(rows []outdatedRow) []outdatedRow {
	out := make([]outdatedRow, 0, len(rows))
	for _, r := range rows {
		if r.Status != "fresh" {
			out = append(out, r)
		}
	}
	return out
}

// emitExplainTable prints one row per derived with aligned columns.
func emitExplainTable(stdout io.Writer, rows []outdatedRow, suppress bool) {
	if suppress {
		return
	}
	maxSrc, maxDst := 0, 0
	for _, r := range rows {
		if len(r.Source) > maxSrc {
			maxSrc = len(r.Source)
		}
		if len(r.Derived) > maxDst {
			maxDst = len(r.Derived)
		}
	}
	const maxCol = 60
	if maxSrc > maxCol {
		maxSrc = maxCol
	}
	if maxDst > maxCol {
		maxDst = maxCol
	}
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-*s  →  %-*s  %s\n",
			maxSrc, r.Source, maxDst, r.Derived, formatRow(r, rowModeExplain))
	}
}

// pluralCommits handles the "1 commit" vs "N commits" quirk.
func pluralCommits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}
