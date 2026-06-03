package main

import (
	"strings"
	"testing"
)

// --- DR-0029: xml dot-path format tests ---------------------------------

func xmlRule(paths ...string) CandidateRule {
	return CandidateRule{Name: "test xml", Format: "xml", VersionPaths: paths}
}

func TestXMLDot_ChildElement(t *testing.T) {
	t.Parallel()
	doc := []byte("<project>\n  <version>1.2.3</version>\n</project>\n")
	insp, err := xmlDotInspect(xmlRule("$.project.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want 1.2.3", insp.Versions)
	}
}

func TestXMLDot_Attribute(t *testing.T) {
	t.Parallel()
	doc := []byte(`<project version="4.5.6"></project>` + "\n")
	insp, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "4.5.6" {
		t.Errorf("Versions = %+v, want 4.5.6", insp.Versions)
	}
}

func TestXMLDot_TextContentTrimmed(t *testing.T) {
	t.Parallel()
	doc := []byte("<root>\n  <ver>\n     7.8.9  \n  </ver>\n</root>\n")
	insp, err := xmlDotInspect(xmlRule("root.ver"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "7.8.9" {
		t.Errorf("Value = %q, want 7.8.9 (trimmed)", insp.Versions[0].Value)
	}
}

func TestXMLDot_AmbiguousDifferentValues(t *testing.T) {
	t.Parallel()
	// version both as attribute (1.0.0) and child element (2.0.0) →
	// ambiguous because values differ.
	doc := []byte(`<project version="1.0.0"><version>2.0.0</version></project>`)
	_, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err == nil {
		t.Fatalf("expected ambiguous error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q should mention ambiguous", err)
	}
}

func TestXMLDot_SameValueAccepted(t *testing.T) {
	t.Parallel()
	// Same value in attr and child → accepted.
	doc := []byte(`<project version="3.3.3"><version>3.3.3</version></project>`)
	insp, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "3.3.3" {
		t.Errorf("Value = %q, want 3.3.3", insp.Versions[0].Value)
	}
}

func TestXMLDot_WriteChildPreservesWhitespace(t *testing.T) {
	t.Parallel()
	doc := []byte("<root>\n  <ver>  1.0.0  </ver>\n</root>\n")
	out, err := xmlDotReplace(xmlRule("root.ver"), doc, "1.0.0", "2.0.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := "<root>\n  <ver>  2.0.0  </ver>\n</root>\n"
	if string(out) != want {
		t.Errorf("Replace =\n%q\nwant\n%q", out, want)
	}
}

func TestXMLDot_WriteAttribute(t *testing.T) {
	t.Parallel()
	doc := []byte(`<project version="1.0.0">body</project>`)
	out, err := xmlDotReplace(xmlRule("project.version"), doc, "1.0.0", "1.0.1")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<project version="1.0.1">body</project>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestXMLDot_WriteBothSameValue(t *testing.T) {
	t.Parallel()
	// child + attr same value → both rewritten.
	doc := []byte(`<project version="5.0.0"><version>5.0.0</version></project>`)
	out, err := xmlDotReplace(xmlRule("project.version"), doc, "5.0.0", "5.0.1")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<project version="5.0.1"><version>5.0.1</version></project>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestXMLDot_WriteDigitGrowthChild(t *testing.T) {
	t.Parallel()
	// 9.9.9 → 10.10.10 (each component grows a digit); surrounding
	// whitespace + tags must stay byte-identical.
	doc := []byte("<root>\n  <ver>  9.9.9  </ver>\n  <other>keep</other>\n</root>\n")
	out, err := xmlDotReplace(xmlRule("root.ver"), doc, "9.9.9", "10.10.10")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := "<root>\n  <ver>  10.10.10  </ver>\n  <other>keep</other>\n</root>\n"
	if string(out) != want {
		t.Errorf("Replace =\n%q\nwant\n%q", out, want)
	}
}

func TestXMLDot_WriteDigitGrowthAttr(t *testing.T) {
	t.Parallel()
	doc := []byte(`<project version="1.0.0" other="x">body</project>`)
	out, err := xmlDotReplace(xmlRule("project.version"), doc, "1.0.0", "1.0.100")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<project version="1.0.100" other="x">body</project>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestXMLDot_WriteDigitGrowthDualSpan(t *testing.T) {
	t.Parallel()
	// child + attr same value, version grows; both spans shift.
	doc := []byte(`<project version="1.9.9"><version>1.9.9</version>tail</project>`)
	out, err := xmlDotReplace(xmlRule("project.version"), doc, "1.9.9", "1.10.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<project version="1.10.0"><version>1.10.0</version>tail</project>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestXMLDot_WriteDigitBigShrink(t *testing.T) {
	t.Parallel()
	// kawaz's example: 1.0.100 → 1.1.0 (7 chars → 5 chars, big shrink).
	// The trailing bytes must shift left correctly.
	doc := []byte(`<root><ver>1.0.100</ver>X</root>`)
	out, err := xmlDotReplace(xmlRule("root.ver"), doc, "1.0.100", "1.1.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<root><ver>1.1.0</ver>X</root>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestXMLDot_AttrLongerNameNoSubstringMatch(t *testing.T) {
	t.Parallel()
	// "appversion" must not be matched when querying "version".
	doc := []byte(`<project appversion="9.9.9" version="1.2.3"></project>`)
	insp, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Value = %q, want 1.2.3 (not appversion's 9.9.9)", insp.Versions[0].Value)
	}
}

func TestXMLDot_SingleQuotedAttr(t *testing.T) {
	t.Parallel()
	doc := []byte(`<project version='2.2.2'></project>`)
	insp, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "2.2.2" {
		t.Errorf("Value = %q, want 2.2.2", insp.Versions[0].Value)
	}
}

func TestXMLDot_NestedPath(t *testing.T) {
	t.Parallel()
	doc := []byte("<Project>\n  <PropertyGroup>\n    <Version>1.4.0</Version>\n  </PropertyGroup>\n</Project>\n")
	insp, err := xmlDotInspect(xmlRule("Project.PropertyGroup.Version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.4.0" {
		t.Errorf("Value = %q, want 1.4.0", insp.Versions[0].Value)
	}
}

func TestXMLDot_MultipleElementsSameValue(t *testing.T) {
	t.Parallel()
	// Two <dep><version> with the SAME value → accepted, both spans
	// returned (rewritten on --write).
	doc := []byte(`<deps><dep><version>1.0.0</version></dep><dep><version>1.0.0</version></dep></deps>`)
	insp, err := xmlDotInspect(xmlRule("deps.dep.version"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.0.0" {
		t.Errorf("Value = %q, want 1.0.0", insp.Versions[0].Value)
	}
	out, err := xmlDotReplace(xmlRule("deps.dep.version"), doc, "1.0.0", "2.0.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `<deps><dep><version>2.0.0</version></dep><dep><version>2.0.0</version></dep></deps>`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q (both occurrences)", out, want)
	}
}

func TestXMLDot_MultipleElementsDifferentValues(t *testing.T) {
	t.Parallel()
	// Two matches with DIFFERENT values → ambiguous (no silent first-match).
	doc := []byte(`<deps><dep><version>1.0.0</version></dep><dep><version>2.0.0</version></dep></deps>`)
	_, err := xmlDotInspect(xmlRule("deps.dep.version"), doc)
	if err == nil {
		t.Fatalf("expected ambiguous error for differing repeated elements")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q should mention ambiguous", err)
	}
}

func TestXMLDot_SelfClosingTargetThenRealMatch(t *testing.T) {
	t.Parallel()
	// codex regression: a self-closing <ver/> matched first would
	// consume its EndElement and (pre-fix) leave the stack unbalanced,
	// so the following real <ver>1.0</ver> was missed. After the fix the
	// real value is found.
	doc := []byte(`<root><ver/><ver>1.0.0</ver></root>`)
	insp, err := xmlDotInspect(xmlRule("root.ver"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.0.0" {
		t.Errorf("Versions = %+v, want one 1.0.0 (self-closing first must not corrupt stack)", insp.Versions)
	}
}

func TestXMLDot_EmptyTargetThenSibling(t *testing.T) {
	t.Parallel()
	// Empty <ver></ver> followed by an unrelated sibling: stack must
	// stay aligned so the sibling's path still resolves.
	doc := []byte(`<root><ver></ver><name>x</name></root>`)
	// root.ver is empty (no value), root.name resolves fine.
	insp, err := xmlDotInspect(xmlRule("root.name"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "x" {
		t.Errorf("Value = %q, want x", insp.Versions[0].Value)
	}
}

func TestXMLDot_SelfClosingThenNestedTarget(t *testing.T) {
	t.Parallel()
	// <a/> self-closing at depth 2, then the real nested target. The
	// stack pop must keep depth tracking correct.
	doc := []byte(`<root><a/><group><ver>2.0.0</ver></group></root>`)
	insp, err := xmlDotInspect(xmlRule("root.group.ver"), doc)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "2.0.0" {
		t.Errorf("Value = %q, want 2.0.0", insp.Versions[0].Value)
	}
}

func TestXMLDot_NotFound(t *testing.T) {
	t.Parallel()
	doc := []byte(`<project><name>foo</name></project>`)
	_, err := xmlDotInspect(xmlRule("project.version"), doc)
	if err == nil {
		t.Fatalf("expected not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err)
	}
}

func TestXMLDot_IndexSyntaxRejected(t *testing.T) {
	t.Parallel()
	_, err := parseXMLDotPath("project.deps[0].version")
	if err == nil {
		t.Fatalf("expected error for [N] index syntax")
	}
	if !strings.Contains(err.Error(), "index syntax") {
		t.Errorf("error %q should mention index syntax", err)
	}
}

func TestParseXMLDotPath_LeadingForms(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"a.b.c", "$.a.b.c", ".a.b.c", "$a.b.c"} {
		segs, err := parseXMLDotPath(p)
		if err != nil {
			t.Errorf("parseXMLDotPath(%q): %v", p, err)
			continue
		}
		if len(segs) != 3 || segs[0] != "a" || segs[2] != "c" {
			t.Errorf("parseXMLDotPath(%q) = %v, want [a b c]", p, segs)
		}
	}
}
