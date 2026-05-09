package main

import (
	"strings"
	"testing"
)

func TestCargoInspect(t *testing.T) {
	t.Parallel()
	in := []byte(`[package]
name = "foo"
version = "1.2.3"
edition = "2021"

[dependencies]
serde = "1"
`)
	insp, err := inspectVia("Cargo.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "[package].version" {
		t.Errorf("Versions = %+v, want one [package].version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "foo" || insp.Names[0].Path != "[package].name" {
		t.Errorf("Names = %+v, want one [package].name=foo", insp.Names)
	}
}

func TestCargoInspect_MissingPackage(t *testing.T) {
	t.Parallel()
	in := []byte(`[dependencies]
serde = "1"
`)
	if _, err := inspectVia("Cargo.toml", in); err == nil {
		t.Error("expected error for missing [package].version")
	}
}

func TestCargoInspect_NoName(t *testing.T) {
	t.Parallel()
	// [package].name は optional 扱い: 無くても Inspect は通す (Versions だけ返す)
	in := []byte(`[package]
version = "1.2.3"
edition = "2021"
`)
	insp, err := inspectVia("Cargo.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 {
		t.Errorf("Versions = %+v, want one entry", insp.Versions)
	}
	if len(insp.Names) != 0 {
		t.Errorf("Names should be empty when [package].name is missing, got %+v", insp.Names)
	}
}

func TestCargoReplace_PreservesOrderAndComments(t *testing.T) {
	t.Parallel()
	in := []byte(`# top comment
[package]
name = "foo"
version = "1.2.3"  # current version
edition = "2021"

[dependencies]
serde = "1"
# the dep below also has a "version" key — must not be touched
serde_json = { version = "1.0.0" }
`)
	out, err := replaceVia("Cargo.toml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "2.0.0"  # current version`) {
		t.Errorf("Replace did not update [package].version line\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, `serde_json = { version = "1.0.0" }`) {
		t.Errorf("Replace touched [dependencies] version\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, "# top comment") {
		t.Errorf("Replace dropped top comment\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, `name = "foo"`) {
		t.Errorf("Replace dropped name line\n--- output ---\n%s", s)
	}
}

func TestCargoReplace_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`[package]
name = "foo"
`)
	if _, err := replaceVia("Cargo.toml", in, "", "2.0.0"); err == nil {
		t.Error("expected error for missing version line")
	}
}

func TestCargoReplace_MissingPackageSection(t *testing.T) {
	t.Parallel()
	in := []byte(`[dependencies]
serde = "1"
`)
	if _, err := replaceVia("Cargo.toml", in, "", "2.0.0"); err == nil {
		t.Error("expected error for missing [package] section")
	}
}

func TestCargoReplace_SingleQuotes(t *testing.T) {
	t.Parallel()
	in := []byte(`[package]
name = "foo"
version = '1.2.3'
edition = "2021"
`)
	out, err := replaceVia("Cargo.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version = '1.2.4'`) {
		t.Errorf("single-quote style not preserved:\n%s", string(out))
	}
}

func TestCargoReplace_MultiSection(t *testing.T) {
	t.Parallel()
	in := []byte(`[package]
name = "foo"
version = "1.2.3"
edition = "2021"
authors = ["kawaz"]
license = "MIT"

[dependencies]
serde = { version = "1.0", features = ["derive"] }

[dev-dependencies]
mockito = "1.0"

[profile.release]
opt-level = 3
`)
	out, err := replaceVia("Cargo.toml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "2.0.0"`) {
		t.Errorf("[package].version not updated:\n%s", s)
	}
	if !strings.Contains(s, `serde = { version = "1.0", features = ["derive"] }`) {
		t.Errorf("dependencies version touched:\n%s", s)
	}
	if !strings.Contains(s, `mockito = "1.0"`) {
		t.Errorf("dev-dependencies line lost:\n%s", s)
	}
	if !strings.Contains(s, `opt-level = 3`) {
		t.Errorf("profile.release section lost:\n%s", s)
	}
}

func TestCargoInspect_WorkspacePackageNotMatched(t *testing.T) {
	t.Parallel()
	// [workspace.package].version は MVP では扱わない (DR-0002 参照)。
	// [package] が無い時点で Inspect がエラーになる必要がある。
	in := []byte(`[workspace]
members = ["crate-a", "crate-b"]

[workspace.package]
version = "1.0.0"
edition = "2021"
`)
	if _, err := inspectVia("Cargo.toml", in); err == nil {
		t.Error("expected error: [workspace.package].version should not be matched as [package].version")
	}
}
