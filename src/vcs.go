package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// vcsKind identifies which VCS is in use for `vcs:` inputs.
//
// Resolution precedence:
//  1. --vcs jj|git CLI flag (--vcs auto falls through to probing)
//  2. .jj exists in the working directory tree → jj
//  3. .git exists in the working directory tree → git
//  4. otherwise → error
//
// jj wins over git when both are present (kawaz's git-bare + jj-workspace
// layout has both `.jj` and `.git` at the repo root; we want jj semantics
// in that case so revsets like `main@origin` work).
//
// vcsKind survives as the *override-spec type* parsed from `--vcs`
// (DR-0008 / DR-0016). The runtime VCS handle is now the vcsBackend
// interface (DR-0020); vcsKind only carries "what the user asked for"
// until newVcsBackend turns it into a concrete backend.
type vcsKind int

const (
	vcsAuto vcsKind = iota // unresolved sentinel
	vcsJj
	vcsGit
)

// String makes vcsKind printable in error messages.
func (k vcsKind) String() string {
	switch k {
	case vcsJj:
		return "jj"
	case vcsGit:
		return "git"
	default:
		return "auto"
	}
}

// parseVcsOverride parses a --vcs value. Empty string and "auto" both
// fall through to auto-detection.
func parseVcsOverride(s string) (vcsKind, error) {
	switch s {
	case "", "auto":
		return vcsAuto, nil
	case "jj":
		return vcsJj, nil
	case "git":
		return vcsGit, nil
	default:
		return vcsAuto, fmt.Errorf("invalid --vcs value %q (expected jj, git, or auto)", s)
	}
}

// detectVcs resolves the VCS to use for a `vcs:` input.
//
// override is from --vcs (highest priority). When override == vcsAuto
// we probe for `.jj` / `.git` directories in the current working
// directory (walking up to find them). The probe behaviour mirrors
// what `jj` and `git` themselves do — they look for the metadata
// directory in cwd or any parent.
func detectVcs(override vcsKind) (vcsKind, error) {
	if override != vcsAuto {
		return override, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return vcsAuto, fmt.Errorf("getwd: %w", err)
	}
	hasJj, hasGit := probeRepoMarkers(cwd)
	switch {
	case hasJj:
		return vcsJj, nil
	case hasGit:
		return vcsGit, nil
	default:
		return vcsAuto, fmt.Errorf("not a git or jj repository (no .jj or .git found in %s or any parent)", cwd)
	}
}

// probeRepoMarkers walks dir and its ancestors looking for a `.jj` and
// `.git` entry. The walk stops at the filesystem root. Both flags are
// returned independently because the two metadata directories can
// coexist (kawaz's git-bare + jj-workspace layout, and jj's colocate
// mode).
func probeRepoMarkers(dir string) (hasJj, hasGit bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".jj")); err == nil {
			hasJj = true
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			hasGit = true
		}
		if hasJj || hasGit {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// vcsParseSpec splits a `vcs:` input into its components.
//
// The string after the `vcs:` prefix is interpreted as:
//
//   - A function call (`<name>(<args>)`) when it contains a `(`. MVP
//     supports `latest-tag()` only; everything else returns an error
//     from resolveVcsInput. funcName is the part before the `(`.
//
//   - Otherwise a `REV[:FILE]` pair. The first `:` of the original
//     string is consumed by the `vcs:` prefix, so the FILE separator
//     is the first remaining `:`. When no `:` is present, file is empty
//     and resolveVcsInput borrows it from the sibling input.
//
// jj's revset syntax `main@origin` does not contain `:` and is preserved
// as-is in rev. git's `HEAD~1:Cargo.toml` is split as expected.
func vcsParseSpec(spec string) (rev, file string, isFunc bool, funcName string) {
	body := strings.TrimPrefix(spec, "vcs:")
	if i := strings.IndexByte(body, '('); i >= 0 {
		isFunc = true
		funcName = body[:i]
		// args portion (between '(' and the trailing ')') is left
		// in `rev` for future extensions; MVP rejects non-empty.
		rest := body[i:]
		if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
			rev = rest[1 : len(rest)-1]
		} else {
			rev = rest // malformed; resolveVcsInput will error.
		}
		return
	}
	if i := strings.IndexByte(body, ':'); i >= 0 {
		rev = body[:i]
		file = body[i+1:]
		return
	}
	rev = body
	return
}

// resolveVcsInput interprets a `vcs:...` argument and returns a fully
// resolved input. otherFile is the FILE-origin name of the sibling
// argument (or "" if none) and is used as the borrowed file path when
// the spec omits one.
//
// All version fields are labelled with the literal `vcs:` spec so that
// mismatch errors can identify the input. The returned resolvedInput
// has handler/file unset, so --write rejection in the bump path treats
// vcs: inputs the same as VER inputs (they are read-only by design).
//
// backend is the resolved vcsBackend produced by newVcsBackend (DR-0020).
// Caller is responsible for the lazy-detection (only build the backend
// when at least one input is `vcs:`).
func resolveVcsInput(spec string, otherFile string, backend vcsBackend) (resolvedInput, error) {
	rev, file, isFunc, funcName := vcsParseSpec(spec)
	if isFunc {
		return resolveVcsFunc(spec, funcName, rev, backend)
	}
	if file == "" {
		if otherFile == "" {
			return resolvedInput{}, fmt.Errorf("%s: file is required (no file argument to borrow from)", spec)
		}
		file = otherFile
	}
	content, err := backend.FetchFile(rev, file)
	if err != nil {
		return resolvedInput{}, err
	}
	h, err := detectHandler(file)
	if err != nil {
		return resolvedInput{}, err
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("%s: %w", spec, err)
	}
	// Origin label is the literal spec so it survives intact in
	// mismatch error column-alignment ("vcs:HEAD~1" reads naturally).
	return resolvedInput{
		originFile: spec,
		fields:     locatedFromInspection(spec, insp.Versions),
		// handler/file/content/insp are intentionally left zero —
		// vcs: is read-only, --write must reject it before this point.
	}, nil
}

// resolveVcsFunc handles function-shaped specs (`vcs:<name>(<args>)`).
//
// DR-0032 (v0.32.0): `vcs:latest-tag([REPO])` and `vcs:latest-release([REPO])`
// are the supported function-mode inputs. Both return the bare SemVer
// version of the largest stable tag / release (prerelease always excluded
// — input record is a value-mode subset; richer options are exposed via
// `vcs get latest-{tag,release}` subcommand instead).
//
// 引数 `args` は `(args)` の中身 (= `(kawaz/pkf-tasks)` なら `args ==
// "kawaz/pkf-tasks"`、`()` なら `args == ""`)。`expandRepoArg` で
// owner/repo 短縮 → URL 展開を行う (= DR-0019 schema、`translateRev`
// (DR-0031) とは別経路、DR-0032 原則 5)。
func resolveVcsFunc(spec, name, args string, backend vcsBackend) (resolvedInput, error) {
	switch name {
	case "latest-tag":
		return resolveLatestTag(spec, strings.TrimSpace(args), backend)
	case "latest-release":
		return resolveLatestRelease(spec, strings.TrimSpace(args), backend)
	default:
		return resolvedInput{}, fmt.Errorf("%s: unknown vcs function: %s() (supported: latest-tag, latest-release; richer option set in `vcs get latest-{tag,release}` subcommand)", spec, name)
	}
}

// translateRev translates a user-supplied rev into a backend-native form
// so every `vcs` rev receptor (FetchFile / Diff / DiffNameStatus /
// resolveJjRev / resolveGitRev / `vcs:` input mode) accepts either git or
// jj syntax without callers having to branch on the backend (DR-0031).
//
// Translation rules (v1, MVP):
//
//  1. `<remote>/<bookmark>` ⇔ `<bookmark>@<remote>` (single-slash / single-@,
//     no other special chars). Direction depends on `kind`:
//     - kind=jjBackendKind: git syntax `origin/main` → `main@origin`
//     - kind=gitBackendKind: jj syntax `main@origin` → `origin/main`
//  2. Multi-slash / multi-@ / anything with revset / revspec syntax
//     (`^`, `~`, `..`, `::`, `@{...}`, etc) is passed through unchanged
//     so the backend resolves its own native syntax.
//  3. Empty input is passed through (callers should reject empty rev
//     themselves; translation is best-effort).
//
// We translate only single-slash / single-@ forms because forms like
// `feature/foo/bar` are ambiguous without knowing the remote name set,
// and users with multi-segment bookmark names should write the explicit
// `bookmark@remote` form themselves.
//
// translateRev never returns an error; resolution failure is the
// downstream resolver's / backend cmd's concern (= they emit
// exit-code-3 / exit-code-4 with native error messages).
func translateRev(rev string, kind vcsKind) string {
	if rev == "" {
		return rev
	}
	// Bail out on anything that looks like a backend-native revspec /
	// revset operator. Keep the set tight: only chars that unambiguously
	// signal "this is already backend syntax".
	if strings.ContainsAny(rev, ".~^:|&{}()") {
		return rev
	}
	hasSlash := strings.Count(rev, "/") == 1
	hasAt := strings.Count(rev, "@") == 1
	// XOR: exactly one of slash-form or at-form is allowed.
	if hasSlash == hasAt {
		return rev
	}
	switch kind {
	case vcsJj:
		if !hasSlash {
			return rev // already jj-native (or unrelated to this rule)
		}
		i := strings.IndexByte(rev, '/')
		if i <= 0 || i == len(rev)-1 {
			return rev // leading or trailing slash → not a remote/bookmark
		}
		return rev[i+1:] + "@" + rev[:i]
	case vcsGit:
		if !hasAt {
			return rev // already git-native (or unrelated to this rule)
		}
		i := strings.IndexByte(rev, '@')
		if i <= 0 || i == len(rev)-1 {
			return rev // leading or trailing @ → not a bookmark@remote
		}
		return rev[i+1:] + "/" + rev[:i]
	}
	return rev
}

// latestTagFromRemote returns the SemVer-largest tag visible at the
// remote URL via `git ls-remote --tags <url>`. The cwd VCS is
// irrelevant — remote queries always go through git because both
// the protocol and the ls-remote output format are git's.
//
// When includePrerelease is false, pre-release tags are filtered
// out (default for `vcs tag latest`). Returns the raw tag string,
// the parsed Version, and any error.
func latestTagFromRemote(url string, includePrerelease bool) (string, Version, error) {
	out, err := runBackendCmd("git", "ls-remote", "--tags", url)
	if err != nil {
		return "", Version{}, err
	}
	tags := parseLsRemoteTags(string(out))
	return pickLatestSemverTag(tags, includePrerelease)
}

// parseLsRemoteTags extracts the tag name list from `git ls-remote
// --tags` output. The format is:
//
//	<sha>\trefs/tags/<tag>
//	<sha>\trefs/tags/<tag>^{}   (annotated tag's peeled commit)
//
// We strip the `refs/tags/` prefix and the `^{}` peeled-commit suffix
// so the caller sees plain tag names matching ListTags output.
func parseLsRemoteTags(out string) []string {
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := parts[1]
		if !strings.HasPrefix(ref, "refs/tags/") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		tag = strings.TrimSuffix(tag, "^{}") // peeled annotated tag
		tags = append(tags, tag)
	}
	return splitAndDedup(strings.Join(tags, "\n"))
}

// splitAndDedup extracts non-empty lines and removes duplicates while
// preserving first-seen order (so reproducibility doesn't depend on
// map iteration).
func splitAndDedup(s string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}

// expandRepoArg normalises a `<arg>` portion of `vcs:latest-tag(<arg>)`
// into a URL/spec that `git ls-remote --tags` accepts.
//
//   - Empty string is preserved (caller uses cwd VCS).
//   - HTTP(S) / SSH (`git@...` / `ssh://`) URLs pass through unchanged.
//   - `<owner>/<repo>` (exactly one `/`, no whitespace) is expanded to
//     `https://github.com/<owner>/<repo>` (GitHub-default convention).
//   - Anything else passes through verbatim; `git ls-remote` will report
//     the parse error which is more accurate than anything we could say.
//
// Whitespace around the input is trimmed so `vcs:latest-tag( foo/bar )`
// behaves the same as `vcs:latest-tag(foo/bar)`.
func expandRepoArg(arg string) string {
	s := strings.TrimSpace(arg)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "ssh://") {
		return s
	}
	// owner/repo: exactly one `/`, no embedded whitespace.
	if strings.Count(s, "/") == 1 && !strings.ContainsAny(s, " \t") {
		return "https://github.com/" + s
	}
	return s
}

// pickLatestSemverTag returns the SemVer-largest entry from `tags`,
// along with the original raw tag string.
//
// Tags that don't parse as semver (e.g. `my-build-2025-01-01`) are
// silently ignored — this lets repos mix release tags with
// build-stamp tags freely. The error path is reserved for "no
// semver-compatible tags found at all", which is actionable (the
// user either has the wrong --vcs or the repo really has no tags).
//
// SemVer order is determined by Version.Compare (DR-0006), so
// pre-release tags rank below their corresponding release as
// expected (`v1.0.0-rc.1` < `v1.0.0`).
//
// When includePrerelease is false, pre-release tags (`v1.2.3-rc.1`
// etc.) are filtered out before ranking — this matches the default
// for the `vcs tag latest` subcommand. The raw return value is the
// original tag string (with the source repo's prefix style intact);
// callers that want the bare SemVer form (no `v` / no `release-`
// prefix) should call `.String()` on the returned Version.
func pickLatestSemverTag(tags []string, includePrerelease bool) (string, Version, error) {
	type parsed struct {
		raw string
		v   Version
	}
	var candidates []parsed
	for _, t := range tags {
		v, err := ParseVersion(t)
		if err != nil {
			// Fallback: monorepo-style `<name>@<version>` (e.g.
			// `pkf-tasks@0.0.11`, `react@18.2.0`). Peel everything up
			// to and including the last `@` and retry. Tag-listing
			// output never contains jj-style `main@origin` revsets so
			// this is unambiguous in this context.
			if i := strings.LastIndex(t, "@"); i >= 0 {
				v, err = ParseVersion(t[i+1:])
			}
			if err != nil {
				continue
			}
		}
		if !includePrerelease && len(v.Pre) > 0 {
			continue
		}
		candidates = append(candidates, parsed{raw: t, v: v})
	}
	if len(candidates) == 0 {
		return "", Version{}, fmt.Errorf("no semver-compatible tags found")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		// Descending order so candidates[0] is the largest.
		return candidates[i].v.Compare(candidates[j].v) > 0
	})
	return candidates[0].raw, candidates[0].v, nil
}
