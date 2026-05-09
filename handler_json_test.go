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
