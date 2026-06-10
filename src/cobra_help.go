package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// This file owns the cobra help rendering. The design goal (cobra
// migration) is a single source of truth for the Options section: it is
// generated from each command's pflag.FlagSet (renderFlagBlock) rather
// than hand-maintained const text, so a flag added in cobra_*.go shows
// up in --help automatically and can never drift.
//
// The descriptive prose (purpose, inputs, modes, exit codes, examples)
// still lives per-command, but in cobra-native slots:
//
//   - cmd.Long            — title line + Usage block + the body prose
//                           (Keys / Modes / Inputs / Notes / ...). Rendered
//                           verbatim first.
//   - Annotations[exit]   — the "Exit codes:" block (rendered AFTER the
//                           auto Options so the section order stays
//                           Usage / commands / description / Options /
//                           Global Options / Exit codes / Examples).
//   - cmd.Example         — the "Examples:" block (rendered last).
//
// Help is English-only and carries no user-facing DR numbers (asserted by
// cobra_help_test.go). DR references survive only in code comments.

// help annotation keys. Stored on cmd.Annotations because cobra has no
// native "exit codes" slot and its default template would otherwise emit
// Examples in the wrong position.
const (
	annExitCodes = "bump-semver/exit-codes"
)

// setHelp wires a command's help prose into the cobra-native slots used
// by renderCommandHelp. long is the title + Usage + body block; exitCodes
// is the "Exit codes:" block (without the heading); examples is the
// "Examples:" block (without the heading). Empty strings are skipped.
func setHelp(cmd *cobra.Command, long, exitCodes, examples string) {
	cmd.Long = strings.TrimRight(long, "\n")
	if exitCodes != "" {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[annExitCodes] = strings.TrimRight(exitCodes, "\n")
	}
	if examples != "" {
		cmd.Example = strings.TrimRight(examples, "\n")
	}
}

// renderCommandHelp assembles the full help screen for cmd in the order
// the kawaz CLI design preferences prescribe:
//
//	Long (title + Usage + description)
//	Commands:        (child subcommands, name + Short — auto)
//	Options:         (this command's local flags — auto)
//	Global Options:  (inherited persistent flags — auto)
//	Exit codes:      (annotation)
//	Examples:        (cmd.Example)
//
// Each section is omitted when it has no content.
func renderCommandHelp(cmd *cobra.Command) string {
	var b strings.Builder

	if cmd.Long != "" {
		b.WriteString(cmd.Long)
		b.WriteString("\n")
	}

	if sub := renderSubcommands(cmd); sub != "" {
		b.WriteString("\nCommands:\n")
		b.WriteString(sub)
	}

	if opts := renderFlagBlock(cmd.LocalFlags()); opts != "" {
		b.WriteString("\nOptions:\n")
		b.WriteString(opts)
	}

	if glob := renderFlagBlock(cmd.InheritedFlags()); glob != "" {
		b.WriteString("\nGlobal Options:\n")
		b.WriteString(glob)
	}

	if ec := cmd.Annotations[annExitCodes]; ec != "" {
		b.WriteString("\nExit codes:\n")
		b.WriteString(ec)
		b.WriteString("\n")
	}

	if cmd.Example != "" {
		b.WriteString("\nExamples:\n")
		b.WriteString(cmd.Example)
		b.WriteString("\n")
	}

	return b.String()
}

// renderSubcommands lists cmd's user-facing child commands as
// "  name   Short" lines, skipping the cobra-generated help/completion
// commands. Returns "" when cmd has no listable children.
func renderSubcommands(cmd *cobra.Command) string {
	type row struct{ name, short string }
	var rows []row
	maxName := 0
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		rows = append(rows, row{c.Name(), c.Short})
		if len(c.Name()) > maxName {
			maxName = len(c.Name())
		}
	}
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-*s   %s\n", maxName, r.name, r.short)
	}
	return b.String()
}

// renderFlagBlock formats a FlagSet into aligned "  --name PLACEHOLDER
// usage" lines, the Options / Global Options body. It is deliberately
// independent of pflag's own FlagUsages so the project controls the
// output exactly:
//
//   - the value placeholder comes from a back-quoted token in the usage
//     string (pflag's UnquoteUsage convention) or, absent that, from the
//     flag's Type() — but a "bool" type yields no placeholder;
//   - the "[=NoOptDefVal]" suffix pflag appends for flags with a bare
//     form is suppressed (several flags use an internal sentinel
//     NoOptDefVal that is meaningless to a user);
//   - aliased flags that share one slot (--branch / --bookmark) both
//     appear, matching their independent registration.
//
// Flags are listed in lexicographic long-name order (pflag's own order)
// so the block is stable regardless of registration order. Returns "" for
// an empty / all-hidden FlagSet.
func renderFlagBlock(fs *pflag.FlagSet) string {
	type entry struct {
		head  string // "  -m, --message MSG" / "      --json"
		name  string // long name, the sort key
		usage string
	}
	var entries []entry
	maxHead := 0

	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		var head strings.Builder
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			fmt.Fprintf(&head, "  -%s, --%s", f.Shorthand, f.Name)
		} else {
			fmt.Fprintf(&head, "      --%s", f.Name)
		}
		placeholder, usage := pflag.UnquoteUsage(f)
		if placeholder != "" {
			head.WriteString(" " + placeholder)
		}
		h := head.String()
		if len(h) > maxHead {
			maxHead = len(h)
		}
		entries = append(entries, entry{head: h, name: f.Name, usage: usage})
	})

	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%-*s   %s\n", maxHead, e.head, e.usage)
	}
	return b.String()
}

// installHelp registers the project help renderer on the root command.
// Children inherit the HelpFunc, so every `<cmd> --help` (and the bare
// no-positional verb help routed through RunE) renders via
// renderCommandHelp. The root's own short / full help is handled by the
// caller before this func is reached (see newRootCmd), so the HelpFunc
// here only fires for subcommands.
func installHelp(root *cobra.Command, stdout io.Writer, rootHelp func()) {
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// The root command keeps its bespoke short / full help.
		if cmd == root {
			rootHelp()
			return
		}
		// `vcs get latest-tag --help` / `latest-release --help`: these are
		// positional keys of `vcs get`, not real subcommands, but they have
		// dedicated help. cobra leaves the key as a positional in args.
		if cmd.CommandPath() == "bump-semver vcs get" {
			for _, a := range args {
				if a == "latest-tag" || a == "latest-release" {
					fmt.Fprint(stdout, renderLatestHelp(cmd, a))
					return
				}
			}
		}
		fmt.Fprint(stdout, renderCommandHelp(cmd))
	})
}

// renderLatestHelp renders the help for the `vcs get latest-tag` /
// `latest-release` positional keys. They are not separate cobra commands
// (they are keys dispatched inside runVcsCmdGet), so their Long / exit /
// examples live in latestHelpData and their Options are the relevant
// subset of `vcs get`'s flags, selected by name from the live FlagSet
// (still single-source-of-truth: the flag definitions are the ones
// registered on `vcs get`).
func renderLatestHelp(vcsGet *cobra.Command, key string) string {
	d := latestHelpData[key]
	var b strings.Builder
	b.WriteString(d.long)
	b.WriteString("\n")

	if opts := renderNamedFlags(vcsGet.LocalFlags(), d.optionFlags); opts != "" {
		b.WriteString("\nOptions:\n")
		b.WriteString(opts)
	}
	if glob := renderFlagBlock(vcsGet.InheritedFlags()); glob != "" {
		b.WriteString("\nGlobal Options:\n")
		b.WriteString(glob)
	}
	if d.exitCodes != "" {
		b.WriteString("\nExit codes:\n")
		b.WriteString(d.exitCodes)
		b.WriteString("\n")
	}
	if d.examples != "" {
		b.WriteString("\nExamples:\n")
		b.WriteString(d.examples)
		b.WriteString("\n")
	}
	return b.String()
}

// renderNamedFlags renders only the named flags from fs (used for the
// latest-tag / latest-release pseudo-commands, which expose a subset of
// `vcs get`'s flags). Names are the long form without dashes.
func renderNamedFlags(fs *pflag.FlagSet, names []string) string {
	sub := pflag.NewFlagSet("", pflag.ContinueOnError)
	for _, n := range names {
		if f := fs.Lookup(n); f != nil {
			sub.AddFlag(f)
		}
	}
	return renderFlagBlock(sub)
}
