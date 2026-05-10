package main

import (
	"strings"
	"testing"
)

// --- DR-0015: pbxproj format ---------------------------------------------

// TestPbxprojInspect_Single covers the trivial single-target /
// single-configuration project where `MARKETING_VERSION` shows up
// once. Inspect should still wrap the single value in a one-element
// `Versions` slice with a `line:N` path so mismatch diagnostics work
// uniformly.
func TestPbxprojInspect_Single(t *testing.T) {
	t.Parallel()
	in := []byte("		ABC = {\n			MARKETING_VERSION = 1.2.3;\n		};\n")
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 {
		t.Fatalf("Versions = %d, want 1", len(insp.Versions))
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions[0].Value = %q, want 1.2.3", insp.Versions[0].Value)
	}
	if insp.Versions[0].Path != "line:2" {
		t.Errorf("Versions[0].Path = %q, want line:2", insp.Versions[0].Path)
	}
}

// TestPbxprojInspect_MultipleAgreeing exercises the canonical
// Debug+Release shape: two `MARKETING_VERSION` lines, both equal.
// Inspect returns one Field per match (so main.go can render
// per-line context if a downstream consistency check fails); the
// caller treats them as one effective version because every value
// agrees.
func TestPbxprojInspect_MultipleAgreeing(t *testing.T) {
	t.Parallel()
	in := []byte(`/* Begin XCBuildConfiguration section */
		ABC123 /* Debug */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				MARKETING_VERSION = 1.2.3;
				CURRENT_PROJECT_VERSION = 42;
			};
		};
		DEF456 /* Release */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				MARKETING_VERSION = 1.2.3;
			};
		};
/* End XCBuildConfiguration section */
`)
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 2 {
		t.Fatalf("Versions = %d, want 2", len(insp.Versions))
	}
	for i, f := range insp.Versions {
		if f.Value != "1.2.3" {
			t.Errorf("Versions[%d].Value = %q, want 1.2.3", i, f.Value)
		}
		if !strings.HasPrefix(f.Path, "line:") {
			t.Errorf("Versions[%d].Path = %q, want line:N prefix", i, f.Path)
		}
	}
	// Lines 5 and 12 in the input above (1-based).
	if insp.Versions[0].Path != "line:5" {
		t.Errorf("Versions[0].Path = %q, want line:5", insp.Versions[0].Path)
	}
	if insp.Versions[1].Path != "line:12" {
		t.Errorf("Versions[1].Path = %q, want line:12", insp.Versions[1].Path)
	}
}

// TestPbxprojInspect_QuotedVariant: a `MARKETING_VERSION` value can be
// wrapped in double quotes (Xcode emits this when the value contains
// punctuation). Both shapes are accepted; the captured value is the
// raw text without quotes.
func TestPbxprojInspect_QuotedVariant(t *testing.T) {
	t.Parallel()
	in := []byte("				MARKETING_VERSION = \"1.2.3\";\n")
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions[0].Value = %q, want 1.2.3 (quotes stripped)", insp.Versions[0].Value)
	}
}

// TestPbxprojInspect_Mixed_QuotedAndUnquoted: the same file may carry
// quoted and unquoted variants for different configurations (Xcode
// rewrites style-by-style). Both surface with the same captured
// value, which the cross-match consistency check then accepts.
func TestPbxprojInspect_Mixed_QuotedAndUnquoted(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3;\nMARKETING_VERSION = \"1.2.3\";\n")
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 2 {
		t.Fatalf("Versions = %d, want 2", len(insp.Versions))
	}
	for i, f := range insp.Versions {
		if f.Value != "1.2.3" {
			t.Errorf("Versions[%d].Value = %q, want 1.2.3", i, f.Value)
		}
	}
}

// TestPbxprojInspect_DisagreeingValues returns multiple Fields with
// different Values so main.go's `formatMismatchError` can surface a
// column-aligned `version mismatch:` block. Inspect itself does NOT
// raise a special-purpose error — keeping the consistency check at
// the dispatcher level avoids duplicating mismatch diagnostics.
func TestPbxprojInspect_DisagreeingValues(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3;\nMARKETING_VERSION = 1.2.4;\n")
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 2 {
		t.Fatalf("Versions = %d, want 2", len(insp.Versions))
	}
	if insp.Versions[0].Value == insp.Versions[1].Value {
		t.Errorf("expected two different values, got %q == %q",
			insp.Versions[0].Value, insp.Versions[1].Value)
	}
}

// TestPbxprojInspect_NoMatch fails the rule cleanly when the file
// doesn't carry any `MARKETING_VERSION` line.
func TestPbxprojInspect_NoMatch(t *testing.T) {
	t.Parallel()
	in := []byte("// nothing relevant in here\n")
	if _, err := inspectVia("project.pbxproj", in); err == nil {
		t.Error("expected error when no MARKETING_VERSION present")
	}
}

// TestPbxprojReplace_AllMatchesSyncedUniformly: Replace overwrites
// every match with the same new value, regardless of the quote
// style each match used (we deliberately don't try to preserve per-
// match quoting differences — uniform output is the simpler and
// safer rule). Each file already has uniform style in practice.
func TestPbxprojReplace_AllMatchesSyncedUniformly(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3;\nMARKETING_VERSION = 1.2.3;\n")
	out, err := replaceVia("project.pbxproj", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	got := string(out)
	if strings.Count(got, "1.2.4") != 2 {
		t.Errorf("expected two 1.2.4 occurrences, got:\n%s", got)
	}
	if strings.Contains(got, "1.2.3") {
		t.Errorf("Replace left an unsynced 1.2.3 behind:\n%s", got)
	}
}

// TestPbxprojReplace_KeepsSurroundingLines covers the byte-range
// preservation property: lines before, between, and after the
// `MARKETING_VERSION` matches are preserved verbatim. Other build
// settings on adjacent lines (e.g. `CURRENT_PROJECT_VERSION`) must
// not move.
func TestPbxprojReplace_KeepsSurroundingLines(t *testing.T) {
	t.Parallel()
	in := []byte(`		ABC = {
			MARKETING_VERSION = 1.2.3;
			CURRENT_PROJECT_VERSION = 42;
		};
		DEF = {
			MARKETING_VERSION = 1.2.3;
			SWIFT_VERSION = 5.9;
		};
`)
	out, err := replaceVia("project.pbxproj", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	got := string(out)
	if strings.Count(got, "MARKETING_VERSION = 2.0.0;") != 2 {
		t.Errorf("expected two synced 2.0.0 lines:\n%s", got)
	}
	if !strings.Contains(got, "CURRENT_PROJECT_VERSION = 42;") {
		t.Errorf("CURRENT_PROJECT_VERSION line lost:\n%s", got)
	}
	if !strings.Contains(got, "SWIFT_VERSION = 5.9;") {
		t.Errorf("SWIFT_VERSION line lost:\n%s", got)
	}
}

// TestPbxprojReplace_QuotedPreservesQuotes: Replace splices into the
// regex's first capture group only, so the literal `"` characters
// surrounding a quoted value remain in place after rewriting.
func TestPbxprojReplace_QuotedPreservesQuotes(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = \"1.2.3\";\n")
	out, err := replaceVia("project.pbxproj", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `MARKETING_VERSION = "1.2.4";`) {
		t.Errorf("quote style not preserved:\n%s", string(out))
	}
}

// TestPbxprojInspect_IgnoresCommentMentions: a `MARKETING_VERSION`
// referenced inside a `/* ... */` block comment without the trailing
// `;` does not match. The line-anchored regex requires the
// statement-terminator `;` so comment text doesn't accidentally
// participate.
func TestPbxprojInspect_IgnoresCommentMentions(t *testing.T) {
	t.Parallel()
	in := []byte(`/* MARKETING_VERSION = bogus, no semicolon */
		ABC = {
			MARKETING_VERSION = 1.2.3;
		};
`)
	insp, err := inspectVia("project.pbxproj", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 {
		t.Errorf("expected 1 match (comment ignored), got %d", len(insp.Versions))
	}
}
