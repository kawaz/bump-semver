package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type jjBackend struct{}

func (j *jjBackend) Kind() string { return "jj" }

// CommitID resolves rev to its 40-char commit SHA via
// `jj log -r REV -T commit_id`, with the DR-0031 translateRev applied
// so git-style `remote/bookmark` works too.
//
// Default rev when empty: heads((::@-) & (~empty() | merges())) — the
// nearest fixed ancestor of the mutable working copy @, matching git's
// HEAD semantics. Plain `@-` isn't enough: it can itself be an empty
// commit (e.g. a bare `jj new`), so we walk back to the nearest
// non-empty change. Empty merges are rescued via `merges()` because
// dropping them (via `~empty()` alone) leaves >1 head in the ancestor
// set with no single answer.
func (j *jjBackend) CommitID(rev string) (string, error) {
	if rev == "" {
		rev = "heads((::@-) & (~empty() | merges()))"
	}
	return resolveJjRev(rev)
}

// Root returns the absolute path to the jj working copy via `jj root`.
func (j *jjBackend) Root() (string, error) {
	out, err := runBackendCmd("jj", "root")
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the bookmark that uniquely names the current
// commit's nearest ancestor head (DR-0020):
//
//	jj log -r 'heads(::@ & bookmarks())' --no-graph \
//	  -T 'bookmarks.map(|b| b.name()).join("\n") ++ "\n"'
//
// The template gives one bookmark name per line per head. Behaviour:
//
//   - 1 unique name on 1 head → success
//   - 0 lines (no bookmark in ancestors)         → exitCodeAmbiguous
//   - >1 lines (multiple bookmarks at the head)  → exitCodeAmbiguous
//   - >1 heads (parallel branches in ancestors)  → exitCodeAmbiguous
//
// We deliberately collapse "multiple heads" into the same exit code as
// the other ambiguity cases: the contract is "single name or error",
// and the caller doesn't need to disambiguate the failure mode.
func (j *jjBackend) CurrentBranch() (string, error) {
	out, err := runBackendCmd("jj", "log", "-r", "heads(::@ & bookmarks())",
		"--no-graph", "-T", `bookmarks.map(|b| b.name()).join("\n") ++ "\n"`)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	names := make([]string, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		names = append(names, s)
	}
	switch len(names) {
	case 0:
		return "", &exitErr{
			code: exitCodeAmbiguous,
			msg:  "current-branch: no bookmark found in ancestors of @",
		}
	case 1:
		return names[0], nil
	default:
		return "", &exitErr{
			code: exitCodeAmbiguous,
			msg:  fmt.Sprintf("current-branch: ambiguous (multiple bookmarks at head: %s)", strings.Join(names, ", ")),
		}
	}
}

// FetchFile returns `file` at `rev` via `jj file show`. `rev` is
// translated up-front so git-style remote refs (`origin/main`) reach
// jj as `main@origin` — see translateRev / DR-0031.
func (j *jjBackend) FetchFile(rev, file string) ([]byte, error) {
	rev = translateRev(rev, vcsJj)
	return runBackendCmd("jj", "file", "show", "-r", rev, file)
}

// ListTags returns every tag known to the local jj repo. The template
// emits one tag name per line per change with tags; the dedup pass
// collapses duplicates from changes that share a tag.
//
// We do not run `jj git fetch` here — DR-0008 makes "no implicit
// network calls" an explicit decision.
func (j *jjBackend) ListTags() ([]string, error) {
	out, err := runBackendCmd("jj", "log", "-r", "tags()", "--no-graph",
		"-T", `tags.map(|t| t.name() ++ "\n").join("")`)
	if err != nil {
		return nil, err
	}
	return splitAndDedup(string(out)), nil
}

// LatestTag picks the SemVer-largest tag from ListTags.
func (j *jjBackend) LatestTag(includePrerelease bool) (string, Version, error) {
	tags, err := j.ListTags()
	if err != nil {
		return "", Version{}, err
	}
	return pickLatestSemverTag(tags, includePrerelease)
}

// Diff returns the patch between `rev` and `@` (jj's working copy). Same
// declarative-convergence path filter as the git backend — see the
// gitBackend.Diff comment for the contract.
func (j *jjBackend) Diff(rev string, paths []string, excludes []string) ([]byte, error) {
	rev = translateRev(rev, vcsJj)
	args := []string{"diff", "--from", rev, "--to", "@"}
	if len(paths) > 0 || len(excludes) > 0 {
		pathspec := buildJjPathspec(paths, excludes)
		if pathspec == nil {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err := runBackendCmd("jj", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return out, nil
}

// DiffNameStatus on jj runs `jj diff --summary --from REV --to @
// [-- PATHS]` and normalizes the native space separator to a tab so the
// output matches git's `--name-status` shape exactly.
//
// jj summary format: `<CODE> <path>` (single space). We split on the FIRST
// space only — paths with embedded spaces stay intact in the right half.
// Lines that don't match the `<CODE> <path>` shape are passed through
// unchanged (defensive: jj could introduce new prefix forms; we don't want
// to silently mangle them).
//
// Rename / copy codes (R/C) are best-effort: jj and git may differ in how
// they render them, but M/A/D — the cases that matter for the kawaz
// "version bumped?" check — are identical.
func (j *jjBackend) DiffNameStatus(rev string, paths []string, excludes []string) ([]byte, error) {
	rev = translateRev(rev, vcsJj)
	args := []string{"diff", "--summary", "--from", rev, "--to", "@"}
	if len(paths) > 0 || len(excludes) > 0 {
		pathspec := buildJjPathspec(paths, excludes)
		if pathspec == nil {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err := runBackendCmd("jj", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return normalizeJjNameStatus(out), nil
}

// buildJjPathspec converts (paths, excludes) into a single jj fileset
// expression argv (or empty for "no pathspec"). jj fileset language:
//
//   - `path` / `glob:pat` — atom
//   - `x | y` — union
//   - `x ~ y` — difference
//
// When excludes are empty, we keep the old behavior of passing each include
// as a separate pathspec arg (= jj unions them implicitly). This preserves
// compatibility with paths-with-spaces and avoids quoting headaches in the
// common case.
//
// When excludes are non-empty, we MUST combine into a single fileset
// expression because separate pathspec args are unioned (= negation as a
// separate arg has no exclude effect, empirically verified on jj 0.41).
//
// Return value semantics: same as buildGitPathspec.
func buildJjPathspec(paths, excludes []string) []string {
	var includes []string
	if len(paths) > 0 {
		includes = filterExistingPaths(paths)
		if len(includes) == 0 {
			return nil
		}
	}
	if len(excludes) == 0 {
		return includes
	}
	// Build a single fileset expression: (inc1 | inc2 | ...) ~ exc1 ~ exc2.
	// Atoms: literal path or `glob:pat` (already in jj's vocabulary).
	if len(includes) == 0 {
		// 防御: dispatcher が positional 無しの --excludes を弾くはずだが、
		// ここに来た場合は filter結果が空 = backend に投げない (nil)。
		return nil
	}
	var sb strings.Builder
	sb.WriteByte('(')
	for i, p := range includes {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(p)
	}
	sb.WriteByte(')')
	for _, e := range excludes {
		sb.WriteString(" ~ ")
		sb.WriteString(e)
	}
	return []string{sb.String()}
}

// normalizeJjNameStatus converts jj's `<CODE> <path>\n` lines into git's
// `<CODE>\t<path>\n` form. The first space on each line becomes a tab; the
// rest of the line is left untouched so paths-with-spaces survive intact.
// Trailing newlines are preserved.
func normalizeJjNameStatus(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	lines := strings.Split(string(in), "\n")
	for i, line := range lines {
		// SplitN with n=2 takes only the first space — paths-with-spaces stay whole.
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && len(parts[0]) > 0 {
			lines[i] = parts[0] + "\t" + parts[1]
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// IsClean returns true when the working-copy change `@` is empty.
//
// jj's `empty` template keyword renders the literal string "true" or
// "false" — no diff text parsing needed. Reading `@` is also what
// triggers jj's automatic snapshot, so this implicitly reflects any
// just-edited (or just-created) files in the worktree.
//
// The `empty` keyword is parent-relative: it returns true when @'s
// tree equals the merge of @-'s parents. For a single-parent commit
// that's "no diff vs @-"; for a merge commit (parents>1) it's "tree
// matches the merge of parents" (= empty merge → clean, evil merge
// with extra tree edits → dirty). This is the correct semantics:
// merge commits with content additions ("evil merges") DO carry
// uncommitted intent and should read as dirty.
//
// Contrast with git: jj treats new files as worktree state by design,
// so an untracked-new-file makes `IsClean` return false (intentional
// asymmetry, documented in DR-0020 PR-2).
func (j *jjBackend) IsClean() (bool, error) {
	out, err := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "empty")
	if err != nil {
		return false, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	switch strings.TrimSpace(string(out)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj log -r @ -T empty: unexpected output %q", strings.TrimSpace(string(out))),
		}
	}
}

// validateNonexistentPaths errors when a path is neither present on the
// filesystem nor tracked at @-. Used in the default (!allowNonexistentPath)
// Commit path mode to surface typos that jj's diff-summary gate would
// otherwise silently swallow as a "no diff → no-op" success.
//
// DR-0037 follow-up: git's `git add -A -- PATHS` naturally errors on truly
// unknown paths, but jj's `jj diff --summary -- PATHS` returns empty for
// both "tracked but no change" and "unknown / untracked". Without this
// pre-check the jj backend would silently accept typos (Codex stop-time
// review caught this asymmetry between the two backends).
//
// "Tracked at @-" is the right reference (not @): a path the user just
// deleted is no longer tracked at @ after snapshot, but is still tracked
// at @- — exactly the deletion-in-progress case this DR exists to support.
func (j *jjBackend) validateNonexistentPaths(paths []string) error {
	var missing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			continue // exists on filesystem (jj snapshot will pick it up)
		}
		out, err := runBackendCmd("jj", "file", "list", "-r", "@-", "--", p)
		if err != nil || strings.TrimSpace(string(out)) == "" {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return &exitErr{code: exitCodeVCSExec,
			msg: fmt.Sprintf("jj: pathspec(s) did not match any tracked or filesystem files: %s (use --allow-nonexistent-path to silently drop)",
				strings.Join(missing, ", "))}
	}
	return nil
}

// --- Commit implementations (DR-0020 PR-4) --------------------------------

// Commit (jj) records the requested change set. See the interface comment
// on `Commit` for the full contract. Implementation notes:
//
//   - paths: `jj commit [FILESETS]... -m MSG` puts only those paths' @
//     changes into a new commit (the rest stays in the new working copy).
//     Pre-gated by `jj diff --summary --from @- --to @ -- PATHS` so an
//     all-nonexistent or no-change set is a no-op (DR-0020 explicitly
//     wants no empty commits — jj would otherwise happily create one).
//   - staged: `jj commit -m MSG` (no paths) commits the entire @ snapshot.
//     Pre-gated by the `empty` template (same predicate as IsClean).
//   - amend (PR-4.1): symmetric with non-amend. With paths,
//     `jj squash --from @ --into @- [-m MSG | -u] -- PATHS` folds only
//     those paths from @ into @-, leaving the rest in @. With staged or
//     bare, drops the `-- PATHS` tail and folds all of @. The no-edit
//     path uses `--use-destination-message` rather than the squash
//     default (which would prompt for a combined description when @ and
//     @- both carry descriptions — observed on jj 0.41 in non-
//     interactive callers and confirmed as the cause of editor-spawn
//     hangs).
func (j *jjBackend) Commit(opts commitOpts) error {
	if opts.amend {
		return j.commitAmend(opts)
	}
	if opts.staged {
		// Gate: is @ empty?
		out, err := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "empty")
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if strings.TrimSpace(string(out)) == "true" {
			return nil // empty @ → no-op success
		}
		if _, err := runBackendCmd("jj", "commit", "-m", opts.message); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// paths mode.
	// --allow-nonexistent-path: legacy declarative-convergence — filter to
	// filesystem-visible paths first (deleted tracked files are dropped).
	// Default (no flag): forward all paths as-is; validateNonexistentPaths
	// pre-checks that each path is either present on the filesystem or
	// tracked at @-, so jj's diff-summary gate (which can't distinguish
	// "no change" from "unknown path") doesn't silently swallow typos.
	paths := opts.paths
	if opts.allowNonexistentPath {
		paths = filterExistingPaths(opts.paths)
		if len(paths) == 0 {
			return nil // all-nonexistent → no-op success (legacy behaviour)
		}
	} else if err := j.validateNonexistentPaths(opts.paths); err != nil {
		return err
	}
	// Gate via `jj diff --summary` over the same paths: if it produces no
	// output, there is nothing to commit even after path filtering.
	gateArgs := append([]string{"diff", "--summary", "--from", "@-", "--to", "@", "--"}, paths...)
	gateOut, err := runBackendCmd("jj", gateArgs...)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if strings.TrimSpace(string(gateOut)) == "" {
		return nil
	}
	commitArgs := append([]string{"commit", "-m", opts.message}, paths...)
	if _, err := runBackendCmd("jj", commitArgs...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// commitAmend handles jj's amend mode by squashing @ into @-. PR-4.1
// added path / staged symmetry:
//
//   - bare amend (no paths, no staged): explicit rewrite, ungated.
//     `jj squash --from @ --into @- [-m MSG | -u]`. Safe on empty @
//     (message-only amend = description update on @-).
//   - amend + staged: same as bare amend — jj has no separate staging
//     area, the entire @ snapshot IS the absorption source. Accepted
//     as an explicit synonym.
//   - amend + paths: gate via `jj diff --summary --from @- --to @ --
//     PATHS` (same predicate as non-amend path mode), then `jj squash
//     --from @ --into @- [-m MSG | -u] -- PATHS` folds only those
//     paths.
//
// Design rationale (no-edit ⇒ --use-destination-message): when @ has a
// description and ends up empty after squash, bare `jj squash` writes a
// combined description and opens an editor for confirmation. In non-
// interactive callers (bump-semver scripted use) this surfaces as
// "Failed to edit description / Editor 'false' exited with exit
// status: 1" (verified on jj 0.41). `-u` keeps @-'s description
// verbatim — exactly the no-edit semantic — and removes the prompt
// path entirely.
func (j *jjBackend) commitAmend(opts commitOpts) error {
	// Path-scoped amend: gate first so all-nonexistent / no-change is
	// a no-op (declarative convergence, mirrors non-amend path mode).
	if len(opts.paths) > 0 {
		// Apply the same allowNonexistentPath logic as non-amend path mode.
		paths := opts.paths
		if opts.allowNonexistentPath {
			paths = filterExistingPaths(opts.paths)
			if len(paths) == 0 {
				return nil // all-nonexistent → no-op success (legacy behaviour)
			}
		} else if err := j.validateNonexistentPaths(opts.paths); err != nil {
			return err
		}
		gateArgs := append([]string{"diff", "--summary", "--from", "@-", "--to", "@", "--"}, paths...)
		gateOut, err := runBackendCmd("jj", gateArgs...)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if strings.TrimSpace(string(gateOut)) == "" {
			return nil
		}
		args := []string{"squash", "--from", "@", "--into", "@-"}
		if opts.noEdit || opts.message == "" {
			args = append(args, "--use-destination-message")
		} else {
			args = append(args, "-m", opts.message)
		}
		args = append(args, "--")
		args = append(args, paths...)
		if _, err := runBackendCmd("jj", args...); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// Bare amend (and amend + staged, which is the same operation in
	// jj's auto-staged model).
	args := []string{"squash", "--from", "@", "--into", "@-"}
	if opts.noEdit || opts.message == "" {
		args = append(args, "--use-destination-message")
	} else {
		args = append(args, "-m", opts.message)
	}
	if _, err := runBackendCmd("jj", args...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// --- Fetch / Push implementations (DR-0020 PR-5) --------------------------

// Fetch (jj) refreshes refs from the named remote via `jj git fetch
// --remote <remote>`. Same wrapping as the git variant.
func (j *jjBackend) Fetch(remote string) error {
	if _, err := runBackendCmd("jj", "git", "fetch", "--remote", remote); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// jjGitExportFunc is the seam tests use to inject deterministic `jj git
// export` outcomes (PR-5.1). Real callers get the default implementation,
// which shells out to `jj git export`. Tests override it to exercise the
// retry-once + recovery-hint paths without needing a fixture that can
// produce a transient-then-clearing failure on demand.
var jjGitExportFunc = func() (stderr string, code int, err error) {
	_, stderrOut, exitCode, runErr := runBackendCapture("jj", "git", "export")
	return stderrOut, exitCode, runErr
}

// Push (jj) uploads opts.name to opts.remote via `jj git push --bookmark
// <name> --remote <remote>`. After a successful push we run `jj git
// export` and propagate its exit code — this keeps the colocated `.git`
// refs in sync and surfaces edge cases (ref-hierarchy conflicts, HEAD
// races) the DR explicitly asks us NOT to swallow.
//
// `--allow-new` is intentionally omitted: jj 0.41 deprecated it in favour
// of remote auto-track configuration, and new bookmarks push fine without
// it in our default config. Future jj versions may flip the default; if
// new-bookmark push starts erroring on a supported version, the fix is
// to switch to `--allow-new` (kept simple here — see DR-0020 PR-5 notes).
//
// Non-ff detection: jj's rejection markers ("stale info", "Failed to push
// some bookmarks") are matched in isNonFastForward. Anything else on
// non-zero exit is a generic VCS error (exit 3).
func (j *jjBackend) Push(opts pushOpts) error {
	if opts.jjBookmarkAutoAdvance {
		if err := j.autoAdvanceBookmark(opts.name); err != nil {
			return err
		}
	}
	stdout, stderr, code, err := runBackendCapture("jj", "git", "push",
		"--bookmark", opts.name, "--remote", opts.remote)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if code == 0 {
		// PR-5.1: forward jj's success-path diagnostic (e.g. "Nothing
		// changed" / "Changes to push to <remote>" / bookmark moves) so
		// the user sees what jj actually said. Error paths skip
		// passthrough — emitVcsErr already folds stderr into ee.msg.
		writePushDiagnostic(opts.stdout, stdout)
		writePushDiagnostic(opts.stderr, stderr)
	}
	if code != 0 {
		if isNonFastForward(stderr) {
			return &exitErr{
				code: exitCodeNonFastForward,
				msg:  formatPushError("jj", stderr, stdout),
			}
		}
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  formatPushError("jj", stderr, stdout),
		}
	}
	// Push succeeded; sync colocated git refs via `jj git export`.
	// PR-5.1: retry once on failure (the common cases — transient
	// packed-refs lock, HEAD races — are cleared on the second attempt),
	// then escalate to exit 3 with a recovery hint pointing at jj's
	// upstream issues. We don't swallow the underlying jj stderr —
	// kawaz's directive is "公式は無視するなで終わりってことないよね"
	// = give the user an actionable next step instead of a bare wrap.
	exStderr1, exCode1, exErr1 := jjGitExportFunc()
	if exErr1 == nil && exCode1 == 0 {
		return nil
	}
	// First attempt failed; try once more (covers the
	// transient-lock-clears class).
	exStderr2, exCode2, exErr2 := jjGitExportFunc()
	if exErr2 == nil && exCode2 == 0 {
		return nil
	}
	// Both attempts failed — pick the most informative stderr (prefer
	// the second attempt's, which reflects the persistent state) and
	// build a recovery hint.
	finalStderr := strings.TrimSpace(exStderr2)
	if finalStderr == "" {
		finalStderr = strings.TrimSpace(exStderr1)
	}
	if exErr2 != nil {
		finalStderr = strings.TrimSpace(finalStderr + "\n" + exErr2.Error())
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  jjGitExportRecoveryMessage(finalStderr),
	}
}

// autoAdvanceBookmark implements the DR-0020 PR-5.2 pre-step for
// `vcs push --jj-bookmark-auto-advance`. The target the bookmark gets
// advanced to is **conditioned on IsClean**:
//
//   - clean (@ empty)     → advance to @-  (kawaz 常用 = jj 慣習: bookmark
//     lives on the confirmed parent, @ is the
//     throw-away working copy)
//   - dirty (@ non-empty) → advance to @   (treat the current commit as
//     the publishable one — the "immutable 化"
//     pattern: after push, the named commit
//     becomes immutable and jj auto-creates a new
//     working copy above it; described or empty
//     description both legal, the user opted in)
//
// Note on the IsClean branching (kawaz 確定 2026-05-31): an earlier
// draft refused the dirty case (exit 3 + "requires clean"). kawaz then
// noted both clean and dirty are legitimate workflows ("clean 前提" and
// "dirty + describe して push" の両運用) and the flag should cover both;
// users wanting strict clean-only can gate with `vcs is clean` themselves
// (= ツール側で禁止しない、最小ガード方針).
//
// DR-0026 (2026-06-02): delegate the move itself to jj's official
// `jj bookmark advance` (jj 0.39+). It handles existence (silent skip
// when the bookmark is absent: "No matching bookmarks ... No bookmarks
// to update."), ancestor / forward-only enforcement ("Refusing to advance
// bookmark backwards or sideways"), and "already at target" (silent
// no-op) in one primitive. We keep only the bits that are bump-semver-
// shaped:
//
//  1. clean/dirty target selection (jj has no policy for this — it's our
//     UX choice).
//  2. **At-@ short-circuit in the clean branch**. If the bookmark already
//     sits at @, the clean target (@-) is strictly backwards, and
//     delegated `advance` would reject. Old PR-5.2 chain skipped this
//     case intentionally ("don't go backwards, just push as-is") — DR-0026
//     preserves that behaviour with a single revset probe.
//  3. **DR-0025 description check on the target**, before delegation.
//     `jj bookmark advance` does NOT check description, so without this
//     guard the bookmark advances onto an undescribed commit and the
//     subsequent push rejects with an opaque error → retry loop. The
//     check uses jj's own template engine so its truthiness exactly
//     matches jj's push gate (whitespace-only descriptions accepted).
//
// Errors from `jj bookmark advance` are wrapped with a bump-semver
// prefix; runBackendCmd already folds jj's stderr into the error, so the
// jj-native message ("Refusing to advance bookmark backwards or
// sideways: NAME" / etc.) is preserved verbatim and the prefix marks
// which feature triggered it.
//
// Exit code 3 (exitCodeVCSExec) is the established taxonomy slot for
// "VCS-layer precondition not met" (same as "unknown remote" / "not a
// repo"). Exit 1 (exitCodeFalse) is reserved for predicate verbs
// (`compare` / `vcs is`) and would mis-classify a refusal as a query
// result. Exit 4 (exitCodeAmbiguous) is for "no single answer" queries
// (`current-branch` with multiple bookmarks), not a precondition fail.
func (j *jjBackend) autoAdvanceBookmark(name string) error {
	// 1. pick target by clean/dirty.
	clean, err := j.IsClean()
	if err != nil {
		return err
	}
	target := "@-"
	if !clean {
		target = "@"
	}
	// 2. clean-mode at-@ short-circuit (DR-0026). When the bookmark sits
	// at @ on a clean working copy, advancing to @- would be a strict
	// backwards move; jj's forward-only `advance` would refuse with
	// "Refusing to advance bookmark backwards or sideways". The old
	// hand-rolled chain treated this as a no-op (kawaz original spec:
	// "<name> が @ 自身を指す → 通常 push (auto-advance 不要)"), and we
	// preserve that contract here. The dirty branch (target=@) doesn't
	// need this guard — bookmark already at @ is handled by jj's own
	// "No bookmarks to update" silent no-op.
	if clean {
		atWcOut, err := runBackendCmd("jj", "log", "-r", "present("+name+") & @", "--no-graph", "-T", `change_id ++ "\n"`)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if strings.TrimSpace(string(atWcOut)) != "" {
			return nil // bookmark at @ on clean — don't drag it backwards, push as-is
		}
	}
	// 3. DR-0025: refuse to advance onto an undescribed target — jj would
	// otherwise reject the subsequent push with no actionable hint. The
	// empty-check is delegated to jj's template engine so the semantics
	// match jj's push gate exactly (whitespace-only is accepted). Applies
	// to both clean (target=@-) and dirty (target=@) paths.
	descOut, err := runBackendCmd("jj", "log", "-r", target, "--no-graph", "-T", `if(description, "T", "F")`)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if strings.TrimSpace(string(descOut)) != "T" {
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("vcs push --jj-bookmark-auto-advance: advance target %s for bookmark %q has no description; jj would refuse to push it. Run `jj describe -r %s` to set a description, then retry (or move bookmark %q manually if %s should not be the target)", target, name, target, name, target),
		}
	}
	// 4. Delegate to `jj bookmark advance` (jj 0.39+). This collapses
	// the old chain (existence / ancestor / at-target / forward-only
	// move) into a single primitive that already handles:
	//   - bookmark absent → "No matching bookmarks ... No bookmarks to
	//     update." exit 0 → return nil (the normal push will surface
	//     the bookmark-missing error from jj's push lane, same as
	//     PR-5 without the flag)
	//   - already at target → "No bookmarks to update." exit 0
	//   - sideways/divergent or backwards → exit 1 with "Refusing to
	//     advance bookmark backwards or sideways" — we wrap with the
	//     bump-semver prefix so the user can identify the flag
	//   - normal forward → bookmark advanced
	if _, err := runBackendCmd("jj", "bookmark", "advance", name, "--to", target); err != nil {
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("vcs push --jj-bookmark-auto-advance: %s", err.Error()),
		}
	}
	return nil
}

// jjGitExportRecoveryMessage builds an actionable error message for a
// persistent `jj git export` failure (PR-5.1). The message folds in the
// raw jj stderr (no paraphrase) and appends pattern-specific remedies
// derived from jj-vcs/jj upstream issues. Unmatched stderr falls back to
// a generic "raw stderr + upstream issue list" body so the user always
// gets a starting point, never a bare wrap.
func jjGitExportRecoveryMessage(jjStderr string) string {
	var hint string
	switch {
	// Ref-hierarchy clash (jj-vcs/jj #493): git's filesystem refs can't
	// hold both `refs/heads/foo` and `refs/heads/foo/bar`.
	case strings.Contains(jjStderr, "there are refs beneath that folder"),
		strings.Contains(jjStderr, "cannot lock ref"):
		hint = "ref-hierarchy clash (jj-vcs/jj #493): " +
			"inspect with 'git for-each-ref refs/heads/', then rename or delete " +
			"the conflicting refs and retry."
	// packed-refs lock not released (jj-vcs/jj #6203).
	case strings.Contains(jjStderr, "packed-refs"):
		hint = "packed-refs lock contention (jj-vcs/jj #6203): " +
			"ensure no other git/jj process is running, remove " +
			"'.git/packed-refs.lock' if stale, then retry."
	// HEAD reference race (jj-vcs/jj #6098).
	case strings.Contains(jjStderr, `reference "HEAD"`),
		strings.Contains(jjStderr, "HEAD\" should have content"):
		hint = "HEAD reference race (jj-vcs/jj #6098): " +
			"run 'jj git import' to resync the working copy with the underlying " +
			"git store, then retry."
	default:
		hint = "see https://github.com/jj-vcs/jj/issues " +
			"(known patterns: #493 ref-hierarchy, #6098 HEAD race, #6203 packed-refs)."
	}
	return fmt.Sprintf("jj git export failed twice after push: %s\nrecovery: %s", jjStderr, hint)
}

// jjGitPushDir returns the directory to pass to `git -C` for the
// tag-push step in jj backend operations.
//
// Two layouts are supported (DR-0020 line 105):
//   - colocated:  `.git` is a real directory inside cwd. Push from cwd
//     itself — pushing from inside `.git/` would lose the worktree
//     context that pre-push hooks expect.
//   - non-colocated: ask `jj git root` for the backing git directory.
//     Bare repos push fine without a worktree, so `git -C <bare>` is
//     correct here.
//
// `jj git root` is the right interface because it transparently handles
// secondary jj workspaces (where `.jj/repo` is a regular file pointing
// to the primary workspace's repo store, which reading
// `.jj/repo/store/git_target` directly cannot follow).
//
// Errors wrap as *exitErr{exitCodeVCSExec}.
func jjGitPushDir() (string, error) {
	// Colocated check: a `.git` entry in cwd that is a directory wins
	// regardless of what jj says. Saves us from "jj git root resolved to
	// the same .git but the bare config doesn't reach our hooks" cases.
	if fi, err := os.Stat(".git"); err == nil && fi.IsDir() {
		// Empty dir-arg means "use cwd" downstream — avoids special-casing
		// the worktree/git-dir split.
		return "", nil
	}
	out, err := runBackendCmd("jj", "git", "root")
	if err != nil {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj git root: %v", err),
		}
	}
	target := strings.TrimSpace(string(out))
	if target == "" {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  "jj git root returned empty",
		}
	}
	return target, nil
}

// resolveJjRev returns the commit SHA `rev` resolves to in the cwd jj
// repo, or *exitErr{exitCodeVCSExec} on resolution failure.
//
// We use `jj log --no-graph -r REV -T commit_id` which (a) prints exactly
// one line per resolved change and (b) emits the canonical 40-char commit
// SHA — same format `git rev-parse` returns so cross-backend SHA
// comparisons stay trivial.
func resolveJjRev(rev string) (string, error) {
	rev = translateRev(rev, vcsJj)
	out, err := runBackendCmd("jj", "log", "--no-graph", "-r", rev, "-T", `commit_id ++ "\n"`)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	// jj prints one line per matched change. A multi-line result means
	// the revset matched more than one change — treat as ambiguous-like
	// VCS error (the caller wrote a revset that doesn't yield a single
	// commit; jj's own error message would be similar).
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj rev %q resolved to nothing", rev),
		}
	}
	if len(lines) > 1 {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj rev %q matched multiple changes", rev),
		}
	}
	return lines[0], nil
}

// existingJjTagSHA returns the commit SHA that NAME points at in jj, or
// "" when the tag is absent. Uses `jj tag list NAME -T` with the
// `self.normal_target().commit_id()` keyword (verified in PR-6 probing on
// jj 0.41). Multi-line output is treated as "not present" so the caller
// proceeds with the create path; a downstream `jj tag set` will surface
// any actual error.
func existingJjTagSHA(name string) string {
	out, err := runBackendCmd("jj", "tag", "list", name,
		"-T", `self.normal_target().commit_id() ++ "\n"`)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) != 1 {
		return ""
	}
	return lines[0]
}

// TagPush (jj): same logic as the git backend but routes the local create
// through `jj tag set`, runs `jj git export` to materialise the tag in
// the underlying git store, then issues a native `git -C <git_target>
// push` against the remote.
//
// Why native git push for the remote half: jj 0.41 has no native
// tag-push command (`jj git push --bookmark` only handles bookmarks,
// and the push-everything `jj git push` is too broad — DR-0020 requires
// per-tag intent). DR-0020 line 70 commits to "create via jj tag set,
// push via native git" so jj retains tag awareness while we get fine-
// grained remote control.
func (j *jjBackend) TagPush(opts tagPushOpts) error {
	targetSHA, err := resolveJjRev(opts.Rev)
	if err != nil {
		return err
	}
	existingSHA := existingJjTagSHA(opts.Name)
	gitTarget, gtErr := jjGitPushDir()
	if gtErr != nil {
		return gtErr
	}
	switch decideTagPush(existingSHA, targetSHA, opts.AllowMove) {
	case tagPushDecisionReject:
		return &exitErr{
			code: exitCodeAmbiguous,
			msg:  formatTagDiffRevError(opts.Name, existingSHA, targetSHA),
		}
	case tagPushDecisionCreate:
		if _, err := runBackendCmd("jj", "tag", "set", opts.Name, "-r", opts.Rev); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, false, gitTarget)
	case tagPushDecisionSkipCreate:
		// Local already has it at the same target — ensure git store has
		// it (export is a no-op if it's already there), then push.
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, false, gitTarget)
	case tagPushDecisionMove:
		if _, err := runBackendCmd("jj", "tag", "set", opts.Name,
			"-r", opts.Rev, "--allow-move"); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, true, gitTarget)
	default:
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("internal: unhandled tag push decision"),
		}
	}
}

// TagDelete (jj): `jj tag delete` is natively idempotent (PR-6 probing on
// jj 0.41 confirms missing-NAME yields "No matching tags" with exit 0),
// so we can run it unconditionally. Export so the git store loses the
// ref, then push the delete to the remote (also idempotent at the git
// layer).
func (j *jjBackend) TagDelete(opts tagDeleteOpts) error {
	if _, err := runBackendCmd("jj", "tag", "delete", opts.Name); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if err := jjGitExportOrWrap(); err != nil {
		return err
	}
	gitTarget, gtErr := jjGitPushDir()
	if gtErr != nil {
		return gtErr
	}
	return gitTagDeleteRemote(opts, gitTarget)
}

// jjGitExportOrWrap runs `jj git export` via the same seam Push uses, so
// the PR-5.1 retry-once + recovery-hint hardening is shared. The retry
// covers transient packed-refs locks and HEAD races that surfaced in
// PR-5 testing (jj-vcs/jj #493, #6098, #6203). PR-6 reuses the seam
// rather than introducing a parallel export path.
func jjGitExportOrWrap() error {
	exStderr1, exCode1, exErr1 := jjGitExportFunc()
	if exErr1 == nil && exCode1 == 0 {
		return nil
	}
	exStderr2, exCode2, exErr2 := jjGitExportFunc()
	if exErr2 == nil && exCode2 == 0 {
		return nil
	}
	finalStderr := strings.TrimSpace(exStderr2)
	if finalStderr == "" {
		finalStderr = strings.TrimSpace(exStderr1)
	}
	if exErr2 != nil {
		finalStderr = strings.TrimSpace(finalStderr + "\n" + exErr2.Error())
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  jjGitExportRecoveryMessage(finalStderr),
	}
}

// --- FileTimestamp / CountCommitsSince (DR-0027) -------------------------

// FileTimestamp (jj): the revset `latest(::@ & files("<path>"))` picks
// the most recent ancestor of @ that touches path; the committer-
// timestamp template gives us the unix epoch. Empty output = path
// untracked → 0.
func (j *jjBackend) FileTimestamp(path string) (int64, error) {
	revset := "latest(::@ & files(" + jjStringLiteral(path) + "))"
	out, err := runBackendCmd("jj", "log", "--no-graph",
		"-r", revset,
		"-T", `committer.timestamp().format("%s")`)
	if err != nil {
		return 0, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, nil
	}
	return parseEpochOrZero(s), nil
}

// CountCommitsSince (jj): revset for source-touching commits in ::@
// with committer_date strictly newer than sinceTS. jj's CLI doesn't
// expose a revset length primitive, so we count the lines emitted by
// a one-line-per-revision template.
//
// jj's `committer_date(after:"<ISO>")` accepts an ISO-8601 timestamp
// (not a unix-epoch literal); we format sinceTS+1 in UTC to match
// the strict-newer semantics the git branch uses with `--since=ts+1`.
//
// `sinceTS == 0` drops the date filter so we get the total source-
// touch count (matches the git untracked-derived branch).
func (j *jjBackend) CountCommitsSince(path string, sinceTS int64) (int, error) {
	revset := "::@ & files(" + jjStringLiteral(path) + ")"
	if sinceTS > 0 {
		iso := time.Unix(sinceTS+1, 0).UTC().Format("2006-01-02T15:04:05Z")
		revset = revset + fmt.Sprintf(" & committer_date(after:\"%s\")", iso)
	}
	out, err := runBackendCmd("jj", "log", "--no-graph",
		"-r", revset,
		"-T", `"X\n"`)
	if err != nil {
		return 0, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "X" {
			count++
		}
	}
	return count, nil
}

// IsWorktree reports whether the jj working copy is a secondary workspace
// (= one added via `jj workspace add`). The default workspace returns false.
//
// Detection relies on jj's on-disk layout: the default workspace's `.jj/repo`
// is a directory (holding the repo's real state), while a secondary workspace
// has `.jj/repo` as a *file* containing a relative path to the default's
// `.jj/repo`. Probing the file kind is O(1) and avoids parsing variable jj
// CLI output across versions.
func (j *jjBackend) IsWorktree() (bool, error) {
	root, err := j.Root()
	if err != nil {
		return false, err
	}
	info, statErr := os.Stat(filepath.Join(root, ".jj", "repo"))
	if statErr != nil {
		return false, &exitErr{code: exitCodeVCSExec, msg: statErr.Error()}
	}
	return !info.IsDir(), nil
}

// WorktreeName returns the current jj workspace name. Returns "" when the
// working copy is the default workspace (= IsWorktree() returns false).
//
// Design rationale: jj does not expose "name of current workspace" as a
// direct command. We rely on the convention (per jj-workflow.md) that
// `jj workspace add <name>` creates a directory named `<name>` and the
// workspace's name equals its dir basename. For kawaz's git bare + jj
// workspace layout this always holds.
func (j *jjBackend) WorktreeName() (string, error) {
	isWt, err := j.IsWorktree()
	if err != nil {
		return "", err
	}
	if !isWt {
		return "", nil
	}
	root, err := j.Root()
	if err != nil {
		return "", err
	}
	return filepath.Base(root), nil
}

// DefaultBranch resolves the default bookmark name via jj-native calls
// (= we do NOT shell out to git here — a jj secondary workspace's cwd
// does not have a `.git` view, so git invocations fail).
//
// Resolution order:
//  1. `jj log -r trunk()` — jj's standard revset alias for the canonical
//     default. Surfaces the bookmark name when defined.
//  2. Local bookmark probe: main → master → trunk. First existing wins.
func (j *jjBackend) DefaultBranch() (string, error) {
	if out, err := runBackendCmd("jj", "log", "-r", "trunk()", "--no-graph",
		"-T", `bookmarks.map(|b| b.name()).join("\n") ++ "\n"`); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.TrimSpace(line)
			if name == "main" || name == "master" || name == "trunk" {
				return name, nil
			}
		}
	}
	if out, err := runBackendCmd("jj", "bookmark", "list", "-T", `name ++ "\n"`); err == nil {
		present := make(map[string]bool)
		for _, line := range strings.Split(string(out), "\n") {
			present[strings.TrimSpace(line)] = true
		}
		for _, candidate := range []string{"main", "master", "trunk"} {
			if present[candidate] {
				return candidate, nil
			}
		}
	}
	return "", &exitErr{
		code: exitCodeVCSExec,
		msg:  "default-branch: cannot determine (no trunk(); no local main/master/trunk bookmark)",
	}
}

// DefaultBranchPath returns the absolute path to the workspace whose
// nearest bookmark to @ equals DefaultBranch(). See the interface
// comment on DefaultBranchPath for the full contract.
//
// Implementation: `jj workspace list -T '...'` emits one line per
// workspace with name + root + @-change_id. For each workspace, a second
// `jj log -r 'heads(::<change_id> & bookmarks())'` resolves its current
// branch (= same query CurrentBranch uses). Candidates whose current
// branch == DefaultBranch() are tie-broken via pickDefaultBranchPath.
//
// The N+1 query shape is acceptable because workspace counts are tiny
// (typically 2-5); a single revset union query would lose the workspace
// ↔ bookmark mapping needed for the tie-break.
func (j *jjBackend) DefaultBranchPath() (string, error) {
	def, err := j.DefaultBranch()
	if err != nil {
		return "", err
	}
	// Template fields: name, absolute root path, @-change_id (tab-separated).
	out, err := runBackendCmd("jj", "workspace", "list",
		"-T", `self.name() ++ "\t" ++ self.root() ++ "\t" ++ self.target().change_id() ++ "\n"`)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	var candidates []defaultBranchPathCandidate
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		name, root, changeID := fields[0], fields[1], fields[2]
		if name == "" || root == "" || changeID == "" {
			continue
		}
		bookmark, berr := jjNearestBookmark(changeID)
		if berr != nil {
			// A workspace whose nearest-bookmark query fails (e.g. abandoned
			// change) is skipped rather than aborting the entire lookup —
			// the other workspaces may still resolve cleanly.
			continue
		}
		if bookmark == def {
			candidates = append(candidates, defaultBranchPathCandidate{
				name: name,
				path: root,
			})
		}
	}
	return pickDefaultBranchPath(candidates, def)
}

// jjNearestBookmark returns the unique bookmark name on the nearest
// bookmark-bearing ancestor of `changeID` (= the same `heads(::X &
// bookmarks())` query CurrentBranch uses, parameterised on X). Returns
// "" when zero or multiple bookmarks match (ambiguous — caller treats
// as "not on default" rather than erroring).
func jjNearestBookmark(changeID string) (string, error) {
	out, err := runBackendCmd("jj", "log",
		"-r", "heads(::"+changeID+" & bookmarks())",
		"--no-graph",
		"-T", `bookmarks.map(|b| b.name()).join("\n") ++ "\n"`)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		names = append(names, s)
	}
	if len(names) != 1 {
		return "", nil // zero or multiple → ambiguous, not on a uniquely-named bookmark
	}
	return names[0], nil
}

// IsOnDefaultBranch reports whether the bookmark unique to @'s ancestor
// head equals DefaultBranch(). Ambiguous current branch (no bookmark, or
// multiple bookmarks at head) is reported as "not on default" (false, nil)
// rather than propagating the ambiguity error — the predicate's contract
// is a boolean, and "ambiguous" maps cleanly to "definitely not on the
// uniquely-named default".
func (j *jjBackend) IsOnDefaultBranch() (bool, error) {
	return isOnDefaultBranchCommon(j)
}

// Promote moves the default bookmark to opts.Rev (default: `@-`) via
// `jj bookmark set <default> -r <rev>`. jj's bookmark set is forward-only
// by default — a backwards move is rejected, which we surface as
// *exitErr{exitCodeNonFastForward} so the dispatcher can hint the user
// toward `vcs sync` before retrying.
func (j *jjBackend) Promote(opts promoteOpts) error {
	def, err := j.DefaultBranch()
	if err != nil {
		return err
	}
	rev := opts.Rev
	if rev == "" {
		rev = "@-"
	}
	stdout, stderr, code, runErr := runBackendCapture("jj", "bookmark", "set", def, "-r", rev)
	if runErr != nil {
		return &exitErr{code: exitCodeVCSExec, msg: runErr.Error()}
	}
	if code != 0 {
		joined := strings.TrimSpace(stderr)
		if joined == "" {
			joined = strings.TrimSpace(stdout)
		}
		// jj 0.42 surfaces backwards / sideways rejections as
		// "Refusing to move bookmark backwards or sideways: <name>". jj's own
		// hint (= `--allow-backwards`) is replaced with bump-semver's sync
		// recommendation so the caller sees the canonical recovery path.
		if strings.Contains(joined, "backwards") || strings.Contains(joined, "Refusing to move") {
			def, dErr := j.DefaultBranch()
			if dErr == nil {
				joined = fmt.Sprintf("%s (run `bump-semver vcs sync --onto %s@origin` first)",
					strings.SplitN(joined, "\n", 2)[0], def)
			}
			return &exitErr{code: exitCodeNonFastForward, msg: joined}
		}
		return &exitErr{code: exitCodeVCSExec, msg: joined}
	}
	writePushDiagnostic(opts.Stdout, stderr)
	return nil
}

// BookmarkSet writes the named jj bookmark so it points at opts.Rev.
// `jj bookmark set` creates the bookmark if absent and is forward-only by
// default; opts.AllowBackwards adds `--allow-backwards` to permit non-FF
// moves.
func (j *jjBackend) BookmarkSet(opts bookmarkSetOpts) error {
	rev := opts.Rev
	if rev == "" {
		rev = "@"
	}
	rev = translateRev(rev, vcsJj)
	args := []string{"bookmark", "set", opts.Name, "-r", rev}
	if opts.AllowBackwards {
		args = append(args, "--allow-backwards")
	}
	stdout, stderr, code, runErr := runBackendCapture("jj", args...)
	if runErr != nil {
		return &exitErr{code: exitCodeVCSExec, msg: runErr.Error()}
	}
	if code != 0 {
		joined := strings.TrimSpace(stderr)
		if joined == "" {
			joined = strings.TrimSpace(stdout)
		}
		// jj surfaces backwards-move rejections as
		// "Refusing to move bookmark backwards" / "would move backwards".
		if strings.Contains(joined, "backwards") || strings.Contains(joined, "Refusing to move") {
			return &exitErr{
				code: exitCodeNonFastForward,
				msg:  joined + " (use --allow-backwards to override)",
			}
		}
		return &exitErr{code: exitCodeVCSExec, msg: joined}
	}
	writePushDiagnostic(opts.Stdout, stderr)
	return nil
}

// Sync rebases the current workspace onto opts.Onto via `jj rebase -d`.
// The dispatcher pre-validates Onto is non-empty.
func (j *jjBackend) Sync(opts syncOpts) error {
	if opts.Onto == "" {
		return &exitErr{code: exitCodeUsage, msg: "sync: --onto is required"}
	}
	stdout, stderr, code, runErr := runBackendCapture("jj", "rebase", "-d", opts.Onto)
	if runErr != nil {
		return &exitErr{code: exitCodeVCSExec, msg: runErr.Error()}
	}
	if code != 0 {
		joined := strings.TrimSpace(stderr)
		if joined == "" {
			joined = strings.TrimSpace(stdout)
		}
		return &exitErr{code: exitCodeVCSExec, msg: joined}
	}
	writePushDiagnostic(opts.Stdout, stderr)
	return nil
}

// jjStringLiteral wraps s in jj-revset double-quote form. Internal `"`
// and `\` are backslash-escaped. Suitable for `files("…")` literals in
// the revset language; not a general-purpose shell quoter.
func jjStringLiteral(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	sb.WriteByte('"')
	return sb.String()
}
