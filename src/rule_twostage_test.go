package main

import (
	"strings"
	"testing"
)

// --- DR-0029: path + regex 2-stage extraction --------------------------

func twoStageRule(format, path, regex string) CandidateRule {
	r := CandidateRule{Name: "test 2stage", Format: format, VersionPaths: []string{path}}
	if regex != "" {
		r.VersionRegex = regex
	}
	return r
}

func TestTwoStage_JSONInspect(t *testing.T) {
	t.Parallel()
	// $.name = "myapp v1.0.5"; regex pulls 1.0.5 out.
	content := []byte(`{"name": "myapp v1.0.5"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	insp, err := h.Inspect(content)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.0.5" {
		t.Errorf("Versions = %+v, want 1.0.5", insp.Versions)
	}
}

func TestTwoStage_JSONWrite(t *testing.T) {
	t.Parallel()
	// Bumping should rewrite ONLY the version inside the container
	// string, leaving the "myapp v" prefix intact.
	content := []byte(`{"name": "myapp v1.0.5"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	out, err := h.Replace(content, "1.0.5", "1.0.6")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `{"name": "myapp v1.0.6"}`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestTwoStage_XMLInspect(t *testing.T) {
	t.Parallel()
	content := []byte("<app>\n  <label>build myapp-2.3.4-rc</label>\n</app>\n")
	h := &cliRuleHandler{
		path:  "app.xml",
		rule:  twoStageRule("xml", "app.label", `myapp-(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "app.xml"},
	}
	insp, err := h.Inspect(content)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "2.3.4" {
		t.Errorf("Value = %q, want 2.3.4", insp.Versions[0].Value)
	}
}

func TestTwoStage_XMLWritePreservesContainer(t *testing.T) {
	t.Parallel()
	content := []byte("<app>\n  <label>build myapp-2.3.4-rc</label>\n</app>\n")
	h := &cliRuleHandler{
		path:  "app.xml",
		rule:  twoStageRule("xml", "app.label", `myapp-(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "app.xml"},
	}
	out, err := h.Replace(content, "2.3.4", "2.3.5")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := "<app>\n  <label>build myapp-2.3.5-rc</label>\n</app>\n"
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestTwoStage_RegexNoMatch(t *testing.T) {
	t.Parallel()
	content := []byte(`{"name": "no version here"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	_, err := h.Inspect(content)
	if err == nil {
		t.Fatalf("expected error: regex did not match path value")
	}
	if !strings.Contains(err.Error(), "did not match") {
		t.Errorf("error %q should mention did not match", err)
	}
}

func TestTwoStage_RegexMultiMatch(t *testing.T) {
	t.Parallel()
	// Two version-shaped substrings in the path value → ambiguous.
	content := []byte(`{"name": "v1.0.0 and v2.0.0"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	_, err := h.Inspect(content)
	if err == nil {
		t.Fatalf("expected error: regex matched twice")
	}
	if !strings.Contains(err.Error(), "exactly one match") {
		t.Errorf("error %q should mention exactly one match", err)
	}
}

func TestTwoStage_JSONWriteDigitGrowth(t *testing.T) {
	t.Parallel()
	// Version string grows in length (1.9.9 → 1.10.0): the byte-range
	// splice must shift the trailing bytes correctly.
	content := []byte(`{"name": "myapp v1.9.9 (stable)"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	out, err := h.Replace(content, "1.9.9", "1.10.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `{"name": "myapp v1.10.0 (stable)"}`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestTwoStage_XMLWriteDigitShrink(t *testing.T) {
	t.Parallel()
	// Version shrinks (1.10.0 → 1.9.0): trailing bytes must shift left.
	content := []byte("<app>\n  <label>build myapp-1.10.0-rc end</label>\n</app>\n")
	h := &cliRuleHandler{
		path:  "app.xml",
		rule:  twoStageRule("xml", "app.label", `myapp-(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "app.xml"},
	}
	out, err := h.Replace(content, "1.10.0", "1.9.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := "<app>\n  <label>build myapp-1.9.0-rc end</label>\n</app>\n"
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestTwoStage_JSONWriteBigShrink(t *testing.T) {
	t.Parallel()
	// kawaz's example: container holds 1.0.100, replaced with 1.1.0
	// (shrinks); the "myapp v" prefix and " end" suffix stay intact.
	content := []byte(`{"name": "myapp v1.0.100 end"}`)
	h := &cliRuleHandler{
		path:  "info.json",
		rule:  twoStageRule("json", ".name", `v(\d+\.\d+\.\d+)`),
		block: ruleBlock{Pattern: "info.json"},
	}
	out, err := h.Replace(content, "1.0.100", "1.1.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `{"name": "myapp v1.1.0 end"}`
	if string(out) != want {
		t.Errorf("Replace = %q, want %q", out, want)
	}
}

func TestTwoStage_MultiPathDifferentContainers(t *testing.T) {
	t.Parallel()
	// codex regression: two paths with DIFFERENT container strings but
	// the same extracted version. Each container must be rewritten
	// independently — path[0]'s container must NOT overwrite path[1].
	content := []byte(`{"name": "myapp v1.2.3", "label": "release-1.2.3"}`)
	h := &cliRuleHandler{
		path: "x.json",
		rule: CandidateRule{
			Name:         "multi",
			Format:       "json",
			VersionPaths: []string{".name", ".label"},
			VersionRegex: `(\d+\.\d+\.\d+)`,
		},
		block: ruleBlock{Pattern: "x.json"},
	}
	// Both extract 1.2.3.
	insp, err := h.Inspect(content)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	for _, f := range insp.Versions {
		if f.Value != "1.2.3" {
			t.Fatalf("Versions = %+v, want all 1.2.3", insp.Versions)
		}
	}
	out, err := h.Replace(content, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	want := `{"name": "myapp v1.2.4", "label": "release-1.2.4"}`
	if string(out) != want {
		t.Errorf("Replace =\n%q\nwant\n%q\n(.label must stay 'release-...', NOT become 'myapp v...')", out, want)
	}
}

func TestTwoStage_PathOnlyStillWorks(t *testing.T) {
	t.Parallel()
	// No regex → path value used verbatim (regression guard for the
	// 1-stage path).
	content := []byte(`{"version": "9.9.9"}`)
	h := &cliRuleHandler{
		path:  "x.json",
		rule:  twoStageRule("json", ".version", ""),
		block: ruleBlock{Pattern: "x.json"},
	}
	insp, err := h.Inspect(content)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.Versions[0].Value != "9.9.9" {
		t.Errorf("Value = %q, want 9.9.9", insp.Versions[0].Value)
	}
	out, err := h.Replace(content, "9.9.9", "10.0.0")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if string(out) != `{"version": "10.0.0"}` {
		t.Errorf("Replace = %q", out)
	}
}
