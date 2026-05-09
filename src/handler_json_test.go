package main

import (
	"strings"
	"testing"
)

func TestJSONInspect(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "version": "1.2.3",
  "dependencies": {
    "bar": "^1.0.0"
  }
}
`)
	insp, err := inspectVia("package.json", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "$.version" {
		t.Errorf("Versions = %+v, want one $.version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "foo" || insp.Names[0].Path != "$.name" {
		t.Errorf("Names = %+v, want one $.name=foo", insp.Names)
	}
}

func TestJSONInspect_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name": "foo"}`)
	if _, err := inspectVia("package.json", in); err == nil {
		t.Error("expected error for missing version")
	}
}

func TestJSONInspect_NoName(t *testing.T) {
	t.Parallel()
	in := []byte(`{"version": "1.2.3"}`)
	insp, err := inspectVia("package.json", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Names) != 0 {
		t.Errorf("Names should be empty when .name is absent, got %+v", insp.Names)
	}
}

func TestJSONReplace_PreservesKeyOrder(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "version": "1.2.3",
  "description": "test",
  "dependencies": {
    "nested": {
      "version": "9.9.9"
    }
  }
}
`)
	out, err := replaceVia("package.json", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"version": "2.0.0"`) {
		t.Errorf("Replace did not update top-level version\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, `"version": "9.9.9"`) {
		t.Errorf("Replace touched nested version\n--- output ---\n%s", s)
	}
	idxName := strings.Index(s, `"name"`)
	idxVer := strings.Index(s, `"version": "2.0.0"`)
	idxDesc := strings.Index(s, `"description"`)
	idxDeps := strings.Index(s, `"dependencies"`)
	if !(idxName < idxVer && idxVer < idxDesc && idxDesc < idxDeps) {
		t.Errorf("key order not preserved: name=%d ver=%d desc=%d deps=%d", idxName, idxVer, idxDesc, idxDeps)
	}
}

func TestJSONReplace_MoonModFixture(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "kawaz/example",
  "version": "0.1.0",
  "deps": {
    "moonbitlang/core": "0.1.0"
  },
  "readme": "README.md",
  "repository": "https://github.com/kawaz/example",
  "license": "MIT",
  "keywords": ["foo"],
  "description": "test"
}
`)
	out, err := replaceVia("moon.mod.json", in, "0.1.0", "0.2.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"version": "0.2.0"`) {
		t.Errorf("top-level version not updated:\n%s", s)
	}
	if !strings.Contains(s, `"moonbitlang/core": "0.1.0"`) {
		t.Errorf("deps version touched:\n%s", s)
	}
}

func TestJSONReplace_VersionAtEnd(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "description": "test",
  "license": "MIT",
  "version": "1.2.3"
}
`)
	insp, err := inspectVia("any.json", in)
	if err != nil || len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Fatalf("Inspect = %+v err=%v", insp, err)
	}
	out, err := replaceVia("any.json", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `"version": "1.2.4"`) {
		t.Errorf("version not updated:\n%s", string(out))
	}
}

// TestMarketplace_HighConfidence verifies the path-pinned high-confidence
// rule for the Claude plugin marketplace.json fixture.
func TestMarketplace_HighConfidence(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "owner": { "name": "kawaz" },
  "metadata": {
    "description": "test",
    "version": "0.3.0",
    "license": "MIT"
  },
  "plugins": []
}
`)
	insp, err := inspectVia(".claude-plugin/marketplace.json", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "0.3.0" || insp.Versions[0].Path != "$.metadata.version" {
		t.Errorf("Versions = %+v, want one $.metadata.version=0.3.0", insp.Versions)
	}
	out, err := replaceVia(".claude-plugin/marketplace.json", in, "0.3.0", "0.4.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"version": "0.4.0"`) {
		t.Errorf("metadata.version not updated:\n%s", s)
	}
	// Description and other fields are untouched
	if !strings.Contains(s, `"description": "test"`) {
		t.Errorf("description lost:\n%s", s)
	}
}

// TestMarketplace_AnyDir_MidConfidence: marketplace.json under arbitrary
// directory still resolves via the confidence-2 basename rule.
func TestMarketplace_AnyDir_MidConfidence(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name":"foo","metadata":{"version":"0.3.0"}}`)
	insp, err := inspectVia("subdir/marketplace.json", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Path != "$.metadata.version" {
		t.Errorf("Versions = %+v, want $.metadata.version", insp.Versions)
	}
}

// TestMarketplace_FallbackToVersion: marketplace.json without .metadata.version
// falls back to the *.json rule (.version) so the file still works.
func TestMarketplace_FallbackToVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name":"foo","version":"1.2.3"}`)
	insp, err := inspectVia("subdir/marketplace.json", in)
	if err != nil {
		t.Fatalf("Inspect error (should have fallen back): %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Path != "$.version" {
		t.Errorf("Versions = %+v, want $.version (fallback)", insp.Versions)
	}
}
