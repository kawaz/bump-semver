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
// MVP supports `latest-tag()` only.
func resolveVcsFunc(spec, name, args string, backend vcsBackend) (resolvedInput, error) {
	switch name {
	case "latest-tag":
		// args is the inside of `latest-tag(...)`. Empty (or whitespace
		// only) means "use the local backend"; non-empty is a remote
		// repo spec resolved by expandRepoArg and queried via git
		// ls-remote (always git regardless of the local backend).
		remoteURL := expandRepoArg(args)
		var v Version
		var err error
		if remoteURL != "" {
			v, err = latestTagFromRemote(remoteURL)
		} else {
			v, err = backend.LatestTag()
		}
		if err != nil {
			return resolvedInput{}, fmt.Errorf("%s: %w", spec, err)
		}
		// Function-derived inputs contribute a single value with no
		// in-file path component, mirroring VER-origin behaviour.
		return resolvedInput{
			originFile: spec,
			fields:     []locatedField{{File: spec, Value: v.String()}},
		}, nil
	default:
		return resolvedInput{}, fmt.Errorf("%s: unknown vcs function: %s()", spec, name)
	}
}

// altJjRev maps a git-style remote ref (`<remote>/<bookmark>`) to jj's
// native `<bookmark>@<remote>` form. Returns ok=false when rev doesn't
// have exactly one `/` (e.g. `HEAD~1`, `main`, `feature/x` are all
// passed through unchanged by the caller).
//
// We accept only single-slash forms because `feature/foo/bar` is
// ambiguous without knowing the remote name set, and jj users who
// genuinely have a slash in a bookmark are better served by writing
// the explicit `bookmark@remote` form themselves.
func altJjRev(rev string) (string, bool) {
	i := strings.IndexByte(rev, '/')
	if i <= 0 || strings.Count(rev, "/") != 1 {
		return "", false
	}
	return rev[i+1:] + "@" + rev[:i], true
}

// latestTagFromRemote returns the SemVer-largest tag visible at the
// remote URL via `git ls-remote --tags <url>`. The cwd VCS is
// irrelevant — remote queries always go through git because both
// the protocol and the ls-remote output format are git's.
func latestTagFromRemote(url string) (Version, error) {
	out, err := runBackendCmd("git", "ls-remote", "--tags", url)
	if err != nil {
		return Version{}, err
	}
	tags := parseLsRemoteTags(string(out))
	return pickLatestSemverTag(tags)
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

// pickLatestSemverTag returns the SemVer-largest entry from `tags`.
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
func pickLatestSemverTag(tags []string) (Version, error) {
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
		candidates = append(candidates, parsed{raw: t, v: v})
	}
	if len(candidates) == 0 {
		return Version{}, fmt.Errorf("no semver-compatible tags found")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		// Descending order so candidates[0] is the largest.
		return candidates[i].v.Compare(candidates[j].v) > 0
	})
	return candidates[0].v, nil
}
