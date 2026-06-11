package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

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
	return resolveInputWithRules(arg, argIdx, totalVERorStdin, stdin, st, backend, borrowedFile, nil)
}

// resolveInputWithRules is the DR-0029 generalisation of resolveInput.
// ruleBlocks is passed through to the FILE-origin path; non-FILE inputs
// (VER / `-` / vcs: / cmd:) are unaffected by user-defined rules.
func resolveInputWithRules(arg string, argIdx, totalVERorStdin int, stdin io.Reader, st *stdinReadState, backend vcsBackend, borrowedFile string, ruleBlocks []ruleBlock) (resolvedInput, error) {
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
		return resolveFileWithRules(arg, ruleBlocks)
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
	return resolveFileWithRules(file, nil)
}

// resolveFileWithRules is the DR-0029 generalisation of resolveFile.
// When ruleBlocks is non-nil, the path is first checked against the
// blocks; a winning named block (or global block with rule flags) is
// applied via detectHandlerWithCliRule. Otherwise (no block matches,
// or ruleBlocks is nil) the existing builtin path is used.
func resolveFileWithRules(file string, ruleBlocks []ruleBlock) (resolvedInput, error) {
	h, err := pickHandlerForFile(file, ruleBlocks)
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

// resolveFilePipeOrDisk handles the "single FILE + stdin pipe" shortcut
// (DR-0004 §6). It reads stdin once and decides:
//
//   - non-empty pipe → the pipe content wins; FILE is only a name hint.
//     Returns the resolved input (fellThrough=false).
//   - empty pipe + FILE exists on disk → fall back to the on-disk file.
//     Returns fellThrough=true so the caller resolves the file via the
//     normal positional path (keeps a single source of truth for disk
//     reads). The returned resolvedInput is zero and must be ignored.
//   - empty pipe + FILE missing → error naming both the missing path and
//     the empty pipe (so a typo'd path is diagnosable, not masked as a
//     downstream "missing version").
//
// Writeback is never allowed through this path: --write is rejected by the
// caller before we get here, so the resolved input's `file` stays empty.
func resolveFilePipeOrDisk(file string, stdin io.Reader, ruleBlocks []ruleBlock) (ri resolvedInput, fellThrough bool, err error) {
	content, err := io.ReadAll(stdin)
	if err != nil {
		return resolvedInput{}, false, fmt.Errorf("read stdin: %w", err)
	}
	if len(content) == 0 {
		// Empty pipe: prefer the on-disk file when it exists; otherwise the
		// path is likely a typo — surface both facts.
		if statFileExists(file) {
			return resolvedInput{}, true, nil
		}
		return resolvedInput{}, false, fmt.Errorf("file %q not found and piped stdin was empty", file)
	}
	h, err := pickHandlerForFile(file, ruleBlocks)
	if err != nil {
		return resolvedInput{}, false, err
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, false, err
	}
	ri = resolvedInput{
		originFile: file,
		fields:     locatedFromInspection(file, insp.Versions),
		file:       "", // stdin pipe: do not allow writeback
		handler:    h,
		content:    content,
		insp:       insp,
	}
	return ri, false, nil
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
// File-borrowing for `vcs:REV` (no explicit FILE) has two modes
// (DR-0023):
//
//   - peerExpand=false (compare): each file-omitted `vcs:REV`
//     borrows the **first** FILE-providing sibling. Used by compare
//     because its semantic is "F1 (base) vs each OTHER" — F1 always
//     wins as the borrow source by construction.
//
//   - peerExpand=true (bump/get): each file-omitted `vcs:REV`
//     expands to one resolved input per **distinct** sibling FILE
//     path. This lets `get a b vcs:main` mean "compare a, b, and
//     both files at main" with no shell loop.
//
// Borrow-source candidates (in both modes) are:
//
//   - a real FILE-origin input (`Cargo.toml`)
//   - another `vcs:REV:FILE` input that names its file explicitly
//
// When every vcs: input omits FILE *and* there's no real FILE-origin,
// we error out — there's nothing to borrow from.
// resolveInputsOpts packs the three behaviour flags that customise
// resolveInputs. Previously these were three positional bool /
// enum-after-bool arguments (`write, vcsOverride, peerExpand`) — easy
// to swap at the call site without the compiler noticing. Keyed
// struct construction at the two callers (runBump / runCompare) makes
// each flag's role obvious.
type resolveInputsOpts struct {
	// Write toggles the "--write requested" assertion: when true, the
	// stdin-pipe shortcut errors out (writing into a pipe is undefined).
	Write bool
	// VCSKind is the parsed --vcs override (vcsAuto when absent). The
	// VCS itself is detected lazily — only when at least one input is
	// `vcs:` — so non-vcs invocations don't error out in environments
	// without a `.jj` / `.git` directory.
	VCSKind vcsKind
	// PeerExpand controls the file-omitted `vcs:REV` borrow shape
	// (DR-0023):
	//   - false (compare): borrow the *first* FILE-providing sibling
	//   - true  (bump/get): expand to one resolved entry per distinct
	//     sibling FILE path
	PeerExpand bool
	// Glob carries the parsed --glob-* flags (DR-0024). Read only when a
	// `glob:` selector is present in inputs.
	Glob globOpts
	// RuleBlocks carries the parsed --define-rule blocks (DR-0029).
	// nil = no user-defined rules → existing builtin auto-detection path
	// stays in effect (= zero behaviour change for non-DR-0029 callers).
	// When non-nil and at least one rule-flag-bearing block is present,
	// each FILE-origin input is resolved through detectHandlerWithCliRule
	// when a block matches the path, else falls through to detectHandler
	// (= builtin) when the global block has no flags either.
	RuleBlocks []ruleBlock
}

func resolveInputs(inputs []string, stdin io.Reader, opts resolveInputsOpts) ([]resolvedInput, error) {
	// DR-0024 pre-pass: expand any `glob:<pat>` selectors into their
	// matched FILE paths. 0-match is silent — the glob: selector simply
	// disappears from the input list (DR-0020 declarative-convergence
	// parity with "FILE that doesn't exist" handling).
	if anyGlob(inputs) {
		expanded, err := expandGlobInputs(inputs, opts.Glob)
		if err != nil {
			return nil, err
		}
		inputs = expanded
	}

	// Pre-classify each input. We need three buckets:
	//   - "raw" (VER, `-`, or `vcs:`): contributes to <argv:N> indexing
	//   - "file": exists on disk
	//   - "vcs": needs lazy VCS detection
	// "raw" subsumes vcs because vcs: is not a path on disk; a `vcs:`
	// arg should not be counted as a writable FILE either.
	//
	// The borrow target is "file-providing arg(s) in position order".
	// A `vcs:REV:FILE` qualifies because its FILE is unambiguous; a
	// `vcs:REV` (no file) does not.
	//
	// Two borrow-set representations are maintained because compare
	// (peerExpand=false) uses only the *first* file (fileForBorrow),
	// while bump/get (peerExpand=true) expands a file-omitted vcs:
	// to one resolved entry per *distinct* sibling FILE path
	// (borrowFiles).
	isRaw := make([]bool, len(inputs))
	rawCount := 0
	hasVcs := false
	var fileForBorrow string // first FILE-providing input, if any
	var borrowFiles []string // every distinct FILE path, in position order
	seenBorrow := make(map[string]bool)
	addBorrow := func(file string) {
		if file == "" || seenBorrow[file] {
			return
		}
		seenBorrow[file] = true
		borrowFiles = append(borrowFiles, file)
		if fileForBorrow == "" {
			fileForBorrow = file
		}
	}
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
				addBorrow(file)
			}
			continue
		}
		if fi, err := os.Stat(in); err == nil && !fi.IsDir() {
			addBorrow(in)
			continue // exists as a file
		}
		isRaw[i] = true
		rawCount++
	}

	// Legacy stdin pipe shortcut: one FILE input (not `-`, not `vcs:`),
	// stdin is a pipe → the FILE is a name hint and content is read from
	// stdin (DR-0004 §6, originally DR-0001). This is the `jj file show
	// <rev> Cargo.toml | bump-semver get Cargo.toml` use case: read a past
	// revision's content from the pipe while the on-disk file holds the
	// current version.
	//
	// Empty-pipe fallback: a writer-less pipe (e.g. GitHub Actions wires a
	// `run:` step's stdin to an empty FIFO) yields 0 bytes. Reading that as
	// the file content silently shadows the real file and fails with a
	// confusing "missing version". So when the pipe is empty we fall back to
	// the on-disk file (if it exists); only then do we error. Non-empty pipe
	// content always wins (the FILE remains a name hint) per DR-0004 §6.
	if len(inputs) == 1 && inputs[0] != "-" && !strings.HasPrefix(inputs[0], "vcs:") && isStdinPipe(stdin) {
		if opts.Write {
			return nil, fmt.Errorf("--write is incompatible with stdin pipe input")
		}
		// DR-0029: the stdin-pipe shortcut must also see CLI rule blocks
		// so a user can apply --define-rule to a piped file (otherwise
		// `cat my.txt | bump-semver get my.txt --define-rule ...` would
		// silently fall through to builtin even when a matching block
		// exists). pickHandlerForFile inside resolveFileFromStdinWithRules
		// chooses cliRuleHandler when a block matches, else falls through
		// to detectHandler (= identical to the pre-DR-0029 behaviour).
		//
		// pathHasAnyRule is intentionally kept as the gate: a path with
		// no builtin rule AND no matching CLI block should still error
		// out with the same `unsupported file: <path>` shape. When a CLI
		// block IS supplied that covers the path, pathHasAnyRule's "no
		// builtin" verdict would wrongly reject; widen the gate to
		// "builtin matches OR a CLI block would match".
		if pathHasAnyRule(inputs[0]) || cliRuleCoversFile(inputs[0], opts.RuleBlocks) {
			ri, fellThrough, err := resolveFilePipeOrDisk(inputs[0], stdin, opts.RuleBlocks)
			if err != nil {
				return nil, err
			}
			if !fellThrough {
				return []resolvedInput{ri}, nil
			}
			// Empty pipe + the path exists on disk: fall through to the
			// normal positional-input loop below, which reads the file via
			// resolveFileWithRules. (We do NOT return here so the standard
			// path stays the single source of truth for on-disk reads.)
		}
	}

	// Detect VCS lazily — only when at least one input is `vcs:`.
	// Detecting up-front would error out in repos that don't use
	// `vcs:` syntax, even though they're valid bump-semver targets.
	var backend vcsBackend
	if hasVcs {
		b, err := newVcsBackend(opts.VCSKind)
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
		// path from FILE-origin sibling(s). The expansion shape
		// depends on peerExpand:
		//
		//   - peerExpand=true (bump/get): if the spec is a
		//     file-omitted `vcs:REV` AND there are 2+ borrow files,
		//     expand to one resolved entry per borrow file. This
		//     turns `get a b vcs:main` into {a, b, vcs:main:a,
		//     vcs:main:b}, the cross-source equality check users
		//     want when comparing the working tree against a
		//     historical snapshot for several files at once.
		//
		//   - peerExpand=false (compare): always borrow the *first*
		//     file. Compare's F1 (= leftmost) is the comparison
		//     base, so OTHERS borrowing F1's path is exactly the
		//     "is OTHER's snapshot of F1 OP F1?" semantic.
		if opts.PeerExpand && strings.HasPrefix(in, "vcs:") {
			if rev, file, isFunc, _ := vcsParseSpec(in); !isFunc && file == "" && len(borrowFiles) > 1 {
				_ = rev
				for _, bf := range borrowFiles {
					ri, err := resolveInputWithRules(in, argIdx, rawCount, stdin, &st, backend, bf, opts.RuleBlocks)
					if err != nil {
						return nil, err
					}
					// Follow-up #35: re-label the expanded source
					// with its borrowed FILE so peer-expanded
					// `vcs:REV` entries are distinguishable in
					// mismatch stderr ("vcs:HEAD:VERSION" vs
					// "vcs:HEAD:b.json", not two bare "vcs:HEAD"
					// lines). The label uses the canonical
					// `vcs:REV:FILE` spec form so it round-trips
					// through vcsParseSpec.
					ri.originFile = in + ":" + bf
					for i := range ri.fields {
						ri.fields[i].File = ri.originFile
					}
					out = append(out, ri)
				}
				continue
			}
		}
		ri, err := resolveInputWithRules(in, argIdx, rawCount, stdin, &st, backend, fileForBorrow, opts.RuleBlocks)
		if err != nil {
			return nil, err
		}
		out = append(out, ri)
	}
	// DR-0029 § "dead block": every named --define-rule must have matched
	// at least one SOURCE; otherwise the user wrote something that has
	// silent no-effect, which is a typo magnet.
	if len(opts.RuleBlocks) > 0 {
		if err := checkDeadBlocks(opts.RuleBlocks, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkDeadBlocks emits a usage error if any named --define-rule block
// failed to match any of the resolved SOURCES (DR-0029 § "dead block").
// Only FILE-origin inputs are checked (= VER / stdin / vcs: / cmd:
// inputs have no path-pattern semantics).
func checkDeadBlocks(blocks []ruleBlock, resolved []resolvedInput) error {
	if len(blocks) == 0 {
		return nil
	}
	matched := map[int]bool{}
	for _, ri := range resolved {
		if ri.file == "" {
			continue
		}
		m, err := resolveRuleBlock(ri.file, blocks)
		if err != nil {
			continue
		}
		if m.BlockIdx > 0 {
			matched[m.BlockIdx] = true
		}
	}
	dead := detectDeadBlocks(blocks, matched)
	if len(dead) == 0 {
		return nil
	}
	labels := make([]string, 0, len(dead))
	for _, idx := range dead {
		labels = append(labels, fmt.Sprintf("--define-rule %q", blocks[idx].Pattern))
	}
	return fmt.Errorf("dead --define-rule block(s) (no SOURCE matched): %s\nhint: remove the unused --define-rule, or add a SOURCE whose path matches the PATTERN",
		strings.Join(labels, ", "))
}

// statFileExists reports whether path resolves to a regular (non-directory)
// file on disk. Used by the empty-pipe branch of resolveFilePipeOrDisk to
// decide between falling back to the on-disk file and erroring on a likely
// typo'd path. Mirrors the "exists as a file" gate used elsewhere in
// resolveInputs.
func statFileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
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
