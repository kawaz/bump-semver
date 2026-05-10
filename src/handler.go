package main

import "fmt"

// Field is one detected version-like or name-like value inside a file.
// Path is a human-readable JSON / TOML / plain path string used in error
// messages (e.g. `$.version`, `$.metadata.version`,
// `$.packages[""].version`, `[package].version`, `(file content)`). It does
// not have to follow any single grammar; it just needs to disambiguate
// which spot inside the file the value came from.
type Field struct {
	Value string
	Path  string
}

// Inspection is everything a handler can extract from a file.
//
//   - Versions: 1 or more version strings detected in the file. Required —
//     a handler must return at least one or it should error out.
//   - Names: 0 or more package-name-like strings detected in the file.
//     Optional. Used solely for cross-file consistency validation; never
//     written back.
//   - MatchedConfidence / MatchedGlob: which DR-0005 rule won. Set by
//     resolveRule on a successful match (zero in format_*Inspect helpers
//     which know nothing about the rule selection). Used by main.go to
//     surface a confidence-1 fallback hint (DR-0010) and not for any
//     downstream processing.
type Inspection struct {
	Versions []Field
	Names    []Field

	// MatchedConfidence is the Confidence of the rule that the
	// dispatcher (DR-0005) finally selected: 3 = path-pinned,
	// 2 = basename-only, 1 = glob fallback. Zero when not applicable
	// (e.g. when an extraction error is returned without a rule
	// being chosen).
	MatchedConfidence int
	// MatchedGlob is the Glob pattern of the matched rule when the
	// rule won by glob (Confidence 1). Empty otherwise. Used to render
	// the DR-0010 fallback hint (`matched as *.json fallback`).
	MatchedGlob string
}

// Handler reads / writes the version string of a single file format.
//
// As of DR-0005 the concrete implementation is a single ruleHandler whose
// behaviour is driven by a `CandidateRule` table indexed by path-aware
// confidence. The interface itself is unchanged from earlier versions so
// the multi-file orchestration in main.go does not need to know about the
// dispatcher's internals.
//
// Replace receives both the current and new version explicitly. current is
// the version string Inspect returned for the same content; threading it
// through means handlers don't have to parse twice and can use the literal
// to disambiguate where the version sits inside the file.
type Handler interface {
	Inspect(content []byte) (Inspection, error)
	Replace(content []byte, current, newVersion string) ([]byte, error)
}

// ruleHandler is the only concrete Handler type. It's stateful: the rule
// is resolved on the first Inspect call (since rule selection depends on
// content, not just path) and reused by the subsequent Replace call.
type ruleHandler struct {
	path string
	rule *CandidateRule // nil until Inspect resolves a rule
}

func (h *ruleHandler) Inspect(content []byte) (Inspection, error) {
	rule, insp, err := resolveRule(h.path, content)
	if err != nil {
		return Inspection{}, err
	}
	h.rule = &rule
	return insp, nil
}

func (h *ruleHandler) Replace(content []byte, current, newVersion string) ([]byte, error) {
	if h.rule == nil {
		// Caller skipped Inspect somehow; resolve again from content.
		rule, _, err := resolveRule(h.path, content)
		if err != nil {
			return nil, err
		}
		h.rule = &rule
	}
	return formatReplace(*h.rule, content, current, newVersion)
}

// detectHandler returns a Handler bound to the given path. Failure here
// is restricted to "no rule could ever match this path" — content-driven
// failures are deferred to Inspect, where they can fall back through
// confidence levels.
func detectHandler(path string) (Handler, error) {
	if !pathHasAnyRule(path) {
		return nil, &unsupportedFileError{path: path}
	}
	return &ruleHandler{path: path}, nil
}

// unsupportedFileError signals "no DR-0005 rule matched this path" —
// distinct from extraction failures, which surface as plain `error`s
// returned from format-specific Inspect helpers. main.go uses
// errors.As to detect this case and tack on a one-line hint pointing
// at the issue tracker (DR-0010), suppressed by `--no-hint` /
// `-q` / `-qq` exactly like every other DR-0010 hint.
type unsupportedFileError struct{ path string }

func (e *unsupportedFileError) Error() string {
	return fmt.Sprintf("unsupported file: %s", e.path)
}
