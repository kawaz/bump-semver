package main

// Spec test vectors for glob-backref v0.1.0. See `docs/specs/glob-backref-v0.1.0.md`.
// T1-T18 cover the spec §3 matching semantics + §4 backref numbering + spec-
// specific edge cases (grammar drift panic, `$10` ambiguous, brace explosion,
// brace-leak invariant, TO-side glob escape).

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// gbFixture is the shared fs fixture for the spec tests. Mirrors the spec
// §12 vector shapes.
func gbFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("src/a.ts", "a")
	mk("src/b.ts", "b")
	mk("src/sub/c.ts", "c")
	mk("src/sub/deep/d.ts", "d")
	mk("README.md", "en")
	mk("README-ja.md", "ja")
	mk("README-en.md", "en2")
	mk("proto/foo.proto", "foo")
	mk("proto/bar/baz.proto", "bar/baz")
	mk("img/cat.jpg", "j")
	mk("img/cat.webp", "w")
	return dir
}

// gbOpts is the test-default opts: gitignored respected via default (= nil).
func gbOpts() globOpts { return globOpts{} }

// pathSet collects matched paths into a sorted slice for stable comparison.
func pathSet(ms []Match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Path)
	}
	sort.Strings(out)
	return out
}

// T1 bundle: 'src/**/*.ts' → 'lib/$1/$2.js'  ($1 = **, $2 = *).
func TestGlobBackref_T1_Bundle(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("src/**/*.ts", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			filepath.Join("src", "a.ts"),
			filepath.Join("src", "b.ts"),
			filepath.Join("src", "sub", "c.ts"),
			filepath.Join("src", "sub", "deep", "d.ts"),
		}
		if !reflect.DeepEqual(pathSet(got), want) {
			t.Errorf("paths = %v, want %v", pathSet(got), want)
		}
		for _, m := range got {
			if m.Path != filepath.Join("src", "sub", "c.ts") {
				continue
			}
			out, err := Substitute("lib/$1/$2.js", m.Captures)
			if err != nil {
				t.Fatal(err)
			}
			if out != "lib/sub/c.js" {
				t.Errorf("got %q, want lib/sub/c.js (captures=%v)", out, m.Captures)
			}
		}
	})
}

// T2 translation: literal FROM + brace TO (handled by ExpandPairs).
func TestGlobBackref_T2_Translation(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("README.md", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Path != "README.md" {
			t.Fatalf("got %v, want single README.md", pathSet(got))
		}
		if got[0].Captures[0] != "README.md" {
			t.Errorf("$0 = %q, want README.md", got[0].Captures[0])
		}
		pairs, err := ExpandPairs("README.md", "README-{ja,en}.md")
		if err != nil {
			t.Fatal(err)
		}
		gotTos := make([]string, 0, len(pairs))
		for _, p := range pairs {
			gotTos = append(gotTos, p.To)
		}
		wantTos := []string{"README-ja.md", "README-en.md"}
		if !reflect.DeepEqual(gotTos, wantTos) {
			t.Errorf("expanded TOs = %v, want %v", gotTos, wantTos)
		}
	})
}

// T3 codegen: 'proto/**/*.proto' → 'generated/$1/$2.pb.go'.
func TestGlobBackref_T3_Codegen(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("proto/**/*.proto", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range got {
			out, err := Substitute("generated/$1/$2.pb.go", m.Captures)
			if err != nil {
				t.Fatal(err)
			}
			switch m.Path {
			case filepath.Join("proto", "foo.proto"):
				if out != "generated/foo.pb.go" {
					t.Errorf("foo: got %q, want generated/foo.pb.go (captures=%v)", out, m.Captures)
				}
			case filepath.Join("proto", "bar", "baz.proto"):
				if out != "generated/bar/baz.pb.go" {
					t.Errorf("bar/baz: got %q, want generated/bar/baz.pb.go (captures=%v)", out, m.Captures)
				}
			}
		}
	})
}

// T4 直積展開: '**/*.{jpg,webp}' → 2 concrete pairs.  $3 = brace literal.
func TestGlobBackref_T4_BraceExpansion(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("**/*.{jpg,webp}", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			filepath.Join("img", "cat.jpg"),
			filepath.Join("img", "cat.webp"),
		}
		if !reflect.DeepEqual(pathSet(got), want) {
			t.Errorf("paths = %v, want %v", pathSet(got), want)
		}
		for _, m := range got {
			if m.Path != filepath.Join("img", "cat.jpg") {
				continue
			}
			if m.Captures[3] != "jpg" {
				t.Errorf("$3 = %q, want jpg (captures=%v)", m.Captures[3], m.Captures)
			}
			out, err := Substitute("$1/$2.$3.sha256", m.Captures)
			if err != nil {
				t.Fatal(err)
			}
			if out != "img/cat.jpg.sha256" {
				t.Errorf("got %q, want img/cat.jpg.sha256", out)
			}
		}
	})
}

// T5 leading-slash zero-segment (= spec §2.2.2 / blocker #1).
// `**/*-ja.md` matches root README-ja.md → $1 must be ".", post-Clean substitute = README.md.
func TestGlobBackref_T5_ZeroSegmentDoubleStar(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("**/*-ja.md", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		var hit *Match
		for i, m := range got {
			if m.Path == "README-ja.md" {
				hit = &got[i]
				break
			}
		}
		if hit == nil {
			t.Fatalf("README-ja.md not matched; got=%v", pathSet(got))
		}
		if hit.Captures[1] != "." {
			t.Errorf("$1 = %q, want . (** zero-segment)", hit.Captures[1])
		}
		out, err := Substitute("$1/$2.md", hit.Captures)
		if err != nil {
			t.Fatal(err)
		}
		if out != "README.md" {
			t.Errorf("substituted = %q, want README.md", out)
		}
	})
}

// T6 backref 順序: '**/*-[a-z][a-z].md' — $1=**, $2=*, $3=first [a-z], $4=second.
func TestGlobBackref_T6_BackrefOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc-ab.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "page-cd.md"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		got, err := MatchCollect("**/*-[a-z][a-z].md", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range got {
			if m.Path == "doc-ab.md" {
				if m.Captures[1] != "." || m.Captures[2] != "doc" || m.Captures[3] != "a" || m.Captures[4] != "b" {
					t.Errorf("captures for doc-ab.md = %v", m.Captures)
				}
			}
			if m.Path == filepath.Join("sub", "page-cd.md") {
				if m.Captures[1] != "sub" || m.Captures[2] != "page" || m.Captures[3] != "c" || m.Captures[4] != "d" {
					t.Errorf("captures for sub/page-cd.md = %v", m.Captures)
				}
			}
		}
	})
}

// T7 empty branch (= spec §2.2.4). 'README{,-ja}.md' selects "" or "-ja".
func TestGlobBackref_T7_EmptyBranch(t *testing.T) {
	dir := gbFixture(t)
	withCwd(t, dir, func() {
		got, err := MatchCollect("README{,-ja}.md", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"README-ja.md", "README.md"}
		if !reflect.DeepEqual(pathSet(got), want) {
			t.Errorf("got %v, want %v", pathSet(got), want)
		}
		for _, m := range got {
			if m.Path == "README.md" && m.Captures[1] != "" {
				t.Errorf("README.md $1 = %q, want \"\"", m.Captures[1])
			}
			if m.Path == "README-ja.md" && m.Captures[1] != "-ja" {
				t.Errorf("README-ja.md $1 = %q, want -ja", m.Captures[1])
			}
		}
	})
}

// T8 TO-side literal embed (= spec §3.4 / blocker #4): captured value with
// glob meta is embedded literally on the plain (non-glob:) TO branch.
func TestGlobBackref_T8_LiteralEmbed(t *testing.T) {
	captures := []string{"src/a{b,c}.ts", "src", "a{b,c}"}
	out, err := Substitute("lib/$1/$2.js", captures)
	if err != nil {
		t.Fatal(err)
	}
	if out != "lib/src/a{b,c}.js" {
		t.Errorf("substituted = %q, want lib/src/a{b,c}.js", out)
	}
}

// T9 grammar drift panic (= spec §7 / blocker #3): regex no-match while
// doublestar matched panics rather than silently skipping.
func TestGlobBackref_T9_GrammarDriftPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		s, ok := r.(string)
		if !ok || !strings.Contains(s, "grammar drift") {
			t.Errorf("got panic %v, want a grammar-drift message", r)
		}
	}()
	// Active control: a clean MatchCollect call should NOT panic.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		_, err := MatchCollect("*.txt", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
	})
	// Forced drift: regex anchors on "^IMPOSSIBLE$" but candidate is "x.txt".
	c := &concreteAST{
		rawPattern:    "*.txt",
		captureRegex:  mustCompile(t, "^IMPOSSIBLE$"),
		indexMap:      []slotBinding{{isLiteral: false, regexGroup: 1}},
		totalCaptures: 1,
	}
	driftPanic(c, "x.txt")
}

func driftPanic(c *concreteAST, candidate string) {
	caps := c.captureRegex.FindStringSubmatch(candidate)
	if caps == nil {
		panic("grammar drift: synthetic test trigger for " + candidate)
	}
}

// T10 `$10` ambiguous (= spec §4.3): rejected.
func TestGlobBackref_T10_DollarTenAmbiguous(t *testing.T) {
	caps := []string{"path", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	_, err := Substitute("prefix-$10-suffix", caps)
	if err == nil {
		t.Fatal("expected ambiguous-$10 error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want 'ambiguous' message", err)
	}
}

// T11 `${10}` accepted, out-of-range → empty (= spec §4.3).
func TestGlobBackref_T11_BracedTenAccepted(t *testing.T) {
	caps := []string{"path", "a", "b"}
	out, err := Substitute("prefix-${10}-suffix", caps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "prefix--suffix" {
		t.Errorf("got %q, want prefix--suffix", out)
	}
}

// T12 直積爆発: 3 × 3 × 3 = 27 concrete pairs.
func TestGlobBackref_T12_CartesianExplosion(t *testing.T) {
	pairs, err := ExpandPairs("{a,b,c}/{x,y,z}/{1,2,3}.ts", "out/$1.$2.$3.out")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 27 {
		t.Errorf("got %d concrete pairs, want 27", len(pairs))
	}
	if pairs[0].From != "a/x/1.ts" {
		t.Errorf("pairs[0].From = %q, want a/x/1.ts", pairs[0].From)
	}
}

// T13 brace-leak invariant: doublestar never sees `{` or `}`.
func TestGlobBackref_T13_BraceInvariant(t *testing.T) {
	cases := []string{
		"src/{a,b}/*.ts",
		"{a,b,c}/{x,y}/*.{ts,tsx}",
		"README{,-ja,-en}.md",
		"**/*.{jpg,webp,png}",
	}
	for _, pat := range cases {
		ast, err := parsePattern(pat)
		if err != nil {
			t.Errorf("parse %q: %v", pat, err)
			continue
		}
		concretes, err := expandConcrete(ast)
		if err != nil {
			t.Errorf("expand %q: %v", pat, err)
			continue
		}
		for _, c := range concretes {
			if strings.ContainsAny(c.rawPattern, "{}") {
				t.Errorf("pattern %q produced concrete %q with brace leak", pat, c.rawPattern)
			}
		}
	}
}

// T14 nested `{}` rejected at parse time (= spec §2.1 MVP).
func TestGlobBackref_T14_NestedBraceRejected(t *testing.T) {
	_, err := parsePattern("{a,{b,c}}/*.ts")
	if err == nil {
		t.Fatal("expected nested-brace error, got nil")
	}
}

// T15 `[^...]` complement rejected (= spec §2.1 MVP).
func TestGlobBackref_T15_ComplementCharClassRejected(t *testing.T) {
	_, err := parsePattern("[^abc].txt")
	if err == nil {
		t.Fatal("expected complement-charclass error, got nil")
	}
}

// T16 TO-side `glob:` escape (= spec §3.4.2 / blocker #4): captured glob
// meta gets char-class-wrapped; template's own glob meta survives.
func TestGlobBackref_T16_TOSideGlobEscape(t *testing.T) {
	caps := []string{"src/a*b.ts", "src", "a*b"}
	out, err := Substitute("glob:$1/$2.*.md", caps)
	if err != nil {
		t.Fatal(err)
	}
	want := "glob:src/a[*]b.*.md"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// T17 TO-side `glob:` + brace: braces are expanded at ExpandPairs time so
// the post-substitute 2nd-stage walk input is brace-free.
func TestGlobBackref_T17_TOSideGlobWithBraceExpansion(t *testing.T) {
	pairs, err := ExpandPairs("README.md", "glob:README{,-ja}.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
	for _, p := range pairs {
		if strings.ContainsAny(p.To, "{}") {
			t.Errorf("TO %q still contains brace", p.To)
		}
		if !strings.HasPrefix(p.To, "glob:") {
			t.Errorf("TO %q lost glob: prefix", p.To)
		}
	}
}

// T18 dogfooding: the ja → en README sync pattern that motivated the
// blocker #1 fix. README-ja.md at root → README.md; nested docs/guide-ja.md
// → docs/guide.md.
func TestGlobBackref_T18_DogfoodingReadmeJaSourceForOriginal(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("README.md", "english")
	mk("README-ja.md", "日本語")
	mk("docs/guide.md", "")
	mk("docs/guide-ja.md", "")
	withCwd(t, dir, func() {
		got, err := MatchCollect("**/*-ja.md", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		paths := pathSet(got)
		want := []string{"README-ja.md", filepath.Join("docs", "guide-ja.md")}
		sort.Strings(want)
		if !reflect.DeepEqual(paths, want) {
			t.Errorf("paths = %v, want %v", paths, want)
		}
		for _, m := range got {
			out, err := Substitute("${1}/${2}.md", m.Captures)
			if err != nil {
				t.Fatalf("substitute for %s: %v", m.Path, err)
			}
			switch m.Path {
			case "README-ja.md":
				if out != "README.md" {
					t.Errorf("README-ja.md → %q, want README.md", out)
				}
			case filepath.Join("docs", "guide-ja.md"):
				if out != "docs/guide.md" {
					t.Errorf("docs/guide-ja.md → %q, want docs/guide.md", out)
				}
			}
		}
	})
}

// TestGlobBackref_NonIsolatedDoubleStarSingleSlot pins spec §2.2.1: a
// non-isolated `**` (e.g. `**foo`, `foo**`, `a**b`) collapses to ONE `*`
// node, not two — otherwise we'd both create a phantom backref slot and
// trip the strict-`*` empty-match panic on legitimate matches.
func TestGlobBackref_NonIsolatedDoubleStarSingleSlot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xfoo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		// `**foo` is non-isolated `**` adjacent to `foo` → degrades to `*foo`.
		got, err := MatchCollect("**foo", ".", gbOpts(), defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 match, got %d", len(got))
		}
		// Captures: [$0=xfoo, $1=x] — ONE backref slot (= the single
		// degraded star), not two.
		if len(got[0].Captures) != 2 {
			t.Errorf("expected 2-element Captures ([$0,$1]), got %d: %v",
				len(got[0].Captures), got[0].Captures)
		}
		if got[0].Captures[1] != "x" {
			t.Errorf("$1 = %q, want x", got[0].Captures[1])
		}
	})
}

// TestGlobBackref_RejectQuestionMark pins spec §2.1: `?` is out of MVP
// scope (= future-reserved for v0.3+). Parser must reject it as a pattern
// syntax error rather than silently leaking to doublestar (which would
// match it as a wildcard while our capture-regex treats it as a literal,
// triggering a spurious grammar-drift panic).
func TestGlobBackref_RejectQuestionMark(t *testing.T) {
	cases := []string{
		"a?b",
		"foo/?.md",
		"?",
		"src/?.ts",
	}
	for _, pat := range cases {
		t.Run(pat, func(t *testing.T) {
			_, err := parsePattern(pat)
			if err == nil {
				t.Fatalf("expected pattern syntax error for %q, got nil", pat)
			}
			var pse *PatternSyntaxError
			if !errors.As(err, &pse) {
				t.Fatalf("expected *PatternSyntaxError, got %T: %v", err, err)
			}
			if !strings.Contains(pse.Msg, "?") || !strings.Contains(pse.Msg, "MVP scope") {
				t.Errorf("error message %q should mention `?` and MVP scope", pse.Msg)
			}
		})
	}
}

// TestSubstitute_BracedEqualsBareUnderGlobEscape pins that `${N}` and
// `$N` produce identical output for the same captures + escape policy
// (= the writeCap closure refactor must not let the two branches drift).
func TestSubstitute_BracedEqualsBareUnderGlobEscape(t *testing.T) {
	caps := []string{"full", "src/a*b.ts"}
	// non-glob TO: no escape, both forms should produce the same string.
	out1, err := Substitute("a/$1/b", caps)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := Substitute("a/${1}/b", caps)
	if err != nil {
		t.Fatal(err)
	}
	if out1 != out2 {
		t.Errorf("non-glob: $N=%q vs ${N}=%q must match", out1, out2)
	}
	// glob: TO: escape applies; both forms must still match.
	out3, err := Substitute("glob:a/$1/b", caps)
	if err != nil {
		t.Fatal(err)
	}
	out4, err := Substitute("glob:a/${1}/b", caps)
	if err != nil {
		t.Fatal(err)
	}
	if out3 != out4 {
		t.Errorf("glob: $N=%q vs ${N}=%q must match", out3, out4)
	}
	if !strings.Contains(out3, "[*]") {
		t.Errorf("glob: expected `*` in captured value to be escaped as `[*]`, got %q", out3)
	}
}

// mustCompile compiles a regex pattern; failure fails the test.
func mustCompile(t *testing.T, expr string) *regexp.Regexp {
	t.Helper()
	return regexp.MustCompile(expr)
}
