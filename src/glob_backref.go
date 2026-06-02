package main

// glob-backref mini-DSL. See `docs/specs/glob-backref-v0.1.0.md` (正本 = 言語
// 非依存 spec) and `docs/decisions/DR-0028-glob-backref-spec-v0.1.0-adoption.md`
// for adoption rationale + v1 supersede範囲.
//
// Hard invariant: `{}` is fully expanded at AST level → doublestar never sees
// a `{` or `}` (= multi-language reimplementation precondition).

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---------- AST ----------------------------------------------------------

type nodeKind int

const (
	nkLiteral nodeKind = iota
	nkStar
	nkDoubleStar
	nkBrace
	nkCharClass
)

type node struct {
	kind      nodeKind
	literal   string
	charClass string
	braceAlts [][]node
}

type patternAST struct {
	nodes []node
}

// captureCount returns the number of backref-eligible slots (= spec §4.1:
// every `*`/`**`/`{}`/`[]` consumes one slot, in appearance order).
func (a *patternAST) captureCount() int {
	n := 0
	for _, nd := range a.nodes {
		switch nd.kind {
		case nkStar, nkDoubleStar, nkBrace, nkCharClass:
			n++
		}
	}
	return n
}

// ---------- Errors -------------------------------------------------------

// PatternSyntaxError is spec §7's "pattern error" class.
type PatternSyntaxError struct {
	Pattern string
	Pos     int
	Msg     string
}

func (e *PatternSyntaxError) Error() string {
	return fmt.Sprintf("pattern syntax error at pos %d in %q: %s", e.Pos, e.Pattern, e.Msg)
}

func patSyn(pat string, pos int, msg string) error {
	return &PatternSyntaxError{Pattern: pat, Pos: pos, Msg: msg}
}

// ---------- Phase A: AST parser -----------------------------------------

func parsePattern(pat string) (*patternAST, error) {
	nodes, _, err := parseNodes(pat, 0, false)
	if err != nil {
		return nil, err
	}
	return &patternAST{nodes: nodes}, nil
}

func parseNodes(pat string, start int, insideBrace bool) ([]node, int, error) {
	var nodes []node
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() > 0 {
			nodes = append(nodes, node{kind: nkLiteral, literal: lit.String()})
			lit.Reset()
		}
	}
	i := start
	for i < len(pat) {
		c := pat[i]
		if insideBrace && (c == ',' || c == '}') {
			flushLit()
			return nodes, i, nil
		}
		switch c {
		case '*':
			flushLit()
			if i+1 < len(pat) && pat[i+1] == '*' {
				if isDoubleStarIsolated(pat, i) {
					nodes = append(nodes, node{kind: nkDoubleStar})
					i += 2
				} else {
					// Spec §2.2.1: `**foo` / `foo**` / `**foo**` collapse
					// to `*foo` / `foo*` / `*foo*` (= a SINGLE `*` per
					// adjacency, one slot total). Two stars at the same
					// position would create a phantom slot AND trigger the
					// strict-`*` empty-match panic spuriously.
					nodes = append(nodes, node{kind: nkStar})
					i += 2
				}
			} else {
				nodes = append(nodes, node{kind: nkStar})
				i++
			}
		case '{':
			flushLit()
			alts, end, err := parseBrace(pat, i)
			if err != nil {
				return nil, 0, err
			}
			nodes = append(nodes, node{kind: nkBrace, braceAlts: alts})
			i = end + 1
		case '}':
			return nil, 0, patSyn(pat, i, "unexpected `}` (no matching `{`)")
		case ',':
			lit.WriteByte(c)
			i++
		case '[':
			flushLit()
			body, end, err := parseCharClass(pat, i)
			if err != nil {
				return nil, 0, err
			}
			nodes = append(nodes, node{kind: nkCharClass, charClass: body})
			i = end + 1
		case ']':
			return nil, 0, patSyn(pat, i, "unexpected `]` (no matching `[`)")
		case '?':
			// Spec §2.1: `?` is out of MVP scope (= future-reserved for v0.3+).
			// Reject explicitly so it never reaches doublestar (which would
			// match it as a wildcard while our capture-regex treats it as
			// literal, triggering a spurious grammar-drift panic).
			return nil, 0, patSyn(pat, i, "`?` is out of MVP scope (§2.1, future-reserved for v0.3+)")
		default:
			lit.WriteByte(c)
			i++
		}
	}
	if insideBrace {
		return nil, 0, patSyn(pat, start-1, "unterminated `{` (missing `}`)")
	}
	flushLit()
	return nodes, i, nil
}

// parseBrace parses `{alt1,alt2,...}` starting at pat[start] == '{'.
// MVP §2.1: nested `{}` is invalid (rejected here).
func parseBrace(pat string, start int) ([][]node, int, error) {
	if pat[start] != '{' {
		return nil, 0, patSyn(pat, start, "internal: parseBrace called on non-`{`")
	}
	var alts [][]node
	i := start + 1
	for {
		altNodes, stop, err := parseNodes(pat, i, true)
		if err != nil {
			return nil, 0, err
		}
		for _, nd := range altNodes {
			if nd.kind == nkBrace {
				return nil, 0, patSyn(pat, start, "nested `{}` not allowed in MVP (§2.1)")
			}
		}
		alts = append(alts, altNodes)
		if stop >= len(pat) {
			return nil, 0, patSyn(pat, start, "unterminated `{` (reached EOF)")
		}
		if pat[stop] == '}' {
			return alts, stop, nil
		}
		// pat[stop] == ','
		i = stop + 1
	}
}

// parseCharClass parses `[...]` starting at pat[start] == '['. MVP §2.1:
// `[^...]` / `[!...]` complement is rejected.
func parseCharClass(pat string, start int) (string, int, error) {
	if pat[start] != '[' {
		return "", 0, patSyn(pat, start, "internal: parseCharClass called on non-`[`")
	}
	end := strings.IndexByte(pat[start:], ']')
	if end < 0 {
		return "", 0, patSyn(pat, start, "unterminated `[` (missing `]`)")
	}
	body := pat[start+1 : start+end]
	if body == "" {
		return "", 0, patSyn(pat, start, "empty char class `[]`")
	}
	if body[0] == '^' || body[0] == '!' {
		return "", 0, patSyn(pat, start, "complement char class `[^...]` is out of MVP scope (§2.1)")
	}
	return body, start + end, nil
}

// isDoubleStarIsolated implements spec §2.2.1: `**` is recursive only when
// both sides are `/` or string-boundary.
func isDoubleStarIsolated(pat string, i int) bool {
	leftOK := i == 0 || pat[i-1] == '/'
	end := i + 2
	rightOK := end == len(pat) || pat[end] == '/'
	return leftOK && rightOK
}

// ---------- Phase B: brace expansion ------------------------------------

// concreteAST is one fully-brace-expanded pattern, ready for fs walk +
// capture extraction. `rawPattern` is `{}`-free (= hard invariant).
//
// `groupKinds` is the wildcard kind for each regex group (1-indexed; index
// 0 is unused). Recorded once at buildRawAndRegex time so the grammar-
// drift assertions (= empty-string match policy in spec §2.2.2 / §2.2.3)
// can branch on kind without a 3x AST re-walk per slot per path
// (= /simplify-1 cleanup).
type concreteAST struct {
	rawPattern    string
	captureRegex  *regexp.Regexp
	indexMap      []slotBinding
	groupKinds    []wildKind
	totalCaptures int
}

// wildKind tags which kind of wildcard a regex group came from. Used by
// the empty-match branches inside MatchCollect.
type wildKind int

const (
	wkNone       wildKind = iota // unused (index 0 in groupKinds)
	wkStar                       // `*` — strict (empty match = grammar drift)
	wkDoubleStar                 // `**` — empty match → "." (spec §2.2.2)
	wkCharClass                  // `[...]` — strict (empty match = grammar drift)
)

func (k wildKind) name() string {
	switch k {
	case wkStar:
		return "*"
	case wkDoubleStar:
		return "**"
	case wkCharClass:
		return "[]"
	default:
		return "?"
	}
}

// slotBinding tells substitute-time how to materialize a given backref slot:
//   - isLiteral=true: use `literal` (= chosen brace alternative's source
//     text). Empty literal is valid (§2.2.4 empty alt, §4.2 unselected alt).
//   - isLiteral=false: fetch `caps[regexGroup]`; `regexGroup<0` means the
//     slot belongs to an unselected brace branch (= empty per §4.2).
type slotBinding struct {
	isLiteral  bool
	literal    string
	regexGroup int
}

// expandConcrete fans out the AST into all `{}` combinations.
//
// `caseInsensitive` controls capture-regex case-folding (= spec §3.2 / fix
// for OQ-25): when the fs walk layer (= doublestar via expandGlob) runs
// case-insensitively, the capture regex MUST do the same; otherwise a
// case-different match (= e.g. pattern `*.md`, on-disk `README.MD`) causes
// the doublestar match × regex no-match split that §3.3 reports as a
// grammar-drift panic. The flag is threaded only here (= the single
// `MatchCollect → expandConcrete → buildRawAndRegex` chain that compiles
// the regex); `ExpandPairs` / `Substitute` do not touch capture regex.
func expandConcrete(ast *patternAST, caseInsensitive bool) ([]*concreteAST, error) {
	total := ast.captureCount()
	type frame struct {
		resolved []node
		bindings []slotBinding
	}
	frames := []frame{{bindings: make([]slotBinding, total)}}
	slotIdx := 0
	for _, nd := range ast.nodes {
		switch nd.kind {
		case nkLiteral:
			for k := range frames {
				frames[k].resolved = append(frames[k].resolved, nd)
			}
		case nkStar, nkDoubleStar, nkCharClass:
			for k := range frames {
				frames[k].resolved = append(frames[k].resolved, nd)
				frames[k].bindings[slotIdx] = slotBinding{isLiteral: false, regexGroup: -1}
			}
			slotIdx++
		case nkBrace:
			localSlot := slotIdx
			var next []frame
			for _, fr := range frames {
				for _, alt := range nd.braceAlts {
					nf := frame{
						resolved: append([]node(nil), fr.resolved...),
						bindings: append([]slotBinding(nil), fr.bindings...),
					}
					nf.bindings[localSlot] = slotBinding{
						isLiteral: true,
						literal:   altLiteral(alt),
					}
					// Alt may contain `*`/`**`/`[]` → those still consume the
					// ORIGINAL slot of the brace (= literal text contains the
					// raw `*`), but the alt nodes themselves are emitted into
					// rawPattern so they participate in fs match + capture.
					// However, per spec §4.1 the brace slot itself is ONE
					// slot, and §2.1 disallows nested braces so alts contain
					// only literals/`*`/`**`/`[]`. Those `*`/`**`/`[]` get
					// emitted as raw chars (no new slot — see test T4 where
					// `{jpg,webp}` binds `$3=jpg` literal, no separate slot
					// for the implicit branch text).
					nf.resolved = append(nf.resolved, alt...)
					next = append(next, nf)
				}
			}
			frames = next
			slotIdx++
		}
	}

	out := make([]*concreteAST, 0, len(frames))
	for _, fr := range frames {
		raw, regex, groupOrder, groupKinds, err := buildRawAndRegex(fr.resolved, caseInsensitive)
		if err != nil {
			return nil, err
		}
		gIdx := 0
		for k := range fr.bindings {
			if fr.bindings[k].isLiteral {
				continue
			}
			if gIdx >= len(groupOrder) {
				return nil, fmt.Errorf("internal: ran out of regex groups for slot %d", k+1)
			}
			fr.bindings[k] = slotBinding{
				isLiteral:  false,
				regexGroup: groupOrder[gIdx],
			}
			gIdx++
		}
		if strings.ContainsAny(raw, "{}") {
			return nil, fmt.Errorf("internal: concrete rawPattern still contains `{` or `}`: %q", raw)
		}
		out = append(out, &concreteAST{
			rawPattern:    raw,
			captureRegex:  regex,
			indexMap:      fr.bindings,
			groupKinds:    groupKinds,
			totalCaptures: total,
		})
	}
	return out, nil
}

// appendNodeSource writes the source-style serialization of a node sequence
// into sb. Used by altLiteral, buildRawAndRegex (raw side), and
// braceLiteralExpansions (non-brace cases) so the three serializers stay
// in sync (= /simplify-2 cleanup).
//
// nkBrace is intentionally not handled here: callers that need brace
// handling (= braceLiteralExpansions) treat it themselves; the other call
// sites operate on brace-free node sequences (post-expansion or inside an
// alternative where §2.1 forbids nesting).
func appendNodeSource(sb *strings.Builder, nodes []node) {
	for _, nd := range nodes {
		switch nd.kind {
		case nkLiteral:
			sb.WriteString(nd.literal)
		case nkStar:
			sb.WriteByte('*')
		case nkDoubleStar:
			sb.WriteString("**")
		case nkCharClass:
			sb.WriteByte('[')
			sb.WriteString(nd.charClass)
			sb.WriteByte(']')
		case nkBrace:
			// Unreachable in the call sites that use appendNodeSource:
			//   - altLiteral: §2.1 forbids nested `{}`.
			//   - buildRawAndRegex raw side: nodes are post-brace-expansion.
			//   - braceLiteralExpansions: brace case handled by caller.
			// Treated as internal error so a future caller can't silently
			// produce wrong output.
			panic("internal: appendNodeSource reached nkBrace (call site must handle brace separately)")
		}
	}
}

// altLiteral serializes an alternative's nodes back to source-style text
// (= the literal `$N` value bound by the brace slot, per spec §4.2 example).
func altLiteral(alt []node) string {
	var sb strings.Builder
	appendNodeSource(&sb, alt)
	return sb.String()
}

// ---------- Phase C: capture regex generation ---------------------------

// buildRawAndRegex emits the doublestar-style rawPattern + a regex with one
// capture group per `*`/`**`/`[]`. `**` zero-segment semantics (= spec
// §2.2.2) require non-greedy `(.*?)` plus surrounding `/` absorption.
//
// Returns:
//   - raw         — doublestar-fed pattern (`{}`-free, hard invariant).
//   - re          — capture regex.
//   - groupOrder  — per-slot regex-group number, in appearance order.
//   - groupKinds  — per-regex-group wildcard kind (1-indexed; index 0 = wkNone).
//     Used by MatchCollect's empty-match branches to know whether to
//     panic (strict `*`/`[]`) or substitute "." (`**`) without a re-walk.
//
// `caseInsensitive` (= fix for OQ-25): when true, the regex is compiled with
// the `(?i)` flag so it matches paths in the same case-folding mode as the
// fs walk (= doublestar's WithCaseInsensitive). Without this, case-different
// matches (= pattern `*.md` vs on-disk `README.MD`) trigger a spurious
// §3.3 grammar-drift panic. `(?i)` is placed right after `^` so the
// subsequent `**` rewrites (which inspect/trim only the regex tail) are
// unaffected.
func buildRawAndRegex(nodes []node, caseInsensitive bool) (string, *regexp.Regexp, []int, []wildKind, error) {
	var raw strings.Builder
	appendNodeSource(&raw, nodes)

	var rx strings.Builder
	rx.WriteByte('^')
	if caseInsensitive {
		rx.WriteString("(?i)")
	}
	var groupOrder []int
	groupKinds := []wildKind{wkNone} // index 0 reserved.
	group := 0
	skipNextLeadingSlash := false
	for idx, nd := range nodes {
		switch nd.kind {
		case nkLiteral:
			lit := nd.literal
			if skipNextLeadingSlash {
				lit = strings.TrimPrefix(lit, "/")
				skipNextLeadingSlash = false
			}
			rx.WriteString(regexp.QuoteMeta(lit))
		case nkStar:
			group++
			groupOrder = append(groupOrder, group)
			groupKinds = append(groupKinds, wkStar)
			rx.WriteString("([^/]*)")
		case nkDoubleStar:
			group++
			groupOrder = append(groupOrder, group)
			groupKinds = append(groupKinds, wkDoubleStar)
			rxStr := rx.String()
			leftSlash := strings.HasSuffix(rxStr, "/")
			rightSlash := false
			if idx+1 < len(nodes) && nodes[idx+1].kind == nkLiteral && strings.HasPrefix(nodes[idx+1].literal, "/") {
				rightSlash = true
			}
			switch {
			case leftSlash && rightSlash:
				trimmed := rxStr[:len(rxStr)-1]
				rx.Reset()
				rx.WriteString(trimmed)
				rx.WriteString("(?:/(.*?))?/?")
				skipNextLeadingSlash = true
			case rightSlash:
				rx.WriteString("(?:(.*?)/)?")
				skipNextLeadingSlash = true
			case leftSlash:
				trimmed := rxStr[:len(rxStr)-1]
				rx.Reset()
				rx.WriteString(trimmed)
				rx.WriteString("(?:/(.*?))?")
			default:
				rx.WriteString("(.*?)")
			}
		case nkCharClass:
			group++
			groupOrder = append(groupOrder, group)
			groupKinds = append(groupKinds, wkCharClass)
			rx.WriteString("([")
			rx.WriteString(nd.charClass)
			rx.WriteString("])")
		case nkBrace:
			return "", nil, nil, nil, fmt.Errorf("internal: nkBrace reached buildRawAndRegex (expansion bug)")
		}
	}
	rx.WriteByte('$')
	re, err := regexp.Compile(rx.String())
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("regex compile failed: %w", err)
	}
	return raw.String(), re, groupOrder, groupKinds, nil
}

// ---------- Phase D: fs walk + match ------------------------------------

// Match is spec §15.1's Match type. `Captures[0]` = $0 (= full matched
// path), `Captures[k]` = $k for k=1..N (= per-slot bindings in original-AST
// appearance order).
type Match struct {
	Path     string
	Captures []string
}

// MatchCollect implements spec §15.1 matchCollect. `pattern` is the pattern
// body (= `glob:` prefix already stripped by caller). `root` selects the
// fs walk base: `""` / `"."` = cwd-relative.
//
// fs walk delegates to `expandGlob` (DR-0024) so `--glob-dotfile` /
// `--glob-gitignored` / `--glob-ignorecase` semantics stay uniform across
// the codebase (= no parallel doublestar option mapping).
func MatchCollect(pattern, root string, opts globOpts, homeFn func() (string, error)) ([]Match, error) {
	ast, err := parsePattern(pattern)
	if err != nil {
		return nil, err
	}
	concretes, err := expandConcrete(ast, opts.IgnoreCase)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []Match
	for _, c := range concretes {
		if strings.ContainsAny(c.rawPattern, "{}") {
			panic(fmt.Sprintf("internal: brace leaked into doublestar input: %q", c.rawPattern))
		}
		paths, err := walkOne(c.rawPattern, root, opts, homeFn)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			if seen[p] {
				continue
			}
			seen[p] = true
			candidate := filepath.ToSlash(p)
			caps := c.captureRegex.FindStringSubmatch(candidate)
			if caps == nil {
				// Spec §3.3 / §7: grammar drift = panic (= release gate
				// silent failure防止).
				panic(fmt.Sprintf("grammar drift: doublestar matched %q but capture-regex %q did not (pattern=%q)", candidate, c.captureRegex.String(), c.rawPattern))
			}
			full := make([]string, c.totalCaptures+1)
			full[0] = p
			for k, b := range c.indexMap {
				if b.isLiteral {
					full[k+1] = b.literal
				} else if b.regexGroup < 0 {
					full[k+1] = "" // unselected branch — spec §4.2
				} else {
					if b.regexGroup >= len(caps) {
						panic(fmt.Sprintf("internal: regexGroup=%d out of range (caps=%d)", b.regexGroup, len(caps)))
					}
					v := caps[b.regexGroup]
					kind := wkNone
					if b.regexGroup >= 0 && b.regexGroup < len(c.groupKinds) {
						kind = c.groupKinds[b.regexGroup]
					}
					if v == "" {
						switch kind {
						case wkStar, wkCharClass:
							// Spec §2.2.3: `*` / `[]` empty match = grammar drift.
							panic(fmt.Sprintf("grammar drift: %s wildcard matched empty string at pattern %q, path %q", kind.name(), c.rawPattern, candidate))
						case wkDoubleStar:
							v = "." // Spec §2.2.2: `**` 0-segment → ".".
						}
					}
					full[k+1] = v
				}
			}
			out = append(out, Match{Path: p, Captures: full})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// walkOne runs the fs walk for one concrete (brace-free) pattern.
// Delegates to expandGlob (DR-0024) so dotfile / gitignored / ignorecase
// flags are uniformly respected.
func walkOne(pattern, root string, opts globOpts, homeFn func() (string, error)) ([]string, error) {
	full := pattern
	if root != "" && root != "." {
		full = filepath.Join(root, pattern)
	}
	matches, err := expandGlob(full, opts, homeFn)
	if err != nil {
		return nil, err
	}
	if root != "" && root != "." {
		prefix := root + string(filepath.Separator)
		for i, m := range matches {
			if strings.HasPrefix(m, prefix) {
				matches[i] = strings.TrimPrefix(m, prefix)
			}
		}
	}
	return matches, nil
}

// ---------- Phase E: TO substitute --------------------------------------

// Substitute implements spec §15.1 substitute. `captures` is `[$0, $1, ...]`.
// If `template` starts with `glob:`, capture values are char-class-wrap
// escaped (§3.4.2) so glob meta in values can't be re-globbed at the 2nd
// stage; the template's own glob meta is preserved.
//
// Errors:
//   - `$10` ambiguous (§4.3)
//   - trailing `$`
//   - unterminated `${`
//   - non-numeric `${...}` body (§4.4 named capture reject)
//
// The `glob:` prefix on `template` controls both the escape policy AND
// whether the output is path.Clean'd (= glob: outputs are fed to a 2nd-
// stage walker that handles its own normalization).
func Substitute(template string, captures []string) (string, error) {
	hasGlob := strings.HasPrefix(template, "glob:")
	body := template
	if hasGlob {
		body = strings.TrimPrefix(template, "glob:")
	}
	out, err := substituteCore(body, captures, hasGlob)
	if err != nil {
		return "", err
	}
	if hasGlob {
		return "glob:" + out, nil // caller does the 2nd-stage walk; no Clean.
	}
	return path.Clean(out), nil
}

// substituteBody is the internal variant that runs `substituteCore` on
// `body` directly (no `glob:` prefix handling) with explicit escape
// policy. Returns the raw substituted string WITHOUT path.Clean — the
// caller decides whether to normalize. Used by `derivedRowsFor` to avoid
// the previous "re-attach glob: → Substitute → strip glob:" dance
// (= /simplify-4 cleanup).
func substituteBody(body string, captures []string, escapeGlob bool) (string, error) {
	return substituteCore(body, captures, escapeGlob)
}

func substituteCore(template string, captures []string, escapeGlob bool) (string, error) {
	var sb strings.Builder
	// writeCap centralizes the lookup + escape + write tail so the `$N`
	// and `${N}` branches stay in sync (= /simplify-3 cleanup; pinned by
	// TestSubstitute_BracedEqualsBareUnderGlobEscape).
	writeCap := func(n int) {
		v := lookupCap(captures, n)
		if escapeGlob {
			v = classWrapEscape(v)
		}
		sb.WriteString(v)
	}
	i := 0
	for i < len(template) {
		c := template[i]
		if c != '$' {
			sb.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(template) {
			return "", fmt.Errorf("trailing `$` in template %q", template)
		}
		next := template[i+1]
		if next == '{' {
			end := strings.IndexByte(template[i:], '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated `${` in template %q", template)
			}
			refBody := template[i+2 : i+end]
			n, err := parseBracedRef(refBody)
			if err != nil {
				return "", fmt.Errorf("template %q: ${%s}: %w", template, refBody, err)
			}
			writeCap(n)
			i += end + 1
			continue
		}
		if next >= '0' && next <= '9' {
			if i+2 < len(template) && template[i+2] >= '0' && template[i+2] <= '9' {
				return "", fmt.Errorf("template %q: `$%c%c` is ambiguous — use `${%c}%c` or `${%c%c}`", template, next, template[i+2], next, template[i+2], next, template[i+2])
			}
			writeCap(int(next - '0'))
			i += 2
			continue
		}
		return "", fmt.Errorf("template %q: invalid `$%c`", template, next)
	}
	return sb.String(), nil
}

// classWrapEscape implements spec §3.4.2: wrap each glob meta in `[]` to
// neutralize it for a 2nd-stage glob walk.
func classWrapEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '*', '?', '{', '}', '[', ']', ',':
			b.WriteByte('[')
			b.WriteRune(c)
			b.WriteByte(']')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// parseBracedRef accepts digits only; `${name}` named refs are spec §4.4
// v0.2 scope-out.
func parseBracedRef(body string) (int, error) {
	if body == "" {
		return 0, errors.New("empty `${}`")
	}
	for _, c := range body {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-numeric ${name} not supported in v0.1.0 (use ${N})")
		}
	}
	n := 0
	for _, c := range body {
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// lookupCap returns captures[n], or "" when n is out of range (spec §4.3).
func lookupCap(captures []string, n int) string {
	if n < 0 || n >= len(captures) {
		return ""
	}
	return captures[n]
}

// ---------- Phase F: ExpandPairs ----------------------------------------

// PairConcrete is one (FROM, TO) pair after both have been brace-expanded.
type PairConcrete struct {
	From string
	To   string
}

// ExpandPairs implements spec §15.1 expandPairs (= §3.1 直積展開). The
// Cartesian product order: outer = FROM branches, inner = TO branches.
//
// IMPORTANT: this is for the TO-side `{}` expansion (= mandatory derived
// path discovery). FROM-side `{}` is handled INSIDE MatchCollect (= it
// preserves the brace slot's `$N` binding); do NOT route FROM through
// ExpandPairs and feed the concrete sub-patterns into MatchCollect — that
// drops the FROM-brace's $N slot.
func ExpandPairs(from, to string) ([]PairConcrete, error) {
	fromAST, err := parsePattern(from)
	if err != nil {
		return nil, fmt.Errorf("FROM: %w", err)
	}
	toAST, err := parsePattern(to)
	if err != nil {
		return nil, fmt.Errorf("TO: %w", err)
	}
	fromConcretes := braceLiteralExpansions(fromAST)
	toConcretes := braceLiteralExpansions(toAST)
	var out []PairConcrete
	for _, f := range fromConcretes {
		for _, t := range toConcretes {
			out = append(out, PairConcrete{From: f, To: t})
		}
	}
	return out, nil
}

// braceLiteralExpansions returns the source-level brace expansions of an
// AST (= only `{}` resolved, `*`/`**`/`[]` emitted back as their source
// forms via appendNodeSource).
func braceLiteralExpansions(ast *patternAST) []string {
	results := []string{""}
	for _, nd := range ast.nodes {
		if nd.kind == nkBrace {
			var next []string
			for _, base := range results {
				for _, alt := range nd.braceAlts {
					next = append(next, base+altLiteral(alt))
				}
			}
			results = next
			continue
		}
		var sb strings.Builder
		appendNodeSource(&sb, []node{nd})
		piece := sb.String()
		for i := range results {
			results[i] += piece
		}
	}
	return results
}
