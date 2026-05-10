package main

import (
	"strings"
	"testing"
)

// TestYamlInspect_TopLevel verifies the DR-0011 `*.yaml` confidence-1
// fallback extracts top-level `.version` from a Helm-style chart file.
func TestYamlInspect_TopLevel(t *testing.T) {
	t.Parallel()
	in := []byte(`apiVersion: v2
name: my-chart
version: 1.2.3
description: A Helm chart for kubernetes
appVersion: "1.0"
`)
	insp, err := inspectVia("Chart.yaml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "version" {
		t.Errorf("Versions = %+v, want one version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "my-chart" || insp.Names[0].Path != "name" {
		t.Errorf("Names = %+v, want one name=my-chart", insp.Names)
	}
}

// TestYamlInspect_YmlExtension exercises the `*.yml` glob alongside
// `*.yaml` (both must resolve through the same yaml format handler).
func TestYamlInspect_YmlExtension(t *testing.T) {
	t.Parallel()
	in := []byte("version: 0.5.1\nname: thing\n")
	insp, err := inspectVia("config.yml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "0.5.1" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

// TestYamlInspect_QuotedVersion tolerates quoted scalar values.
func TestYamlInspect_QuotedVersion(t *testing.T) {
	t.Parallel()
	in := []byte("version: \"1.2.3\"\nname: 'my-pkg'\n")
	insp, err := inspectVia("manifest.yaml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if insp.Names[0].Value != "my-pkg" {
		t.Errorf("Names = %+v", insp.Names)
	}
}

// TestYamlInspect_MissingVersion fails the rule cleanly so the
// dispatcher can keep walking (or, since this is already confidence-1,
// surface the error to the caller).
func TestYamlInspect_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte("name: my-chart\n")
	if _, err := inspectVia("Chart.yaml", in); err == nil {
		t.Error("expected error for missing top-level version")
	}
}

// TestYamlInspect_NoName accepts files without a `name` key — name is
// optional like in JSON/TOML rules.
func TestYamlInspect_NoName(t *testing.T) {
	t.Parallel()
	in := []byte("version: 1.2.3\n")
	insp, err := inspectVia("anon.yaml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if len(insp.Names) != 0 {
		t.Errorf("Names should be empty, got %+v", insp.Names)
	}
}

// TestYamlReplace_Unquoted preserves an unquoted bare scalar.
func TestYamlReplace_Unquoted(t *testing.T) {
	t.Parallel()
	in := []byte(`apiVersion: v2
name: foo
version: 1.2.3
description: hi
`)
	out, err := replaceVia("Chart.yaml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "version: 1.2.4\n") {
		t.Errorf("Replace did not write 1.2.4 unquoted:\n%s", s)
	}
	if !strings.Contains(s, "name: foo") {
		t.Errorf("Replace dropped name line:\n%s", s)
	}
	if !strings.Contains(s, "description: hi") {
		t.Errorf("Replace dropped description line:\n%s", s)
	}
}

// TestYamlReplace_DoubleQuoted keeps double-quote characters intact.
func TestYamlReplace_DoubleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte("version: \"1.2.3\"\nname: foo\n")
	out, err := replaceVia("Chart.yaml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version: \"2.0.0\"\n") {
		t.Errorf("double-quote style not preserved:\n%s", string(out))
	}
}

// TestYamlReplace_SingleQuoted keeps single-quote characters intact.
func TestYamlReplace_SingleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte("version: '1.2.3'\nname: foo\n")
	out, err := replaceVia("Chart.yaml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version: '2.0.0'\n") {
		t.Errorf("single-quote style not preserved:\n%s", string(out))
	}
}

// TestYamlReplace_PreservesTrailingComment keeps an inline `# comment`
// untouched after the rewritten value.
func TestYamlReplace_PreservesTrailingComment(t *testing.T) {
	t.Parallel()
	in := []byte("version: 1.2.3  # current\nname: foo\n")
	out, err := replaceVia("Chart.yaml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version: 1.2.4  # current\n") {
		t.Errorf("trailing comment not preserved:\n%s", string(out))
	}
}

// TestYamlReplace_DoesNotTouchNestedVersion guards the column-0 anchor:
// a nested `version:` (under another mapping) must not be picked up.
func TestYamlReplace_DoesNotTouchNestedVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`version: 1.2.3
deps:
  version: 9.9.9
`)
	out, err := replaceVia("config.yaml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "version: 1.2.4\n") {
		t.Errorf("top-level version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "  version: 9.9.9\n") {
		t.Errorf("nested version was incorrectly modified:\n%s", s)
	}
}

// TestYamlReplace_MissingVersion errors cleanly when no top-level
// version line exists.
func TestYamlReplace_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte("name: foo\n")
	if _, err := replaceVia("Chart.yaml", in, "", "1.0.0"); err == nil {
		t.Error("expected error when top-level version is absent")
	}
}

// TestYamlInspect_MultiDocumentTakesFirst documents the DR-0011 design
// decision: only the first document is examined, so the second
// document's `version:` is ignored.
func TestYamlInspect_MultiDocumentTakesFirst(t *testing.T) {
	t.Parallel()
	in := []byte(`version: 1.2.3
name: a
---
version: 9.9.9
name: b
`)
	insp, err := inspectVia("multi.yaml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("multi-document YAML must read first document, got %+v", insp.Versions)
	}
}
