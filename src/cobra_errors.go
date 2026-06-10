package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// flagErrorFunc translates cobra/pflag flag-parsing errors into the
// project's "bump-semver: <reason>" stderr line + *exitErr{code: 2}
// shape.
//
// Stage 1 only routes the global short-circuit forms (version / help)
// through cobra, so no real flag parsing reaches this yet. It exists as
// the minimal hook required by newRootCmd; later stages flesh it out to
// reproduce the legacy unknown-flag / requires-a-value / specified-twice
// wording (plan §3.2 / §3.3).
func flagErrorFunc(cmd *cobra.Command, err error) error {
	fmt.Fprintln(cmd.ErrOrStderr(), "bump-semver: "+err.Error())
	return &exitErr{code: exitCodeUsage, msg: err.Error()}
}
