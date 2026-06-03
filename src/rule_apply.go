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
	case "json", "yaml", "toml":
		if opts.VersionPath == nil && opts.VersionRegex == nil {
			return fmt.Errorf("--define-rule %q: --format %s requires --version-path or --version-regex (or both, see DR-0029)", block.Pattern, f)
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
	insp, err := tryRule(h.rule, content)
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
	return insp, nil
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
	out, err := formatReplace(h.rule, content, current, newVersion)
	if err != nil {
		return nil, fmt.Errorf("--define-rule %q applied to %s: %w", h.block.Pattern, h.path, err)
	}
	return out, nil
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
