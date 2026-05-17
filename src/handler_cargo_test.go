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

func TestCargoInspect_WorkspacePackageFallback(t *testing.T) {
	t.Parallel()
	// DR-0021 (supersedes DR-0002): a workspace-root Cargo.toml has no
	// [package] section; its version lives in [workspace.package].version,
	// which member crates inherit via `version.workspace = true`. The
	// Cargo.toml rule falls back to [workspace.package].version when
	// [package].version is absent (OR / first-match-wins, same machinery
	// as pyproject.toml's [project] → [tool.poetry] fallback).
	in := []byte(`[workspace]
members = ["crate-a", "crate-b"]

[workspace.package]
version = "1.0.0"
edition = "2021"
`)
	insp, err := inspectVia("Cargo.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.0.0" || insp.Versions[0].Path != "[workspace.package].version" {
		t.Errorf("Versions = %+v, want one [workspace.package].version=1.0.0", insp.Versions)
	}
}

func TestCargoInspect_PackageWinsOverWorkspacePackage(t *testing.T) {
	t.Parallel()
	// When both [package].version and [workspace.package].version exist
	// (a member crate that also declares workspace-shared fields), the
	// crate's own [package].version takes precedence — that's the version
	// the crate actually publishes. [workspace.package] is only the
	// template inherited by members that opt in via `version.workspace`.
	in := []byte(`[package]
name = "foo"
version = "2.0.0"

[workspace.package]
version = "1.0.0"
`)
	insp, err := inspectVia("Cargo.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "2.0.0" || insp.Versions[0].Path != "[package].version" {
		t.Errorf("Versions = %+v, want [package].version=2.0.0 to win", insp.Versions)
	}
}

func TestCargoReplace_WorkspacePackage(t *testing.T) {
	t.Parallel()
	in := []byte(`[workspace]
members = ["crate-a", "crate-b"]

[workspace.package]
version = "1.0.0"  # shared by members via version.workspace = true
edition = "2021"
`)
	out, err := replaceVia("Cargo.toml", in, "1.0.0", "1.1.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "1.1.0"  # shared by members via version.workspace = true`) {
		t.Errorf("Replace did not update [workspace.package].version line\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, `members = ["crate-a", "crate-b"]`) {
		t.Errorf("Replace touched [workspace].members\n--- output ---\n%s", s)
	}
	if !strings.Contains(s, `edition = "2021"`) {
		t.Errorf("Replace dropped edition line\n--- output ---\n%s", s)
	}
}

func TestCargoReplace_PackageWinsOverWorkspacePackage(t *testing.T) {
	t.Parallel()
	// Replace must rewrite the same path Inspect chose: [package].version,
	// not [workspace.package].version.
	in := []byte(`[package]
name = "foo"
version = "2.0.0"

[workspace.package]
version = "1.0.0"
`)
	out, err := replaceVia("Cargo.toml", in, "2.0.0", "2.1.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "2.1.0"`) {
		t.Errorf("[package].version not updated:\n%s", s)
	}
	if !strings.Contains(s, `version = "1.0.0"`) {
		t.Errorf("[workspace.package].version must stay untouched:\n%s", s)
	}
}

func TestCargoInspect_NeitherPackageNorWorkspacePackage(t *testing.T) {
	t.Parallel()
	// A Cargo.toml with neither [package].version nor
	// [workspace.package].version still errors (no top-level version to
	// fall back to either — the confidence-3 Cargo.toml rule fails and
	// no lower-confidence rule rescues a non-top-level version).
	in := []byte(`[workspace]
members = ["crate-a", "crate-b"]
resolver = "2"
`)
	if _, err := inspectVia("Cargo.toml", in); err == nil {
		t.Error("expected error: no [package].version nor [workspace.package].version")
	}
}
