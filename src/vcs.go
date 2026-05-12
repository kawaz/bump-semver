package main

import (
	"fmt"
	"os"
	"os/exec"
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
func resolveVcsInput(spec string, otherFile string, vcs vcsKind) (resolvedInput, error) {
	rev, file, isFunc, funcName := vcsParseSpec(spec)
	if isFunc {
		return resolveVcsFunc(spec, funcName, rev, vcs)
	}
	if file == "" {
		if otherFile == "" {
			return resolvedInput{}, fmt.Errorf("%s: file is required (no file argument to borrow from)", spec)
		}
		file = otherFile
	}
	content, err := vcsFetchFile(vcs, rev, file)
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
func resolveVcsFunc(spec, name, args string, vcs vcsKind) (resolvedInput, error) {
	switch name {
	case "latest-tag":
		// args is the inside of `latest-tag(...)`. Empty (or whitespace
		// only) means "use cwd VCS"; non-empty is a remote repo spec
		// resolved by expandRepoArg.
		remoteURL := expandRepoArg(args)
		v, err := vcsLatestTag(vcs, remoteURL)
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

// vcsFetchFile reads `file` at revision `rev` from the underlying VCS.
//
// jj path:  `jj file show -r <rev> <file>`
//
//	when <rev> looks like `<remote>/<bookmark>` (e.g. `origin/main`)
//	and the first try fails, we retry with jj's native form
//	`<bookmark>@<remote>` (e.g. `main@origin`). git users habitually
//	write `origin/main` so we accept both spellings transparently.
//
// git path: `git show <rev>:<file>`. No fallback is needed.
//
// Errors from the VCS subprocess are surfaced verbatim (with stderr
// included) so users see jj/git's own diagnostics. We do not try to
// add hints — the user knows their VCS, and jj/git's messages are
// usually more accurate than anything we could synthesize.
func vcsFetchFile(vcs vcsKind, rev, file string) ([]byte, error) {
	switch vcs {
	case vcsJj:
		out, err := runVcs("jj", "file", "show", "-r", rev, file)
		if err == nil {
			return out, nil
		}
		// Fallback: convert `origin/main` → `main@origin` and retry.
		if alt, ok := altJjRev(rev); ok {
			if out2, err2 := runVcs("jj", "file", "show", "-r", alt, file); err2 == nil {
				return out2, nil
			}
		}
		return nil, err
	case vcsGit:
		return runVcs("git", "show", rev+":"+file)
	default:
		return nil, fmt.Errorf("vcs not detected (set --vcs)")
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

// vcsListTags returns every tag known to the VCS, in whatever order
// the VCS reports them. Caller is responsible for filtering / sorting.
//
// jj:  `jj log -r 'tags()' --no-graph -T '<one tag per line>'`
//
//	The template emits one line per tag name across all change
//	commits with tags. We do not run `jj git fetch` here — DR-0008
//	makes "no implicit network calls" an explicit decision.
//
// git: `git tag --list`
func vcsListTags(vcs vcsKind) ([]string, error) {
	switch vcs {
	case vcsJj:
		// `tags.map(|t| t.name() ++ "\n").join("")` gives one tag per
		// line. Multiple changes with overlapping tags would emit each
		// tag once per change; we de-duplicate after.
		out, err := runVcs("jj", "log", "-r", "tags()", "--no-graph",
			"-T", `tags.map(|t| t.name() ++ "\n").join("")`)
		if err != nil {
			return nil, err
		}
		return splitAndDedup(string(out)), nil
	case vcsGit:
		out, err := runVcs("git", "tag", "--list")
		if err != nil {
			return nil, err
		}
		return splitAndDedup(string(out)), nil
	default:
		return nil, fmt.Errorf("vcs not detected (set --vcs)")
	}
}

// vcsListTagsRemote returns tag names visible via `git ls-remote --tags
// <url>` against a remote repository (no cwd VCS state involved).
//
// Output format of `git ls-remote --tags <url>`:
//
//	<sha>\trefs/tags/<tag>
//	<sha>\trefs/tags/<tag>^{}   (annotated tag's peeled commit)
//
// We strip the `refs/tags/` prefix and the `^{}` peeled-commit suffix so
// the caller sees plain tag names matching `vcsListTags` output.
func vcsListTagsRemote(url string) ([]string, error) {
	out, err := runVcs("git", "ls-remote", "--tags", url)
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, line := range strings.Split(string(out), "\n") {
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
	return splitAndDedup(strings.Join(tags, "\n")), nil
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

// vcsLatestTag returns the SemVer-largest tag known to the VCS.
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
func vcsLatestTag(vcs vcsKind, remoteURL string) (Version, error) {
	var tags []string
	var err error
	if remoteURL != "" {
		// Remote query: cwd VCS is irrelevant, always `git ls-remote`.
		tags, err = vcsListTagsRemote(remoteURL)
	} else {
		tags, err = vcsListTags(vcs)
	}
	if err != nil {
		return Version{}, err
	}
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

// runVcs runs an external VCS command and returns stdout. Stderr is
// captured separately and folded into the error message so the user
// sees the VCS's own diagnostic verbatim — that's almost always more
// accurate than anything we could rephrase.
func runVcs(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			return nil, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errOut)
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}
