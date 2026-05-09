package main

import (
	"strings"
	"testing"
)

func TestJSONGet(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "version": "1.2.3",
  "dependencies": {
    "bar": "^1.0.0"
  }
}
`)
	got, err := (jsonHandler{}).Get(in)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("Get = %q, want 1.2.3", got)
	}
}

func TestJSONGet_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name": "foo"}`)
	if _, err := (jsonHandler{}).Get(in); err == nil {
		t.Error("expected error for missing version")
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
	out, err := (jsonHandler{}).Replace(in, "2.0.0")
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
	// key order must be preserved
	idxName := strings.Index(s, `"name"`)
	idxVer := strings.Index(s, `"version": "2.0.0"`)
	idxDesc := strings.Index(s, `"description"`)
	idxDeps := strings.Index(s, `"dependencies"`)
	if !(idxName < idxVer && idxVer < idxDesc && idxDesc < idxDeps) {
		t.Errorf("key order not preserved: name=%d ver=%d desc=%d deps=%d", idxName, idxVer, idxDesc, idxDeps)
	}
}

func TestJSONReplace_AmbiguousValueErrors(t *testing.T) {
	t.Parallel()
	// Top-level version 1.0.0 and a nested "version": "1.0.0" — same value.
	// Replace must refuse rather than guess.
	in := []byte(`{
  "name": "foo",
  "version": "1.0.0",
  "dependencies": {
    "bar": {"version": "1.0.0"}
  }
}
`)
	if _, err := (jsonHandler{}).Replace(in, "2.0.0"); err == nil {
		t.Error("expected error for ambiguous version match")
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
	out, err := (jsonHandler{}).Replace(in, "0.2.0")
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

func TestJSONReplace_MarketplaceFixture(t *testing.T) {
	t.Parallel()
	// claude-plugin/marketplace.json 風: top-level に version、plugins[] 各要素にも
	// version が入る。値が異なるので top-level の現在値で値特定 → 単一マッチ。
	in := []byte(`{
  "$schema": "https://example/schema.json",
  "version": "1.0.0",
  "plugins": [
    {"name": "foo", "version": "2.5.0"},
    {"name": "bar", "version": "3.4.5"}
  ]
}
`)
	out, err := (jsonHandler{}).Replace(in, "1.1.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"version": "1.1.0"`) {
		t.Errorf("top-level version not updated:\n%s", s)
	}
	if !strings.Contains(s, `"version": "2.5.0"`) || !strings.Contains(s, `"version": "3.4.5"`) {
		t.Errorf("nested plugin versions touched:\n%s", s)
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
	got, err := (jsonHandler{}).Get(in)
	if err != nil || got != "1.2.3" {
		t.Fatalf("Get = %q err=%v", got, err)
	}
	out, err := (jsonHandler{}).Replace(in, "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `"version": "1.2.4"`) {
		t.Errorf("version not updated:\n%s", string(out))
	}
}
