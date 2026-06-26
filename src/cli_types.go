package main

import (
	"fmt"
	"strings"
)

// --- cliArgs sub-structs (PR-Simplify-1 A) -----------------------------
//
// cliArgs is the parsed command-line shared by the dispatcher
// (runBump / runCompare / runVcsCmd*) and the cobra RunE adapters that
// assemble it. The verb-specific flags (bump's --pre / --build-metadata,
// vcs commit's -m / --staged, vcs push's --branch / --remote, vcs tag's
// --rev / --allow-move) are grouped into per-verb opts sub-structs so the
// top-level stays scannable. Common fields (kind/action/inputs/write/
// vcsVerb/vcsArgs/compareOp/comparePrecision) remain at the top because
// they're read by the dispatcher independently of which verb is active.

// bumpOpts groups the --pre / --build-metadata flags consumed by the
// bump path (runBump). The shape mirrors BumpOptions in semver.go but
// stays parser-side; the bump path bridges into BumpOptions at the
// call site. compare/get/vcs verbs ignore this sub-struct.
//
// Pre and BuildMetadata use *string so "unset" (nil) is structurally
// distinguishable from "set to empty" (non-nil &""). The parser
// rejects empty values explicitly so callers can safely treat a
// non-nil pointer as a non-empty string, but the distinction matters
// to runBump (which forwards PreSet/BuildMetadataSet into BumpOptions
// for the bump semantics in semver.go).
type bumpOpts struct {
	Pre             *string
	NoPre           bool
	BuildMetadata   *string
	NoBuildMetadata bool
}

// outputVerbosity is an ordered enum capturing the -q / -qq / --no-hint
// precedence as a single field. Each successive level adds suppression
// on top of the previous one, so a simple `>=` comparison answers
// "should we suppress X?". Design rationale: the original three
// independent bools (Quiet / QuietAll / NoHint) had a precedence rule
// (QuietAll > Quiet > NoHint) that every dispatch site re-derived via
// hand-rolled boolean combinators, which was both noisy and easy to get
// subtly wrong (e.g. forgetting a `|| QuietAll` when checking `Quiet`).
// Collapsing to an ordered enum makes "raise to max" the only legal
// transition and reduces each predicate to a single comparison via the
// ShouldSuppress* helpers below.
type outputVerbosity int

const (
	outputNormal   outputVerbosity = iota // no flag: nothing suppressed.
	outputNoHint                          // --no-hint: hint suppressed only.
	outputQuiet                           // -q / --quiet: stdout + hint suppressed.
	outputQuietAll                        // -qq / --quiet-all: stdout + hint + stderr suppressed.
)

// raise sets *v to the higher of its current value and lvl. The flag
// layer uses this at every flag-assignment site so the order of CLI
// flags never downgrades the suppression level (e.g. `-qq -q` stays at
// outputQuietAll, matching the historical "QuietAll dominates Quiet"
// precedence).
func (v *outputVerbosity) raise(lvl outputVerbosity) {
	if lvl > *v {
		*v = lvl
	}
}

// ShouldSuppressHint reports whether the post-action hint line should be
// silenced. True for --no-hint, -q, and -qq (any non-Normal level).
func (v outputVerbosity) ShouldSuppressHint() bool { return v >= outputNoHint }

// ShouldSuppressStdout reports whether the verb's primary stdout output
// should be silenced. True for -q and -qq.
func (v outputVerbosity) ShouldSuppressStdout() bool { return v >= outputQuiet }

// ShouldSuppressError reports whether the verb's stderr error output
// should be silenced. True only for -qq.
func (v outputVerbosity) ShouldSuppressError() bool { return v >= outputQuietAll }

// outputOpts groups the suppression / format-toggle flags shared across
// every verb (bump, compare, vcs, version). DR-0007 (--json) and
// Phase 5 (-q / -qq / --no-hint). JSON is intentionally a separate
// axis: it's a format toggle, not a suppression level.
type outputOpts struct {
	Verbosity outputVerbosity // -q / -qq / --no-hint (collapsed enum)
	JSON      bool            // --json: structured single-line JSON output (DR-0007)
}

// vcsBaseOpts groups the --vcs override (DR-0008). Accepted by both the
// bump/compare shared flag group AND the vcs subtree; consumed by both
// runBump / runCompare (via newVcsBackend) and every runVcsCmd*
// dispatcher.
//
// Override is *string so "unset" (nil) is distinguishable from
// explicit "auto" (&"auto"); both fall through to parseVcsOverride =
// vcsAuto, but keeping the distinction lets future code (e.g. a
// `--vcs` echo in `bump --json`) report what the user actually typed.
type vcsBaseOpts struct {
	Override *string // nil / "auto" / "jj" / "git" (validated during flag assembly)
}

// vcsDiffOpts groups verb-local flags for `vcs diff` (DR-0020 PR-3).
type vcsDiffOpts struct {
	// NameStatus toggles `-s/--name-status` mode: emit one
	// `<CODE>\t<path>` line per changed file (git --name-status shape)
	// instead of a raw patch. -q wins over -s for stdout but the exit
	// code still reflects diff presence.
	NameStatus bool

	// Excludes holds `--excludes PATTERN` values (DR-0033). Each value is
	// a literal path / `glob:` / `file:` selector that is post-filtered
	// out of the include set (= position-independent set subtraction).
	// Repeatable + append; nil = no exclusion.
	Excludes []string
}

// vcsCommitOpts groups verb-local flags for `vcs commit` (DR-0020 PR-4
// / PR-4.1). DashA is captured only so runVcsCmdCommit can emit a
// tailored exit-2 rejection (DR-0020 safety: --staged is the supported
// "all" mode, -a's unstaged-grab is intentionally absent).
//
// Message is *string so "no -m given" (nil) is distinguishable from
// "-m empty" (non-nil pointer to ""); the parser rejects bare -m but
// runVcsCmdCommit needs the distinction for the --amend "keep
// existing message" path (noEdit = amend && Message == nil).
type vcsCommitOpts struct {
	Message              *string
	Staged               bool
	Amend                bool
	DashA                bool
	AllowNonexistentPath bool
}

// vcsPushOpts groups verb-local flags for `vcs push` (DR-0020 PR-5 /
// PR-5.2). Remote is shared with `vcs fetch` and `vcs tag` (both verbs
// also accept --remote); the dispatcher reads it based on which verb is
// active.
//
// Name carries the value of --branch OR --bookmark; the two flags are
// aliases of one field (DR-0020 命名規律: common-vocabulary "branch" is
// canonical, "bookmark" is the jj-flavoured alias). The parser treats
// them as one slot — specifying both is an "already set" usage error.
type vcsPushOpts struct {
	Name   *string
	Remote *string
	// JjBookmarkAutoAdvance: jj-only opt-in. When true the dispatcher
	// runs the bookmark auto-advance pre-step (clean → bookmark set to
	// @-, dirty → bookmark set to @) before the push. The `--jj-`
	// prefix names the backend the flag is scoped to (= structural typo
	// guard: a git repo getting this flag is exit-2 rejected at the
	// dispatcher, not silently no-op'd at the backend).
	JjBookmarkAutoAdvance bool
}

// vcsOutdatedOpts groups verb-local flags for `vcs outdated` (DR-0027 /
// DR-0028). See `docs/specs/glob-backref-v0.1.0.md` for the matching spec.
//
// Explain: `--explain` — list every expanded (source → derived) row with
// status. Exit code stays 0 (diagnostic mode).
//
// Strict: `--strict` — treat a literal FROM that matches no file (= likely
// typo) as exit 1. Default warns to stderr and exits 0 for compatibility
// with the original DR-0027 silent-skip semantics (= blocker #3 mitigation).
type vcsOutdatedOpts struct {
	Explain bool
	Strict  bool
}

// vcsTagOpts groups verb-local flags for `vcs tag` (DR-0020 PR-6).
// `vcs tag` is the first two-tier verb in the family — argv[1] is the
// parent "tag", argv[2] is the sub-verb ("push" | "delete"), argv[3..]
// is the sub-verb's payload. SubVerb carries argv[2]; the parser
// captures it before scanning flags so verb-local flag gating ("--rev
// only under tag push") works the same way the existing single-tier
// verbs do.
//
// AllowMove encodes `--allow-move` (tag push only); --remote is reused
// from vcsPushOpts.Remote because the semantics are identical across
// fetch / push / tag.
type vcsTagOpts struct {
	SubVerb   string
	Rev       *string
	AllowMove bool
}

// vcsSyncOpts captures flags for `vcs sync` (rebase the current
// worktree/workspace onto a named ref).
type vcsSyncOpts struct {
	// Onto is the target ref (required). The flag parser surfaces a missing
	// value as exit 2.
	Onto *string
}

// vcsBookmarkOpts groups verb-local flags for `vcs bookmark` (two-tier verb
// like `vcs tag`). SubVerb captures argv[2] ("set" today; future "delete" /
// "list" land here without restructuring).
//
// Rev / AllowBackwards are scoped to `vcs bookmark set`. The dispatcher
// validates them only on that sub-verb; setting them on other sub-verbs (or
// the bare parent) is an exit-2 usage error.
type vcsBookmarkOpts struct {
	SubVerb        string
	Rev            *string
	AllowBackwards bool
}

// vcsGetOpts captures flags for `vcs get`:
//   - LatestRepository / LatestIncludePre — `vcs get latest-{tag,release}` (DR-0032)
//   - Rev — `vcs get commit-id` target rev (nil = backend default: `@` for jj /
//     `HEAD` for git). translateRev (DR-0031) normalizes cross-backend forms.
type vcsGetOpts struct {
	LatestRepository *string
	LatestIncludePre bool
	Rev              *string
}

// cliArgs is the parsed command-line.
type cliArgs struct {
	kind             string // "bump" | "compare" | "vcs" | "version" | "help" | "helpFull" | "helpAction"
	action           string // bump 時: "major"/"minor"/"patch"/"pre"/"get"; vcs 時: "get" / ...
	compareOp        string // compare 時 base: "eq"/"lt"/"gt"/"le"/"ge"
	comparePrecision string // compare 時 precision (DR-0017): "" / "major" / "minor" / "patch"
	vcsVerb          string // vcs 時 1st verb (e.g. "get"); "" = parent-level (show help)
	vcsArgs          []string
	inputs           []string
	write            bool

	// Verb-grouped opts. Each sub-struct is read only when its owning
	// verb is dispatched (bump for `bump`, vcsCommit for `vcs commit`,
	// etc). vcsBase / output are common to multiple verbs.
	bump        bumpOpts
	output      outputOpts
	vcsBase     vcsBaseOpts
	vcsDiff     vcsDiffOpts
	vcsCommit   vcsCommitOpts
	vcsPush     vcsPushOpts
	vcsTag      vcsTagOpts
	vcsGet      vcsGetOpts
	vcsOutdated vcsOutdatedOpts
	vcsSync     vcsSyncOpts
	vcsBookmark vcsBookmarkOpts
	glob        globOpts

	// ruleBlocks captures DR-0029 user-defined rule blocks (--define-rule).
	// Element 0 is always the implicit global block (Pattern == ""), even
	// when no rule-definition flag was given (in which case Opts.hasAny()
	// returns false and the global block has no effect). Each subsequent
	// element is opened by --define-rule PATTERN. Rule-definition flags
	// (--format / --version-path / --version-regex / --name-path /
	// --name-regex) are appended to the **last** block, so flag order
	// within a block does not matter but the --define-rule position
	// determines which block each flag joins.
	//
	// hasDefineRule reflects whether any --define-rule appeared on the
	// command line. Used to enforce the 0a 補強 rule: once a
	// --define-rule has appeared, rule-definition flags MUST belong to one
	// of the named blocks. The global block becomes effectively closed
	// (= you cannot mix global rule flags with per-PATTERN blocks once
	// --define-rule starts).
	ruleBlocks    []ruleBlock
	hasDefineRule bool
}

// parseBoolValue parses the value side of a `--flag=value` glob bool flag.
// Accepts "true"/"false" (lowercase only — matches kawaz CLI norms; the
// flag spec is exact, no permissive parsing). DR-0024. Reused by the
// cobra globBoolValue (cobra_values.go) so the wording stays identical.
func parseBoolValue(flag, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s requires true or false, got %q", flag, value)
	}
}

// parseGlobFlag dispatches a single `--glob-*` flag for the `vcs outdated`
// hand-tokenised path (DisableFlagParsing, see parseOutdatedTokens).
// Returns (matched, err): matched=false means the caller should fall
// through to other branches. The other verbs register the --glob-* family
// as real cobra flags (addGlobFlags); only outdated still tokenises by
// hand because its `--` pair-separator grammar is incompatible with pflag.
//
// Design rationale: the `--glob-dotfile` / `--glob-gitignored` flags require
// an explicit `true|false` value (no space form, only `--flag=value`). This
// is a deliberate deviation from every other value-taking flag in this
// codebase (which accept both space and `=` forms). The reason is that
// "include dotfiles" / "exclude dotfiles" is exactly the kind of polarity
// that single-flag toggles get wrong — see DR-0024 for the full rationale.
// `--glob-ignorecase` is the third in the family and accepts optional value
// (bare = true) because the verb name itself carries the polarity.
func parseGlobFlag(a string, out *cliArgs) (matched bool, err error) {
	switch {
	case a == "--glob-dotfile":
		return true, fmt.Errorf("--glob-dotfile requires =true or =false (e.g. --glob-dotfile=false)")
	case strings.HasPrefix(a, "--glob-dotfile="):
		v, perr := parseBoolValue("--glob-dotfile", strings.TrimPrefix(a, "--glob-dotfile="))
		if perr != nil {
			return true, perr
		}
		out.glob.Dotfile = v
		return true, nil
	case a == "--glob-gitignored":
		return true, fmt.Errorf("--glob-gitignored requires =true or =false (e.g. --glob-gitignored=true)")
	case strings.HasPrefix(a, "--glob-gitignored="):
		v, perr := parseBoolValue("--glob-gitignored", strings.TrimPrefix(a, "--glob-gitignored="))
		if perr != nil {
			return true, perr
		}
		out.glob.Gitignored = ptr(v)
		return true, nil
	case a == "--glob-ignorecase":
		// Bare form = true (= "turn it on"). The verb name's polarity
		// removes the dotfile/gitignored ambiguity (= "ignore case" is
		// itself a directional verb).
		out.glob.IgnoreCase = true
		return true, nil
	case strings.HasPrefix(a, "--glob-ignorecase="):
		v, perr := parseBoolValue("--glob-ignorecase", strings.TrimPrefix(a, "--glob-ignorecase="))
		if perr != nil {
			return true, perr
		}
		out.glob.IgnoreCase = v
		return true, nil
	}
	return false, nil
}

var compareOps = map[string]bool{
	"eq": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// comparePrecisions enumerates DR-0017 precision suffixes. Empty
// string (full SemVer comparison) is implicit and not represented
// here.
var comparePrecisions = map[string]bool{
	"major": true, "minor": true, "patch": true,
}

// parseCompareOp splits a CLI compare operator into its base ("eq" /
// "lt" / etc.) and optional precision suffix ("major" / "minor" /
// "patch", DR-0017). An empty precision means SemVer-full comparison.
// Returns ok=false for any unrecognised combination so the caller can
// emit a uniform error.
func parseCompareOp(s string) (base, precision string, ok bool) {
	if compareOps[s] {
		return s, "", true
	}
	if i := strings.LastIndex(s, "-"); i > 0 {
		b, p := s[:i], s[i+1:]
		if compareOps[b] && comparePrecisions[p] {
			return b, p, true
		}
	}
	return "", "", false
}
