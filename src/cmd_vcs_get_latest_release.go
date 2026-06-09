package main

import (
	"fmt"
	"io"
)

// runVcsCmdGetLatestRelease implements `vcs get latest-release
// [--include-prerelease] [--repository REPO] [--json]` (DR-0032).
//
// Source:
//   - No --repository  → cwd repo via `gh release list` (gh auto-detects repo)
//   - --repository R   → `gh release list -R <R>`
//
// gh is always required (= source = release axis is in the verb name).
// Drafts are dropped unconditionally; prereleases are dropped unless
// --include-prerelease.
//
// Output:
//   - default → bare SemVer (`1.2.3`)
//   - --json  → 12-field version schema (= same as `get --json`)
//
// Exit codes (DR-0020 family):
//
//	0  success (release found and emitted)
//	2  usage error (positional args supplied, invalid flag combinations)
//	3  gh subprocess error, gh missing, or no semver-compatible releases
func runVcsCmdGetLatestRelease(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get latest-release does not accept positional arguments (got %d extra)", len(args.vcsArgs)-1))
	}
	includePre := args.vcsGet.LatestIncludePre
	var repo string
	if args.vcsGet.LatestRepository != nil {
		repo = *args.vcsGet.LatestRepository
	}
	raw, v, err := fetchLatestRelease(repo, includePre)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	if werr := emitLatestVersion(args, raw, v, stdout); werr != nil {
		return werr
	}
	return nil
}

// resolveLatestRelease implements the `vcs:latest-release([REPO])` input
// record (DR-0032 input record revival). Returns the bare SemVer version
// of the largest stable release (prerelease always excluded).
//
// gh is required for this path; missing gh surfaces a clean error with an
// install hint (= exit 3 when used via compare/get).
func resolveLatestRelease(spec, repoArg string, _ vcsBackend) (resolvedInput, error) {
	_, v, err := fetchLatestRelease(repoArg, false)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("%s: %w", spec, err)
	}
	return resolvedInput{
		originFile: spec,
		fields:     []locatedField{{File: spec, Value: bareSemverString(v)}},
	}, nil
}
