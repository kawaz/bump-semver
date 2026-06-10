package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// This file holds the custom pflag.Value implementations the cobra
// command tree uses to reproduce the legacy hand-rolled parser's
// behaviour that pflag does not provide out of the box (plan §3.3 /
// §3.7 / §3.8):
//
//   - "specified twice" rejection for value-taking flags (pflag's
//     default is last-wins, the legacy parser errors instead);
//   - the --glob-* family's "=true/=false required" / "bare = true"
//     polarity rules, with the legacy error wording;
//   - --branch / --bookmark aliasing onto a single slot;
//   - --excludes append (repeatable, empty-value rejection).
//
// Every Value returns the *final* legacy error string from Set(); the
// FlagErrorFunc (cobra_errors.go) recognises these self-authored errors
// and emits them verbatim instead of reshaping them into the generic
// unknown-flag / requires-a-value wording.

// onceStringValue is a string flag that may be supplied at most once.
// The second Set() reports `<name> specified twice` (the legacy wording)
// and writes the captured value through into a *string slot (nil = unset
// is preserved, matching the cliArgs sub-struct pointer fields).
type onceStringValue struct {
	name string   // display name in the duplicate error ("--vcs", "--pre", ...)
	slot **string // address of the cliArgs *string field to populate
	set  bool
}

func newOnceString(name string, slot **string) *onceStringValue {
	return &onceStringValue{name: name, slot: slot}
}

func (v *onceStringValue) Set(s string) error {
	if v.set {
		return fmt.Errorf("%s specified twice", v.name)
	}
	v.set = true
	val := s
	*v.slot = &val
	return nil
}

func (v *onceStringValue) Type() string { return "string" }

func (v *onceStringValue) String() string {
	if v.set && *v.slot != nil {
		return **v.slot
	}
	return ""
}

// onceBoolValue is a boolean flag (no argument) that may be supplied at
// most once. The legacy parser rejects a repeated --write / --no-pre /
// --no-build-metadata with `<name> specified twice` rather than the
// last-wins pflag default; this reproduces that. It is registered with
// NoOptDefVal = "true" so the bare flag form consumes no argument.
type onceBoolValue struct {
	name string // "--write" / "--no-pre" / "--no-build-metadata"
	slot *bool
	set  bool
}

func newOnceBool(name string, slot *bool) *onceBoolValue {
	return &onceBoolValue{name: name, slot: slot}
}

func (v *onceBoolValue) Set(string) error {
	if v.set {
		return fmt.Errorf("%s specified twice", v.name)
	}
	v.set = true
	*v.slot = true
	return nil
}

func (v *onceBoolValue) Type() string { return "bool" }

func (v *onceBoolValue) String() string {
	if v.set && *v.slot {
		return "true"
	}
	return "false"
}

// addOnceBool registers a onceBoolValue on fs and sets NoOptDefVal so the
// bare flag form (no `=value`) consumes no argument and toggles it on.
func addOnceBool(fs *pflag.FlagSet, v *onceBoolValue, name, usage string) {
	fs.Var(v, name, usage)
	fs.Lookup(name).NoOptDefVal = "true"
}

// excludesValue implements `--excludes PATTERN` (DR-0033): repeatable +
// append, with an empty value rejected as a usage error (matching the
// legacy "--excludes value must not be empty" wording). The bare /
// missing-value form ("--excludes" with no value) is handled by the
// FlagErrorFunc requiresValueMsg table, not here.
type excludesValue struct {
	slot *[]string
}

func (v *excludesValue) Set(s string) error {
	if s == "" {
		return fmt.Errorf("--excludes value must not be empty")
	}
	*v.slot = append(*v.slot, s)
	return nil
}

func (v *excludesValue) Type() string   { return "string" }
func (v *excludesValue) String() string { return "" }

// globBoolValue implements the --glob-dotfile / --glob-gitignored /
// --glob-ignorecase family (DR-0024). The polarity rules differ per flag
// and are reproduced from parseGlobFlag:
//
//   - dotfile / gitignored: an explicit =true/=false is required; the
//     bare form is a usage error. pflag would otherwise treat the bare
//     form as "flag needs an argument", so the flag is registered with
//     NoOptDefVal = bareGlobSentinel and Set() turns that sentinel back
//     into the legacy "requires =true or =false" wording.
//   - ignorecase: the bare form means true (NoOptDefVal = "true"); an
//     explicit =true/=false is also accepted.
//
// The actual value validation reuses parseBoolValue (temporarily kept in
// cli_parse.go) so the "requires true or false, got %q" wording stays
// identical.
type globBoolValue struct {
	name      string // "--glob-dotfile" / "--glob-gitignored" / "--glob-ignorecase"
	bareIsErr bool   // dotfile / gitignored: bare form is an error
	apply     func(bool)
}

// bareGlobSentinel is the NoOptDefVal stand-in for the bare
// --glob-dotfile / --glob-gitignored forms. It is an otherwise
// impossible value (a NUL-wrapped token never appears in real argv) so
// Set() can detect "the flag was given without =value" and emit the
// legacy error.
const bareGlobSentinel = "\x00glob-bare\x00"

func (v *globBoolValue) Set(s string) error {
	if v.bareIsErr && s == bareGlobSentinel {
		// Mirror parseGlobFlag's per-flag example suffix.
		example := "false"
		if v.name == "--glob-gitignored" {
			example = "true"
		}
		return fmt.Errorf("%s requires =true or =false (e.g. %s=%s)", v.name, v.name, example)
	}
	b, err := parseBoolValue(v.name, s)
	if err != nil {
		return err
	}
	v.apply(b)
	return nil
}

func (v *globBoolValue) Type() string   { return "bool" }
func (v *globBoolValue) String() string { return "" }

// verbosityRaiseValue is a no-argument flag that raises a shared
// outputVerbosity to a fixed level on each occurrence (matching the
// legacy `out.output.Verbosity.raise(...)` calls). It is registered with
// NoOptDefVal so it consumes no argument; the `-qq` / --quiet-all level
// is reached via the --quiet-all long flag (the `-qq` token is rewritten
// to --quiet-all before cobra parses — see normalizeQuietAll).
type verbosityRaiseValue struct {
	slot  *outputVerbosity
	level outputVerbosity
}

func (v *verbosityRaiseValue) Set(string) error { v.slot.raise(v.level); return nil }
func (v *verbosityRaiseValue) Type() string     { return "bool" }
func (v *verbosityRaiseValue) String() string   { return "" }

// addVerbosityFlags registers -q/--quiet, --quiet-all and --no-hint on
// fs, all raising the shared verbosity. `-qq` is intentionally NOT a
// shorthand here (pflag would tokenise it as `-q -q` = quiet, not
// quiet-all); runCobra rewrites the literal `-qq` token to --quiet-all
// before parsing.
func addVerbosityFlags(fs *pflag.FlagSet, v *outputVerbosity) {
	quiet := &verbosityRaiseValue{slot: v, level: outputQuiet}
	fs.VarP(quiet, "quiet", "q", "suppress stdout and hints")
	fs.Lookup("quiet").NoOptDefVal = "x"

	quietAll := &verbosityRaiseValue{slot: v, level: outputQuietAll}
	fs.Var(quietAll, "quiet-all", "suppress stdout, hints, and errors (use with caution)")
	fs.Lookup("quiet-all").NoOptDefVal = "x"

	noHint := &verbosityRaiseValue{slot: v, level: outputNoHint}
	fs.Var(noHint, "no-hint", "suppress hints only")
	fs.Lookup("no-hint").NoOptDefVal = "x"
}

// valueTakingFlags derives the set of flag tokens that consume a separate
// argument (NoOptDefVal == "" in pflag terms), walking the entire cobra
// command tree from newRootCmd. Both spellings are recorded: the long
// form `--name` and, when present, the shorthand `-x`. Bool-like flags
// (verbosity raisers, --write, glob flags, etc.) set NoOptDefVal and are
// therefore excluded — they never swallow the next token.
//
// The result feeds normalizeQuietAll so a `-qq` that is the *value* of a
// value-taking flag (e.g. `--pre -qq`, `-m -qq`) is left literal instead
// of being rewritten to --quiet-all. Deriving from the FlagSet (rather
// than hardcoding a list) keeps the guard in sync as flags are added.
func valueTakingFlags() map[string]bool {
	set := map[string]bool{}
	root := newRootCmd(nil, io.Discard, io.Discard)

	var visit func(cmd *cobra.Command)
	visit = func(cmd *cobra.Command) {
		collect := func(f *pflag.Flag) {
			if f.NoOptDefVal != "" {
				return // bare form consumes no argument
			}
			set["--"+f.Name] = true
			if f.Shorthand != "" {
				set["-"+f.Shorthand] = true
			}
		}
		cmd.Flags().VisitAll(collect)
		cmd.PersistentFlags().VisitAll(collect)
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
	return set
}

// normalizeQuietAll rewrites a standalone `-qq` token to `--quiet-all`,
// but only when it appears in a flag position. pflag cannot represent
// `-qq` as a single shorthand (it tokenises it as `-q -q` = quiet), so
// this runs over argv before cobra parses.
//
// Two boundaries leave a `-qq` untouched:
//   - anything at/after a `--` separator (post-separator positionals are
//     never reinterpreted, matching the legacy whole-token match)
//   - a `-qq` that sits in a value position: the immediately preceding
//     token is a value-taking flag passed in its separate-argument form
//     (e.g. `--pre -qq`, `-m -qq`). An inline `--flag=value` does not
//     consume the next token, so a following `-qq` is a flag position again.
//
// valueFlags is the set produced by valueTakingFlags().
func normalizeQuietAll(argv []string, valueFlags map[string]bool) []string {
	out := make([]string, 0, len(argv))
	prevConsumesValue := false
	for i, a := range argv {
		if a == "--" {
			out = append(out, argv[i:]...)
			break
		}
		if a == "-qq" && !prevConsumesValue {
			out = append(out, "--quiet-all")
			prevConsumesValue = false
			continue
		}
		// A value-taking flag in `--flag value` / `-x value` form swallows
		// the next token; the `=`-joined form (`--flag=value`) does not.
		prevConsumesValue = valueFlags[a]
		out = append(out, a)
	}
	return out
}

// ruleEvent records one rule-related flag occurrence (--define-rule or a
// rule-definition flag) together with its value. DR-0029's block model is
// argv-order-dependent, and pflag does not preserve the relative order of
// different flags — but a custom pflag.Value's Set() IS called in argv
// order, so ruleFlagValue appends to a shared recorder to reconstruct it.
type ruleEvent struct {
	flag  string // "--define-rule" / "--format" / "--version-path" / ...
	value string
}

// ruleRecorder collects ruleEvents in argv order across all rule flags.
type ruleRecorder struct {
	events []ruleEvent
}

// ruleFlagValue is the custom pflag.Value backing one rule-related flag.
// Several of them (one per flag name) share a single recorder so the
// combined Set() call sequence reflects the argv order of the whole rule
// flag family. Validation is deferred to buildRuleBlocks (replayed in
// order) so the legacy assignRuleFlag / --define-rule wording is reused
// verbatim.
type ruleFlagValue struct {
	rec  *ruleRecorder
	flag string
}

func (v *ruleFlagValue) Set(s string) error {
	v.rec.events = append(v.rec.events, ruleEvent{flag: v.flag, value: s})
	return nil
}

func (v *ruleFlagValue) Type() string   { return "string" }
func (v *ruleFlagValue) String() string { return "" }

// buildRuleBlocks replays the recorded rule events in argv order, driving
// the unchanged ensureRuleBlocks / assignRuleFlag logic so every DR-0029
// error message (empty PATTERN, duplicate in block, invalid format, the
// 0a 補強 rule) stays byte-identical to the legacy parser. ruleBlocks
// stays nil (lazy init) when no rule flag was given.
func buildRuleBlocks(rec *ruleRecorder) (blocks []ruleBlock, hasDefineRule bool, err error) {
	var out cliArgs
	for _, ev := range rec.events {
		switch ev.flag {
		case "--define-rule":
			if ev.value == "" {
				return nil, false, fmt.Errorf("--define-rule PATTERN cannot be empty")
			}
			ensureRuleBlocks(&out)
			out.ruleBlocks = append(out.ruleBlocks, ruleBlock{Pattern: ev.value})
			out.hasDefineRule = true
		case "--format":
			if err := assignRuleFlag(&out, "--format", "Format", ev.value); err != nil {
				return nil, false, err
			}
		case "--version-path":
			if err := assignRuleFlag(&out, "--version-path", "VersionPath", ev.value); err != nil {
				return nil, false, err
			}
		case "--version-regex":
			if err := assignRuleFlag(&out, "--version-regex", "VersionRegex", ev.value); err != nil {
				return nil, false, err
			}
		case "--name-path":
			if err := assignRuleFlag(&out, "--name-path", "NamePath", ev.value); err != nil {
				return nil, false, err
			}
		case "--name-regex":
			if err := assignRuleFlag(&out, "--name-regex", "NameRegex", ev.value); err != nil {
				return nil, false, err
			}
		}
	}
	return out.ruleBlocks, out.hasDefineRule, nil
}

// sharedBumpFlags carries the per-invocation state for the bump/compare
// shared flag group: the bool slots cobra writes directly, plus the rule
// recorder whose events are replayed in buildRuleBlocks. The string /
// once-string flags write straight into the cliArgs sub-structs.
type sharedBumpFlags struct {
	rules ruleRecorder
}

// addSharedBumpFlags registers the flag group shared by `compare` and the
// bump/get actions (plan §2 Stage 3 item 2): --write, --pre / --no-pre,
// --build-metadata / --no-build-metadata, --json, --vcs, the verbosity
// flags, the --glob-* family and the DR-0029 rule flags. Every value-
// taking flag reuses the legacy wording via a custom Value or the
// FlagErrorFunc requiresValueMsg table. Returns the shared state whose
// rule recorder the caller replays in its RunE.
func addSharedBumpFlags(cmd *cobra.Command, args *cliArgs) *sharedBumpFlags {
	st := &sharedBumpFlags{}
	f := cmd.Flags()

	// --write / --no-pre / --no-build-metadata reject a second occurrence
	// (legacy "<flag> specified twice"); --json / --no-hint are idempotent
	// (silently absorbed). onceBoolValue needs NoOptDefVal so the bare form
	// consumes no argument.
	addOnceBool(f, newOnceBool("--write", &args.write), "write", "write the new version back to each FILE input")
	f.Var(newOnceString("--pre", &args.bump.Pre), "pre", "set pre-release identifiers `PRE` (e.g. rc.0)")
	addOnceBool(f, newOnceBool("--no-pre", &args.bump.NoPre), "no-pre", "strip the pre-release identifier")
	f.Var(newOnceString("--build-metadata", &args.bump.BuildMetadata), "build-metadata", "set build metadata `META` (e.g. sha.abc)")
	addOnceBool(f, newOnceBool("--no-build-metadata", &args.bump.NoBuildMetadata), "no-build-metadata", "strip the build metadata")
	f.BoolVar(&args.output.JSON, "json", false, "structured JSON output")
	f.Var(newOnceString("--vcs", &args.vcsBase.Override), "vcs", "force backend for vcs: inputs (`jj|git|auto`)")

	addVerbosityFlags(f, &args.output.Verbosity)
	addGlobFlags(cmd, &args.glob)

	ruleUsages := map[string]string{
		"--define-rule":   "open a custom rule for sources matching `PATTERN` (path / basename / glob:)",
		"--format":        "rule body: source `FMT` (text|json|yaml|toml|xml)",
		"--version-path":  "rule body: version `DOTPATH` for json/yaml/toml/xml",
		"--version-regex": "rule body: version `REGEX` (text; one capture group)",
		"--name-path":     "rule body: optional package-name `DOTPATH`",
		"--name-regex":    "rule body: optional package-name `REGEX`",
	}
	for _, name := range []string{"--define-rule", "--format", "--version-path", "--version-regex", "--name-path", "--name-regex"} {
		f.Var(&ruleFlagValue{rec: &st.rules, flag: name}, strings.TrimPrefix(name, "--"), ruleUsages[name])
	}

	return st
}

// applySharedTail runs the bump/compare shared exclusivity, empty-value
// and --vcs validation checks at the end of flag assembly, mirroring the
// tail of the legacy parseSharedFlags so both verbs inherit them
// identically (and in the same order).
func applySharedTail(args *cliArgs) error {
	if args.bump.Pre != nil && args.bump.NoPre {
		return fmt.Errorf("--pre and --no-pre are mutually exclusive")
	}
	if args.bump.BuildMetadata != nil && args.bump.NoBuildMetadata {
		return fmt.Errorf("--build-metadata and --no-build-metadata are mutually exclusive")
	}
	if args.bump.Pre != nil && *args.bump.Pre == "" {
		return fmt.Errorf("--pre value cannot be empty, use --no-pre to remove")
	}
	if args.bump.BuildMetadata != nil && *args.bump.BuildMetadata == "" {
		return fmt.Errorf("--build-metadata value cannot be empty, use --no-build-metadata to remove")
	}
	if args.vcsBase.Override != nil {
		if _, err := parseVcsOverride(*args.vcsBase.Override); err != nil {
			return err
		}
	}
	return nil
}

// addGlobFlags registers the --glob-* family on cmd, wiring each into the
// given cliArgs slots. Shared by every verb that accepts glob: selectors
// (vcs diff / commit / outdated; later compare / bump).
func addGlobFlags(cmd *cobra.Command, glob *globOpts) {
	dotfile := &globBoolValue{name: "--glob-dotfile", bareIsErr: true, apply: func(b bool) { glob.Dotfile = b }}
	gitignored := &globBoolValue{name: "--glob-gitignored", bareIsErr: true, apply: func(b bool) { glob.Gitignored = ptr(b) }}
	ignorecase := &globBoolValue{name: "--glob-ignorecase", bareIsErr: false, apply: func(b bool) { glob.IgnoreCase = b }}

	cmd.Flags().Var(dotfile, "glob-dotfile", "glob: include dotfiles (=true|=false required)")
	cmd.Flags().Lookup("glob-dotfile").NoOptDefVal = bareGlobSentinel

	cmd.Flags().Var(gitignored, "glob-gitignored", "glob: respect .gitignore (=true|=false required)")
	cmd.Flags().Lookup("glob-gitignored").NoOptDefVal = bareGlobSentinel

	cmd.Flags().Var(ignorecase, "glob-ignorecase", "glob: case-insensitive match (bare = true)")
	cmd.Flags().Lookup("glob-ignorecase").NoOptDefVal = "true"
}
