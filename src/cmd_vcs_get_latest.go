package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// cmd_vcs_get_latest.go — DR-0032 共通基盤
//
// `vcs get latest-tag` / `vcs get latest-release` の共通 helper:
//
//   - fetchLatestTag()     — tag 列 (cwd backend or git ls-remote) から最大 SemVer
//   - fetchLatestRelease() — gh release list から最大 SemVer
//   - emitLatestVersion()  — text default (bare semver) / --json (12 field schema)
//     の共通 stdout 出力
//   - ghRunner / ensureGhAvailable — gh CLI 経路の subprocess helper
//
// 入力 record (`vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])`) と
// subcommand (`vcs get latest-{tag,release}`) は同じ fetch helper を共有して
// 出力経路だけ差し替える。
//
// 設計原則 (DR-0032 原則 5):
//   - `expandRepoArg` (= owner/repo → GitHub URL 展開、in src/vcs.go) は
//     `--repository` / `(REPO)` の **repository 引数** 専用。`translateRev`
//     (DR-0031、rev 翻訳) と字面が似ていても**共通 normalize 層に乗せない**。
//     文脈で意味が確定する (関数名 / 引数位置)。

// fetchLatestTag resolves the latest semver-parseable tag from either the cwd
// VCS (when repo is empty) or an external repo via `git ls-remote --tags`.
// No gh dependency.
//
// vcsOverride is the value of `--vcs jj|git|auto` for the cwd path only;
// remote queries always go through git regardless of cwd backend.
func fetchLatestTag(repo string, includePre bool, vcsOverride vcsKind) (string, Version, error) {
	if repo == "" {
		b, err := newVcsBackend(vcsOverride)
		if err != nil {
			return "", Version{}, err
		}
		return b.LatestTag(includePre)
	}
	url := expandRepoArg(repo)
	if url == "" {
		return "", Version{}, fmt.Errorf("--repository value must not be empty")
	}
	return latestTagFromRemote(url, includePre)
}

// fetchLatestRelease resolves the latest semver-parseable GitHub Release
// (drafts dropped, prereleases dropped unless includePre). Requires gh.
func fetchLatestRelease(repo string, includePre bool) (string, Version, error) {
	if err := ensureGhAvailable(); err != nil {
		return "", Version{}, err
	}
	ghArgs := []string{"release", "list",
		"--limit", "100",
		"--json", "tagName,isDraft,publishedAt"}
	if repo != "" {
		ghArgs = append(ghArgs, "-R", repo)
	}
	out, err := ghRunner(ghArgs...)
	if err != nil {
		return "", Version{}, fmt.Errorf("gh %s: %w", strings.Join(ghArgs, " "), err)
	}
	// gh's `isPrerelease` flag is intentionally not consulted — prerelease
	// filtering goes through SemVer parsing in pickLatestSemverTag so the
	// tag path and release path apply the same rule.
	type ghRelease struct {
		TagName     string `json:"tagName"`
		IsDraft     bool   `json:"isDraft"`
		PublishedAt string `json:"publishedAt"`
	}
	var releases []ghRelease
	if err := json.Unmarshal(out, &releases); err != nil {
		return "", Version{}, fmt.Errorf("gh release list: parse JSON: %w", err)
	}
	tags := make([]string, 0, len(releases))
	for _, r := range releases {
		if r.IsDraft {
			continue
		}
		tags = append(tags, r.TagName)
	}
	return pickLatestSemverTag(tags, includePre)
}

// emitLatestVersion renders the resolved (raw, v) pair to stdout. With
// --json, emits the 12-field version schema (= same as `get --json`).
// Default text mode is bare SemVer (`1.2.3`).
//
// raw is the raw tag/release name string (e.g. `v1.2.3`, `pkf-tasks@0.0.13`).
// For --json, raw populates `.version` (the raw input form, parallel to
// `get --json` which preserves the input string in `.version`); the parsed
// SemVer goes to `.semver`. The monorepo prefix (e.g. `pkf-tasks`) is
// extracted into `.name`.
func emitLatestVersion(args cliArgs, raw string, v Version, stdout io.Writer) error {
	if args.output.Verbosity.ShouldSuppressStdout() {
		return nil
	}
	if args.output.JSON {
		name := monorepoPrefix(raw)
		out := v.ToJSON(name)
		// Override .version with the raw tag/release string so callers can
		// recover the original prefix (=旧 `--raw` 相当の情報を JSON に内包)。
		out.Version = raw
		b, err := marshalJSONOutput(out)
		if err != nil {
			return err
		}
		_, werr := stdout.Write(b)
		return werr
	}
	fmt.Fprintln(stdout, bareSemverString(v))
	return nil
}

// bareSemverString returns the SemVer-canonical form of v with prefix
// cleared (e.g. `1.2.3` from input `v1.2.3` / `release-1.2.3` / `pkf-tasks@0.0.13`).
func bareSemverString(v Version) string {
	v.Prefix = ""
	return v.String()
}

// monorepoPrefix returns the leading `<name>` segment from a monorepo-style
// tag `<name>@<version>` (e.g. `pkf-tasks@0.0.13` → `"pkf-tasks"`). Returns
// nil when raw has no `@` (= conventional `v1.2.3` style) or when the part
// before `@` is empty.
//
// This mirrors pickLatestSemverTag's `@`-peel fallback (DR-0019): the same
// monorepo convention that lets the parser accept `pkf-tasks@0.0.13` as a
// version source gets surfaced in JSON as the `.name` field.
func monorepoPrefix(raw string) *string {
	i := strings.LastIndex(raw, "@")
	if i <= 0 {
		return nil
	}
	name := raw[:i]
	return &name
}

// ghRunner shells out to `gh` and returns stdout (or an error wrapping
// stderr when gh exits non-zero). var so tests can stub.
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

// ghLookPath answers "does gh exist on PATH?". var so tests can stub.
var ghLookPath = func() error {
	_, err := exec.LookPath("gh")
	return err
}

// ensureGhAvailable returns a clean exit-3-ready error when gh isn't on PATH,
// with an actionable install hint.
func ensureGhAvailable() error {
	if err := ghLookPath(); err != nil {
		return fmt.Errorf("gh CLI is required for vcs get latest-release; install: https://cli.github.com/")
	}
	return nil
}
