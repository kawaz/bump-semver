package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a strict X.Y.Z semver triple.
// Pre-release / build metadata (e.g. -alpha, +build.1) is rejected by Parse.
type Version struct {
	Major, Minor, Patch int
}

func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	if strings.ContainsAny(s, "-+") {
		return Version{}, fmt.Errorf("invalid version %q: pre-release / build metadata not supported", s)
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected X.Y.Z (got %d components)", s, len(parts))
	}
	var nums [3]int
	for i, p := range parts {
		if p == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty component", s)
		}
		if len(p) > 1 && p[0] == '0' {
			return Version{}, fmt.Errorf("invalid version %q: leading zero in component %q", s, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q: non-numeric component %q", s, p)
		}
		if n < 0 {
			return Version{}, fmt.Errorf("invalid version %q: negative component %q", s, p)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) Bump(action string) (Version, error) {
	switch action {
	case "major":
		return Version{Major: v.Major + 1}, nil
	case "minor":
		return Version{Major: v.Major, Minor: v.Minor + 1}, nil
	case "patch":
		return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}, nil
	case "get":
		return v, nil
	default:
		return Version{}, fmt.Errorf("invalid action %q", action)
	}
}
