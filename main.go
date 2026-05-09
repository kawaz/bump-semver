// bump-semver: a focused semver bump CLI.
//
// Detects supported version files by basename (Cargo.toml / *.json / VERSION)
// and provides four flat actions: major, minor, patch, get.
//
// Implementation pending — this skeleton lets the CI / release pipeline come
// online before the actual logic lands. See README.md and docs/DESIGN.md.
package main

import (
	"fmt"
	"os"
)

// version is filled in at build time via -ldflags "-X main.version=v..."
var version = "dev"

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "--version" || os.Args[1] == "-V") {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "bump-semver: skeleton — implementation pending")
	os.Exit(2)
}
