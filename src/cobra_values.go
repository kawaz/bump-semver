package main

import (
	"fmt"

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
	fs.VarP(quiet, "quiet", "q", "suppress stdout + hints")
	fs.Lookup("quiet").NoOptDefVal = "x"

	quietAll := &verbosityRaiseValue{slot: v, level: outputQuietAll}
	fs.Var(quietAll, "quiet-all", "suppress stdout + hints + errors")
	fs.Lookup("quiet-all").NoOptDefVal = "x"

	noHint := &verbosityRaiseValue{slot: v, level: outputNoHint}
	fs.Var(noHint, "no-hint", "suppress hints only")
	fs.Lookup("no-hint").NoOptDefVal = "x"
}

// normalizeQuietAll rewrites the standalone `-qq` token to `--quiet-all`
// (and any `--` separator stops the rewrite, mirroring the legacy
// whole-token match that never reinterprets post-`--` positionals).
// pflag cannot represent `-qq` as a single shorthand, so this runs over
// argv before cobra parses the vcs subtree.
func normalizeQuietAll(argv []string) []string {
	out := make([]string, 0, len(argv))
	for i, a := range argv {
		if a == "--" {
			out = append(out, argv[i:]...)
			break
		}
		if a == "-qq" {
			out = append(out, "--quiet-all")
			continue
		}
		out = append(out, a)
	}
	return out
}

// addGlobFlags registers the --glob-* family on cmd, wiring each into the
// given cliArgs slots. Shared by every verb that accepts glob: selectors
// (vcs diff / commit / outdated; later compare / bump).
func addGlobFlags(cmd *cobra.Command, glob *globOpts) {
	dotfile := &globBoolValue{name: "--glob-dotfile", bareIsErr: true, apply: func(b bool) { glob.Dotfile = b }}
	gitignored := &globBoolValue{name: "--glob-gitignored", bareIsErr: true, apply: func(b bool) { glob.Gitignored = ptr(b) }}
	ignorecase := &globBoolValue{name: "--glob-ignorecase", bareIsErr: false, apply: func(b bool) { glob.IgnoreCase = b }}

	cmd.Flags().Var(dotfile, "glob-dotfile", "include dotfiles (=true|=false, required)")
	cmd.Flags().Lookup("glob-dotfile").NoOptDefVal = bareGlobSentinel

	cmd.Flags().Var(gitignored, "glob-gitignored", "respect .gitignore (=true|=false, required)")
	cmd.Flags().Lookup("glob-gitignored").NoOptDefVal = bareGlobSentinel

	cmd.Flags().Var(ignorecase, "glob-ignorecase", "case-insensitive match (bare = true)")
	cmd.Flags().Lookup("glob-ignorecase").NoOptDefVal = "true"
}
