// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json / VERSION)
// and provides four flat actions: major, minor, patch, get.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

const helpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <ACTION> <FILE | --value VER> [--write]
  bump-semver --version
  bump-semver --help

Actions:
  major   Bump major (X.0.0)
  minor   Bump minor (x.Y.0)
  patch   Bump patch (x.y.Z)
  get     Print the current version

Options:
  --value VER   Use VER instead of FILE (mutually exclusive with FILE)
  --write       Write the new version back to FILE (only with major/minor/patch)
  --version, -V Print the binary version
  --help, -h    Show this help

Supported file formats (auto-detected by basename):
  Cargo.toml      TOML, [package].version
  *.json          JSON, .version
  VERSION         plain text

When stdin is a pipe, FILE is used as a name hint only and the content is
read from stdin. --write is incompatible with stdin pipe input.

Examples:
  bump-semver patch Cargo.toml --write
  bump-semver minor package.json
  bump-semver get  .claude-plugin/plugin.json
  bump-semver patch --value 1.2.3
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "bump-semver: "+err.Error())
		os.Exit(1)
	}
}

type cliArgs struct {
	action string
	file   string
	value  string
	write  bool
	// internal "actions" routed by argv parsing
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
	fileSeen := false
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
			// allow `-- <FILE>` to disambiguate file paths starting with '-'
			if i+1 >= len(rest) {
				return cliArgs{}, fmt.Errorf("-- requires a FILE argument")
			}
			if fileSeen {
				return cliArgs{}, fmt.Errorf("multiple FILE arguments: %s", rest[i+1])
			}
			out.file = rest[i+1]
			fileSeen = true
			i = len(rest)
		case strings.HasPrefix(a, "-"):
			return cliArgs{}, fmt.Errorf("unknown option: %s", a)
		default:
			if fileSeen {
				return cliArgs{}, fmt.Errorf("multiple FILE arguments: %s", a)
			}
			out.file = a
			fileSeen = true
		}
	}
	if !fileSeen && !valueSeen {
		return cliArgs{}, fmt.Errorf("either FILE or --value is required")
	}
	if fileSeen && valueSeen {
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
		newV, err := v.Bump(args.action)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, newV.String())
		return nil
	}

	handler, err := detectHandler(args.file)
	if err != nil {
		return err
	}

	pipe := isStdinPipe(stdin)
	if pipe && args.write {
		return fmt.Errorf("--write is incompatible with stdin pipe input")
	}

	var content []byte
	if pipe {
		content, err = io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		content, err = os.ReadFile(args.file)
		if err != nil {
			return fmt.Errorf("read %s: %w", args.file, err)
		}
	}

	cur, err := handler.Get(content)
	if err != nil {
		return err
	}
	v, err := ParseVersion(cur)
	if err != nil {
		return err
	}
	newV, err := v.Bump(args.action)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, newV.String())

	if args.write {
		out, err := handler.Replace(content, newV.String())
		if err != nil {
			return err
		}
		if err := os.WriteFile(args.file, out, 0644); err != nil {
			return fmt.Errorf("write %s: %w", args.file, err)
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
