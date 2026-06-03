package main

import (
	"fmt"
	"regexp"
)

// rule_apply.go bridges DR-0029 user-defined rules into the existing
// builtin extraction machinery (jsonInspect / tomlInspect / yamlInspect
// / textInspect). It does so by converting a ruleBlock to a
// CandidateRule shape that the existing dispatcher already knows how
// to handle, then layering DR-0029-specific guards on top (CLI rule
// failure is hard error, --version-regex is exact-one-match).
//
// Design rationale: rather than duplicate every format's parser, the
// CLI rule path reuses the builtin format dispatchers verbatim. The
// only deviations are surfaced here as wrapping helpers (= validation
// before extraction, cardinality enforcement for the text path).

// validateRuleBlock checks that a non-global rule block carries a
// coherent set of rule-definition flags. Called BEFORE extraction so
// usage errors are surfaced cleanly without touching the file.
//
// Rules enforced (DR-0029 § "Phase 1 で必要な flag" + § "Path / Regex
// 併記時の挙動"):
//
//   - Format must be set (otherwise we don't know which dispatcher to
//     invoke).
//   - Format text requires VersionRegex; VersionPath is rejected (text
//     has no path semantics).
//   - Format json/yaml/toml requires VersionPath or VersionRegex (at
//     least one).
//
// xml is filtered upstream (parser layer) so it never reaches this
// function — assert defensively to catch programmer errors.
func validateRuleBlock(block ruleBlock) error {
	opts := block.Opts
	if opts.Format == nil {
		return fmt.Errorf("--define-rule %q: --format is required (set --format text|json|yaml|toml)", block.Pattern)
	}
	f := *opts.Format
	switch f {
	case "text":
		if opts.VersionRegex == nil {
			return fmt.Errorf("--define-rule %q: --format text requires --version-regex (regex with one capture group, exact-one match)", block.Pattern)
		}
		if opts.VersionPath != nil {
			return fmt.Errorf("--define-rule %q: --format text does not support --version-path (text has no structured path; use --format json|yaml|toml for path)", block.Pattern)
		}
		if opts.NamePath != nil {
			return fmt.Errorf("--define-rule %q: --format text does not support --name-path", block.Pattern)
		}
	case "json", "yaml", "toml", "xml":
		// Structured formats require a path. A regex MAY accompany it
		// (2-stage extraction: regex pulls the version out of the
		// path's container value). A regex WITHOUT a path would mean
		// "whole-file regex", which is exactly what --format text does;
		// steering users there keeps each format's role distinct and
		// avoids paying a parse cost we then ignore.
		if opts.VersionPath == nil {
			if opts.VersionRegex != nil {
				return fmt.Errorf("--define-rule %q: --format %s needs --version-path; for whole-file regex extraction use --format text instead", block.Pattern, f)
			}
			return fmt.Errorf("--define-rule %q: --format %s requires --version-path", block.Pattern, f)
		}
	default:
		return fmt.Errorf("--define-rule %q: internal — unhandled --format %q (should have been rejected at parse time)", block.Pattern, f)
	}
	return nil
}

// ruleBlockToCandidate converts a validated ruleBlock into the
// CandidateRule shape consumed by tryRule / formatReplace. The
// returned rule's Name carries a human-readable label for error
// messages ("CLI rule for <path>"). MatchedConfidence stays at zero
// (CLI rules use their own match strength axis, not DR-0005's).
//
// PathSuffix / Basename / Glob / Confidence are NOT set because the
// rule binding is decided externally by resolveRuleBlock — the
// returned CandidateRule is fed directly to tryRule, no further
// dispatcher matching happens.
func ruleBlockToCandidate(path string, block ruleBlock) CandidateRule {
	r := CandidateRule{
		Name:   fmt.Sprintf("CLI rule for %s", path),
		Format: *block.Opts.Format,
	}
	if block.Opts.VersionPath != nil {
		r.VersionPaths = []string{*block.Opts.VersionPath}
	}
	if block.Opts.VersionRegex != nil {
		r.VersionRegex = *block.Opts.VersionRegex
	}
	if block.Opts.NamePath != nil {
		r.NamePaths = []string{*block.Opts.NamePath}
	}
	if block.Opts.NameRegex != nil {
		r.NameRegex = *block.Opts.NameRegex
	}
	return r
}

// cliRuleHandler is the Handler implementation backing DR-0029
// CLI-supplied rules. Distinct type from ruleHandler so we can apply
// DR-0029-specific guards (= hard error on extraction failure, no
// builtin fallback) without conditional branches inside the shared
// type.
type cliRuleHandler struct {
	path  string
	rule  CandidateRule
	block ruleBlock // kept for label / error message context
}

func (h *cliRuleHandler) Inspect(content []byte) (Inspection, error) {
	insp, err := h.inspectRaw(content)
	if err != nil {
		return Inspection{}, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, err)
	}
	// DR-0029 § "--version-regex の cardinality 規約": CLI rule with
	// VersionRegex must match EXACTLY ONE occurrence (0 → no match
	// error from tryRule; 2+ → silent in tryRule, surfaced here).
	if h.rule.Format == "text" && h.rule.VersionRegex != "" {
		if n, regexErr := countRegexMatches(h.rule.VersionRegex, content); regexErr != nil {
			return Inspection{}, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, regexErr)
		} else if n > 1 {
			return Inspection{}, fmt.Errorf("--define-rule %q applied to %s: --version-regex matched %d times, exactly one match required (use a more specific regex, e.g. add line anchors (?m)^ or stricter context)", h.block.Pattern, h.path, n)
		}
	}
	// DR-0029 § "Path / Regex 併記時の挙動": structured format
	// (json/yaml/toml/xml) WITH both a path and a regex performs a
	// 2-stage extraction — the path locates a scalar string, then the
	// regex pulls capture group 1 out of that string. The inspectRaw
	// step already extracted the raw path value(s); apply the regex on
	// top here.
	if h.isStructured() && h.rule.VersionRegex != "" {
		for i := range insp.Versions {
			v, rerr := regexExtractGroup1(insp.Versions[i].Value, h.rule.VersionRegex)
			if rerr != nil {
				return Inspection{}, fmt.Errorf("--define-rule %q applied to %s: --version-path value %q: %w", h.block.Pattern, h.path, insp.Versions[i].Value, rerr)
			}
			insp.Versions[i].Value = v
		}
	}
	if h.isStructured() && h.rule.NameRegex != "" {
		for i := range insp.Names {
			if v, rerr := regexExtractGroup1(insp.Names[i].Value, h.rule.NameRegex); rerr == nil {
				insp.Names[i].Value = v
			}
		}
	}
	return insp, nil
}

// isStructured reports whether the rule's format is a tree-parsed
// format (json/yaml/toml/xml) as opposed to text. Only structured
// formats support the path+regex 2-stage extraction.
func (h *cliRuleHandler) isStructured() bool {
	switch h.rule.Format {
	case "json", "yaml", "toml", "xml":
		return len(h.rule.VersionPaths) > 0
	default:
		return false
	}
}

func (h *cliRuleHandler) Replace(content []byte, current, newVersion string) ([]byte, error) {
	// Re-run the cardinality check before write (defence in depth: if
	// the caller skipped Inspect for some reason, we still must not
	// silently write to the wrong place).
	if h.rule.Format == "text" && h.rule.VersionRegex != "" {
		if n, regexErr := countRegexMatches(h.rule.VersionRegex, content); regexErr != nil {
			return nil, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, regexErr)
		} else if n == 0 {
			return nil, fmt.Errorf("--define-rule %q applied to %s: --version-regex did not match (cannot write)", h.block.Pattern, h.path)
		} else if n > 1 {
			return nil, fmt.Errorf("--define-rule %q applied to %s: --version-regex matched %d times, exactly one match required for --write", h.block.Pattern, h.path, n)
		}
	}
	// DR-0029 § "Path / Regex 併記時の挙動" (write side): for a
	// structured format with both path and regex, each path value is a
	// container string (e.g. "myapp v1.0.5") whose regex group 1 must
	// be replaced in place — NOT the whole path value. We achieve this
	// without per-format byte offsets by computing the new container
	// string (group 1 swapped) and asking the existing format Replace
	// to swap the whole path value for it. Because every format's
	// Replace rewrites only the value's byte range, the surrounding
	// document (and the container string's non-version bytes) stay
	// byte-identical.
	//
	// CRITICAL (codex review): each --version-path may hold a DIFFERENT
	// container even when the extracted version agrees (e.g. .name =
	// "myapp v1.2.3" and .label = "release-1.2.3" both extract 1.2.3).
	// We therefore compute and splice the new container PER PATH — never
	// reuse path[0]'s container for the others, which would corrupt them.
	if h.isStructured() && h.rule.VersionRegex != "" {
		insp, ierr := h.inspectRaw(content)
		if ierr != nil {
			return nil, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, ierr)
		}
		if len(insp.Versions) != len(h.rule.VersionPaths) {
			return nil, fmt.Errorf("--define-rule %q applied to %s: internal — %d path values for %d --version-path (cannot map containers safely)", h.block.Pattern, h.path, len(insp.Versions), len(h.rule.VersionPaths))
		}
		cur := content
		for i, vp := range h.rule.VersionPaths {
			raw := insp.Versions[i].Value
			newContainer, rerr := regexReplaceGroup1(raw, h.rule.VersionRegex, newVersion)
			if rerr != nil {
				return nil, fmt.Errorf("--define-rule %q applied to %s: --version-path %q value %q: %w", h.block.Pattern, h.path, vp, raw, rerr)
			}
			// Restrict the rule to this single path so the format
			// Replace touches only its container; re-locate against the
			// updated buffer each iteration so byte offsets stay valid.
			sub := h.rule
			sub.VersionPaths = []string{vp}
			out, err := h.replaceRawWith(sub, cur, raw, newContainer)
			if err != nil {
				return nil, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, err)
			}
			cur = out
		}
		return cur, nil
	}

	out, err := h.replaceRaw(content, current, newVersion)
	if err != nil {
		return nil, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, err)
	}
	return out, nil
}

// inspectRaw dispatches extraction to the right engine. CLI xml rules
// use the unified dot-path resolver (xmlDotInspect), NOT the builtin
// plist-flavoured "xml" format that tryRule would pick. Every other
// format goes through the shared tryRule dispatcher.
func (h *cliRuleHandler) inspectRaw(content []byte) (Inspection, error) {
	if h.rule.Format == "xml" {
		return xmlDotInspect(h.rule, content)
	}
	return tryRule(h.rule, content)
}

// replaceRaw is the write counterpart of inspectRaw.
func (h *cliRuleHandler) replaceRaw(content []byte, current, newVersion string) ([]byte, error) {
	return h.replaceRawWith(h.rule, content, current, newVersion)
}

// replaceRawWith dispatches a write for an explicit rule (used by the
// path+regex 2-stage write to restrict the rule to a single path).
func (h *cliRuleHandler) replaceRawWith(rule CandidateRule, content []byte, current, newVersion string) ([]byte, error) {
	if rule.Format == "xml" {
		return xmlDotReplace(rule, content, current, newVersion)
	}
	return formatReplace(rule, content, current, newVersion)
}

// countRegexMatches counts how many disjoint matches `vrx` has in
// `content`. The regex must compile successfully or an error is
// returned; the count is used purely to enforce DR-0029's exact-one-
// match cardinality rule for CLI --version-regex.
func countRegexMatches(vrx string, content []byte) (int, error) {
	re, err := regexp.Compile(vrx)
	if err != nil {
		return 0, fmt.Errorf("invalid --version-regex: %w", err)
	}
	return len(re.FindAllIndex(content, -1)), nil
}

// regexExtractGroup1 applies `vrx` to `s` and returns capture group 1.
// Enforces exactly-one-match (DR-0029 cardinality): 0 matches → error,
// 2+ matches → ambiguous error. Used by the structured-format path+regex
// 2-stage extraction (= regex applied to the value located by a path).
func regexExtractGroup1(s, vrx string) (string, error) {
	re, err := regexp.Compile(vrx)
	if err != nil {
		return "", fmt.Errorf("invalid --version-regex: %w", err)
	}
	if re.NumSubexp() < 1 {
		return "", fmt.Errorf("--version-regex has no capture group")
	}
	all := re.FindAllStringSubmatchIndex(s, -1)
	if len(all) == 0 {
		return "", fmt.Errorf("--version-regex did not match")
	}
	if len(all) > 1 {
		return "", fmt.Errorf("--version-regex matched %d times, exactly one match required", len(all))
	}
	loc := all[0]
	if loc[2] < 0 {
		return "", fmt.Errorf("--version-regex capture group did not participate")
	}
	return s[loc[2]:loc[3]], nil
}

// regexReplaceGroup1 returns `s` with capture group 1 of `vrx` replaced
// by `newVal`, preserving every other byte (= pinpoint rewrite of the
// version inside a container string located by a path). Enforces the
// same exactly-one-match cardinality as regexExtractGroup1.
func regexReplaceGroup1(s, vrx, newVal string) (string, error) {
	re, err := regexp.Compile(vrx)
	if err != nil {
		return "", fmt.Errorf("invalid --version-regex: %w", err)
	}
	if re.NumSubexp() < 1 {
		return "", fmt.Errorf("--version-regex has no capture group")
	}
	all := re.FindAllStringSubmatchIndex(s, -1)
	if len(all) == 0 {
		return "", fmt.Errorf("--version-regex did not match")
	}
	if len(all) > 1 {
		return "", fmt.Errorf("--version-regex matched %d times, exactly one match required", len(all))
	}
	loc := all[0]
	if loc[2] < 0 {
		return "", fmt.Errorf("--version-regex capture group did not participate")
	}
	return s[:loc[2]] + newVal + s[loc[3]:], nil
}

// detectHandlerWithCliRule wires a validated CLI ruleBlock into a
// Handler bound to `path`. Returns an error only when the block fails
// validation (= caller can choose to surface that as a parse-tail
// usage error instead of a runtime error if it knows the SOURCE list
// upfront).
func detectHandlerWithCliRule(path string, block ruleBlock) (Handler, error) {
	if err := validateRuleBlock(block); err != nil {
		return nil, err
	}
	return &cliRuleHandler{
		path:  path,
		rule:  ruleBlockToCandidate(path, block),
		block: block,
	}, nil
}

// pickHandlerForFile is the DR-0029 entry point used by resolveFile /
// resolveFileFromStdin. When ruleBlocks is non-nil and a block matches
// `path` (or the global block carries rule flags), a cliRuleHandler is
// returned; otherwise the existing builtin detectHandler is invoked.
//
// Returns the same error shapes detectHandler emits (= unsupportedFileError
// wrapper survives) so call sites can still errors.As against it.
func pickHandlerForFile(path string, ruleBlocks []ruleBlock) (Handler, error) {
	if len(ruleBlocks) == 0 {
		return detectHandler(path)
	}
	match, err := resolveRuleBlock(path, ruleBlocks)
	if err != nil {
		return nil, err
	}
	if match.BlockIdx < 0 {
		// No CLI rule applies — use the builtin auto-detection path.
		return detectHandler(path)
	}
	return detectHandlerWithCliRule(path, ruleBlocks[match.BlockIdx])
}

// cliRuleCoversFile reports whether any --define-rule block (named or
// global-with-flags) would match `path`. Used by resolveInputs's
// stdin-pipe shortcut to widen the pathHasAnyRule gate when the user
// supplied CLI rules — without this the path lookup would reject as
// "unsupported file" even though a CLI rule covers it. Errors from
// pattern matching are squashed to false (= conservative: let the
// downstream resolveRuleBlock surface the real diagnostic).
func cliRuleCoversFile(path string, ruleBlocks []ruleBlock) bool {
	if len(ruleBlocks) == 0 {
		return false
	}
	match, err := resolveRuleBlock(path, ruleBlocks)
	if err != nil {
		return false
	}
	return match.BlockIdx >= 0
}
