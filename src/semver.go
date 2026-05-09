package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents an X.Y.Z triple, optionally decorated with a textual
// prefix (e.g. "v", "ver-", "version_") and a single separator character
// shared between the major-minor and minor-patch boundaries (".", "_", or
// "-"). Pre-release / build metadata (`-alpha`, `+build.1`) is rejected.
//
// The prefix and separator are preserved through Bump and String so that
// `v1.2.3` patch -> `v1.2.4`, `version_1_2_3` minor -> `version_1_3_0`, etc.
type Version struct {
	Prefix              string // "" / "v" / "ver-" / "version_" など
	Sep                 string // "." / "_" / "-"
	Major, Minor, Patch int
}

// versionRe is anchored. It rejects pre-release / build metadata at the
// regex level (anything trailing the patch component fails to match).
var versionRe = regexp.MustCompile(
	`^(v(?:er(?:sion)?)?[_.\-]?)?(\d+)([_.\-])(\d+)([_.\-])(\d+)$`,
)

func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	if strings.Contains(s, "+") {
		return Version{}, fmt.Errorf("invalid version %q: build metadata (+...) not supported", s)
	}
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("invalid version %q: expected [v|ver|version][_.-]?X[._-]Y[._-]Z (no pre-release / build metadata)", s)
	}
	if m[3] != m[5] {
		return Version{}, fmt.Errorf("invalid version %q: inconsistent separators (%q vs %q)", s, m[3], m[5])
	}
	var nums [3]int
	for i, p := range []string{m[2], m[4], m[6]} {
		if len(p) > 1 && p[0] == '0' {
			return Version{}, fmt.Errorf("invalid version %q: leading zero in component %q", s, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q: non-numeric component %q", s, p)
		}
		nums[i] = n
	}
	return Version{
		Prefix: m[1],
		Sep:    m[3],
		Major:  nums[0],
		Minor:  nums[1],
		Patch:  nums[2],
	}, nil
}

func (v Version) String() string {
	sep := v.Sep
	if sep == "" {
		sep = "."
	}
	return fmt.Sprintf("%s%d%s%d%s%d", v.Prefix, v.Major, sep, v.Minor, sep, v.Patch)
}

func (v Version) Bump(action string) (Version, error) {
	switch action {
	case "major":
		return Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major + 1}, nil
	case "minor":
		return Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major, Minor: v.Minor + 1}, nil
	case "patch":
		return Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}, nil
	case "get":
		return v, nil
	default:
		return Version{}, fmt.Errorf("invalid action %q", action)
	}
}
