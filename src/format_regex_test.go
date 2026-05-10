package main

import (
	"strings"
	"testing"
)

// --- DR-0012: regex format -----------------------------------------------

// --- xcconfig ------------------------------------------------------------

// TestRegexInspect_Xcconfig pulls MARKETING_VERSION out of an Xcode
// build configuration file. xcconfig values are unquoted; the regex
// trims at whitespace / `;` / inline `//` comment.
func TestRegexInspect_Xcconfig(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3\nCURRENT_PROJECT_VERSION = 42\n")
	insp, err := inspectVia("Release.xcconfig", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want one 1.2.3", insp.Versions)
	}
}

// TestRegexInspect_XcconfigWithInlineComment exercises the inline-comment
// terminator: an xcconfig value runs up to `//`, not into it.
func TestRegexInspect_XcconfigWithInlineComment(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3 // bumped weekly\n")
	insp, err := inspectVia("Release.xcconfig", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want 1.2.3 (no comment)", insp.Versions)
	}
}

// TestRegexReplace_Xcconfig keeps the `=` spacing and surrounding lines
// intact; only the captured value is rewritten.
func TestRegexReplace_Xcconfig(t *testing.T) {
	t.Parallel()
	in := []byte("MARKETING_VERSION = 1.2.3\nCURRENT_PROJECT_VERSION = 42\n")
	out, err := replaceVia("Release.xcconfig", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "MARKETING_VERSION = 1.2.4\n") {
		t.Errorf("Replace did not write 1.2.4:\n%s", s)
	}
	if !strings.Contains(s, "CURRENT_PROJECT_VERSION = 42") {
		t.Errorf("CURRENT_PROJECT_VERSION line lost:\n%s", s)
	}
}

// --- podspec -------------------------------------------------------------

// TestRegexInspect_Podspec extracts version from a CocoaPods podspec.
// Both `s.version = '...'` (single-quoted) and `spec.version = "..."`
// (double-quoted, alternate receiver) must work.
func TestRegexInspect_Podspec(t *testing.T) {
	t.Parallel()
	in := []byte(`Pod::Spec.new do |s|
  s.name         = 'MyPod'
  s.version      = '1.2.3'
  s.summary      = 'short summary'
end
`)
	insp, err := inspectVia("MyPod.podspec", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "MyPod" {
		t.Errorf("Names = %+v, want MyPod", insp.Names)
	}
}

func TestRegexInspect_PodspecSpecReceiver(t *testing.T) {
	t.Parallel()
	in := []byte(`Pod::Spec.new do |spec|
  spec.name    = "AltPod"
  spec.version = "0.5.1"
end
`)
	insp, err := inspectVia("AltPod.podspec", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "0.5.1" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if insp.Names[0].Value != "AltPod" {
		t.Errorf("Names = %+v, want AltPod", insp.Names)
	}
}

// TestRegexReplace_PodspecPreservesQuoteStyle keeps single quotes single
// and double quotes double when rewriting.
func TestRegexReplace_PodspecPreservesQuoteStyle(t *testing.T) {
	t.Parallel()
	in := []byte("s.version = '1.2.3'\n")
	out, err := replaceVia("MyPod.podspec", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "s.version = '2.0.0'") {
		t.Errorf("single-quote style not preserved:\n%s", string(out))
	}
}

func TestRegexReplace_PodspecDoubleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte("spec.version = \"1.2.3\"\n")
	out, err := replaceVia("MyPod.podspec", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `spec.version = "2.0.0"`) {
		t.Errorf("double-quote style not preserved:\n%s", string(out))
	}
}

// --- nimble --------------------------------------------------------------

// TestRegexInspect_Nimble pulls `version = "..."` out of a Nim package
// manifest (NimScript top-level assignment).
func TestRegexInspect_Nimble(t *testing.T) {
	t.Parallel()
	in := []byte(`# Package
version       = "1.2.3"
author        = "Someone"
description   = "A neat library"
license       = "MIT"
`)
	insp, err := inspectVia("foo.nimble", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

func TestRegexReplace_Nimble(t *testing.T) {
	t.Parallel()
	in := []byte("version = \"1.2.3\"\nauthor = \"x\"\n")
	out, err := replaceVia("foo.nimble", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version = "1.2.4"`) {
		t.Errorf("Replace did not write 1.2.4:\n%s", string(out))
	}
}

// --- v.mod ---------------------------------------------------------------

// TestRegexInspect_VMod parses V's `v.mod` (basename rule, confidence 2).
// V's manifest uses a mapping-like syntax with single-quoted strings.
func TestRegexInspect_VMod(t *testing.T) {
	t.Parallel()
	in := []byte(`Module {
	name: 'mything'
	description: ''
	version: '1.2.3'
	license: 'MIT'
	dependencies: []
}
`)
	insp, err := inspectVia("v.mod", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "mything" {
		t.Errorf("Names = %+v, want mything", insp.Names)
	}
}

func TestRegexReplace_VMod(t *testing.T) {
	t.Parallel()
	in := []byte("Module {\n\tversion: '1.2.3'\n}\n")
	out, err := replaceVia("v.mod", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version: '2.0.0'") {
		t.Errorf("Replace did not preserve single quotes:\n%s", string(out))
	}
}

// --- build.zig.zon -------------------------------------------------------

// TestRegexInspect_BuildZigZon parses Zig's package manifest. ZON is a
// struct-literal-shaped file; `.version = "..."` is the version field.
func TestRegexInspect_BuildZigZon(t *testing.T) {
	t.Parallel()
	in := []byte(`.{
    .name = "myapp",
    .version = "1.2.3",
    .dependencies = .{},
}
`)
	insp, err := inspectVia("build.zig.zon", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

func TestRegexReplace_BuildZigZon(t *testing.T) {
	t.Parallel()
	in := []byte(".{\n    .version = \"1.2.3\",\n    .name = \"x\",\n}\n")
	out, err := replaceVia("build.zig.zon", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `.version = "1.2.4"`) {
		t.Errorf("version not bumped:\n%s", s)
	}
	if !strings.Contains(s, `.name = "x"`) {
		t.Errorf("name line lost:\n%s", s)
	}
}

// --- gemspec -------------------------------------------------------------

// TestRegexInspect_Gemspec mirrors the podspec test against a Ruby
// gemspec (same DSL shape, different ecosystem).
func TestRegexInspect_Gemspec(t *testing.T) {
	t.Parallel()
	in := []byte(`Gem::Specification.new do |s|
  s.name    = "mygem"
  s.version = "1.2.3"
  s.summary = "..."
end
`)
	insp, err := inspectVia("mygem.gemspec", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if insp.Names[0].Value != "mygem" {
		t.Errorf("Names = %+v, want mygem", insp.Names)
	}
}

func TestRegexReplace_Gemspec(t *testing.T) {
	t.Parallel()
	in := []byte("s.version = \"1.2.3\"\n")
	out, err := replaceVia("mygem.gemspec", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `s.version = "1.2.4"`) {
		t.Errorf("Replace did not write 1.2.4:\n%s", string(out))
	}
}

// --- mix.exs -------------------------------------------------------------

// TestRegexInspect_MixExs reads version from an Elixir project file.
// version sits inside `def project do [..., version: "..."]`.
func TestRegexInspect_MixExs(t *testing.T) {
	t.Parallel()
	in := []byte(`defmodule MyApp.MixProject do
  use Mix.Project

  def project do
    [
      app: :my_app,
      version: "1.2.3",
      elixir: "~> 1.14"
    ]
  end
end
`)
	insp, err := inspectVia("mix.exs", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

func TestRegexReplace_MixExs(t *testing.T) {
	t.Parallel()
	in := []byte("    version: \"1.2.3\",\n    elixir: \"~> 1.14\"\n")
	out, err := replaceVia("mix.exs", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version: "2.0.0"`) {
		t.Errorf("Replace did not write 2.0.0:\n%s", string(out))
	}
}

// --- build.sbt -----------------------------------------------------------

// TestRegexInspect_BuildSbt reads version from a Scala SBT file.
// `version := "..."` is the canonical form (`:=` is the SBT setting
// assignment operator). Plain `=` is also tolerated.
func TestRegexInspect_BuildSbt(t *testing.T) {
	t.Parallel()
	in := []byte(`name := "my-app"
version := "1.2.3"
scalaVersion := "2.13.10"
`)
	insp, err := inspectVia("build.sbt", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

func TestRegexReplace_BuildSbt(t *testing.T) {
	t.Parallel()
	in := []byte("version := \"1.2.3\"\n")
	out, err := replaceVia("build.sbt", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version := "1.2.4"`) {
		t.Errorf("Replace did not write 1.2.4 with `:=`:\n%s", string(out))
	}
}

// --- shared edge cases ---------------------------------------------------

// TestRegexInspect_FirstMatchOnly documents the DR-0012 design: when
// multiple version-shaped lines match, only the first is taken. Here we
// use a podspec with two `s.version = ...` lines (one main definition,
// one inside a `dependency` block) — the first wins.
func TestRegexInspect_FirstMatchOnly(t *testing.T) {
	t.Parallel()
	in := []byte(`s.version = '1.2.3'
s.dependency 'OtherPod', :version => '9.9.9'
s.version = '0.0.0'
`)
	insp, err := inspectVia("MyPod.podspec", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want first match 1.2.3", insp.Versions)
	}
}

// TestRegexReplace_FirstMatchOnly: only the first match is rewritten;
// later matches stay verbatim.
func TestRegexReplace_FirstMatchOnly(t *testing.T) {
	t.Parallel()
	in := []byte("s.version = '1.2.3'\n# later: s.version = '1.2.3'\n")
	out, err := replaceVia("MyPod.podspec", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "s.version = '1.2.4'") {
		t.Errorf("first match not bumped:\n%s", s)
	}
	if !strings.Contains(s, "# later: s.version = '1.2.3'") {
		t.Errorf("second match was incorrectly modified:\n%s", s)
	}
}

// TestRegexInspect_MissingVersion fails the rule cleanly so the
// dispatcher can keep walking.
func TestRegexInspect_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte("# nothing here\n")
	if _, err := inspectVia("foo.nimble", in); err == nil {
		t.Error("expected error when version regex doesn't match")
	}
}

// TestRegexInspect_NimblePoundLeadingComment handles a comment-only
// preamble before the version assignment (a common nimble layout).
func TestRegexInspect_NimblePoundLeadingComment(t *testing.T) {
	t.Parallel()
	in := []byte("# leading comment\n# multi-line\nversion = \"1.2.3\"\n")
	insp, err := inspectVia("foo.nimble", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}
