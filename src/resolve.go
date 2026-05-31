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
}

func resolveInputs(inputs []string, stdin io.Reader, opts resolveInputsOpts) ([]resolvedInput, error) {
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

	// Legacy stdin pipe shortcut: one FILE input (not `-`, exists or
	// at least matches a known rule), stdin is a pipe → read content
	// from stdin and treat the path as a name hint. vcs: inputs are
	// not eligible for this shortcut.
	if len(inputs) == 1 && inputs[0] != "-" && !strings.HasPrefix(inputs[0], "vcs:") && isStdinPipe(stdin) {
		if opts.Write {
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
					ri, err := resolveInput(in, argIdx, rawCount, stdin, &st, backend, bf)
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
