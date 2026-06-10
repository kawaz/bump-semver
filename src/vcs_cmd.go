package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// runVcsCmd is the dispatcher for the `vcs <verb>` family (DR-0020). PR-2
// adds `vcs is` alongside `vcs get`; future verbs (diff / commit / push /
// tag) plug in here as additional cases.
func runVcsCmd(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	switch args.vcsVerb {
	case "get":
		return runVcsCmdGet(args, stdout, stderr)
	case "is":
		return runVcsCmdIs(args, stdout, stderr)
	case "diff":
		return runVcsCmdDiff(args, stdout, stderr)
	case "commit":
		return runVcsCmdCommit(args, stdout, stderr)
	case "fetch":
		return runVcsCmdFetch(args, stdout, stderr)
	case "push":
		return runVcsCmdPush(args, stdout, stderr)
	case "tag":
		return runVcsCmdTag(args, stdout, stderr)
	case "outdated":
		return runVcsCmdOutdated(args, stdout, stderr)
	default:
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs verb: %s (expected: get / is / diff / commit / fetch / push / tag / outdated)", args.vcsVerb))
	}
}

// vcsGetKeys lists the keys recognised by `vcs get`. Kept as a slice so
// the order is preserved when we surface it in error messages.
//
// DR-0032: latest-tag / latest-release replace the v0.29.0 `vcs tag latest
// --source <tag|release>` (= source 軸を verb 名に畳む)。
var vcsGetKeys = []string{"root", "backend", "current-branch", "commit-id", "latest-tag", "latest-release"}

// runVcsCmdGet implements `vcs get <key>`.
//
// Exit codes (DR-0020):
//
//   - 0  on success
//   - 2  when the key is missing / unknown (usage)
//   - 3  when the VCS subprocess fails or the cwd is not a vcs repo
//   - 4  when the answer is ambiguous (detached HEAD, multi-bookmark)
//
// The output is unadorned (no JSON wrapper) — `vcs get` is intentionally
// shell-friendly, like `git rev-parse --show-toplevel`.
func runVcsCmdGet(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get requires a key (one of: %s)", strings.Join(vcsGetKeys, " / ")))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs get takes exactly one key, got %d", len(args.vcsArgs)))
	}
	key := args.vcsArgs[0]

	// The `backend` key is the only one that doesn't need to actually
	// build a backend before answering — but for consistency (and so a
	// non-vcs cwd reports exit 3 here too) we resolve the backend up
	// front and let the unknown-key check fall through.
	known := false
	for _, k := range vcsGetKeys {
		if k == key {
			known = true
			break
		}
	}
	if !known {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs get key: %s (expected one of: %s)", key, strings.Join(vcsGetKeys, " / ")))
	}

	// DR-0032 flag gating: --repository / --include-prerelease / --json are
	// only meaningful for latest-tag / latest-release. The flag parser
	// accepts them on any `vcs get` invocation so the positional order is
	// free (`vcs get --json latest-tag` and `vcs get latest-tag --json`
	// both parse), then this dispatcher rejects them against unrelated keys.
	isLatest := key == "latest-tag" || key == "latest-release"
	if !isLatest {
		if args.vcsGet.LatestRepository != nil {
			return emitVcsUsage(stderr, args,
				fmt.Errorf("--repository is only valid with vcs get latest-tag / latest-release"))
		}
		if args.vcsGet.LatestIncludePre {
			return emitVcsUsage(stderr, args,
				fmt.Errorf("--include-prerelease is only valid with vcs get latest-tag / latest-release"))
		}
		if args.output.JSON {
			return emitVcsUsage(stderr, args,
				fmt.Errorf("--json is only valid with vcs get latest-tag / latest-release"))
		}
	}
	if key != "commit-id" && args.vcsGet.Rev != nil {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("--rev is only valid with vcs get commit-id"))
	}

	// latest-tag / latest-release dispatch — they have their own backend
	// handling (cwd VCS via newVcsBackend for tag without --repository, or
	// gh subprocess for release).
	switch key {
	case "latest-tag":
		return runVcsCmdGetLatestTag(args, stdout, stderr)
	case "latest-release":
		return runVcsCmdGetLatestRelease(args, stdout, stderr)
	}

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// -q / -qq both suppress the stdout value (the exit code carries the
	// information the caller actually needs in scripted contexts).
	emit := func(s string) {
		if args.output.Verbosity.ShouldSuppressStdout() {
			return
		}
		fmt.Fprintln(stdout, s)
	}
	switch key {
	case "backend":
		emit(b.Kind())
		return nil
	case "root":
		root, err := b.Root()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		emit(root)
		return nil
	case "current-branch":
		name, err := b.CurrentBranch()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		emit(name)
		return nil
	case "commit-id":
		rev := derefOr(args.vcsGet.Rev, "")
		// 引数インジェクション対策 (C-1): leading-`-` rev would reach
		// `git rev-parse <rev>` / `jj log -r <rev>` as a flag.
		if err := validateUserRev(rev); err != nil {
			return emitVcsUsage(stderr, args, err)
		}
		sha, err := b.CommitID(rev)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		emit(sha)
		return nil
	}
	// Unreachable: key was validated against vcsGetKeys above.
	return emitVcsUsage(stderr, args, fmt.Errorf("internal: unhandled vcs get key %q", key))
}

// vcsIsPreds lists the predicates recognised by `vcs is`. Kept as a
// slice so the order is preserved when surfaced in error messages.
//
// DR-0020 scope rule: only predicates that read the same way for git
// and jj users land here. Backend-specific concepts (e.g. jj's
// `empty @`) stay out — they would not be transferable to git users
// reading shared Taskfiles.
var vcsIsPreds = []string{"clean", "dirty", "git", "jj"}

// runVcsCmdIs implements `vcs is <pred>`.
//
// Exit codes (DR-0020):
//
//   - 0  predicate true
//   - 1  predicate false (silent on stderr, mirroring `compare`)
//   - 2  usage error (missing / unknown / too many args)
//   - 3  VCS subprocess error or "not a vcs repo"
//
// `clean` / `dirty` need to consult the backend's worktree state.
// `git` / `jj` need to know which backend the auto-probe (or override)
// selected — for both we build the backend up front and surface its
// exit-3 if we're outside a repo. That distinguishes "not git" from
// "can't tell" (DR-0020: 曖昧・期待外はエラー).
func runVcsCmdIs(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs is requires a predicate (one of: %s)", strings.Join(vcsIsPreds, " / ")))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs is takes exactly one predicate, got %d", len(args.vcsArgs)))
	}
	pred := args.vcsArgs[0]

	known := false
	for _, p := range vcsIsPreds {
		if p == pred {
			known = true
			break
		}
	}
	if !known {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs is predicate: %s (expected one of: %s)", pred, strings.Join(vcsIsPreds, " / ")))
	}

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	var result bool
	switch pred {
	case "clean":
		result, err = b.IsClean()
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
	case "dirty":
		clean, ierr := b.IsClean()
		if ierr != nil {
			return emitVcsErr(stderr, args, ierr)
		}
		result = !clean
	case "git", "jj":
		result = b.Kind() == pred
	default:
		// Unreachable: pred was validated against vcsIsPreds above.
		return emitVcsUsage(stderr, args, fmt.Errorf("internal: unhandled vcs is predicate %q", pred))
	}

	if result {
		return nil
	}
	// Predicate-false is silent on stderr — matches `compare` semantics
	// so shell `if`/`&&` chains work without filtering output.
	return &exitErr{code: exitCodeFalse}
}

// runVcsCmdDiff implements `vcs diff REV [PATH..]` (DR-0020 PR-3, PR-3.1).
//
// Exit codes (DR-0020):
//
//   - 0  success: with -q, "no diff"; otherwise patch written to stdout
//     (which may legitimately be empty)
//   - 1  with -q only: "diff present" (predicate-false, mirrors
//     `git diff --quiet`'s --exit-code semantic)
//   - 2  usage error (parser surfaces "no REV" as help; reserved for
//     future verb-level usage problems)
//   - 3  VCS subprocess error or "not a vcs repo"
//
// Design rationale (-q overload):
//
//	`-q/--quiet` on `vcs diff` overloads the global "suppress stdout"
//	meaning to ALSO reflect diff presence in the exit code. This is
//	consistency with `git diff --quiet` (which implies --exit-code:
//	0 = clean, 1 = differs), the right mental model for a diff command.
//	Other vcs verbs (`get`/`is`) keep the pure stdout-suppression
//	meaning — diff is the only verb whose "is there anything?" question
//	is well-posed.
//
// `-s/--name-status` switches the output to one `<CODE>\t<path>\n` line
// per changed file (git's --name-status shape, normalized in the jj
// backend). `-s -q` collapses to `-q` (stdout empty, exit reflects
// presence) — one code path feeds both views: name-status output's
// emptiness == diff absence.
//
// Path-handling rule (kawaz's declarative-convergence): nonexistent paths
// are silently ignored. When every supplied path is filtered out we emit
// nothing — `vcs diff REV nope.txt` deliberately does NOT widen back to
// "diff everything" the way `git diff REV --` would. Under -q this yields
// exit 0 (= "no diff to report").
func runVcsCmdDiff(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		// The cobra layer (bareVerb) normally short-circuits "vcs diff"
		// with no further args to the per-verb help; this branch only fires
		// if a future code path reaches the dispatcher with an empty arg list.
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs diff requires a REV (usage: vcs diff REV [PATH..])"))
	}
	rev := args.vcsArgs[0]
	// 引数インジェクション対策 (C-1): a leading-`-` rev reaches `git diff <rev>`
	// as a flag (e.g. `--output=<path>` writes an arbitrary file). The `--`
	// arg separator lets such a value into vcsArgs[0], bypassing the parser's
	// own leading-`-` flag rejection, so we re-check here at the dispatch layer.
	if err := validateUserRev(rev); err != nil {
		return emitVcsUsage(stderr, args, err)
	}
	rawPaths := args.vcsArgs[1:]

	// DR-0024: expand `glob:` / `file:` selectors. Literal paths pass through
	// to the backend so that:
	//   - directory literals (e.g. `src/`) keep their pathspec semantics
	//     (= backend sees directory pathspec, handles deletions natively)
	//   - file literals that were deleted in the working copy still surface
	//     in the backend's diff (= they exist in REV, backend computes the
	//     deletion entry)
	//
	// selectorsGiven reflects whether the user supplied any positional
	// selector; the declarative-convergence rule (= "selectors given but
	// all-nonexistent → diff nothing") is preserved by the backend's
	// existing filterExistingPaths call.
	paths, err := expandGlobInputs(rawPaths, args.glob)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	selectorsGiven := len(rawPaths) > 0
	// DR-0033: --excludes is forwarded to the backend as native pathspec
	// (`:(exclude,glob)pat` for git, `(includes) ~ pat ~ ...` fileset for jj),
	// so the backend handles literal directory includes and deletions
	// uniformly. `file:` excludes are expanded locally first since the
	// backend doesn't know our `file:` shape; the resulting flat list of
	// patterns is then forwarded as separate excludes.
	var excludes []string
	if len(args.vcsDiff.Excludes) > 0 {
		if !selectorsGiven {
			return emitVcsUsage(stderr, args,
				fmt.Errorf("vcs diff: --excludes requires at least one positional PATH (= explicit include set; bare 'diff everything' minus excludes is not supported)"))
		}
		excludes, err = flattenExcludePatterns(args.vcsDiff.Excludes, args.glob)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
	}

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// Selectors given but expansion empty → short-circuit to "no diff".
	// Both predicate and display modes return cleanly (exit 0, no stdout).
	if selectorsGiven && len(paths) == 0 {
		return nil
	}

	// -q (and -qq) trigger the predicate-only path: derive presence from
	// name-status output (cheap; same shape feeds -s display). Doing this
	// before -s keeps `-q` strictly authoritative when both are set.
	if args.output.Verbosity.ShouldSuppressStdout() {
		ns, err := b.DiffNameStatus(rev, paths, excludes)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		if len(ns) == 0 {
			return nil
		}
		// Silent predicate-false (matches runVcsCmdIs / compare).
		return &exitErr{code: exitCodeFalse}
	}

	// -s: name-status display (no quiet → stdout gets the codes).
	if args.vcsDiff.NameStatus {
		out, err := b.DiffNameStatus(rev, paths, excludes)
		if err != nil {
			return emitVcsErr(stderr, args, err)
		}
		if len(out) > 0 {
			if _, werr := stdout.Write(out); werr != nil {
				return werr
			}
		}
		return nil
	}

	// Default: raw patch.
	out, err := b.Diff(rev, paths, excludes)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	if len(out) > 0 {
		if _, werr := stdout.Write(out); werr != nil {
			return werr
		}
	}
	return nil
}

// runVcsCmdCommit implements `vcs commit` (DR-0020 PR-4, PR-4.1).
//
// The verb is fully symmetric with `--amend`: the only difference
// between `commit` and `commit --amend` is "create a new commit" vs
// "fold into the previous one". Both forms accept identical path
// selectors (PR-4.1):
//
//   - `vcs commit -m MSG PATH..`               — new commit, listed paths
//   - `vcs commit -m MSG --staged`             — new commit, all staged
//   - `vcs commit --amend [-m MSG]`            — fold all current into prev
//   - `vcs commit --amend [-m MSG] PATH..`     — fold listed paths into prev
//   - `vcs commit --amend [-m MSG] --staged`   — fold all staged into prev
//
// `-a` / `--all` is intentionally not provided (DR-0020 safety: kawaz
// CLI design + jj's auto-staged worldview). It's parsed only so we can
// reject it with a tailored exit-2 hint instead of the generic
// "unknown flag" catch-all.
//
// Argument-error ordering (advisor #3):
//
//  1. -a   → exit 2 with hint (backend-independent, before resolve)
//  2. path + --staged → exit 2 (amend-agnostic mutex; before resolve)
//  3. !amend && !message → exit 2 (backend-independent, before resolve)
//  4. Resolve backend (exit 3 if not a vcs repo)
//  5. no PATH && no --staged && !amend → exit 2 with backend-Kind() hint
//  6. Dispatch to backend.Commit
//
// Exit codes (DR-0020):
//
//   - 0  success (commit created, or no-op if there was nothing to commit)
//   - 2  usage error
//   - 3  VCS subprocess error or "not a vcs repo"
func runVcsCmdCommit(args cliArgs, stdout, stderr io.Writer) error {
	// Step 1: -a explicit reject (before backend resolve so non-repo
	// cwd still gets the tailored hint).
	if args.vcsCommit.DashA {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: -a / --all is not supported (use --staged to commit all staged changes, or pass PATH..)"))
	}
	// Step 2: path + --staged exclusivity.
	if args.vcsCommit.Staged && len(args.vcsArgs) > 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: --staged and PATH.. are mutually exclusive"))
	}
	// Step 3: -m required for !amend.
	if !args.vcsCommit.Amend && args.vcsCommit.Message == nil {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: -m MSG is required (unless --amend)"))
	}
	// PR-4.1: the PR-4 step-3.5 reject (`--amend PATH..` / `--amend
	// --staged`) was removed. Commit and amend are now fully symmetric
	// in which path selectors they accept; the only difference is "new
	// commit vs absorb into previous". The PATH / --staged exclusivity
	// from step 2 still guards the `--amend PATH.. --staged` triple-
	// combo (both modes mutually exclude each other regardless of
	// amend), and the dispatch into backend.Commit fans the four
	// accepted shapes (paths / --staged / bare / amend-of-each) into
	// the backend implementations.
	// Step 4: resolve backend.
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// Step 5: no mode (= no path, no --staged, no --amend) → backend-
	// specific hint. By this point we know we're in a vcs repo, so the
	// hint can be specific to git's "you usually want --staged" vs jj's
	// "auto-staged world, name a PATH".
	if !args.vcsCommit.Amend && !args.vcsCommit.Staged && len(args.vcsArgs) == 0 {
		var hint string
		switch b.Kind() {
		case "git":
			hint = "use --staged to commit staged changes, or specify PATH.."
		case "jj":
			hint = "specify PATH.. (commit -a is not supported by design); or use --staged for the entire @ change"
		default:
			hint = "specify PATH.. or --staged"
		}
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs commit: nothing to commit (%s)", hint))
	}
	// DR-0024: expand `glob:` selectors in PATH.. before dispatching to
	// the backend. selectorsGiven=true means the user supplied at least
	// one positional selector; if expansion is empty we short-circuit to
	// "no-op success" — mirrors the existing all-nonexistent path
	// behavior in gitBackend.Commit / jjBackend.Commit, plus avoids the
	// "expanded paths means commit nothing was intended" case slipping
	// into a different mode.
	paths, err := expandGlobInputs(args.vcsArgs, args.glob)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	selectorsGiven := len(args.vcsArgs) > 0
	if selectorsGiven && len(paths) == 0 && !args.vcsCommit.Staged && !args.vcsCommit.Amend {
		// Path selectors collapsed to empty AND no other mode: no-op success.
		return nil
	}
	// Step 6: dispatch.
	opts := commitOpts{
		paths:   paths,
		message: derefOr(args.vcsCommit.Message, ""),
		staged:  args.vcsCommit.Staged,
		amend:   args.vcsCommit.Amend,
		noEdit:  args.vcsCommit.Amend && args.vcsCommit.Message == nil,
	}
	if err := b.Commit(opts); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// commit is silent on success (mirrors `git commit -q` philosophy
	// for scripted callers; jj users get jj's own snapshot text in
	// stderr from the subprocess but we don't echo on stdout). The
	// stdout writer is therefore unused — kept in the signature for
	// dispatcher uniformity with the other vcs verbs.
	return nil
}

// runVcsCmdFetch implements `vcs fetch [REMOTE]` (DR-0020 PR-5).
//
// Grammar:
//
//   - 0 positional → fetch the default remote ("origin", or the value of
//     `--remote NAME` if supplied)
//   - 1 positional → fetch that remote (positional and `--remote NAME`
//     are mutually exclusive; double-source is rejected)
//   - 2+ positionals → usage error
//
// Exit codes (DR-0020):
//
//   - 0  success
//   - 2  usage error
//   - 3  VCS subprocess error (unknown remote, network failure, not a repo)
func runVcsCmdFetch(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs fetch takes at most one remote name, got %d", len(args.vcsArgs)))
	}
	// Resolve REMOTE precedence: positional > --remote > "origin".
	remote := "origin"
	if args.vcsPush.Remote != nil {
		remote = *args.vcsPush.Remote
	}
	if len(args.vcsArgs) == 1 {
		// Positional and --remote together is over-specification; reject
		// to avoid silent precedence surprises.
		if args.vcsPush.Remote != nil {
			return emitVcsUsage(stderr, args,
				fmt.Errorf("vcs fetch: REMOTE positional and --remote are mutually exclusive"))
		}
		remote = args.vcsArgs[0]
	}
	// 引数インジェクション対策 (C-1): leading-`-` remote would reach
	// `git fetch <remote>` as a flag.
	if err := validateRemote(remote); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs fetch: %w", err))
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	if err := b.Fetch(remote); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	return nil
}

// runVcsCmdPush implements `vcs push --branch|--bookmark NAME [--remote
// REMOTE]` (DR-0020 PR-5).
//
// Grammar requirements:
//
//   - NAME is required (no auto-detection — the user always names the
//     branch / bookmark explicitly so a typo in the verb cannot lead to
//     "wait, which ref did that just push?")
//   - REMOTE defaults to "origin"
//   - No positional args accepted (NAME comes via --branch/--bookmark)
//   - --force / --tags / friends are intentionally NOT provided
//     (DR-0020 PR-5 safety: divergent remotes require a fetch +
//     reconcile, not a force push)
//
// Exit codes (DR-0020):
//
//   - 0  success (incl. idempotent no-op "remote already has it")
//   - 2  usage error (NAME missing, positional args supplied, unknown flag)
//   - 3  VCS subprocess error (unknown remote, network failure, not a repo)
//   - 5  non-fast-forward rejection — the remote has commits we don't
//     have. Hint mentions fetch+reconcile and that force push is
//     intentionally unsupported.
func runVcsCmdPush(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) > 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs push does not accept positional arguments (use --branch/--bookmark NAME)"))
	}
	if args.vcsPush.Name == nil {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs push: --branch (or --bookmark) NAME is required"))
	}
	if *args.vcsPush.Name == "" {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs push: --branch/--bookmark value must not be empty"))
	}
	remote := "origin"
	if args.vcsPush.Remote != nil {
		remote = *args.vcsPush.Remote
	}
	// 引数インジェクション対策 (C-1): leading-`-` remote would reach
	// `git push <remote> <refspec>` as a flag.
	if err := validateRemote(remote); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs push: %w", err))
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in validateVcsOverride
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// DR-0020 PR-5.2.1 (backend-prefix general rule, kawaz 2026-06-01 確定):
	// --jj-* / --git-* flags are backend-specific by *name*. The structural
	// prefix already tells the user "this is for backend X only", so when
	// the active backend is the other one, the flag is **silent no-op**
	// (not an error). Rationale: a user-or-script-driven `vcs push
	// --jj-bookmark-auto-advance` should "just work" on both jj and git
	// repos — on jj it auto-advances, on git it does nothing, push proceeds
	// either way. Subcommand split for jj-specific operations is the wrong
	// granularity (kawaz: 「jj 固有の操作である bookmark-auto-advance を
	// サブコマンドにするのも違う」). PR-5.2 originally exited 2 on git;
	// PR-5.2.1 removes that reject (here and in gitBackend.Push).
	// PR-5.1: forward the underlying tool's success-path diagnostic
	// ("Everything up-to-date" / "Nothing changed" / bookmark moves) by
	// handing the backend the dispatcher's own stdout/stderr. Quiet
	// rules:
	//   - default          : show both stdout + stderr from git/jj
	//   - -q  (quiet)      : suppress informational diagnostic (the
	//                        success-path output is informational, not
	//                        error-class; treating it as a hint matches
	//                        the rest of the bump-semver --quiet contract)
	//   - -qq (quiet-all)  : suppress everything (existing contract)
	//
	// kawaz (PR-5.1) confirmed: no editorial hint on non-ff. On error
	// paths the backend skips the passthrough writers and emitVcsErr
	// surfaces the wrapped error via formatPushError (which already
	// folds git/jj's stderr into ee.msg).
	opts := pushOpts{
		name:                  *args.vcsPush.Name,
		remote:                remote,
		jjBookmarkAutoAdvance: args.vcsPush.JjBookmarkAutoAdvance,
	}
	if !args.output.Verbosity.ShouldSuppressStdout() {
		opts.stdout = stdout
		opts.stderr = stderr
	}
	if err := b.Push(opts); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	return nil
}

// runVcsCmdTag dispatches `vcs tag push` / `vcs tag delete` (DR-0020 PR-6).
//
// `vcs tag` is the first two-tier verb in the family. The parser captures
// the sub-verb in args.vcsTag.SubVerb; we route here on it and emit a
// uniform exit-2 error for unknown / missing sub-verbs (mirroring the
// top-level "unknown verb" handling).
//
// Exit codes (DR-0020):
//
//   - 0  success (incl. idempotent same-rev push, absent-tag delete)
//   - 2  usage error (sub-verb missing/unknown, NAME missing, bad shape)
//   - 3  VCS subprocess error (unknown remote, bad REV, network failure)
//   - 4  integrity violation: tag exists at a different REV without
//     --allow-move (distinct from 3 so callers can detect "your tag
//     has drifted" vs "git/jj broke")
func runVcsCmdTag(args cliArgs, stdout, stderr io.Writer) error {
	switch args.vcsTag.SubVerb {
	case "push":
		return runVcsCmdTagPush(args, stdout, stderr)
	case "delete":
		return runVcsCmdTagDelete(args, stdout, stderr)
	case "latest":
		// DR-0032: `vcs tag latest` was moved to `vcs get latest-tag`
		// (= source 軸を verb 名に畳む再整理)。v0 break policy で alias
		// は残さない、明示的な migration hint だけ返す。
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag latest was moved in v0.32.0; use `vcs get latest-tag` (or `vcs get latest-release` for GitHub Releases)"))
	default:
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs tag sub-verb: %q (expected: push / delete)", args.vcsTag.SubVerb))
	}
}

// validTagName screens for bad NAME values before they reach the backend.
// We catch:
//   - empty string ("")
//   - whitespace anywhere in the name (spaces, tabs, newlines — none of
//     these are valid ref-name characters and silently passing them to
//     git would create a confusingly-quoted ref)
//   - "refs/" prefix (a common copy-paste mistake — the user typed
//     "refs/tags/v1" thinking we want the full ref, but we prefix
//     "refs/tags/" ourselves and would create refs/tags/refs/tags/...)
//
// More aggressive checks (e.g. all of git's
// `check-ref-format --refname-component` rules) would be over-engineering
// for the cases users actually hit; git/jj will surface deeper issues
// with their own error messages.
func validTagName(name string) error {
	if name == "" {
		return fmt.Errorf("NAME must not be empty")
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return fmt.Errorf("NAME %q contains whitespace (not a valid ref name)", name)
	}
	if strings.HasPrefix(name, "refs/") {
		return fmt.Errorf("NAME %q must not start with refs/ (the tag-ref prefix is added automatically)", name)
	}
	// 引数インジェクション対策 (C-1): a leading-`-` NAME would be parsed by
	// `git tag [-f] <name>` / `git tag -d <name>` as a flag (e.g. NAME=`-d`
	// turns a create into a delete). Reject it explicitly.
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("NAME %q must not start with '-' (would be parsed as a git/jj option)", name)
	}
	return nil
}

// runVcsCmdTagPush implements `vcs tag push --rev REV NAME
// [--remote REMOTE] [--allow-move]` (DR-0020 PR-6).
//
// Grammar requirements:
//   - NAME is the sole positional, required. No auto-derivation from
//     existing tags / latest version — explicit is safer than guessed.
//   - --rev is required (no implicit "tag HEAD" — same explicit-only
//     stance as `vcs push`'s --branch).
//   - REMOTE defaults to "origin" when --remote is omitted.
//   - --allow-move opts into moving an existing tag (DR-0020 line 71).
//     Without it, an existing tag at a different REV is exit 4.
//   - --force is intentionally not provided (use --allow-move instead).
//     The parser doesn't capture --force as a special case — it falls
//     through to the unknown-flag catch-all (exit 2 with hint pointing
//     to --allow-move).
func runVcsCmdTagPush(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag push: NAME is required (usage: vcs tag push --rev REV NAME)"))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag push: takes exactly one NAME, got %d", len(args.vcsArgs)))
	}
	if args.vcsTag.Rev == nil {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag push: --rev REV is required"))
	}
	if *args.vcsTag.Rev == "" {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag push: --rev value must not be empty"))
	}
	// 引数インジェクション対策 (C-1): leading-`-` rev would reach
	// `git tag <name> <rev>` resolution as a flag.
	if err := validateUserRev(*args.vcsTag.Rev); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag push: %w", err))
	}
	name := args.vcsArgs[0]
	if err := validTagName(name); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag push: %w", err))
	}
	remote := "origin"
	if args.vcsPush.Remote != nil {
		remote = *args.vcsPush.Remote
	}
	if err := validateRemote(remote); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag push: %w", err))
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	opts := tagPushOpts{
		Name:      name,
		Rev:       *args.vcsTag.Rev,
		Remote:    remote,
		AllowMove: args.vcsTag.AllowMove,
	}
	if !args.output.Verbosity.ShouldSuppressStdout() {
		opts.Stdout = stdout
		opts.Stderr = stderr
	}
	if err := b.TagPush(opts); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	return nil
}

// runVcsCmdTagDelete implements `vcs tag delete NAME [--remote REMOTE]`
// (DR-0020 PR-6).
//
// Grammar requirements:
//   - NAME is the sole positional, required (no auto-detection: even
//     though "delete all" would be technically definable, it's the kind
//     of bulk destructive intent the verb design rejects per DR line 91).
//   - REMOTE defaults to "origin".
//
// Delete is natively idempotent per DR line 74 (rm -f semantic) — an
// absent tag is exit 0 with no error, because the verb's intent is the
// end-state "no tag at NAME" which an absent tag already satisfies.
func runVcsCmdTagDelete(args cliArgs, stdout, stderr io.Writer) error {
	if len(args.vcsArgs) == 0 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag delete: NAME is required (usage: vcs tag delete NAME)"))
	}
	if len(args.vcsArgs) > 1 {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs tag delete: takes exactly one NAME, got %d", len(args.vcsArgs)))
	}
	name := args.vcsArgs[0]
	if err := validTagName(name); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag delete: %w", err))
	}
	remote := "origin"
	if args.vcsPush.Remote != nil {
		remote = *args.vcsPush.Remote
	}
	if err := validateRemote(remote); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag delete: %w", err))
	}
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	opts := tagDeleteOpts{Name: name, Remote: remote}
	if !args.output.Verbosity.ShouldSuppressStdout() {
		opts.Stdout = stdout
		opts.Stderr = stderr
	}
	if err := b.TagDelete(opts); err != nil {
		return emitVcsErr(stderr, args, err)
	}
	return nil
}

// emitVcsUsage prints a "bump-semver: <msg>" line and returns an
// exitErr with exitCodeUsage. Separate from emitErr because the
// existing emitErr hardcodes exit code 2 (kept as exitCodeUsage), but
// future vcs errors need a different code path (exitCodeAmbiguous /
// exitCodeVCSExec etc.) and we want a focused helper for those.
func emitVcsUsage(stderr io.Writer, args cliArgs, err error) error {
	if !args.output.Verbosity.ShouldSuppressError() {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	}
	return &exitErr{code: exitCodeUsage, msg: err.Error()}
}

// emitVcsErr surfaces an error from a vcs verb. When the error already
// carries an exit code (= an *exitErr produced by the backend layer),
// we preserve it. Anything else is treated as a VCS-exec failure
// (exit 3), so a stray non-coded error doesn't silently downgrade into
// the generic exit 2.
func emitVcsErr(stderr io.Writer, args cliArgs, err error) error {
	if !args.output.Verbosity.ShouldSuppressError() {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee
	}
	return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
}
