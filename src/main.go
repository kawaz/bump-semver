// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json /
// package-lock.json / VERSION) and provides four flat actions: major,
// minor, patch, get. Multiple FILEs may be given at once; their versions
// (and optional package names) must be consistent and are bumped together.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

const helpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <ACTION> <FILE...> [--write]
  bump-semver <ACTION> --value VER
  bump-semver --version
  bump-semver --help

Actions:
  major   Bump major (X.0.0)
  minor   Bump minor (x.Y.0)
  patch   Bump patch (x.y.Z)
  get     Print the current version

Options:
  --value VER   Use VER instead of FILE(s) (mutually exclusive with FILE)
  --write       Write the new version back to each FILE (only with major/minor/patch)
  --version, -V Print the binary version
  --help, -h    Show this help

Supported file formats (auto-detected by basename):
  Cargo.toml         TOML, [package].version (and [package].name for cross-file checks)
  package-lock.json  npm 7+ lockfile, $.version + $.packages[""].version (deps untouched)
  *.json             JSON, $.version (and optional $.name)
  VERSION            plain text

When multiple FILEs are given, all detected versions across them must be
identical (otherwise: "version mismatch:" with paths). Detected names are
also cross-checked when available, to guard against accidentally bumping
files from a different project together. Names are never written back.

When stdin is a pipe (only valid for a single FILE), FILE is treated as a
name hint and the content is read from stdin. --write is incompatible with
stdin pipe input.

Examples:
  bump-semver patch Cargo.toml --write
  bump-semver minor package.json package-lock.json --write
  bump-semver get  .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
  bump-semver patch --value 1.2.3
  bump-semver patch --value v1.2.3                 # v1.2.4 (prefix preserved)
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "bump-semver: "+err.Error())
		os.Exit(1)
	}
}

type cliArgs struct {
	action  string
	files   []string
	value   string
	write   bool
	special string // "version" | "help" | ""
}

func parseArgs(argv []string) (cliArgs, error) {
	if len(argv) == 0 {
		return cliArgs{special: "help"}, nil
	}
	switch argv[0] {
	case "--version", "-V":
		return cliArgs{special: "version"}, nil
	case "--help", "-h":
		return cliArgs{special: "help"}, nil
	}
	out := cliArgs{action: argv[0]}
	switch out.action {
	case "major", "minor", "patch", "get":
	default:
		return cliArgs{}, fmt.Errorf("unknown action: %s (expected one of major|minor|patch|get)", out.action)
	}
	rest := argv[1:]
	valueSeen := false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--write":
			if out.write {
				return cliArgs{}, fmt.Errorf("--write specified twice")
			}
			out.write = true
		case a == "--value":
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("--value requires a value")
			}
			if valueSeen {
				return cliArgs{}, fmt.Errorf("--value specified twice")
			}
			out.value = rest[i+1]
			valueSeen = true
			i++
		case strings.HasPrefix(a, "--value="):
			if valueSeen {
				return cliArgs{}, fmt.Errorf("--value specified twice")
			}
			out.value = strings.TrimPrefix(a, "--value=")
			valueSeen = true
		case a == "--":
			// Treat all remaining argv as files (allows file paths
			// starting with `-`).
			out.files = append(out.files, rest[i+1:]...)
			i = len(rest)
		case strings.HasPrefix(a, "-"):
			return cliArgs{}, fmt.Errorf("unknown option: %s", a)
		default:
			out.files = append(out.files, a)
		}
	}
	if len(out.files) == 0 && !valueSeen {
		return cliArgs{}, fmt.Errorf("at least one FILE or --value is required")
	}
	if len(out.files) > 0 && valueSeen {
		return cliArgs{}, fmt.Errorf("FILE and --value are mutually exclusive")
	}
	if out.write && valueSeen {
		return cliArgs{}, fmt.Errorf("--write is incompatible with --value")
	}
	if out.write && out.action == "get" {
		return cliArgs{}, fmt.Errorf("--write is incompatible with get")
	}
	return out, nil
}

// locatedField is a Field plus the file it came from. Used to render
// multi-file mismatch errors.
type locatedField struct {
	File, Path, Value string
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

func formatMismatchError(kind string, items []locatedField) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s mismatch:", kind)
	for _, x := range items {
		fmt.Fprintf(&sb, "\n  %s:%s = %s", x.File, x.Path, x.Value)
	}
	return errors.New(sb.String())
}

func run(argv []string, stdin io.Reader, stdout io.Writer) error {
	args, err := parseArgs(argv)
	if err != nil {
		return err
	}
	switch args.special {
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help":
		fmt.Fprint(stdout, helpText)
		return nil
	}

	// --value path: pure parse + bump, no I/O.
	if args.value != "" {
		v, err := ParseVersion(args.value)
		if err != nil {
			return err
		}
		newV, err := v.Bump(args.action, BumpOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, newV.String())
		return nil
	}

	// stdin pipe is only honored for a single FILE — that case treats the
	// FILE as a name hint and reads content from stdin (DR-0001). With
	// multiple FILEs we always read from disk; this matches the cat / sed
	// convention of "explicit files override stdin".
	pipe := isStdinPipe(stdin) && len(args.files) == 1
	if pipe && args.write {
		return fmt.Errorf("--write is incompatible with stdin pipe input")
	}

	type entry struct {
		path    string
		handler Handler
		content []byte
		insp    Inspection
	}
	entries := make([]entry, 0, len(args.files))

	for _, file := range args.files {
		h, err := detectHandler(file)
		if err != nil {
			return err
		}
		var content []byte
		if pipe {
			content, err = io.ReadAll(stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
		} else {
			content, err = os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}
		}
		insp, err := h.Inspect(content)
		if err != nil {
			return err
		}
		entries = append(entries, entry{path: file, handler: h, content: content, insp: insp})
	}

	// Aggregate every detected version field, across all files, with
	// file+path provenance. Required: every entry contributes ≥1.
	var allVersions []locatedField
	for _, e := range entries {
		allVersions = append(allVersions, locatedFromInspection(e.path, e.insp.Versions)...)
	}
	cur, ok := allSameValue(allVersions)
	if !ok {
		return formatMismatchError("version", allVersions)
	}

	// Aggregate names where available (handlers may return zero names).
	// Cross-check only when at least one file provided a name.
	var allNames []locatedField
	for _, e := range entries {
		allNames = append(allNames, locatedFromInspection(e.path, e.insp.Names)...)
	}
	if _, ok := allSameValue(allNames); len(allNames) > 0 && !ok {
		return formatMismatchError("name", allNames)
	}

	v, err := ParseVersion(cur)
	if err != nil {
		return err
	}
	newV, err := v.Bump(args.action, BumpOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, newV.String())

	if args.write {
		// Sequential writes; on first failure we surface the error and
		// stop. Already-written files remain at the new version (rollback
		// is the user's job — jj/git makes this trivial).
		for _, e := range entries {
			out, err := e.handler.Replace(e.content, cur, newV.String())
			if err != nil {
				return fmt.Errorf("replace %s: %w", e.path, err)
			}
			mode := os.FileMode(0644)
			if fi, statErr := os.Stat(e.path); statErr == nil {
				mode = fi.Mode().Perm()
			}
			if err := os.WriteFile(e.path, out, mode); err != nil {
				return fmt.Errorf("write %s: %w", e.path, err)
			}
		}
	}
	return nil
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
