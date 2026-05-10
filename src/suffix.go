package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

// DR-0013: known backup-style suffixes that resolveRule will strip from
// the basename (one level only) before retrying the rule table.
//
// The list is intentionally limited to **backup-style** suffixes — names
// users add when keeping a hand-rolled copy of a manifest alongside the
// real one. Template-style suffixes (`.template`, `.example`, `.sample`,
// `.dist`) are deliberately *not* on this list: their content is usually
// a placeholder (`__VERSION__`, `0.0.0`) and silently treating them as
// real manifests would be more dangerous than the current
// `unsupported file:` behaviour.
//
// A trailing `~` (Emacs / vi backup convention) is handled separately
// because it sits flush against the basename rather than after a `.`.
var knownLiteralSuffixes = []string{
	".bak",
	".backup",
	".orig",
	".tmp",
	".old",
}

// dateStampSuffix matches a final dotted segment of either:
//
//   - 8 digits           (`.YYYYMMDD`)
//   - 8 digits + `_` + 6 digits (`.YYYYMMDD_HHMMSS`)
//
// Anchored with `$` so it only fires when the segment is at the very
// end of the basename. `_HHMMSS` is greedy; `Cargo.toml.20260510_120000`
// strips the whole `.20260510_120000` segment in one go (the seconds
// part is not a separate suffix).
var dateStampSuffix = regexp.MustCompile(`\.[0-9]{8}(_[0-9]{6})?$`)

// stripKnownSuffix attempts to remove exactly one backup-style suffix
// from the tail of `path`'s basename. On success it returns
// (newPath, suffix, true); on failure (returnedPath == path, "", false).
//
// Matching order:
//
//  1. trailing `~` (single character, no `.` in front)
//  2. dotted literal suffix (`.bak` / `.backup` / `.orig` / `.tmp` / `.old`)
//  3. dotted date stamp (`.YYYYMMDD` or `.YYYYMMDD_HHMMSS`)
//
// The first hit wins and the function returns immediately — multi-stage
// suffixes (`Cargo.toml.bak.20260510`) are *not* stripped recursively
// (DR-0013 § 4).
//
// The directory portion of the path is preserved verbatim. Only the
// basename is rewritten, so `path/to/Cargo.toml.bak` becomes
// `path/to/Cargo.toml`.
func stripKnownSuffix(path string) (string, string, bool) {
	dir, base := filepath.Split(path)

	// 1. Trailing `~` (Emacs / vi). Single character, no leading dot.
	//    `Cargo.toml~` → strip the `~`. `~` alone (no preceding name)
	//    is not stripped — that would leave an empty basename.
	if strings.HasSuffix(base, "~") && len(base) > 1 {
		return dir + strings.TrimSuffix(base, "~"), "~", true
	}

	// 2. Dotted literal suffixes.
	for _, suf := range knownLiteralSuffixes {
		// Require a non-empty stem so `.bak` (basename == suffix)
		// doesn't collapse to an empty path.
		if strings.HasSuffix(base, suf) && len(base) > len(suf) {
			return dir + strings.TrimSuffix(base, suf), suf, true
		}
	}

	// 3. Dotted date-stamp suffixes (regex). `FindStringIndex` returns
	//    the byte range of the match; we need a non-empty stem before it.
	if loc := dateStampSuffix.FindStringIndex(base); loc != nil && loc[0] > 0 {
		suf := base[loc[0]:loc[1]]
		return dir + base[:loc[0]], suf, true
	}

	return path, "", false
}
