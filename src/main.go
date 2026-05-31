// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json /
// package-lock.json / VERSION) and provides five flat actions plus a
// nested `compare` subcommand:
//
//   - bump: major / minor / patch / pre
//   - read: get
//   - cmp:  compare {eq|lt|gt|le|ge}
//
// Inputs are positional: each argument may be a FILE path, a raw
// semver VER (e.g. `1.2.3`), `-` (read VER from stdin once), or
// `vcs:REV[:FILE]` / `vcs:<func>(...)` (read version from the VCS;
// DR-0008). When multiple inputs are given the values must agree;
// FILE-origin entries can be written back with `--write`, VER /
// stdin / vcs entries are reference values only.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		var ee *exitErr
		if errors.As(err, &ee) {
			// run() is responsible for writing any user-facing error
			// message to stderr (so it can honor --quiet-all). main only
			// translates the carried code into the process exit status.
			os.Exit(ee.code)
		}
		// Unexpected: run() must always wrap errors as *exitErr before
		// returning. Defensive fallback so we still exit non-zero.
		fmt.Fprintln(os.Stderr, "bump-semver: "+err.Error())
		os.Exit(2)
	}
}

// exitErr carries an explicit exit code through the call stack so we
// can distinguish "compare predicate is false" (exit 1) from "an error
// occurred" (exit 2). Plain errors propagate as exit 2 in main.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }
func (e *exitErr) ExitCode() int { return e.code }

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

// outputOpts groups the suppression / format-toggle flags shared across
// every verb (bump, compare, vcs, version). DR-0007 (--json) and
// Phase 5 (-q / -qq / --no-hint). Precedence: QuietAll > Quiet > NoHint.
type outputOpts struct {
	Quiet    bool // -q / --quiet:     suppress stdout + hint
	QuietAll bool // -qq / --quiet-all: also suppress error output
	NoHint   bool // --no-hint:         suppress only the hint
	JSON     bool // --json: structured single-line JSON output (DR-0007)
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

// ptr returns a pointer to v. It exists so verb-local opts that use
// `*T` for "explicitly set" semantics can assign in one line:
//
//	out.vcsCommit.Message = ptr(value)
//
// instead of the two-line `tmp := v; field = &tmp` boilerplate.
func ptr[T any](v T) *T { return &v }

// derefOr returns *p when p is non-nil, otherwise def. Used at the
// few sites that need a bare value from a `*string` opt and want
// "unset → empty/default" semantics (e.g. forwarding the parser's
// `*string` into a downstream struct whose field is still `string`).
func derefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
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
			out.output.Quiet = true
		case a == "-qq", a == "--quiet-all":
			out.output.QuietAll = true
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
		// DR-0020 PR-5.2: --jj-bookmark-auto-advance (vcs push only,
		// jj backend only). Boolean opt-in. Parsed here so the
		// parser doesn't emit "unknown flag for 'vcs push'"; the
		// jj-vs-git semantic check happens in runVcsCmdPush after
		// backend detection (= exit 2 with a hint naming the flag
		// and the jj-specific reason, instead of the generic
		// unknown-flag message).
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
			out.output.NoHint = true
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
			// Phase 5 (a -qq subsumes --no-hint anyway).
			out.output.NoHint = true
		case a == "-q", a == "--quiet":
			out.output.Quiet = true
		case a == "-qq", a == "--quiet-all":
			out.output.QuietAll = true
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

// locatedField is one detected version-or-name value, annotated with
// the origin label used for display in mismatch errors.
type locatedField struct {
	// File is the origin label. For FILE-origin fields it's the file
	// path; for VER-origin fields it's "<argv>" or "<argv:N>"; for
	// stdin (`-`) origin it's "<stdin>". Path is the in-file location
	// (e.g. "$.version") or empty for VER/stdin origins.
	File, Path, Value string
}

// label returns the human-readable origin label for column-aligned
// mismatch error rendering. FILE: "<file>:<path>"; non-FILE: just File.
func (lf locatedField) label() string {
	if lf.Path == "" {
		return lf.File
	}
	return lf.File + ":" + lf.Path
}

func locatedFromInspection(file string, fields []Field) []locatedField {
	out := make([]locatedField, 0, len(fields))
	for _, f := range fields {
		out = append(out, locatedField{File: file, Path: f.Path, Value: f.Value})
	}
	return out
}

func allSameValue(items []locatedField) (string, bool) {
	if len(items) == 0 {
		return "", true
	}
	first := items[0].Value
	for _, x := range items[1:] {
		if x.Value != first {
			return first, false
		}
	}
	return first, true
}

// formatMismatchError renders a "<kind> mismatch:" error with column-
// aligned origin labels (確定論点 F).
func formatMismatchError(kind string, items []locatedField) error {
	maxW := 0
	for _, x := range items {
		if w := len(x.label()); w > maxW {
			maxW = w
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s mismatch:", kind)
	for _, x := range items {
		fmt.Fprintf(&sb, "\n  %-*s = %s", maxW, x.label(), x.Value)
	}
	return errors.New(sb.String())
}

// resolvedInput is one positional input fully resolved into its origin
// label and the version field(s) it contributes to consistency checks.
type resolvedInput struct {
	originFile string // value used for locatedField.File on every contribution
	fields     []locatedField

	// FILE-origin only — needed for --write.
	file    string
	handler Handler
	content []byte
	insp    Inspection
}

// stdinReadState tracks at-most-once stdin consumption for `-` inputs.
type stdinReadState struct {
	consumed bool
	value    string
	err      error
}

// resolveInput interprets one positional argument according to the
// precedence rules from 確定論点 B (extended for DR-0008's `vcs:` input):
//  1. `-`             → read VER from stdin (once)
//  2. `vcs:...`       → resolve via VCS (DR-0008)
//  3. file exists     → FILE
//  4. ParseVersion OK → VER
//  5. otherwise       → error
//
// argIdx is the 1-based positional index used to disambiguate VER
// labels when there are multiple raw VERs ("<argv:2>" etc).
//
// borrowedFile is the path used when a `vcs:REV` spec omits its FILE
// component (it borrows from a sibling FILE-origin input). Empty
// string means "no sibling to borrow from", which is an error for
// vcs: rev-mode specs.
func resolveInput(arg string, argIdx, totalVERorStdin int, stdin io.Reader, st *stdinReadState, backend vcsBackend, borrowedFile string) (resolvedInput, error) {
	if arg == "-" {
		if !st.consumed {
			st.consumed = true
			st.value, st.err = readStdinLine(stdin)
		}
		if st.err != nil {
			return resolvedInput{}, st.err
		}
		v, err := ParseVersion(st.value)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("<stdin>: %w", err)
		}
		ri := resolvedInput{originFile: "<stdin>"}
		ri.fields = []locatedField{{File: ri.originFile, Value: v.String()}}
		return ri, nil
	}

	if strings.HasPrefix(arg, "vcs:") {
		return resolveVcsInput(arg, borrowedFile, backend)
	}

	if strings.HasPrefix(arg, "cmd:") {
		return resolveCmdInput(arg)
	}

	// Try as file first if it exists. Use Stat so we don't masquerade
	// directories or sockets as parseable VERs.
	if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
		return resolveFile(arg)
	}

	// Try as VER.
	if v, err := ParseVersion(arg); err == nil {
		label := "<argv>"
		if totalVERorStdin > 1 {
			label = fmt.Sprintf("<argv:%d>", argIdx)
		}
		ri := resolvedInput{originFile: label}
		ri.fields = []locatedField{{File: ri.originFile, Value: v.String()}}
		// Preserve the input string verbatim so prefix/sep round-trip.
		ri.fields[0].Value = strings.TrimSpace(arg)
		return ri, nil
	}

	return resolvedInput{}, fmt.Errorf("%q is neither a file nor a valid version", arg)
}

func resolveFile(file string) (resolvedInput, error) {
	h, err := detectHandler(file)
	if err != nil {
		return resolvedInput{}, err
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("read %s: %w", file, err)
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, err
	}
	ri := resolvedInput{
		originFile: file,
		fields:     locatedFromInspection(file, insp.Versions),
		file:       file,
		handler:    h,
		content:    content,
		insp:       insp,
	}
	return ri, nil
}

// resolveFileFromStdin handles the legacy "single FILE + stdin pipe"
// shortcut: the path is treated as a name hint and content is read
// from stdin.
func resolveFileFromStdin(file string, stdin io.Reader) (resolvedInput, error) {
	h, err := detectHandler(file)
	if err != nil {
		return resolvedInput{}, err
	}
	content, err := io.ReadAll(stdin)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("read stdin: %w", err)
	}
	insp, err := h.Inspect(content)
	if err != nil {
		return resolvedInput{}, err
	}
	ri := resolvedInput{
		originFile: file,
		fields:     locatedFromInspection(file, insp.Versions),
		file:       "", // stdin pipe: do not allow writeback (already rejected at parse time)
		handler:    h,
		content:    content,
		insp:       insp,
	}
	return ri, nil
}

func readStdinLine(stdin io.Reader) (string, error) {
	br := bufio.NewReader(stdin)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("stdin: empty input")
	}
	return line, nil
}

// wrapOriginErr prefixes a semver-layer error with its origin context
// (確定論点 E). For FILE-origin inputs we include the version path.
func wrapOriginErr(originLabel, value string, err error) error {
	if err == nil {
		return nil
	}
	// VER / stdin origin: keep the message as-is.
	if !strings.Contains(originLabel, ":") && (originLabel == "<argv>" ||
		strings.HasPrefix(originLabel, "<argv:") || originLabel == "<stdin>") {
		if originLabel == "<stdin>" {
			return fmt.Errorf("<stdin> (%s): %w", value, err)
		}
		return err
	}
	// FILE-origin: "<file>:<path>=<value>: <semver-error>" form.
	return fmt.Errorf("%s=%s: %w", originLabel, value, err)
}

// run is the testable entry point. It always returns nil on success or
// an *exitErr on failure (so main only has to translate the carried code
// into a process exit status). User-facing diagnostics — including the
// "bump-semver: <reason>" prefix — are written to stderr from here so
// the --quiet-all flag can suppress them.
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	args, err := parseArgs(argv)
	if err != nil {
		// parse errors precede any quiet flag taking effect (the flag
		// itself may be malformed). Always print to stderr.
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		return &exitErr{code: 2}
	}
	switch args.kind {
	case "version":
		if args.output.JSON {
			v, perr := ParseVersion(version)
			if perr != nil {
				return emitErr(stderr, args, fmt.Errorf("parse own version %q: %w", version, perr))
			}
			data, mErr := marshalJSONOutput(v.ToJSON(nil))
			if mErr != nil {
				return emitErr(stderr, args, fmt.Errorf("marshal json: %w", mErr))
			}
			_, _ = stdout.Write(data)
			return nil
		}
		fmt.Fprintln(stdout, version)
		return nil
	case "help":
		fmt.Fprint(stdout, shortHelpText)
		return nil
	case "helpFull":
		fmt.Fprint(stdout, fullHelpText)
		return nil
	case "helpAction":
		text, ok := actionHelpTexts[args.action]
		if !ok {
			// defensive: parseArgs should only set helpAction for
			// known actions; fall back to the short help so the
			// caller still sees something useful.
			fmt.Fprint(stdout, shortHelpText)
			return nil
		}
		fmt.Fprint(stdout, text)
		return nil
	case "compare":
		return runCompare(args, stdin, stdout, stderr)
	case "vcs":
		return runVcsCmd(args, stdin, stdout, stderr)
	}

	return runBump(args, stdin, stdout, stderr)
}

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
	default:
		return emitVcsUsage(stderr, args,
			fmt.Errorf("unknown vcs verb: %s (expected: get / is / diff / commit / fetch / push / tag)", args.vcsVerb))
	}
}

// vcsGetKeys lists the keys recognised by `vcs get`. Kept as a slice so
// the order is preserved when we surface it in error messages.
var vcsGetKeys = []string{"root", "backend", "current-branch"}

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

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// -q / -qq both suppress the stdout value (the exit code carries the
	// information the caller actually needs in scripted contexts).
	emit := func(s string) {
		if args.output.Quiet || args.output.QuietAll {
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

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
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
		// The parseArgs layer normally short-circuits "vcs diff" with no
		// further args to the per-verb help; this branch only fires if a
		// future code path reaches the dispatcher with an empty arg list.
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs diff requires a REV (usage: vcs diff REV [PATH..])"))
	}
	rev := args.vcsArgs[0]
	paths := args.vcsArgs[1:]

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}

	// -q (and -qq) trigger the predicate-only path: derive presence from
	// name-status output (cheap; same shape feeds -s display). Doing this
	// before -s keeps `-q` strictly authoritative when both are set.
	if args.output.Quiet || args.output.QuietAll {
		ns, err := b.DiffNameStatus(rev, paths)
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
		out, err := b.DiffNameStatus(rev, paths)
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
	out, err := b.Diff(rev, paths)
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
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
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
	// Step 6: dispatch.
	opts := commitOpts{
		paths:   args.vcsArgs,
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
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
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
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // validated in parseArgs
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	// PR-5.2: --jj-bookmark-auto-advance is jj-only. Reject at the
	// dispatcher (exit 2 + hint naming the flag and the jj-specific
	// reason) so the message tells the user exactly what to drop. The
	// gitBackend.Push also has a defensive reject for the unreachable
	// case, but the user-facing diagnostic must come from here — the
	// backend's "please file a bug" wording is meant for the
	// unreachable branch.
	if args.vcsPush.JjBookmarkAutoAdvance && b.Kind() == "git" {
		return emitVcsUsage(stderr, args,
			fmt.Errorf("vcs push: --jj-bookmark-auto-advance is jj-specific; this repo uses git (drop the flag, or run from a jj workspace)"))
	}
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
	if !args.output.QuietAll && !args.output.Quiet {
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
	name := args.vcsArgs[0]
	if err := validTagName(name); err != nil {
		return emitVcsUsage(stderr, args, fmt.Errorf("vcs tag push: %w", err))
	}
	remote := "origin"
	if args.vcsPush.Remote != nil {
		remote = *args.vcsPush.Remote
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
	if !args.output.QuietAll && !args.output.Quiet {
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
	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, ""))
	b, err := newVcsBackend(vcsOverride)
	if err != nil {
		return emitVcsErr(stderr, args, err)
	}
	opts := tagDeleteOpts{Name: name, Remote: remote}
	if !args.output.QuietAll && !args.output.Quiet {
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
	if !args.output.QuietAll {
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
	if !args.output.QuietAll {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee
	}
	return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
}

// emitErr writes a "bump-semver: <reason>" line to stderr unless the
// caller requested -qq/--quiet-all, then returns *exitErr{code: 2}
// carrying the same message so callers (especially tests) can still
// inspect err.Error() for substrings. main does NOT re-print the
// message because run() has already done so.
//
// DR-0010: when err is an *unsupportedFileError, we follow the
// "bump-semver:" line with a one-line `hint:` pointing the user at
// the issue tracker. The hint is suppressed by `--no-hint` / `-q` /
// `-qq` to match every other DR-0010 hint (and every other v0.5.0
// stderr-side hint).
func emitErr(stderr io.Writer, args cliArgs, err error) error {
	if !args.output.QuietAll {
		fmt.Fprintln(stderr, "bump-semver: "+err.Error())
		var ufe *unsupportedFileError
		if errors.As(err, &ufe) && !args.output.Quiet && !args.output.NoHint {
			fmt.Fprintln(stderr, "hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.")
		}
	}
	return &exitErr{code: 2, msg: err.Error()}
}

// emitFallbackHints prints the DR-0010 fallback hint and the DR-0013
// suffix-stripping hint for FILE-origin resolved inputs. The hints are
// suppressed by `--no-hint` / `-q` / `-qq` and printed before any
// other stderr hint so they appear in event order (rule-resolution →
// bump action).
//
// `compare` also goes through this helper because the hints reflect
// the file detection, not the action — passing an unrecognised
// filename to `compare` is just as informative as to `get` / bump.
//
// When both hints fire for the same input (e.g. `unknown.json.bak`:
// suffix `.bak` stripped → `*.json` glob fallback), they are emitted
// in **source order** (suffix stripping is the filename-level
// observation; the *.json fallback is the content-level observation).
// Both share the `hint:` prefix so a single grep / `--no-hint` flag
// captures both.
func emitFallbackHints(stderr io.Writer, args cliArgs, resolved []resolvedInput) {
	if args.output.Quiet || args.output.QuietAll || args.output.NoHint {
		return
	}
	for _, ri := range resolved {
		if ri.handler == nil || ri.file == "" {
			continue
		}
		// DR-0013: suffix stripping happened first (filename-level),
		// so emit it before the DR-0010 fallback hint.
		if suffix := ri.insp.MatchedSuffixStripped; suffix != "" {
			stripped := ri.insp.MatchedStrippedBasename
			if stripped == "" {
				stripped = "(unknown)"
			}
			fmt.Fprintf(stderr,
				"hint: %s matched as %s rule (suffix %s stripped); use --no-hint to suppress\n",
				ri.file, stripped, suffix)
		}
		if ri.insp.MatchedConfidence != 1 {
			continue
		}
		glob := ri.insp.MatchedGlob
		if glob == "" {
			// Defensive: if the rule had no Glob recorded for some
			// reason, fall back to a generic phrasing rather than
			// printing "matched as  fallback".
			glob = "fallback"
		}
		fmt.Fprintf(stderr, "hint: %s matched as %s fallback. Open issue if explicit support is needed.\n", ri.file, glob)
	}
}

func runBump(args cliArgs, stdin io.Reader, stdout, stderr io.Writer) error {
	// DR-0008: --write + any read-only input (vcs: / cmd:) is rejected up-front.
	// Both schemas resolve to a value without a writable backing file —
	// writing back would require commit/amend semantics for vcs: or
	// process re-execution for cmd:, both far out of scope. Silently
	// dropping the read-only portion of a multi-input --write would
	// surprise users. The cleanest answer is to refuse the combination
	// and let the caller split the invocation.
	if args.write {
		for _, in := range args.inputs {
			if strings.HasPrefix(in, "vcs:") {
				return emitErr(stderr, args, fmt.Errorf("--write cannot be used with vcs: inputs (vcs: is read-only)"))
			}
			if strings.HasPrefix(in, "cmd:") {
				return emitErr(stderr, args, fmt.Errorf("--write cannot be used with cmd: inputs (cmd: is read-only)"))
			}
		}
	}

	vcsOverride, _ := parseVcsOverride(derefOr(args.vcsBase.Override, "")) // already validated in parseArgs
	// PeerExpand=true: bump/get want N-arg cross-source equality
	// across all sibling FILE paths when a file-omitted vcs:REV is
	// present (DR-0023). Compare uses PeerExpand=false.
	resolved, err := resolveInputs(args.inputs, stdin, resolveInputsOpts{
		Write:      args.write,
		VCSKind:    vcsOverride,
		PeerExpand: true,
	})
	if err != nil {
		return emitErr(stderr, args, err)
	}

	// DR-0010: warn the user about confidence-1 fallback matches before
	// doing the actual bump, so the hint appears in event order
	// (rule-resolution → bump). Suppression flags are honored inside the
	// helper.
	emitFallbackHints(stderr, args, resolved)

	// Validate --write has at least one FILE-origin input early, before
	// any side effects (printing the bumped version).
	if args.write {
		writable := 0
		for _, ri := range resolved {
			if ri.handler != nil && ri.file != "" {
				writable++
			}
		}
		if writable == 0 {
			return emitErr(stderr, args, fmt.Errorf("--write requires at least one FILE"))
		}
	}

	// Aggregate every detected version field, across all inputs, with
	// origin provenance.
	var allVersions []locatedField
	for _, ri := range resolved {
		allVersions = append(allVersions, ri.fields...)
	}
	cur, ok := allSameValue(allVersions)
	if !ok {
		// DR-0023: get treats all sources as equal peers. A
		// disagreement is a predicate-false outcome (exit 1) with the
		// per-source listing on stderr — mirroring compare's
		// false-with-diagnostic shape rather than the bump-time
		// "internal data is wrong, refuse to act" exit 2. Genuine
		// bump actions (major/minor/patch/pre) still flow through
		// emitErr (exit 2) because inconsistent inputs there are an
		// error condition, not a queryable result.
		if args.action == "get" {
			if !args.output.QuietAll {
				fmt.Fprintln(stderr, formatMismatchError("version", allVersions).Error())
			}
			return &exitErr{code: exitCodeFalse}
		}
		return emitErr(stderr, args, formatMismatchError("version", allVersions))
	}

	// Aggregate names across FILE-origin entries (VER/stdin contribute none).
	var allNames []locatedField
	for _, ri := range resolved {
		if ri.file != "" || ri.handler != nil {
			allNames = append(allNames, locatedFromInspection(ri.originFile, ri.insp.Names)...)
		}
	}
	if _, ok := allSameValue(allNames); len(allNames) > 0 && !ok {
		return emitErr(stderr, args, formatMismatchError("name", allNames))
	}

	// Use the first contributing field as the origin source for parse
	// errors (any one would work; they're all equal by construction).
	origin := allVersions[0]

	v, err := ParseVersion(cur)
	if err != nil {
		return emitErr(stderr, args, wrapOriginErr(origin.label(), cur, err))
	}
	// Bridge cliArgs.bump (*string) → BumpOptions (string + Set bool).
	// BumpOptions stays as-is to avoid expanding the refactor blast
	// radius into semver.go / its tests (PR-Simplify-1 is cliArgs-only).
	opts := BumpOptions{
		Pre:              derefOr(args.bump.Pre, ""),
		PreSet:           args.bump.Pre != nil,
		NoPre:            args.bump.NoPre,
		BuildMetadata:    derefOr(args.bump.BuildMetadata, ""),
		BuildMetadataSet: args.bump.BuildMetadata != nil,
		NoBuildMetadata:  args.bump.NoBuildMetadata,
	}
	newV, err := v.Bump(args.action, opts)
	if err != nil {
		return emitErr(stderr, args, wrapOriginErr(origin.label(), cur, err))
	}

	// Hint output (Phase 5): bump-only, when at least one FILE was given
	// but --write was not, and no quiet/no-hint flag suppresses it. The
	// hint reminds users that a successful bump did not touch any file.
	if shouldShowHint(args, resolved) {
		n := countFileInputs(resolved)
		suffix := "files"
		if n == 1 {
			suffix = "file"
		}
		fmt.Fprintf(stderr, "hint: %d %s not modified; use --write to update or --no-hint to suppress\n", n, suffix)
	}

	// stdout output (suppressed by -q/-qq). With --json the bumped/get
	// version is rendered as a single-line JSON object (DR-0007); the
	// `name` field is populated from the cross-input-validated set of
	// FILE-origin names (which DR-0004 already collapses to one value).
	if !args.output.Quiet && !args.output.QuietAll {
		if args.output.JSON {
			var name *string
			if len(allNames) > 0 {
				n := allNames[0].Value
				name = &n
			}
			data, mErr := marshalJSONOutput(newV.ToJSON(name))
			if mErr != nil {
				return emitErr(stderr, args, fmt.Errorf("marshal json: %w", mErr))
			}
			_, _ = stdout.Write(data)
		} else {
			fmt.Fprintln(stdout, newV.String())
		}
	}

	if args.write {
		for _, ri := range resolved {
			if ri.handler == nil || ri.file == "" {
				continue
			}
			out, err := ri.handler.Replace(ri.content, cur, newV.String())
			if err != nil {
				return emitErr(stderr, args, fmt.Errorf("replace %s: %w", ri.file, err))
			}
			mode := os.FileMode(0644)
			if fi, statErr := os.Stat(ri.file); statErr == nil {
				mode = fi.Mode().Perm()
			}
			if err := os.WriteFile(ri.file, out, mode); err != nil {
				return emitErr(stderr, args, fmt.Errorf("write %s: %w", ri.file, err))
			}
		}
	}
	return nil
}

// shouldShowHint returns true when runBump should emit the
// "files not modified" hint. The hint is bump-specific (not get) and
// only meaningful when the user passed at least one FILE input but
// omitted --write — it surfaces a successful no-op write.
func shouldShowHint(args cliArgs, resolved []resolvedInput) bool {
	if args.kind != "bump" {
		return false
	}
	switch args.action {
	case "major", "minor", "patch", "pre":
	default:
		return false // get is read-only, never has a "modified" outcome
	}
	if args.write {
		return false
	}
	if args.output.Quiet || args.output.QuietAll || args.output.NoHint {
		return false
	}
	return countFileInputs(resolved) > 0
}

// countFileInputs returns the number of FILE-origin resolved inputs
// (i.e. anything whose Inspect succeeded against an on-disk file). VER
// and stdin (`-`) inputs are not counted because they were never going
// to be "modified" in the first place.
func countFileInputs(resolved []resolvedInput) int {
	n := 0
	for _, ri := range resolved {
		if ri.handler != nil && ri.file != "" {
			n++
		}
	}
	return n
}

// resolveInputs walks the positional inputs and resolves each. It also
// handles the legacy "single FILE + stdin pipe" shortcut: if exactly
// one input is present, that input is a FILE path (not `-`, not a VER
// pattern), and stdin is a pipe, the FILE's content is read from stdin.
//
// vcsOverride is the parsed --vcs flag value (vcsAuto when absent). The
// VCS itself is detected lazily — only when at least one input is
// `vcs:` — so non-vcs invocations don't error out in environments
// without a `.jj` / `.git` directory.
//
// File-borrowing for `vcs:REV` (no explicit FILE) has two modes
// (DR-0023):
//
//   - peerExpand=false (compare): each file-omitted `vcs:REV`
//     borrows the **first** FILE-providing sibling. Used by compare
//     because its semantic is "F1 (base) vs each OTHER" — F1 always
//     wins as the borrow source by construction.
//
//   - peerExpand=true (bump/get): each file-omitted `vcs:REV`
//     expands to one resolved input per **distinct** sibling FILE
//     path. This lets `get a b vcs:main` mean "compare a, b, and
//     both files at main" with no shell loop.
//
// Borrow-source candidates (in both modes) are:
//
//   - a real FILE-origin input (`Cargo.toml`)
//   - another `vcs:REV:FILE` input that names its file explicitly
//
// When every vcs: input omits FILE *and* there's no real FILE-origin,
// we error out — there's nothing to borrow from.
// resolveInputsOpts packs the three behaviour flags that customise
// resolveInputs. Previously these were three positional bool /
// enum-after-bool arguments (`write, vcsOverride, peerExpand`) — easy
// to swap at the call site without the compiler noticing. Keyed
// struct construction at the two callers (runBump / runCompare) makes
// each flag's role obvious.
type resolveInputsOpts struct {
	// Write toggles the "--write requested" assertion: when true, the
	// stdin-pipe shortcut errors out (writing into a pipe is undefined).
	Write bool
	// VCSKind is the parsed --vcs override (vcsAuto when absent). The
	// VCS itself is detected lazily — only when at least one input is
	// `vcs:` — so non-vcs invocations don't error out in environments
	// without a `.jj` / `.git` directory.
	VCSKind vcsKind
	// PeerExpand controls the file-omitted `vcs:REV` borrow shape
	// (DR-0023):
	//   - false (compare): borrow the *first* FILE-providing sibling
	//   - true  (bump/get): expand to one resolved entry per distinct
	//     sibling FILE path
	PeerExpand bool
}

func resolveInputs(inputs []string, stdin io.Reader, opts resolveInputsOpts) ([]resolvedInput, error) {
	// Pre-classify each input. We need three buckets:
	//   - "raw" (VER, `-`, or `vcs:`): contributes to <argv:N> indexing
	//   - "file": exists on disk
	//   - "vcs": needs lazy VCS detection
	// "raw" subsumes vcs because vcs: is not a path on disk; a `vcs:`
	// arg should not be counted as a writable FILE either.
	//
	// The borrow target is "file-providing arg(s) in position order".
	// A `vcs:REV:FILE` qualifies because its FILE is unambiguous; a
	// `vcs:REV` (no file) does not.
	//
	// Two borrow-set representations are maintained because compare
	// (peerExpand=false) uses only the *first* file (fileForBorrow),
	// while bump/get (peerExpand=true) expands a file-omitted vcs:
	// to one resolved entry per *distinct* sibling FILE path
	// (borrowFiles).
	isRaw := make([]bool, len(inputs))
	rawCount := 0
	hasVcs := false
	var fileForBorrow string // first FILE-providing input, if any
	var borrowFiles []string // every distinct FILE path, in position order
	seenBorrow := make(map[string]bool)
	addBorrow := func(file string) {
		if file == "" || seenBorrow[file] {
			return
		}
		seenBorrow[file] = true
		borrowFiles = append(borrowFiles, file)
		if fileForBorrow == "" {
			fileForBorrow = file
		}
	}
	for i, in := range inputs {
		if in == "-" {
			isRaw[i] = true
			rawCount++
			continue
		}
		if strings.HasPrefix(in, "vcs:") {
			isRaw[i] = true
			rawCount++
			hasVcs = true
			// `vcs:REV:FILE` (file-explicit) qualifies as a borrow
			// source for downstream `vcs:REV` (file-omitted) args.
			if _, file, isFunc, _ := vcsParseSpec(in); !isFunc && file != "" {
				addBorrow(file)
			}
			continue
		}
		if fi, err := os.Stat(in); err == nil && !fi.IsDir() {
			addBorrow(in)
			continue // exists as a file
		}
		isRaw[i] = true
		rawCount++
	}

	// Legacy stdin pipe shortcut: one FILE input (not `-`, exists or
	// at least matches a known rule), stdin is a pipe → read content
	// from stdin and treat the path as a name hint. vcs: inputs are
	// not eligible for this shortcut.
	if len(inputs) == 1 && inputs[0] != "-" && !strings.HasPrefix(inputs[0], "vcs:") && isStdinPipe(stdin) {
		if opts.Write {
			return nil, fmt.Errorf("--write is incompatible with stdin pipe input")
		}
		if pathHasAnyRule(inputs[0]) {
			ri, err := resolveFileFromStdin(inputs[0], stdin)
			if err != nil {
				return nil, err
			}
			return []resolvedInput{ri}, nil
		}
	}

	// Detect VCS lazily — only when at least one input is `vcs:`.
	// Detecting up-front would error out in repos that don't use
	// `vcs:` syntax, even though they're valid bump-semver targets.
	var backend vcsBackend
	if hasVcs {
		b, err := newVcsBackend(opts.VCSKind)
		if err != nil {
			return nil, err
		}
		backend = b
	}

	st := stdinReadState{}
	out := make([]resolvedInput, 0, len(inputs))
	rawIdx := 0
	for i, in := range inputs {
		argIdx := 0
		if isRaw[i] {
			rawIdx++
			argIdx = rawIdx
		}
		// vcs: rev-mode specs without a FILE component borrow the
		// path from FILE-origin sibling(s). The expansion shape
		// depends on peerExpand:
		//
		//   - peerExpand=true (bump/get): if the spec is a
		//     file-omitted `vcs:REV` AND there are 2+ borrow files,
		//     expand to one resolved entry per borrow file. This
		//     turns `get a b vcs:main` into {a, b, vcs:main:a,
		//     vcs:main:b}, the cross-source equality check users
		//     want when comparing the working tree against a
		//     historical snapshot for several files at once.
		//
		//   - peerExpand=false (compare): always borrow the *first*
		//     file. Compare's F1 (= leftmost) is the comparison
		//     base, so OTHERS borrowing F1's path is exactly the
		//     "is OTHER's snapshot of F1 OP F1?" semantic.
		if opts.PeerExpand && strings.HasPrefix(in, "vcs:") {
			if rev, file, isFunc, _ := vcsParseSpec(in); !isFunc && file == "" && len(borrowFiles) > 1 {
				_ = rev
				for _, bf := range borrowFiles {
					ri, err := resolveInput(in, argIdx, rawCount, stdin, &st, backend, bf)
					if err != nil {
						return nil, err
					}
					out = append(out, ri)
				}
				continue
			}
		}
		ri, err := resolveInput(in, argIdx, rawCount, stdin, &st, backend, fileForBorrow)
		if err != nil {
			return nil, err
		}
		out = append(out, ri)
	}
	return out, nil
}

func isStdinPipe(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}
