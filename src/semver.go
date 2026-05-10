package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a SemVer 2.0.0 version with the kawaz prefix/sep
// extension.
//
// The X.Y.Z triple may be optionally decorated with a textual prefix
// (e.g. "v", "ver-", "version_") and a single body separator character
// shared between the major-minor and minor-patch boundaries (".", or
// "_"). Body separator "-" is NOT supported (DR-0006: collides with
// pre-release `-`).
//
// Pre-release (`-alpha.1`) and build metadata (`+build.42`) are SemVer
// 2.0.0 compliant and stored as identifier slices.
//
// The prefix and separator are preserved through Bump and String so that
// `v1.2.3` patch -> `v1.2.4`, `version_1_2_3` minor -> `version_1_3_0`,
// etc.
type Version struct {
	Prefix              string // "" / "v" / "ver-" / "version_" など
	Sep                 string // "." / "_"
	Major, Minor, Patch int
	Pre                 []string // SemVer 2.0.0 pre-release identifiers (no leading `-`)
	BuildMetadata       []string // SemVer 2.0.0 build metadata identifiers (no leading `+`)
}

// versionRe is the regex for SemVer 2.0.0 + kawaz prefix/sep extension.
//
// Capture groups:
//
//	[1] prefix  (v|ver|version, optional + optional sep)
//	[2] major   (numeric, no leading zero)
//	[3] sep1    (`.` or `_`)
//	[4] minor
//	[5] sep2    (`.` or `_`) — must equal sep1, enforced in code
//	[6] patch
//	[7] pre     (without leading `-`)
//	[8] build   (without leading `+`)
//
// Go RE2 has no backreferences, so sep1 == sep2 is enforced by an
// explicit check in ParseVersion.
const (
	prefixRE = `(v(?:er(?:sion)?)?[_.\-]?)?`
	mainRE   = `(0|[1-9]\d*)([._])(0|[1-9]\d*)([._])(0|[1-9]\d*)`
	preIdRE  = `(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)`
	bldIdRE  = `[0-9a-zA-Z-]+`
)

var versionRe = regexp.MustCompile(
	`^` + prefixRE + mainRE +
		`(?:-(` + preIdRE + `(?:\.` + preIdRE + `)*))?` +
		`(?:\+(` + bldIdRE + `(?:\.` + bldIdRE + `)*))?` +
		`$`,
)

func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("invalid version %q: expected [v|ver|version][_.-]?X[._]Y[._]Z[-PRE][+BUILD]", s)
	}
	if m[3] != m[5] {
		return Version{}, fmt.Errorf("invalid version %q: inconsistent separators (%q vs %q)", s, m[3], m[5])
	}
	var nums [3]int
	for i, p := range []string{m[2], m[4], m[6]} {
		// Leading-zero check is guaranteed by the regex (0|[1-9]\d*)
		// but we keep an explicit Atoi for safety / clearer errors.
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q: non-numeric component %q", s, p)
		}
		nums[i] = n
	}
	v := Version{
		Prefix: m[1],
		Sep:    m[3],
		Major:  nums[0],
		Minor:  nums[1],
		Patch:  nums[2],
	}
	// strings.Split("", ".") returns [""], so guard with explicit check.
	if m[7] != "" {
		v.Pre = strings.Split(m[7], ".")
	}
	if m[8] != "" {
		v.BuildMetadata = strings.Split(m[8], ".")
	}
	return v, nil
}

func (v Version) String() string {
	sep := v.Sep
	if sep == "" {
		sep = "."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s%d%s%d%s%d", v.Prefix, v.Major, sep, v.Minor, sep, v.Patch)
	if len(v.Pre) > 0 {
		sb.WriteByte('-')
		sb.WriteString(strings.Join(v.Pre, "."))
	}
	if len(v.BuildMetadata) > 0 {
		sb.WriteByte('+')
		sb.WriteString(strings.Join(v.BuildMetadata, "."))
	}
	return sb.String()
}

// Compare implements SemVer 2.0.0 § 11 ordering.
//
// Returns -1 if v < other, 0 if v == other, +1 if v > other.
//
// Notes (DR-0006):
//   - prefix / sep is ignored (v1.2.3 == 1.2.3 == version_1_2_3)
//   - build metadata is ignored (1.0.0+a == 1.0.0+b)
//   - pre-release is "less than" the corresponding release
//     (1.0.0-rc.1 < 1.0.0)
//   - pre-release identifiers are compared field-by-field; numeric
//     identifiers are compared numerically and rank below
//     alphanumeric identifiers; if all identifiers match up to the
//     length of the shorter list, the shorter list is "less than" the
//     longer list.
func (v Version) Compare(other Version) int {
	if c := cmpInt(v.Major, other.Major); c != 0 {
		return c
	}
	if c := cmpInt(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := cmpInt(v.Patch, other.Patch); c != 0 {
		return c
	}
	// pre-release rules (build metadata is ignored).
	switch {
	case len(v.Pre) == 0 && len(other.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return +1 // release > pre-release
	case len(other.Pre) == 0:
		return -1
	}
	return comparePreSlices(v.Pre, other.Pre)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return +1
	default:
		return 0
	}
}

// comparePreSlices implements SemVer 2.0.0 § 11.4 identifier comparison.
func comparePreSlices(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := comparePreIdent(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

// comparePreIdent compares two pre-release identifiers per SemVer
// § 11.4.{1,2,3}: numeric vs numeric numerically; alphanumeric vs
// alphanumeric by ASCII; numeric < alphanumeric.
func comparePreIdent(a, b string) int {
	an, aIsNum := tryNumeric(a)
	bn, bIsNum := tryNumeric(b)
	switch {
	case aIsNum && bIsNum:
		return cmpInt(an, bn)
	case aIsNum && !bIsNum:
		return -1
	case !aIsNum && bIsNum:
		return +1
	default:
		return strings.Compare(a, b)
	}
}

func tryNumeric(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	// SemVer pre-release numeric identifiers cannot have leading zeros,
	// but that's a parse-time concern; here we just need to know "is
	// this a non-negative integer literal". We accept any digit run.
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// BumpOptions controls Bump behavior for cross-cutting flags.
//
// `--pre PRE` / `--no-pre` are exclusive at the CLI layer; if both
// PreSet and NoPre are true here, PreSet wins (we trust the caller).
// Same for the build-metadata pair.
type BumpOptions struct {
	Pre              string // value passed via --pre PRE (only valid when PreSet=true)
	PreSet           bool   // --pre was explicitly given
	NoPre            bool   // --no-pre was explicitly given
	BuildMetadata    string
	BuildMetadataSet bool
	NoBuildMetadata  bool
}

// Bump applies the action to v and returns the resulting Version.
//
// Default behavior (DR-0006): pre-release and build metadata are
// dropped on numeric bumps (major/minor/patch). To carry them over,
// the caller must explicitly supply BumpOptions{Pre: ..., PreSet:
// true} etc.
//
// `pre` action has three modes (mutually exclusive at the CLI layer):
//
//   - opts.PreSet=true: overwrite Pre with parsed opts.Pre (build dropped)
//   - opts.NoPre=true:  remove Pre (idempotent), build dropped
//   - neither:          counter-advance the trailing pure-numeric
//     identifier; error if there's no pre or the
//     trailing identifier isn't pure numeric
//
// `get` returns v with optional --no-pre / --no-build-metadata
// stripping applied, but never sets new identifiers.
func (v Version) Bump(action string, opts BumpOptions) (Version, error) {
	switch action {
	case "major":
		return applyOptsToBumped(Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major + 1}, opts)
	case "minor":
		return applyOptsToBumped(Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major, Minor: v.Minor + 1}, opts)
	case "patch":
		return applyOptsToBumped(Version{Prefix: v.Prefix, Sep: v.Sep, Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}, opts)
	case "pre":
		return bumpPre(v, opts)
	case "get":
		out := v
		// `get` keeps everything by default; --no-pre / --no-build-metadata strip.
		if opts.NoPre {
			out.Pre = nil
		}
		if opts.NoBuildMetadata {
			out.BuildMetadata = nil
		}
		// `get` does NOT honor --pre / --build-metadata as setters
		// (that would be a "modify" not a "read"). The CLI layer
		// rejects those combinations, but we defensively ignore here.
		return out, nil
	default:
		return Version{}, fmt.Errorf("invalid action %q", action)
	}
}

// applyOptsToBumped applies --pre / --build-metadata options to a
// freshly-bumped (and stripped) base. By default both pre and build are
// already absent from `base`; opts can put them back.
func applyOptsToBumped(base Version, opts BumpOptions) (Version, error) {
	if opts.PreSet {
		ids, err := splitPre(opts.Pre)
		if err != nil {
			return Version{}, err
		}
		base.Pre = ids
	}
	if opts.BuildMetadataSet {
		ids, err := splitBuild(opts.BuildMetadata)
		if err != nil {
			return Version{}, err
		}
		base.BuildMetadata = ids
	}
	return base, nil
}

// bumpPre handles the `pre` action.
func bumpPre(v Version, opts BumpOptions) (Version, error) {
	// --pre PRE: overwrite (build dropped).
	if opts.PreSet {
		ids, err := splitPre(opts.Pre)
		if err != nil {
			return Version{}, err
		}
		out := Version{
			Prefix: v.Prefix, Sep: v.Sep,
			Major: v.Major, Minor: v.Minor, Patch: v.Patch,
			Pre: ids,
		}
		// --build-metadata may be set in the same invocation
		if opts.BuildMetadataSet {
			bids, err := splitBuild(opts.BuildMetadata)
			if err != nil {
				return Version{}, err
			}
			out.BuildMetadata = bids
		}
		return out, nil
	}
	// --no-pre: remove (build also dropped).
	if opts.NoPre {
		out := Version{
			Prefix: v.Prefix, Sep: v.Sep,
			Major: v.Major, Minor: v.Minor, Patch: v.Patch,
		}
		if opts.BuildMetadataSet {
			bids, err := splitBuild(opts.BuildMetadata)
			if err != nil {
				return Version{}, err
			}
			out.BuildMetadata = bids
		}
		return out, nil
	}
	// counter advance.
	// Error message formats follow DR-0006:
	//   - no pre:           "<X.Y.Z> does not have a pre-release, use --pre PRE"
	//   - non-incremental:  "<last-id> is not incremental, use --pre PRE"
	if len(v.Pre) == 0 {
		// Strip build metadata so users see the actionable X.Y.Z, not noise.
		return Version{}, fmt.Errorf("%s does not have a pre-release, use --pre PRE", stringWithoutBuild(v))
	}
	last := v.Pre[len(v.Pre)-1]
	n, ok := tryNumericNoLeadingZero(last)
	if !ok {
		return Version{}, fmt.Errorf("%s is not incremental, use --pre PRE", last)
	}
	newPre := make([]string, len(v.Pre))
	copy(newPre, v.Pre)
	newPre[len(newPre)-1] = strconv.Itoa(n + 1)
	out := Version{
		Prefix: v.Prefix, Sep: v.Sep,
		Major: v.Major, Minor: v.Minor, Patch: v.Patch,
		Pre: newPre,
	}
	if opts.BuildMetadataSet {
		bids, err := splitBuild(opts.BuildMetadata)
		if err != nil {
			return Version{}, err
		}
		out.BuildMetadata = bids
	}
	return out, nil
}

// tryNumericNoLeadingZero reports "is s a pure numeric identifier with
// no leading zero" — the SemVer-valid numeric identifier shape.
//
// `0` itself is OK (single zero is canonical zero, no leading-zero
// concept). `01`, `02` etc are NOT OK.
func tryNumericNoLeadingZero(s string) (int, bool) {
	n, ok := tryNumeric(s)
	if !ok {
		return 0, false
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	return n, true
}

// stringWithoutBuild returns v's String() form minus build metadata.
// Used in error messages so build noise doesn't appear in the user's
// terminal when the actionable problem is in the pre-release portion.
func stringWithoutBuild(v Version) string {
	clone := v
	clone.BuildMetadata = nil
	return clone.String()
}

// splitPre validates and splits a --pre PRE value into identifiers.
//
// Empty string is allowed at this layer (Phase 1 stops short of
// enforcing 確定論点 D — the CLI layer in Phase 2 will reject empty
// --pre); we accept it as "set Pre to nothing" for now, but reject it
// here so test expectations in Phase 2 can build on top.
//
// We reuse the pre-release portion of versionRe by parsing
// `1.0.0-<PRE>` to verify SemVer compliance.
func splitPre(s string) ([]string, error) {
	if s == "" {
		// Phase 1: caller-side concern. Treat empty as "no pre" so the
		// resulting Version is valid SemVer; the CLI layer will fence
		// this in Phase 2.
		return nil, nil
	}
	probe := "1.0.0-" + s
	pv, err := ParseVersion(probe)
	if err != nil {
		return nil, fmt.Errorf("invalid pre-release %q: %w", s, err)
	}
	return pv.Pre, nil
}

// splitBuild validates and splits a --build-metadata META value into identifiers.
func splitBuild(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	probe := "1.0.0+" + s
	pv, err := ParseVersion(probe)
	if err != nil {
		return nil, fmt.Errorf("invalid build metadata %q: %w", s, err)
	}
	return pv.BuildMetadata, nil
}
