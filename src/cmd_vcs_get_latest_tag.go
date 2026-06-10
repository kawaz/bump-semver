package main

import (
	"fmt"
	"io"
)

// runVcsCmdGetLatestTag implements `vcs get latest-tag [--include-prerelease]
// [--repository REPO] [--json]` (DR-0032).
//
// Source:
//   - No --repository  → cwd VCS via backend.LatestTag(includePre)
//   - --repository R   → `git ls-remote --tags <R>` (gh-free)
//
// Output:
//   - default → bare SemVer (`1.2.3`)
//   - --json  → 12-field version schema (= same as `get --json`)
//
// Exit codes (DR-0020 family):
//
//	0  success (tag found and emitted)
//	2  usage error (positional args supplied, invalid flag combinations)
//	3  VCS subprocess error / no semver-compatible tags found
//
// `vcs get latest-tag` reads tag refs only (= source = tag axis is in the
// verb name, DR-0032 原則 1). For GH Release objects use
// `vcs get latest-release`.
func runVcsCmdGetLatestTag(args cliArgs, stdout, stderr io.Writer) error {
	// `vcs get latest-tag` takes no positional argument beyond the key
	// itself (= args.vcsArgs[0]). Extra positionals are usage errors.
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get latest-tag does not accept positional arguments (got %d extra)", len(args.vcsArgs)-1))
	}
	includePre := args.vcsGet.LatestIncludePre
	var repo string
	if args.vcsGet.LatestRepository != nil {
		repo = *args.vcsGet.LatestRepository
	}
	// 引数インジェクション対策 (C-1): a leading-`-` --repository would reach
	// `git ls-remote --tags <repo>` as a flag. Reject at the usage layer
	// (exit 2) before resolving the backend. expandRepoArg re-checks as
	// defense-in-depth for the `vcs:latest-tag(REPO)` input-mode path.
	if _, err := expandRepoArg(repo); err != nil {
		return emitVcsUsage(stderr, args, err)
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	raw, v, err := fetchLatestTag(repo, includePre, vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	if werr := emitLatestVersion(args, raw, v, stdout); werr != nil {
		return werr
	}
	return nil
}

// resolveLatestTag implements the `vcs:latest-tag([REPO])` input record
// (DR-0032 input record revival). Returns the bare SemVer version of the
// largest stable tag (prerelease always excluded, = subset of subcommand).
//
// repoArg is the raw `(REPO)` argument content (empty for `vcs:latest-tag()`).
// The spec string is preserved for error labels.
func resolveLatestTag(spec, repoArg string, backend vcsBackend) (resolvedInput, error) {
	// For input record, prerelease is always excluded (= DR-0032 input
	// record subset constraint). Repository = empty → cwd VCS.
	vcsKindForCwd := vcsAuto
	if backend != nil {
		// Match the cwd backend so `vcs:latest-tag()` honours --vcs auto
		// resolution from the caller (same as `vcs:REV` handling).
		switch backend.Kind() {
		case "jj":
			vcsKindForCwd = vcsJj
		case "git":
			vcsKindForCwd = vcsGit
		}
	}
	_, v, err := fetchLatestTag(repoArg, false, vcsKindForCwd)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("%s: %w", spec, err)
	}
	// Return the bare SemVer string — input record is value-mode (= the
	// resolved version is what gets compared / inspected, the raw tag
	// string is not surfaced here, use `vcs get latest-tag --json` if
	// the raw form is needed).
	return resolvedInput{
		originFile: spec,
		fields:     []locatedField{{File: spec, Value: bareSemverString(v)}},
	}, nil
}
