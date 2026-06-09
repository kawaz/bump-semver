package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	doublestar "github.com/bmatcuk/doublestar/v4"
	ignore "github.com/sabhiram/go-gitignore"
)

// globOpts groups the verb-shared `--glob-*` flags (DR-0024).
//
// Defaults (see DR-0024 for rationale):
//   - Dotfile:    false (exclude dotfiles)        — required-value flag
//   - Gitignored: true  (respect .gitignore)      — required-value flag (*bool: nil = default true)
//   - IgnoreCase: false (case-sensitive)          — optional-value flag (bare flag = true)
//
// Dotfile/Gitignored take a required value (`=true`/`=false`) to remove the
// "what does --glob-dotfile alone mean?" ambiguity. IgnoreCase follows the
// established convention (bare flag = enable) because the verb name itself
// carries the polarity.
type globOpts struct {
	Dotfile    bool  // --glob-dotfile=true|false (default false; include hidden when true)
	Gitignored *bool // --glob-gitignored=true|false (default true; *bool so absent != false)
	IgnoreCase bool  // --glob-ignorecase[=true|false] (default false; bare = true)
}

// Gitignored returns the resolved gitignored-respect setting (default true).
func (g globOpts) GitignoredRespect() bool {
	if g.Gitignored == nil {
		return true
	}
	return *g.Gitignored
}

// hasGlobPrefix reports whether spec begins with the `glob:` selector prefix.
func hasGlobPrefix(spec string) bool {
	return strings.HasPrefix(spec, "glob:")
}

// parseGlobSpec strips the `glob:` prefix from spec. Empty patterns are
// rejected — `glob:` with no body is a usage error.
func parseGlobSpec(spec string) (string, error) {
	if !hasGlobPrefix(spec) {
		return "", fmt.Errorf("not a glob: spec: %q", spec)
	}
	pat := strings.TrimPrefix(spec, "glob:")
	if pat == "" {
		return "", fmt.Errorf("glob: pattern is empty")
	}
	return pat, nil
}

// expandTilde resolves a leading `~` / `~/...` to the user's home directory.
// `~user/...` is intentionally unsupported (out of MVP scope, DR-0024); the
// path is passed through unchanged so doublestar treats it as a literal.
//
// homeFn is injectable so tests can pin a fake home without HOME env mutation.
func expandTilde(pat string, homeFn func() (string, error)) (string, error) {
	if pat == "" || pat[0] != '~' {
		return pat, nil
	}
	// `~user/...` form: not supported, pass through.
	if len(pat) > 1 && pat[1] != '/' && pat[1] != filepath.Separator {
		return pat, nil
	}
	home, err := homeFn()
	if err != nil {
		return "", fmt.Errorf("glob: cannot resolve ~ (home directory): %w", err)
	}
	if pat == "~" {
		return home, nil
	}
	return filepath.Join(home, pat[2:]), nil
}

// expandGlob expands a glob pattern (post-`glob:` strip) into a sorted, dedup'd
// list of relative file paths.
//
// Behavior (DR-0024):
//   - Uses doublestar v4 for `*` / `**` / `[...]` / `{a,b}` semantics.
//   - `~` / `~/...` expanded via homeFn.
//   - `--glob-dotfile=false` (default) → dotfile-bearing paths filtered out
//     via doublestar's WithNoHidden.
//   - `--glob-gitignored=true` (default) → paths matching .gitignore (if one
//     exists at the search base) are filtered out. Fidelity: single-file
//     .gitignore at the base only (no nested .gitignore, no core.excludesfile);
//     adequate for kawaz's common case (= bump-semver targets sit at the repo
//     root). See DR-0024 for the fidelity disclosure.
//   - `--glob-ignorecase=true` → doublestar's WithCaseInsensitive (case
//     sensitivity also depends on the underlying filesystem).
//   - Directories are excluded (WithFilesOnly): glob: is a *file* selector.
//   - No-match → empty slice, no error (silent skip, DR-0020 declarative
//     convergence parity).
//
// Returned paths are filesystem-style (use the OS separator) and are NOT
// canonicalized — kawaz callers want the same string the user could have
// typed (so `glob:src/**/*.ts` → `src/a.ts`, not `/abs/.../src/a.ts`).
func expandGlob(pat string, opts globOpts, homeFn func() (string, error)) ([]string, error) {
	if pat == "" {
		return nil, fmt.Errorf("glob: pattern is empty")
	}
	expanded, err := expandTilde(pat, homeFn)
	if err != nil {
		return nil, err
	}
	var dsOpts []doublestar.GlobOption
	if !opts.Dotfile {
		dsOpts = append(dsOpts, doublestar.WithNoHidden())
	}
	if opts.IgnoreCase {
		dsOpts = append(dsOpts, doublestar.WithCaseInsensitive())
	}
	dsOpts = append(dsOpts, doublestar.WithFilesOnly())

	// FilepathGlob handles absolute and relative patterns by splitting at
	// the first meta into base + relative pattern, then calling Glob on
	// os.DirFS(base). For us this means `src/**/*.ts` is searched against
	// cwd's `src/` subtree, and absolute `~/foo/**` is searched after
	// tilde-expand.
	matches, err := doublestar.FilepathGlob(expanded, dsOpts...)
	if err != nil {
		return nil, fmt.Errorf("glob:%s: %w", pat, err)
	}
	if opts.GitignoredRespect() {
		matches = filterGitignored(matches, expanded)
	}
	// Deterministic order — stabilizes diff/compare output and tests.
	sort.Strings(matches)
	return uniqueStrings(matches), nil
}

// filterGitignored drops entries from matches that match the .gitignore file
// at the glob base (best-effort fidelity, DR-0024). Failure to read .gitignore
// is a silent no-op — glob: must continue to work outside a repo.
func filterGitignored(matches []string, pattern string) []string {
	if len(matches) == 0 {
		return matches
	}
	base, _ := doublestar.SplitPattern(pattern)
	if base == "" {
		base = "."
	}
	gitignorePath := filepath.Join(base, ".gitignore")
	ig, err := ignore.CompileIgnoreFile(gitignorePath)
	if err != nil || ig == nil {
		// .gitignore absent / unreadable → don't filter anything. This is
		// the gracefully-degrades branch for non-repo use.
		return matches
	}
	out := matches[:0]
	for _, m := range matches {
		// MatchesPath wants a path relative to the .gitignore file. We
		// approximate by stripping the leading base/ prefix when present;
		// matches may also be absolute (when pattern is absolute) — in
		// that case strip base too.
		rel := m
		if strings.HasPrefix(m, base+string(filepath.Separator)) {
			rel = strings.TrimPrefix(m, base+string(filepath.Separator))
		}
		if ig.MatchesPath(rel) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// uniqueStrings returns ss with consecutive duplicates removed. Caller must
// pre-sort to make this an actual dedup (we do, in expandGlob).
func uniqueStrings(ss []string) []string {
	if len(ss) < 2 {
		return ss
	}
	out := ss[:1]
	for _, s := range ss[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// defaultHomeFn is the default home directory lookup used by expandGlob in
// production paths (tests substitute their own).
func defaultHomeFn() (string, error) {
	return os.UserHomeDir()
}

// anyGlob reports whether any item in inputs uses the `glob:` selector.
func anyGlob(inputs []string) bool {
	for _, in := range inputs {
		if hasGlobPrefix(in) {
			return true
		}
	}
	return false
}

// expandGlobInputs walks inputs and expands `glob:<pat>` / `file:<path>`
// selectors into the list of matched paths. Non-prefixed inputs pass through
// unchanged. The resulting slice preserves position order (matches from each
// selector are spliced in at the selector's position).
//
// `glob:` (DR-0024) and `file:` (DR-0033) share this entry point so include /
// exclude post-filter (DR-0033) and downstream verbs see a uniform pre-expanded
// path list regardless of which prefix was used.
//
// A 0-match glob: selector contributes nothing to the output (silent skip).
// This means a sole `bump-semver get glob:none.txt` ends up with 0 inputs;
// the downstream "at least one input is required" check then surfaces as
// the same exit-2 error the user would get from a literal missing FILE
// list. That's the right level — the parser stays uniform, the dispatcher
// owns the "did you give me anything?" assertion.
//
// `file:` selectors are read once per occurrence; nested `file:` inside the
// list is rejected (see DR-0033 § scope-out).
func expandGlobInputs(inputs []string, opts globOpts) ([]string, error) {
	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		switch {
		case hasGlobPrefix(in):
			pat, err := parseGlobSpec(in)
			if err != nil {
				return nil, err
			}
			matches, err := expandGlob(pat, opts, defaultHomeFn)
			if err != nil {
				return nil, err
			}
			out = append(out, matches...)
		case hasFilePrefix(in):
			path, err := parseFileSpec(in)
			if err != nil {
				return nil, err
			}
			paths, err := expandFileSpec(path, opts)
			if err != nil {
				return nil, err
			}
			out = append(out, paths...)
		default:
			out = append(out, in)
		}
	}
	return dedupPreserveOrder(out), nil
}

// dedupPreserveOrder removes duplicate strings while preserving first-seen
// order. Used by expandGlobInputs so include lists from `file:LIST` /
// overlapping globs don't pass redundant entries to the backend pathspec
// builder (= cleaner argv, no semantic change since duplicate pathspecs
// are no-ops for git/jj diff).
func dedupPreserveOrder(ss []string) []string {
	if len(ss) < 2 {
		return ss
	}
	seen := make(map[string]struct{}, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// flattenExcludePatterns expands `file:<path>` exclude patterns into the
// concrete list of patterns they contain, leaving `glob:` / literal entries
// untouched (DR-0033 phase 2 v2). The backend pathspec builder
// (buildGitPathspec / buildJjPathspec) doesn't know our `file:` shape, so we
// resolve it here; each non-comment line from the file becomes a separate
// exclude pattern (= same expansion rule as expandFileSpec but skipping the
// glob-side expansion since we're producing patterns, not file lists).
func flattenExcludePatterns(patterns []string, opts globOpts) ([]string, error) {
	_ = opts // reserved for future flag-sensitive expansion
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if !hasFilePrefix(p) {
			out = append(out, p)
			continue
		}
		path, err := parseFileSpec(p)
		if err != nil {
			return nil, fmt.Errorf("--excludes %s: %w", p, err)
		}
		lines, err := readPatternListFile(path)
		if err != nil {
			return nil, fmt.Errorf("--excludes %s: %w", p, err)
		}
		out = append(out, lines...)
	}
	return out, nil
}

// excludeInputs returns includes with paths matching any excludePattern removed.
// Each excludePattern accepts the same shape as positional selectors (literal /
// `glob:` / `file:`) — DR-0033 原則 3 で対称性を保証している。
//
// Semantics: post-filter, order-independent (= 順序非依存). The final set is
// the include set minus the union of all exclude patterns. Empty excludes
// returns includes unchanged.
//
// When an exclude pattern is a literal (= no `glob:` / `file:` prefix), it is
// matched **exactly** against include entries (same byte sequence). This
// mirrors git pathspec's literal-match semantics; users wanting prefix /
// subtree exclusion should use `glob:dir/**` explicitly.
func excludeInputs(includes []string, excludePatterns []string, opts globOpts) ([]string, error) {
	if len(excludePatterns) == 0 || len(includes) == 0 {
		return includes, nil
	}
	// Expand the union of exclude patterns into a concrete path set.
	// (Used by older unit-tests that exercise set-subtraction directly;
	// the production `vcs diff` path forwards excludes to the backend via
	// buildGitPathspec / buildJjPathspec instead — DR-0033 phase 2 v2.)
	excludeSet := make(map[string]struct{})
	for _, pat := range excludePatterns {
		expanded, err := expandGlobInputs([]string{pat}, opts)
		if err != nil {
			return nil, fmt.Errorf("--excludes %s: %w", pat, err)
		}
		for _, p := range expanded {
			excludeSet[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(includes))
	for _, p := range includes {
		if _, drop := excludeSet[p]; drop {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
