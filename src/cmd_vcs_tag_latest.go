package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// runVcsCmdTagLatest implements `vcs tag latest [--source <tag|release>]
// [--include-prerelease] [--repository REPO] [--raw | --json]`
// (DR-0020 PR-Tag-Latest, 2026-06-01).
//
// Source matrix:
//
//	--source tag (default)        cwd / external git refs (no gh)
//	  - no --repository           backend.LatestTag(includePre)
//	  - --repository owner/repo   latestTagFromRemote(url, includePre)
//	    or full URL               (git ls-remote --tags <url>)
//
//	--source release              gh release list -R <repo> (gh required)
//	  - no --repository           cwd repo (gh-detected)
//	  - --repository owner/repo   external GitHub repo
//
// Output formats (mutually exclusive):
//
//	default  → bare SemVer (1.2.3 — prefix stripped via Version.String
//	           with Prefix cleared)
//	--raw    → original tag string with prefix intact (v1.2.3 /
//	           release-1.2.3 / pkf-tasks@0.0.13)
//	--json   → {"tag": "v1.2.3", "version": "1.2.3",
//	             "commit": "...", "date": "..."} — commit/date are
//	             best-effort (only populated when the source provides
//	             them; never spawns an extra subprocess just to fill).
//
// Exit codes:
//
//	0  success (tag found and emitted)
//	2  usage error (bad --source value, --raw + --json combo, …)
//	3  VCS / gh subprocess error, OR `gh` missing for a path that
//	   requires it (with an actionable install hint)
//
// Per DR-0020 design-thinking: gh is a runtime dependency only when
// the user opts into `--source release`; `--source tag --repository
// <X>` deliberately stays gh-free (uses `git ls-remote --tags`).
func runVcsCmdTagLatest(args cliArgs, stdout, stderr io.Writer) error {
	// --- argument validity (uniform exit-2 path before subprocess) ---
	if len(args.vcsArgs) > 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag latest does not accept positional arguments (got %d)", len(args.vcsArgs)))
	}
	if args.vcsTag.LatestRaw && args.output.JSON {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag latest: --raw and --json are mutually exclusive"))
	}
	source := "tag"
	if args.vcsTag.LatestSource != nil {
		source = *args.vcsTag.LatestSource
	}
	if source != "tag" && source != "release" {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag latest: invalid --source value %q (expected: tag, release)", source))
	}

	includePre := args.vcsTag.LatestIncludePre
	var repo string
	if args.vcsTag.LatestRepository != nil {
		repo = *args.vcsTag.LatestRepository
	}

	// Resolve to a (raw, Version, info) tuple. info carries the
	// best-effort commit/date for --json (empty strings when the
	// source doesn't supply them).
	var (
		raw     string
		v       Version
		info    tagInfo
		fetchEr error
	)
	// Honour --vcs override for the cwd-VCS branch of --source tag.
	// (parseVcsOverride was already validated in parseArgs.)
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	switch source {
	case "tag":
		raw, v, info, fetchEr = fetchLatestTagFromTags(repo, includePre, vcsOverride)
	case "release":
		raw, v, info, fetchEr = fetchLatestTagFromReleases(repo, includePre)
	}
	if fetchEr != nil {
		return emitVcsErr(stderr, args, fetchEr)
	}

	// -q / -qq still suppress stdout (other vcs verbs do this; mirrors
	// `vcs get` behaviour — the exit code is the answer for callers
	// that just want presence).
	if args.output.Verbosity.ShouldSuppressStdout() {
		return nil
	}

	if args.output.JSON {
		// commit/date are best-effort; always emit tag + version.
		out := map[string]string{
			"tag":     raw,
			"version": semverString(v),
			"commit":  info.commit,
			"date":    info.date,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	}

	if args.vcsTag.LatestRaw {
		fmt.Fprintln(stdout, raw)
		return nil
	}
	fmt.Fprintln(stdout, semverString(v))
	return nil
}

// tagInfo carries the best-effort metadata for --json. Both fields are
// empty when the chosen source doesn't naturally provide them — we do
// NOT spawn extra subprocesses just to populate them (the dispatcher
// stays one-subprocess where possible).
type tagInfo struct {
	commit string
	date   string
}

// semverString returns the bare SemVer form (`1.2.3`) for the default
// output by clearing the Prefix the original tag carried (`v`, `ver-`,
// `release-`-style isn't in Prefix but `v` typically is). Sep is left
// alone so `1_2_3`-style versions (rare on tags but possible) survive.
func semverString(v Version) string {
	v.Prefix = ""
	return v.String()
}

// fetchLatestTagFromTags resolves the latest tag via the git/jj tag list
// (no gh dependency). Empty repo = cwd VCS via backend.LatestTag;
// non-empty repo = remote query via `git ls-remote --tags`. The vcsOverride
// honours --vcs (= forwarded from args.vcsBase.Override) so colocated
// jj+git repos can be forced to a specific backend.
func fetchLatestTagFromTags(repo string, includePre bool, vcsOverride vcsKind) (string, Version, tagInfo, error) {
	if repo == "" {
		// cwd VCS path — backend handles jj vs git probing.
		b, err := newVcsBackend(vcsOverride)
		if err != nil {
			return "", Version{}, tagInfo{}, err
		}
		raw, v, err := b.LatestTag(includePre)
		if err != nil {
			return "", Version{}, tagInfo{}, err
		}
		return raw, v, tagInfo{}, nil
	}
	// External repo path — `git ls-remote --tags <url>` (no gh).
	url := expandRepoArg(repo)
	if url == "" {
		// expandRepoArg trims; an empty string here means the user
		// passed --repository "" explicitly. Treat as usage-like
		// error but route through emitVcsErr so the exit code is 3
		// uniformly with other "bad input to subprocess" failures.
		return "", Version{}, tagInfo{}, fmt.Errorf("vcs tag latest: --repository value must not be empty")
	}
	raw, v, err := latestTagFromRemote(url, includePre)
	if err != nil {
		return "", Version{}, tagInfo{}, err
	}
	return raw, v, tagInfo{}, nil
}

// fetchLatestTagFromTagsViaGh / Releases — `gh release list` returns
// GitHub Release objects (= drafts not included by default), filtered
// by tagName == semver. gh is required; absence is a clean exit-3 with
// an install hint.
//
// We use the package-level ghRunner so tests can stub gh out.
func fetchLatestTagFromReleases(repo string, includePre bool) (string, Version, tagInfo, error) {
	if err := ensureGhAvailable(); err != nil {
		return "", Version{}, tagInfo{}, err
	}
	// `gh release list` defaults to cwd repo when -R is omitted; the
	// flag is only added when the caller supplied --repository.
	ghArgs := []string{"release", "list",
		"--limit", "100",
		"--json", "tagName,isDraft,publishedAt"}
	if repo != "" {
		ghArgs = append(ghArgs, "-R", repo)
	}
	out, err := ghRunner(ghArgs...)
	if err != nil {
		return "", Version{}, tagInfo{}, fmt.Errorf("gh %s: %w", strings.Join(ghArgs, " "), err)
	}
	// Decode the JSON shape gh emits (one array of objects). gh's
	// isPrerelease flag is intentionally not used here — prerelease
	// filtering goes through SemVer parsing (pickLatestSemverTag with
	// includePre), so the tag path and release path apply the same
	// rule. The field is still requested in --json so callers debugging
	// the upstream behaviour can see it; the unused-field omission is
	// deliberate consistency, not a bug.
	type ghRelease struct {
		TagName     string `json:"tagName"`
		IsDraft     bool   `json:"isDraft"`
		PublishedAt string `json:"publishedAt"`
	}
	var releases []ghRelease
	if err := json.Unmarshal(out, &releases); err != nil {
		return "", Version{}, tagInfo{}, fmt.Errorf("gh release list: parse JSON: %w", err)
	}
	// Build the candidate tag list (drafts dropped; prereleases dropped
	// unless includePre). Drop drafts unconditionally — they're an
	// in-progress state, not a released artefact, regardless of the
	// prerelease setting.
	type relCand struct {
		raw  string
		date string
	}
	rawToRel := make(map[string]relCand, len(releases))
	tags := make([]string, 0, len(releases))
	for _, r := range releases {
		if r.IsDraft {
			continue
		}
		tags = append(tags, r.TagName)
		rawToRel[r.TagName] = relCand{raw: r.TagName, date: r.PublishedAt}
	}
	raw, v, err := pickLatestSemverTag(tags, includePre)
	if err != nil {
		return "", Version{}, tagInfo{}, err
	}
	rc := rawToRel[raw]
	return raw, v, tagInfo{date: rc.date}, nil
}

// ghRunner is the package-level entrypoint for shelling out to gh.
// Tests rewrite this to stub the subprocess out (kept as a var rather
// than an interface to keep the call-site shape identical to the
// other backend subprocess helpers).
var ghRunner = func(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			return nil, fmt.Errorf("%s", errOut)
		}
		return nil, err
	}
	return out, nil
}

// ghLookPath is the package-level entrypoint for "does gh exist on
// PATH?", overridable in tests. Default uses exec.LookPath.
var ghLookPath = func() error {
	_, err := exec.LookPath("gh")
	return err
}

// ensureGhAvailable returns a clean exit-3-ready error when gh isn't
// on PATH. The hint mentions the install URL so users in CI logs get
// the actionable next step without needing to read help.
func ensureGhAvailable() error {
	if err := ghLookPath(); err != nil {
		return fmt.Errorf("gh CLI is required for --source release; install: https://cli.github.com/")
	}
	return nil
}
