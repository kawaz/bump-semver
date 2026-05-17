package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// CandidateRule describes one (path-pattern, format, version-paths) tuple
// that the dispatcher can try against an input file.
//
// The dispatcher considers rules in **descending Confidence order**. When
// the rule's path pattern matches but extraction (Inspect) fails on the
// given content, the dispatcher falls through to the next matching rule.
// That's how a generic `marketplace.json` (anywhere in the tree) gets a
// chance at `.metadata.version`, but eventually falls back to `.version`
// for unrelated JSON files that happen to share the basename.
type CandidateRule struct {
	// Name is a human-readable label shown in errors / debug output.
	Name string
	// PathSuffix is matched as a clean path-suffix (slash-aware) against
	// the input path. An empty string means "match by basename only".
	PathSuffix string
	// Basename, if non-empty, requires filepath.Base(path) to equal it.
	// Used for confidence-2 rules that don't pin a directory.
	Basename string
	// Glob, if non-empty, is a basename glob like "*.json" matched as a
	// suffix. Used for the lowest-confidence fallback.
	Glob string
	// Confidence: 3 = path-pinned, 2 = basename-only, 1 = glob fallback.
	Confidence int
	// Format selects the parser/serializer pair: "json", "toml", "plain".
	Format string
	// NamePaths lists every place the rule should look for a package
	// name. Names are optional — a missing name does not cause the rule
	// to fail (unlike a missing version). Multiple paths are useful for
	// formats like `package-lock.json` that record the same name in two
	// places, where a discrepancy is itself a useful diagnostic
	// (DR-0004 cross-file name consistency picks it up).
	NamePaths []string
	// VersionPaths lists every place the rule expects a version string;
	// all of them must extract successfully for the rule to count as a hit.
	VersionPaths []string
	// VersionRegex is used by Format == "regex" rules (DR-0012). The
	// regex must contain exactly one capture group `(...)`; its
	// matched byte range is the version value, and Replace rewrites
	// only that range — everything else (quotes, identifier text,
	// trailing comments) is preserved verbatim. Empty for non-regex
	// rules.
	VersionRegex string
	// NameRegex is the optional name counterpart to VersionRegex
	// (DR-0012). Same single-capture-group shape. A failed match here
	// does not fail the rule (names are advisory across all formats).
	NameRegex string
}

// rules is the master table. Order is irrelevant for matching (the
// dispatcher sorts by Confidence), but readers should still see the
// high-confidence path-pinned rules first.
var rules = []CandidateRule{
	{
		Name:         "Claude plugin marketplace.json",
		PathSuffix:   ".claude-plugin/marketplace.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".metadata.version"},
	},
	{
		Name:         "Claude plugin plugin.json",
		PathSuffix:   ".claude-plugin/plugin.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		Name:         "package.json",
		Basename:     "package.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		Name:         "package-lock.json (npm 7+)",
		Basename:     "package-lock.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name", `.packages[""].name`},
		VersionPaths: []string{".version", `.packages[""].version`},
	},
	{
		Name:         "marketplace.json (any directory)",
		Basename:     "marketplace.json",
		Confidence:   2,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".metadata.version"},
	},
	{
		Name:         "plugin.json (any directory)",
		Basename:     "plugin.json",
		Confidence:   2,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0021 (supersedes DR-0002): a single Cargo.toml rule covers
		// both single-crate and workspace-root layouts via the TOML
		// format's OR / first-match-wins VersionPaths (the same mechanism
		// pyproject.toml uses for [project] → [tool.poetry]).
		//
		// Precedence is deliberate: a crate's own [package].version is the
		// version it publishes, so it wins. Only when [package] is absent
		// (the typical workspace-root layout) does the rule fall back to
		// [workspace.package].version — the shared template member crates
		// inherit via `version.workspace = true`.
		//
		// The matched path is surfaced verbatim in `get` output and
		// diagnostics (e.g. `[workspace.package].version`), so the user
		// always sees which version they are bumping. That transparency is
		// what answers DR-0002's "too implicit" objection without a mode
		// flag (DR-0001) or a separate content-dispatched handler (DR-0005).
		Name:         "Cargo.toml",
		Basename:     "Cargo.toml",
		Confidence:   3,
		Format:       "toml",
		NamePaths:    []string{".package.name", ".workspace.package.name"},
		VersionPaths: []string{".package.version", ".workspace.package.version"},
	},
	{
		// DR-0014: PEP 621 (`[project]` section) is the modern Python
		// packaging standard; Poetry's legacy `[tool.poetry]` is still
		// in widespread use mid-migration. The TOML format treats
		// VersionPaths as OR (first match wins), so this rule reads /
		// rewrites whichever section the file uses, with PEP 621
		// taking precedence when both happen to be present.
		Name:         "pyproject.toml",
		Basename:     "pyproject.toml",
		Confidence:   3,
		Format:       "toml",
		NamePaths:    []string{".project.name", ".tool.poetry.name"},
		VersionPaths: []string{".project.version", ".tool.poetry.version"},
	},
	{
		// DR-0014: Modular Mojo's package manifest. `[workspace]` is
		// the only place the project's name / version live in this
		// format, so a single-path rule suffices.
		Name:         "mojoproject.toml",
		Basename:     "mojoproject.toml",
		Confidence:   3,
		Format:       "toml",
		NamePaths:    []string{".workspace.name"},
		VersionPaths: []string{".workspace.version"},
	},
	{
		Name:       "VERSION (plain text)",
		Basename:   "VERSION",
		Confidence: 3,
		Format:     "plain",
	},
	{
		// DR-0015: Xcode `<project>.xcodeproj/project.pbxproj`. The
		// file is OpenStep plist (Apple's pre-XML key=value notation)
		// and carries one `MARKETING_VERSION = ...;` line per
		// build configuration per target. Every match must agree —
		// the dedicated `pbxproj` format reports each match as a
		// `Field` with `Path = "line:N"` so main.go's existing
		// `formatMismatchError` produces a column-aligned diagnostic
		// when they don't.
		Name:       "project.pbxproj",
		Basename:   "project.pbxproj",
		Confidence: 3,
		Format:     "pbxproj",
	},
	{
		// DR-0015: Apple `Info.plist` (XML plist). Reads / writes the
		// `<key>CFBundleShortVersionString</key><string>X.Y.Z</string>`
		// pair. Files that use `$(MARKETING_VERSION)` placeholders
		// (Xcode 11+ default) extract the placeholder text verbatim,
		// which then fails ParseVersion further up the pipeline and
		// the input becomes an `unsupported file:` outcome — that's
		// the intended cue for users to add `project.pbxproj` to the
		// invocation, where the real value lives.
		Name:         "Info.plist",
		Basename:     "Info.plist",
		Confidence:   3,
		Format:       "xml",
		VersionPaths: []string{"CFBundleShortVersionString"},
	},
	{
		// Maven `pom.xml`. The root `<project>/<version>` is the
		// project's own version. `<parent>/<version>` references a
		// different artefact and is intentionally not touched; using a
		// path-based xml-element rule keeps them strictly separated.
		// XML namespaces (`<project xmlns="http://maven.apache.org/...">`)
		// are matched by local name, so the rule works regardless of
		// the declared schema.
		Name:         "pom.xml",
		Basename:     "pom.xml",
		Confidence:   3,
		Format:       "xml-element",
		VersionPaths: []string{"/project/version"},
		NamePaths:    []string{"/project/artifactId"},
	},
	{
		// .NET MSBuild project files (`*.csproj`, `*.fsproj`, `*.vbproj`).
		// Version sits as `<Project>/<PropertyGroup>/<Version>`. When a
		// file has multiple `<PropertyGroup>` blocks (Configurations
		// etc.), the first child Version wins — typical layouts put the
		// shared Version field in the first PropertyGroup at the top of
		// the file.
		Name:         "*.csproj (fallback)",
		Glob:         "*.csproj",
		Confidence:   1,
		Format:       "xml-element",
		VersionPaths: []string{"/Project/PropertyGroup/Version"},
	},
	{
		Name:         "*.fsproj (fallback)",
		Glob:         "*.fsproj",
		Confidence:   1,
		Format:       "xml-element",
		VersionPaths: []string{"/Project/PropertyGroup/Version"},
	},
	{
		Name:         "*.vbproj (fallback)",
		Glob:         "*.vbproj",
		Confidence:   1,
		Format:       "xml-element",
		VersionPaths: []string{"/Project/PropertyGroup/Version"},
	},
	// --- DR-0012: regex format rules (basename, confidence 2) ----------
	//
	// These are fixed-name files for languages whose version is a
	// single line of source code rather than a structured manifest.
	// `regex` format extracts and rewrites the first capture group of
	// VersionRegex; everything else on the line (quotes, identifier
	// text, trailing comments) is preserved verbatim.
	{
		// V language manifest (https://vlang.io/). Single-quoted
		// version literal in the top-level mapping-like block.
		Name:         "v.mod",
		Basename:     "v.mod",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)^\s*version\s*:\s*'([^']+)'`,
		NameRegex:    `(?m)^\s*name\s*:\s*'([^']+)'`,
	},
	{
		// Zig package manifest (ZON format). `.version = "1.2.3"`
		// inside a struct literal. Name extraction is omitted because
		// the name field has too many shapes (identifier, `@"..."`,
		// enum literal `.foo`) to capture safely with one regex.
		Name:         "build.zig.zon",
		Basename:     "build.zig.zon",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)\.version\s*=\s*"([^"]+)"`,
	},
	{
		// Elixir mix manifest. Version sits inside `def project do`
		// as `version: "1.2.3"`. The app name is a separate `app: :foo`
		// atom (different shape from version) so name regex is omitted.
		Name:         "mix.exs",
		Basename:     "mix.exs",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)version:\s*"([^"]+)"`,
	},
	{
		// Scala SBT build file. Either `version := "1.2.3"` (assignment
		// for plain Setting) or `version = "1.2.3"` (less common).
		// Name (`name := "..."`) sits on a different line; not extracted.
		Name:         "build.sbt",
		Basename:     "build.sbt",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)^\s*version\s*:?=\s*"([^"]+)"`,
	},
	{
		// Gradle Groovy DSL. Top-level `version = '1.2.3'` /
		// `version "1.2.3"` (Groovy method-call shorthand) /
		// `version = "1.2.3"`. Sub-project / allprojects blocks
		// aren't traversed — only the root-most `^version` line
		// counts. Name lives on `rootProject.name = ...` in
		// settings.gradle and is intentionally not extracted from
		// build.gradle (different file).
		Name:         "build.gradle",
		Basename:     "build.gradle",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)^version\s*=?\s*['"]([^'"]+)['"]`,
	},
	{
		// Gradle Kotlin DSL. Same root-version idea, but the only
		// valid syntax is `version = "1.2.3"` (no Groovy method-call
		// shorthand). The regex still accepts either quote style
		// for symmetry.
		Name:         "build.gradle.kts",
		Basename:     "build.gradle.kts",
		Confidence:   2,
		Format:       "regex",
		VersionRegex: `(?m)^version\s*=\s*['"]([^'"]+)['"]`,
	},

	// --- DR-0012: regex format rules (glob, confidence 1) --------------
	//
	// These extension-based fallbacks share the regex format with the
	// basename rules above but match by `*.ext` so any file in the
	// repository with the right shape is rescued. They emit the
	// DR-0010 fallback hint like every other confidence-1 rule.
	{
		// Xcode build configuration files. The `MARKETING_VERSION`
		// variable is the user-visible version (vs. `CURRENT_PROJECT_VERSION`
		// which is the build number). Value can be unquoted, ends at
		// whitespace / `;` / inline `//` comment.
		Name:         "*.xcconfig (fallback)",
		Glob:         "*.xcconfig",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^\s*MARKETING_VERSION\s*=\s*([^\s;/]+)`,
	},
	{
		// CocoaPods podspec (Ruby DSL). Either `s.version = '...'` or
		// `spec.version = "..."`. Both quote styles are accepted; the
		// rewriter preserves whichever was used.
		Name:         "*.podspec (fallback)",
		Glob:         "*.podspec",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^\s*(?:s|spec)\.version\s*=\s*['"]([^'"]+)['"]`,
		NameRegex:    `(?m)^\s*(?:s|spec)\.name\s*=\s*['"]([^'"]+)['"]`,
	},
	{
		// Nim package manifest (NimScript). `version = "1.2.3"` at
		// top level. Name typically derives from the file's basename
		// (e.g. `foo.nimble` → package "foo"); not extracted from content.
		Name:         "*.nimble (fallback)",
		Glob:         "*.nimble",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^\s*version\s*=\s*"([^"]+)"`,
	},
	{
		// Ruby gemspec. Same `s.version = '...'` / `spec.version = "..."`
		// shape as podspec; the two ecosystems intentionally share the
		// pattern so a unified regex covers both.
		Name:         "*.gemspec (fallback)",
		Glob:         "*.gemspec",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^\s*(?:s|spec)\.version\s*=\s*['"]([^'"]+)['"]`,
		NameRegex:    `(?m)^\s*(?:s|spec)\.name\s*=\s*['"]([^'"]+)['"]`,
	},
	{
		// Haskell Cabal package manifest. `^version: 1.2.3` and
		// `^name: foo` at top level. `cabal-version:` is a separate
		// field; regex anchors strictly to start of line with no
		// leading hyphen so it doesn't get caught.
		Name:         "*.cabal (fallback)",
		Glob:         "*.cabal",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^version\s*:\s*([^\s]+)`,
		NameRegex:    `(?m)^name\s*:\s*([^\s]+)`,
	},
	{
		// RPM spec file. `^Version: 1.2.3` (capital V). `Name:` and
		// `Release:` are separate fields; line-anchored regex keeps
		// them distinct. Macros like `%{version_major}.%{...}` are
		// not interpreted — bump-semver treats the literal value.
		Name:         "*.spec (fallback)",
		Glob:         "*.spec",
		Confidence:   1,
		Format:       "regex",
		VersionRegex: `(?m)^Version\s*:\s*([^\s]+)`,
		NameRegex:    `(?m)^Name\s*:\s*([^\s]+)`,
	},

	{
		Name:         "*.json (fallback)",
		Glob:         "*.json",
		Confidence:   1,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: top-level `.version` fallback for arbitrary YAML
		// (Helm Chart.yaml, GitHub Actions workflow metadata, etc.).
		// Multi-document YAML (`---` separators) is intentionally
		// out of scope — only the first document is examined.
		Name:         "*.yaml (fallback)",
		Glob:         "*.yaml",
		Confidence:   1,
		Format:       "yaml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: same as `*.yaml` but for the `.yml` extension
		// (carried separately because the rule table doesn't model
		// alternation in glob patterns).
		Name:         "*.yml (fallback)",
		Glob:         "*.yml",
		Confidence:   1,
		Format:       "yaml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: top-level `version = "..."` fallback for arbitrary
		// TOML files. Cargo.toml's `[package].version` is handled by
		// the confidence-3 rule above; this one only catches files
		// that put `version` at top level (e.g. `pyproject.toml` with
		// the version outside `[project]`, custom manifest TOMLs).
		Name:         "*.toml (fallback)",
		Glob:         "*.toml",
		Confidence:   1,
		Format:       "toml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
}

// pathMatches checks whether the rule could apply to path on its own (no
// content inspection yet).
func (r CandidateRule) pathMatches(path string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	if r.PathSuffix != "" {
		want := filepath.ToSlash(r.PathSuffix)
		return cleanPath == want || strings.HasSuffix(cleanPath, "/"+want)
	}
	if r.Basename != "" {
		return filepath.Base(path) == r.Basename
	}
	if r.Glob != "" {
		// We only use globs of the form "*.ext".
		if strings.HasPrefix(r.Glob, "*.") {
			return strings.HasSuffix(filepath.Base(path), r.Glob[1:])
		}
		matched, err := filepath.Match(r.Glob, filepath.Base(path))
		return err == nil && matched
	}
	return false
}

// rulesByConfidenceDesc returns the rules sorted by Confidence descending,
// preserving the original table order within the same confidence band.
func rulesByConfidenceDesc() []CandidateRule {
	out := make([]CandidateRule, len(rules))
	copy(out, rules)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out
}

// resolveRule walks every rule whose path-pattern matches `path`, tries
// extraction with each, and returns the first hit (highest confidence).
// If every matching rule fails extraction, the last error is wrapped and
// returned to the caller.
//
// On a successful match, the chosen rule's Confidence and (for confidence
// 1) Glob are stamped on the returned Inspection so the caller can render
// a DR-0010 fallback hint without re-resolving the rule.
//
// DR-0013: when no rule matches the path *and* the basename ends in a
// known backup-style suffix (`.bak` / `.20260510` / `~` / etc.), one
// extra pass is made with the suffix stripped from the basename. The
// extra pass:
//
//   - retries every rule against the stripped path (no recursion: at
//     most one stripping per resolve)
//   - downgrades the chosen rule's reported Confidence by one band,
//     floored at 1, so callers can see the rule was reached via the
//     suffix-stripped fallback
//   - stamps the original suffix and stripped basename onto the
//     Inspection so the suffix-stripping hint can be emitted
//
// Recursion is intentionally avoided (DR-0013 § 4): multi-stage
// suffixes like `Cargo.toml.bak.20260510` strip to `Cargo.toml.bak`
// once and stop. The 95% case is single-suffix files; multi-stage
// chains are deferred to a future DR if the need surfaces.
func resolveRule(path string, content []byte) (CandidateRule, Inspection, error) {
	rule, insp, err := resolveRuleDirect(path, content)
	if err == nil {
		return rule, insp, nil
	}

	// Only try suffix stripping when *no* rule matched the original
	// path. If a rule matched but extraction failed, that's a genuine
	// content-shape mismatch and we should propagate the original
	// error rather than masking it with a different rule's failure.
	var ufe *unsupportedFileError
	if !errors.As(err, &ufe) {
		return rule, insp, err
	}

	stripped, suffix, ok := stripKnownSuffix(path)
	if !ok {
		return rule, insp, err // original unsupportedFileError
	}

	rule2, insp2, err2 := resolveRuleDirect(stripped, content)
	if err2 != nil {
		// Suffix stripping didn't help. Surface the *original*
		// unsupported-file error keyed on the user-visible path
		// (otherwise the user sees "unsupported file: Cargo.toml"
		// when they typed Cargo.toml.bak).
		return CandidateRule{}, Inspection{}, &unsupportedFileError{path: path}
	}

	// Downgrade the reported confidence one band, floored at 1.
	if insp2.MatchedConfidence > 1 {
		insp2.MatchedConfidence--
	}
	insp2.MatchedSuffixStripped = suffix
	insp2.MatchedStrippedBasename = filepath.Base(stripped)
	return rule2, insp2, nil
}

// resolveRuleDirect is the inner loop without DR-0013 suffix stripping.
// It returns an *unsupportedFileError when no rule's path-pattern
// matches, or a wrapped extraction error when at least one rule
// matched but every match failed Inspect.
func resolveRuleDirect(path string, content []byte) (CandidateRule, Inspection, error) {
	var lastErr error
	var lastRule CandidateRule
	matched := false
	for _, rule := range rulesByConfidenceDesc() {
		if !rule.pathMatches(path) {
			continue
		}
		matched = true
		insp, err := tryRule(rule, content)
		if err == nil {
			insp.MatchedConfidence = rule.Confidence
			if rule.Confidence == 1 {
				insp.MatchedGlob = rule.Glob
			}
			return rule, insp, nil
		}
		lastErr = err
		lastRule = rule
	}
	if !matched {
		return CandidateRule{}, Inspection{}, &unsupportedFileError{path: path}
	}
	return CandidateRule{}, Inspection{}, fmt.Errorf("%s: %s: %w", path, lastRule.Name, lastErr)
}

// tryRule dispatches to the format-specific Inspect implementation.
func tryRule(rule CandidateRule, content []byte) (Inspection, error) {
	switch rule.Format {
	case "json":
		return jsonInspect(rule, content)
	case "toml":
		return tomlInspect(rule, content)
	case "yaml":
		return yamlInspect(rule, content)
	case "plain":
		return plainInspect(rule, content)
	case "regex":
		return regexInspect(rule, content)
	case "pbxproj":
		return pbxprojInspect(rule, content)
	case "xml":
		return xmlInspect(rule, content)
	case "xml-element":
		return xmlElementInspect(rule, content)
	default:
		return Inspection{}, fmt.Errorf("unknown format %q in rule %q", rule.Format, rule.Name)
	}
}

func formatReplace(rule CandidateRule, content []byte, current, newVersion string) ([]byte, error) {
	switch rule.Format {
	case "json":
		return jsonReplace(rule, content, current, newVersion)
	case "toml":
		return tomlReplace(rule, content, current, newVersion)
	case "yaml":
		return yamlReplace(rule, content, current, newVersion)
	case "plain":
		return plainReplace(rule, content, current, newVersion)
	case "regex":
		return regexReplace(rule, content, current, newVersion)
	case "pbxproj":
		return pbxprojReplace(rule, content, current, newVersion)
	case "xml":
		return xmlReplace(rule, content, current, newVersion)
	case "xml-element":
		return xmlElementReplace(rule, content, current, newVersion)
	default:
		return nil, fmt.Errorf("unknown format %q in rule %q", rule.Format, rule.Name)
	}
}

// pathHasAnyRule reports whether at least one rule's path-pattern matches.
// Used by detectHandler to fail fast on unsupported file names.
//
// DR-0013: a path is also considered supported if a known backup-style
// suffix can be stripped from its basename and the stripped form
// matches some rule. This keeps `detectHandler("Cargo.toml.bak")` from
// erroring early — the actual rule selection (and confidence
// downgrade) still happens later in resolveRule.
func pathHasAnyRule(path string) bool {
	for _, r := range rules {
		if r.pathMatches(path) {
			return true
		}
	}
	if stripped, _, ok := stripKnownSuffix(path); ok {
		for _, r := range rules {
			if r.pathMatches(stripped) {
				return true
			}
		}
	}
	return false
}
