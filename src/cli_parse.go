package main

import (
	"fmt"
	"strings"
)

// --- cliArgs sub-structs (PR-Simplify-1 A) -----------------------------
//
// cliArgs has historically been a flat 30+ field grab-bag. The verb-
// specific flags (bump's --pre / --build-metadata, vcs commit's -m /
// --staged, vcs push's --branch / --remote, vcs tag's --rev / --allow-
// move) are grouped into per-verb opts sub-structs so the top-level
// stays scannable. Common fields (kind/action/inputs/write/vcsVerb/
// vcsArgs/compareOp/comparePrecision) remain at the top because they're
// read by the dispatcher independently of which verb is active.
//
// Step A (this commit) keeps the existing `XxxSet bool` companion
// fields — they're just relocated into the sub-struct. Step B
// (subsequent commit) collapses each `X string + XSet bool` pair into a
// single `X *string` field (nil = unset).

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
// transition in parseArgs and reduces each predicate to a single
// comparison via the ShouldSuppress* helpers below.
type outputVerbosity int

const (
	outputNormal   outputVerbosity = iota // no flag: nothing suppressed.
	outputNoHint                          // --no-hint: hint suppressed only.
	outputQuiet                           // -q / --quiet: stdout + hint suppressed.
	outputQuietAll                        // -qq / --quiet-all: stdout + hint + stderr suppressed.
)

// raise sets *v to the higher of its current value and lvl. parseArgs
// uses this at every flag-assignment site so the order of CLI flags
// never downgrades the suppression level (e.g. `-qq -q` stays at
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

// vcsBaseOpts groups the --vcs override (DR-0008). Accepted via the
// global parser AND the vcs sub-parser; consumed by both runBump /
// runCompare (via newVcsBackend) and every runVcsCmd* dispatcher.
//
// Override is *string so "unset" (nil) is distinguishable from
// explicit "auto" (&"auto"); both fall through to parseVcsOverride =
// vcsAuto, but keeping the distinction lets future code (e.g. a
// `--vcs` echo in `bump --json`) report what the user actually typed.
type vcsBaseOpts struct {
	Override *string // nil / "auto" / "jj" / "git" (validated in parseArgs)
}

// vcsDiffOpts groups verb-local flags for `vcs diff` (DR-0020 PR-3).
type vcsDiffOpts struct {
	// NameStatus toggles `-s/--name-status` mode: emit one
	// `<CODE>\t<path>` line per changed file (git --name-status shape)
	// instead of a raw patch. -q wins over -s for stdout but the exit
	// code still reflects diff presence.
	NameStatus bool
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
	Message *string
	Staged  bool
	Amend   bool
	DashA   bool
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
	bump      bumpOpts
	output    outputOpts
	vcsBase   vcsBaseOpts
	vcsDiff   vcsDiffOpts
	vcsCommit vcsCommitOpts
	vcsPush   vcsPushOpts
	vcsTag    vcsTagOpts
}

var bumpActions = map[string]bool{
	"major": true, "minor": true, "patch": true, "pre": true, "get": true,
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

// parseArgs is the top-level CLI dispatcher. It handles the no-argv /
// --version / --help / --help-full short-circuits and then delegates to
// a verb-specific subparser:
//
//   - `vcs`     → parseVcsArgs    (two-tier verb family, DR-0020)
//   - `compare` → parseCompareArgs (predicate-only command, DR-0006)
//   - <action>  → parseBumpArgs    (the flat bump/get family)
//
// All three subparsers return `(cliArgs, error)` so the dispatcher is a
// uniform fan-out. The shared bump/compare flag loop lives in
// parseSharedFlags; the vcs branch keeps its own verb-gated loop
// (intentionally — see the comment at the top of parseVcsArgs).
func parseArgs(argv []string) (cliArgs, error) {
	if len(argv) == 0 {
		return cliArgs{kind: "help"}, nil
	}
	switch argv[0] {
	case "--version", "-V":
		out := cliArgs{kind: "version"}
		// --version は他フラグを基本受け付けないが、--json だけは
		// バイナリ自身のバージョンを構造化 JSON で出力する用に解釈する
		// (CI で `bump-semver --version --json | jq -r .semver` のような使い方)
		for _, a := range argv[1:] {
			if a == "--json" {
				out.output.JSON = true
				continue
			}
			return cliArgs{}, fmt.Errorf("--version only accepts --json")
		}
		return out, nil
	case "--help", "-h":
		return cliArgs{kind: "help"}, nil
	case "--help-full":
		return cliArgs{kind: "helpFull"}, nil
	}

	if argv[0] == "vcs" {
		return parseVcsArgs(argv)
	}
	if argv[0] == "compare" {
		return parseCompareArgs(argv)
	}
	return parseBumpArgs(argv)
}

// parseVcsArgs parses `vcs <verb> [<subverb>] [flags...] [args...]`
// (DR-0020). The vcs branch has its own flag loop because each value-
// taking flag (-m, --branch, --rev, --remote, --allow-move, ...) is
// verb-gated on `vcsVerb` (and for `tag`, also on `vcsTag.SubVerb`).
// Unifying it with the shared bump/compare loop would require a verb→
// flags lookup table that is more complex than the current explicit
// switch-case; see DR-0020 implementation notes.
func parseVcsArgs(argv []string) (cliArgs, error) {
	// `vcs` is a two-tier subcommand (vcs <verb> [args...]) — we
	// parse it specially because the existing flat-action grammar
	// doesn't fit. Help routing:
	//
	//   bump-semver vcs            → show vcs parent help
	//   bump-semver vcs --help     → show vcs parent help
	//   bump-semver vcs get        → show vcs get help (no key given)
	//   bump-semver vcs <verb> --help → show vcs <verb> help
	//   bump-semver vcs <verb> <args...> → dispatch to runVcsCmd
	out := cliArgs{kind: "vcs"}
	if len(argv) == 1 {
		return cliArgs{kind: "helpAction", action: "vcs"}, nil
	}
	if argv[1] == "--help" || argv[1] == "-h" {
		return cliArgs{kind: "helpAction", action: "vcs"}, nil
	}
	out.vcsVerb = argv[1]
	// Per-verb help only for known verbs — unknown verbs must
	// surface as an exit-2 error, not as a silent help fallthrough.
	// We route them to runVcsCmd which emits the proper usage error.
	isKnownVerb := out.vcsVerb == "get" || out.vcsVerb == "is" || out.vcsVerb == "diff" || out.vcsVerb == "commit" || out.vcsVerb == "fetch" || out.vcsVerb == "push" || out.vcsVerb == "tag"
	// PR-6: `vcs tag` is the first two-tier verb. Sub-verb capture
	// lives here so flag scanning can gate `--rev` / `--allow-move`
	// on it the same way the single-tier verbs gate their flags.
	//
	// Help routing for tag:
	//   vcs tag                                 → vcs tag help
	//   vcs tag --help / -h                     → vcs tag help
	//   vcs tag <subverb>                       → vcs tag <subverb> help
	//   vcs tag <subverb> --help / -h           → vcs tag <subverb> help
	//   vcs tag <subverb> <args>                → dispatch
	// Unknown <subverb> falls through to runVcsCmd which reports
	// the exit-2 usage error (mirroring unknown top-level verb
	// handling above).
	tagSubVerbStart := 2 // index where the sub-verb / args begin
	if out.vcsVerb == "tag" {
		if len(argv) == 2 {
			return cliArgs{kind: "helpAction", action: "vcs tag"}, nil
		}
		if argv[2] == "--help" || argv[2] == "-h" {
			return cliArgs{kind: "helpAction", action: "vcs tag"}, nil
		}
		out.vcsTag.SubVerb = argv[2]
		isKnownSub := out.vcsTag.SubVerb == "push" || out.vcsTag.SubVerb == "delete"
		if len(argv) == 3 {
			if isKnownSub {
				return cliArgs{
					kind:   "helpAction",
					action: "vcs tag " + out.vcsTag.SubVerb,
				}, nil
			}
			// Unknown sub-verb with no further args: still send to
			// dispatcher so the exit-2 usage error fires (no silent
			// help fallthrough).
			return out, nil
		}
		if argv[3] == "--help" || argv[3] == "-h" {
			if isKnownSub {
				return cliArgs{
					kind:   "helpAction",
					action: "vcs tag " + out.vcsTag.SubVerb,
				}, nil
			}
			return out, nil
		}
		tagSubVerbStart = 3
	} else {
		if len(argv) == 2 {
			if isKnownVerb {
				return cliArgs{kind: "helpAction", action: "vcs " + out.vcsVerb}, nil
			}
			return out, nil
		}
		if argv[2] == "--help" || argv[2] == "-h" {
			if isKnownVerb {
				return cliArgs{kind: "helpAction", action: "vcs " + out.vcsVerb}, nil
			}
			return out, nil
		}
	}
	// Split flags from positional vcsArgs. The vcs branch supports a
	// curated subset of the global flags: --vcs (override), -q/-qq,
	// --no-hint. `-s/--name-status` is verb-local to `vcs diff`.
	// Anything else is reported as an unknown flag (exit 2, names the
	// verb in the hint so typos like `vcs get -s root` are caught).
	// Unlike the main flat-action grammar, we don't process --pre /
	// --write etc. here — those are bump-only.
	//
	// Design rationale: there is exactly one verb-local flag, so a
	// `(verb == "diff")` guard inline is simpler than a verb→flags
	// table. If verb-local flags grow, refactor to a table keyed by
	// verb (see DR-0020 implementation notes).
	rest := argv[tagSubVerbStart:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--vcs":
			if out.vcsBase.Override != nil {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--vcs requires a value (jj, git, or auto)")
			}
			out.vcsBase.Override = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--vcs="):
			if out.vcsBase.Override != nil {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			out.vcsBase.Override = ptr(strings.TrimPrefix(a, "--vcs="))
		case a == "-q", a == "--quiet":
			out.output.Verbosity.raise(outputQuiet)
		case a == "-qq", a == "--quiet-all":
			out.output.Verbosity.raise(outputQuietAll)
		case (a == "-s" || a == "--name-status") && out.vcsVerb == "diff":
			// Verb-local to `vcs diff` only. For other verbs this
			// case is skipped and the generic unknown-flag catch-all
			// below rejects with exit 2.
			out.vcsDiff.NameStatus = true
		case a == "-m" && out.vcsVerb == "commit":
			// Verb-local to `vcs commit`. Takes a value.
			if out.vcsCommit.Message != nil {
				return cliArgs{}, fmt.Errorf("-m specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("-m requires a value (commit message)")
			}
			out.vcsCommit.Message = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "-m=") && out.vcsVerb == "commit":
			if out.vcsCommit.Message != nil {
				return cliArgs{}, fmt.Errorf("-m specified twice")
			}
			out.vcsCommit.Message = ptr(strings.TrimPrefix(a, "-m="))
		case a == "--message" && out.vcsVerb == "commit":
			if out.vcsCommit.Message != nil {
				return cliArgs{}, fmt.Errorf("--message specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--message requires a value")
			}
			out.vcsCommit.Message = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--message=") && out.vcsVerb == "commit":
			if out.vcsCommit.Message != nil {
				return cliArgs{}, fmt.Errorf("--message specified twice")
			}
			out.vcsCommit.Message = ptr(strings.TrimPrefix(a, "--message="))
		case a == "--staged" && out.vcsVerb == "commit":
			out.vcsCommit.Staged = true
		case a == "--amend" && out.vcsVerb == "commit":
			out.vcsCommit.Amend = true
		case (a == "-a" || a == "--all") && out.vcsVerb == "commit":
			// Captured here only so we can give a tailored exit-2
			// rejection in runVcsCmdCommit (instead of the generic
			// "unknown flag" message). DR-0020: -a is intentionally
			// non-provided to prevent unstaged-grab accidents in
			// jj's auto-staged world.
			out.vcsCommit.DashA = true
		// --- DR-0020 PR-5: vcs fetch / vcs push flags --------------
		//
		// --branch and --bookmark are aliases of one field for `vcs
		// push`. We don't track which spelling the user typed
		// (downstream only cares about the value), but we DO reject
		// "both spellings supplied" via the same already-set rule
		// applied to every other value-taking flag — surprising the
		// user with "your --branch was overwritten by --bookmark"
		// would be worse than a sharp usage error.
		//
		// --remote is shared between fetch and push (both verbs
		// accept it). Anything else is the parser's generic
		// unknown-flag catch-all.
		case (a == "--branch" || a == "--bookmark") && out.vcsVerb == "push":
			if out.vcsPush.Name != nil {
				return cliArgs{}, fmt.Errorf("--branch/--bookmark specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("%s requires a value (the branch/bookmark name)", a)
			}
			out.vcsPush.Name = ptr(rest[i+1])
			i++
		case (strings.HasPrefix(a, "--branch=") || strings.HasPrefix(a, "--bookmark=")) && out.vcsVerb == "push":
			if out.vcsPush.Name != nil {
				return cliArgs{}, fmt.Errorf("--branch/--bookmark specified twice")
			}
			eq := strings.IndexByte(a, '=')
			out.vcsPush.Name = ptr(a[eq+1:])
		case a == "--remote" && (out.vcsVerb == "fetch" || out.vcsVerb == "push" || out.vcsVerb == "tag"):
			if out.vcsPush.Remote != nil {
				return cliArgs{}, fmt.Errorf("--remote specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--remote requires a value")
			}
			out.vcsPush.Remote = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--remote=") && (out.vcsVerb == "fetch" || out.vcsVerb == "push" || out.vcsVerb == "tag"):
			if out.vcsPush.Remote != nil {
				return cliArgs{}, fmt.Errorf("--remote specified twice")
			}
			out.vcsPush.Remote = ptr(strings.TrimPrefix(a, "--remote="))
		// DR-0020 PR-5.2 / PR-5.2.1: --jj-bookmark-auto-advance is
		// the canonical example of the backend-prefix general rule
		// (--jj-* / --git-* flags are routed by name to their
		// backend, ignored silently on the other backend). Parsed
		// here as a verb-local boolean on `vcs push`; the actual
		// auto-advance step runs in jjBackend.Push, gitBackend.Push
		// just ignores it.
		case a == "--jj-bookmark-auto-advance" && out.vcsVerb == "push":
			out.vcsPush.JjBookmarkAutoAdvance = true
		// --- DR-0020 PR-6: vcs tag push flags ----------------------
		//
		// `--rev` carries the target revision for `vcs tag push`;
		// `--allow-move` opts into moving an existing tag (DR-0020
		// line 71). Both are verb-local to `vcs tag push` — when
		// the sub-verb is `delete` or anything else, the generic
		// unknown-flag catch-all below rejects them with exit 2,
		// preserving the "wrong verb for this flag" guardrail that
		// caught typos like `vcs get -s root` for PR-3.
		case a == "--rev" && out.vcsVerb == "tag" && out.vcsTag.SubVerb == "push":
			if out.vcsTag.Rev != nil {
				return cliArgs{}, fmt.Errorf("--rev specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--rev requires a value (the target revision)")
			}
			out.vcsTag.Rev = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--rev=") && out.vcsVerb == "tag" && out.vcsTag.SubVerb == "push":
			if out.vcsTag.Rev != nil {
				return cliArgs{}, fmt.Errorf("--rev specified twice")
			}
			out.vcsTag.Rev = ptr(strings.TrimPrefix(a, "--rev="))
		case a == "--allow-move" && out.vcsVerb == "tag" && out.vcsTag.SubVerb == "push":
			out.vcsTag.AllowMove = true
		case a == "--no-hint":
			out.output.Verbosity.raise(outputNoHint)
		case a == "--":
			out.vcsArgs = append(out.vcsArgs, rest[i+1:]...)
			i = len(rest)
		case strings.HasPrefix(a, "-") && a != "-":
			verbLabel := out.vcsVerb
			if out.vcsVerb == "tag" && out.vcsTag.SubVerb != "" {
				verbLabel = "tag " + out.vcsTag.SubVerb
			}
			return cliArgs{}, fmt.Errorf("unknown flag for 'vcs %s': %s", verbLabel, a)
		default:
			out.vcsArgs = append(out.vcsArgs, a)
		}
	}
	if out.vcsBase.Override != nil {
		if _, err := parseVcsOverride(*out.vcsBase.Override); err != nil {
			return cliArgs{}, err
		}
	}
	return out, nil
}

// parseCompareArgs parses `compare OP[-prec] [flags...] inputs...`
// (DR-0006 / DR-0023 / DR-0017). The flag loop is shared with
// parseBumpArgs (see parseSharedFlags) — bump-only flags (`--write`,
// `--pre`, `--build-metadata`, `--json`) are accepted by the shared
// loop and then rejected here in the compare-specific validity tail.
// That keeps the rejection error messages ("--write is not valid with
// compare" etc.) consistent with the pre-refactor behaviour rather
// than degrading them to the generic "unknown option:" form.
func parseCompareArgs(argv []string) (cliArgs, error) {
	// `bump-semver compare --help` / `compare -h`: アクション固有 help
	// に短絡 (OP の解釈は始めない)。OP 後に置かれた `--help` は通常の
	// rest 走査で拾う。
	if len(argv) >= 2 && (argv[1] == "--help" || argv[1] == "-h") {
		return cliArgs{kind: "helpAction", action: "compare"}, nil
	}
	if len(argv) < 2 {
		return cliArgs{}, fmt.Errorf("compare requires an operator (eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)")
	}
	op := argv[1]
	base, precision, ok := parseCompareOp(op)
	if !ok {
		return cliArgs{}, fmt.Errorf("unknown compare operator: %s (expected eq|lt|le|gt|ge, optionally with -major / -minor / -patch suffix)", op)
	}
	out := cliArgs{kind: "compare", compareOp: base, comparePrecision: precision}
	rest := argv[2:]
	if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
		return cliArgs{kind: "helpAction", action: "compare"}, nil
	}
	out, err := parseSharedFlags(out, rest)
	if err != nil {
		return cliArgs{}, err
	}
	// --- compare-specific validity tail --------------------------------
	if out.write {
		return cliArgs{}, fmt.Errorf("--write is not valid with compare")
	}
	if out.bump.Pre != nil {
		return cliArgs{}, fmt.Errorf("--pre is not valid with compare")
	}
	if out.bump.BuildMetadata != nil {
		return cliArgs{}, fmt.Errorf("--build-metadata is not valid with compare")
	}
	// DR-0007: compare is a predicate-only command — exit code is
	// the answer, stdout is intentionally empty. There is nothing
	// to render as JSON.
	if out.output.JSON {
		return cliArgs{}, fmt.Errorf("compare does not support --json")
	}
	// DR-0023: compare accepts F1 + N OTHERS (N>=1). The legacy
	// 2-input form (`compare OP F1 F2`) is the N=1 case.
	if len(out.inputs) < 2 {
		return cliArgs{}, fmt.Errorf("compare requires at least two inputs (BASE OTHERS...), got %d", len(out.inputs))
	}
	return out, nil
}

// parseBumpArgs parses `<action> [flags...] inputs...` for the flat
// bump/get family (major / minor / patch / pre / get). The flag loop is
// shared with parseCompareArgs (see parseSharedFlags); get-specific
// rejections (`--write` / `--pre` / `--build-metadata` not valid with
// get) and the at-least-one-input requirement live in the bump
// validity tail.
func parseBumpArgs(argv []string) (cliArgs, error) {
	if !bumpActions[argv[0]] {
		return cliArgs{}, fmt.Errorf("unknown action: %s (expected one of major|minor|patch|pre|get|compare)", argv[0])
	}
	out := cliArgs{kind: "bump", action: argv[0]}
	rest := argv[1:]
	if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
		return cliArgs{kind: "helpAction", action: out.action}, nil
	}
	out, err := parseSharedFlags(out, rest)
	if err != nil {
		return cliArgs{}, err
	}
	// --- bump-specific validity tail -----------------------------------
	if out.action == "get" {
		if out.write {
			return cliArgs{}, fmt.Errorf("--write is not valid with get")
		}
		if out.bump.Pre != nil {
			return cliArgs{}, fmt.Errorf("--pre is not valid with get (use --no-pre to strip)")
		}
		if out.bump.BuildMetadata != nil {
			return cliArgs{}, fmt.Errorf("--build-metadata is not valid with get (use --no-build-metadata to strip)")
		}
	}
	if len(out.inputs) == 0 {
		return cliArgs{}, fmt.Errorf("at least one input (FILE | VER | -) is required")
	}
	return out, nil
}

// parseSharedFlags is the flag loop shared by parseCompareArgs and
// parseBumpArgs. It accepts every flag permissively — bump-only flags
// (`--write`, `--pre`, `--build-metadata`, `--no-pre`,
// `--no-build-metadata`) are parsed even under `compare` and then
// rejected in parseCompareArgs's validity tail with a verb-specific
// message ("--write is not valid with compare"). Splitting the loop
// per-verb would degrade those rejections to the generic
// "unknown option:" form, which long-standing tests pin verbatim.
//
// Common exclusivity / value-validity checks (--pre vs --no-pre, empty
// values, `--vcs` value validation) run at the end of this helper so
// both verbs inherit them identically.
func parseSharedFlags(out cliArgs, rest []string) (cliArgs, error) {
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--write":
			if out.write {
				return cliArgs{}, fmt.Errorf("--write specified twice")
			}
			out.write = true
		case a == "--pre":
			if out.bump.Pre != nil {
				return cliArgs{}, fmt.Errorf("--pre specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--pre requires a value")
			}
			out.bump.Pre = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--pre="):
			if out.bump.Pre != nil {
				return cliArgs{}, fmt.Errorf("--pre specified twice")
			}
			out.bump.Pre = ptr(strings.TrimPrefix(a, "--pre="))
		case a == "--no-pre":
			if out.bump.NoPre {
				return cliArgs{}, fmt.Errorf("--no-pre specified twice")
			}
			out.bump.NoPre = true
		case a == "--build-metadata":
			if out.bump.BuildMetadata != nil {
				return cliArgs{}, fmt.Errorf("--build-metadata specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--build-metadata requires a value")
			}
			out.bump.BuildMetadata = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--build-metadata="):
			if out.bump.BuildMetadata != nil {
				return cliArgs{}, fmt.Errorf("--build-metadata specified twice")
			}
			out.bump.BuildMetadata = ptr(strings.TrimPrefix(a, "--build-metadata="))
		case a == "--no-build-metadata":
			if out.bump.NoBuildMetadata {
				return cliArgs{}, fmt.Errorf("--no-build-metadata specified twice")
			}
			out.bump.NoBuildMetadata = true
		case a == "--no-hint":
			// Idempotent: silently absorb duplicates rather than erroring,
			// to match the "no-op flags are silently accepted" policy from
			// Phase 5 (a -qq subsumes --no-hint anyway). raise() also
			// keeps a stronger level (e.g. -qq) from being downgraded by
			// a later --no-hint.
			out.output.Verbosity.raise(outputNoHint)
		case a == "-q", a == "--quiet":
			out.output.Verbosity.raise(outputQuiet)
		case a == "-qq", a == "--quiet-all":
			out.output.Verbosity.raise(outputQuietAll)
		case a == "--json":
			// Idempotent: silently absorb duplicates. Same policy as
			// --no-hint — boolean flags don't benefit from a strict
			// double-set check (no value is being lost).
			out.output.JSON = true
		case a == "--vcs":
			if out.vcsBase.Override != nil {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--vcs requires a value (jj, git, or auto)")
			}
			out.vcsBase.Override = ptr(rest[i+1])
			i++
		case strings.HasPrefix(a, "--vcs="):
			if out.vcsBase.Override != nil {
				return cliArgs{}, fmt.Errorf("--vcs specified twice")
			}
			out.vcsBase.Override = ptr(strings.TrimPrefix(a, "--vcs="))
		case a == "--":
			// Treat all remaining argv as inputs (lets paths starting with `-` through).
			out.inputs = append(out.inputs, rest[i+1:]...)
			i = len(rest)
		case strings.HasPrefix(a, "-") && a != "-":
			return cliArgs{}, fmt.Errorf("unknown option: %s", a)
		default:
			out.inputs = append(out.inputs, a)
		}
	}

	// --- exclusivity / value-validity checks (shared by bump + compare)
	if out.bump.Pre != nil && out.bump.NoPre {
		return cliArgs{}, fmt.Errorf("--pre and --no-pre are mutually exclusive")
	}
	if out.bump.BuildMetadata != nil && out.bump.NoBuildMetadata {
		return cliArgs{}, fmt.Errorf("--build-metadata and --no-build-metadata are mutually exclusive")
	}
	if out.bump.Pre != nil && *out.bump.Pre == "" {
		return cliArgs{}, fmt.Errorf("--pre value cannot be empty, use --no-pre to remove")
	}
	if out.bump.BuildMetadata != nil && *out.bump.BuildMetadata == "" {
		return cliArgs{}, fmt.Errorf("--build-metadata value cannot be empty, use --no-build-metadata to remove")
	}
	if out.vcsBase.Override != nil {
		if _, err := parseVcsOverride(*out.vcsBase.Override); err != nil {
			return cliArgs{}, err
		}
	}
	return out, nil
}
