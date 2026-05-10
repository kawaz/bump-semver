package main

import (
	"encoding/json"
	"strings"
)

// jsonOutput represents the public JSON schema produced by --json.
//
// All optional fields use *string so absent values render as JSON `null`
// rather than empty strings; this keeps the schema unambiguous when CI
// scripts pipe the output through jq.
//
// Schema is documented in DR-0007 (and was prototyped in
// docs/issue/2026-05-10-json-output.md). Two version-shaped fields are
// emitted intentionally:
//
//   - Version: input format preserved (prefix + body separator kept)
//   - Semver:  strict SemVer 2.0.0 form (prefix removed, body sep
//     normalised to ".")
//
// pre / build_metadata are split at the first `.` into <id>/<rest>
// purely as structural decomposition; semantic judgements (numeric vs
// alphanumeric, "advanceable" etc.) are intentionally left to the
// caller (see DR-0007 Rationale).
type jsonOutput struct {
	Name          *string `json:"name"`
	Version       string  `json:"version"`
	Semver        string  `json:"semver"`
	Major         int     `json:"major"`
	Minor         int     `json:"minor"`
	Patch         int     `json:"patch"`
	Pre           *string `json:"pre"`
	PreID         *string `json:"pre_id"`
	PreRest       *string `json:"pre_rest"`
	BuildMetadata *string `json:"build_metadata"`
	BuildID       *string `json:"build_id"`
	BuildRest     *string `json:"build_rest"`
}

// ToJSON renders v as a jsonOutput, using name as the package-name field
// (nil when no name is available, e.g. VER / stdin origin or a FILE
// without a name field).
func (v Version) ToJSON(name *string) jsonOutput {
	out := jsonOutput{
		Name:    name,
		Version: v.String(),
		Semver:  v.strict(),
		Major:   v.Major,
		Minor:   v.Minor,
		Patch:   v.Patch,
	}
	if len(v.Pre) > 0 {
		joined := strings.Join(v.Pre, ".")
		out.Pre = &joined
		id, rest := splitAtFirstDot(joined)
		out.PreID = &id
		if rest != "" {
			out.PreRest = &rest
		}
	}
	if len(v.BuildMetadata) > 0 {
		joined := strings.Join(v.BuildMetadata, ".")
		out.BuildMetadata = &joined
		id, rest := splitAtFirstDot(joined)
		out.BuildID = &id
		if rest != "" {
			out.BuildRest = &rest
		}
	}
	return out
}

// strict returns v in canonical SemVer 2.0.0 form: prefix removed, body
// separator forced to ".". Pre-release and build metadata are emitted
// verbatim (their separators are SemVer-fixed and don't vary with v.Sep).
func (v Version) strict() string {
	clone := v
	clone.Prefix = ""
	clone.Sep = "."
	return clone.String()
}

// splitAtFirstDot splits s at the first '.' into (head, tail). When s
// contains no '.', tail is "". The DR-0007 schema represents tail=""
// as JSON null (via *string with nil), so callers must decide whether
// to dereference based on the empty check.
func splitAtFirstDot(s string) (head, tail string) {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// marshalJSONOutput produces the canonical "one line + trailing newline"
// rendering used by the CLI. Marshalling lives here (rather than at the
// call site) so the formatting decision can be revisited in one place.
func marshalJSONOutput(out jsonOutput) ([]byte, error) {
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
