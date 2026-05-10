package main

import (
	"strings"
	"testing"
)

// --- DR-0015: xml format (Info.plist) ------------------------------------

const samplePlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>1.2.3</string>
	<key>CFBundleIdentifier</key>
	<string>com.example.app</string>
	<key>CFBundleVersion</key>
	<string>42</string>
</dict>
</plist>
`

// TestXmlInspect_InfoPlist pulls the marketing version out of a
// canonical Apple `Info.plist`. Other keys (`CFBundleIdentifier`,
// `CFBundleVersion`) are not part of the rule's `VersionPaths` /
// `NamePaths`, so they're correctly ignored.
func TestXmlInspect_InfoPlist(t *testing.T) {
	t.Parallel()
	insp, err := inspectVia("Info.plist", []byte(samplePlist))
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 {
		t.Fatalf("Versions = %+v, want one CFBundleShortVersionString", insp.Versions)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions[0].Value = %q, want 1.2.3", insp.Versions[0].Value)
	}
	if insp.Versions[0].Path != "CFBundleShortVersionString" {
		t.Errorf("Versions[0].Path = %q, want bare key name", insp.Versions[0].Path)
	}
}

// TestXmlReplace_InfoPlistPreservesEverything covers the byte-range
// rewrite property: the DOCTYPE, declarations, attribute order,
// whitespace / indentation, and adjacent keys all survive byte-for-
// byte. Only the captured `<string>...</string>` value is rewritten.
func TestXmlReplace_InfoPlistPreservesEverything(t *testing.T) {
	t.Parallel()
	out, err := replaceVia("Info.plist", []byte(samplePlist), "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "<string>2.0.0</string>") {
		t.Errorf("Replace did not write 2.0.0 inside <string>:\n%s", got)
	}
	// DOCTYPE preserved verbatim.
	if !strings.Contains(got, `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"`) {
		t.Errorf("DOCTYPE lost:\n%s", got)
	}
	// XML declaration preserved.
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("XML declaration not preserved at head:\n%s", got)
	}
	// Other keys / values left intact.
	if !strings.Contains(got, "<string>com.example.app</string>") {
		t.Errorf("CFBundleIdentifier value lost:\n%s", got)
	}
	if !strings.Contains(got, "<string>42</string>") {
		t.Errorf("CFBundleVersion value lost:\n%s", got)
	}
	// The original 1.2.3 must be entirely gone.
	if strings.Contains(got, "1.2.3") {
		t.Errorf("Replace left stale 1.2.3 behind:\n%s", got)
	}
	// Trailing newline preserved.
	if !strings.HasSuffix(got, "</plist>\n") {
		t.Errorf("trailing newline / closing element lost:\n%q", got[len(got)-20:])
	}
}

// TestXmlInspect_PlaceholderExtractsLiterally documents the agreed
// behaviour for Xcode 11+ default Info.plist files: the rule extracts
// the placeholder text verbatim. ParseVersion (further up the
// pipeline in main.go) then rejects the value, and the input is
// surfaced as an `unsupported file:` outcome, which is the cue for
// the user to add `project.pbxproj` to the invocation. The rule
// itself does NOT silently skip placeholders — that would mask the
// situation from the dispatcher.
func TestXmlInspect_PlaceholderExtractsLiterally(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>$(MARKETING_VERSION)</string>
</dict>
</plist>
`)
	insp, err := inspectVia("Info.plist", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if insp.Versions[0].Value != "$(MARKETING_VERSION)" {
		t.Errorf("Versions[0].Value = %q, want literal placeholder text",
			insp.Versions[0].Value)
	}
}

// TestRun_PlaceholderInfoPlistFailsParseVersion is the integration
// counterpart of the placeholder behaviour: when the placeholder
// makes it through Inspect, the get/bump path errors with a
// ParseVersion-style diagnostic. We assert the error path, not the
// exact wording.
func TestRun_PlaceholderInfoPlistFailsParseVersion(t *testing.T) {
	t.Parallel()
	dir := tempWriteFiles(t, map[string]string{
		"Info.plist": `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>$(MARKETING_VERSION)</string>
</dict>
</plist>
`,
	})
	if err := tryRun("get", dir+"/Info.plist"); err == nil {
		t.Error("expected error for placeholder Info.plist, got nil")
	}
}

// TestXmlInspect_MissingKey fails the rule cleanly so the dispatcher
// can keep walking to lower-confidence rules (or end up with a clean
// `unsupported file:` for files named `Info.plist` that lack the key).
func TestXmlInspect_MissingKey(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.example.app</string>
</dict>
</plist>
`)
	if _, err := inspectVia("Info.plist", in); err == nil {
		t.Error("expected error when CFBundleShortVersionString is absent")
	}
}

// TestXmlInspect_NonStringValueSkipsPair: a `<key>` followed by a
// non-`<string>` element (`<true/>`, `<integer>`, etc.) is not
// rewriteable by this format; the pair is silently dropped, and
// asking for that key returns "missing".
func TestXmlInspect_NonStringValueSkipsPair(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<integer>42</integer>
</dict>
</plist>
`)
	if _, err := inspectVia("Info.plist", in); err == nil {
		t.Error("expected error: <integer> is not a rewriteable value")
	}
}

// TestXmlReplace_PreservesIndentation covers the secondary value of
// byte-range splicing: tab-indented Apple defaults stay tab-indented,
// space-indented manual edits stay space-indented. We only mutate the
// captured CharData range, so surrounding whitespace is preserved.
func TestXmlReplace_PreservesIndentation(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>CFBundleShortVersionString</key>
    <string>1.2.3</string>
</dict>
</plist>
`)
	out, err := replaceVia("Info.plist", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "    <string>1.2.4</string>") {
		t.Errorf("4-space indentation not preserved:\n%s", string(out))
	}
}

// TestXmlInspect_MalformedXMLErrorsClean: a syntactically broken
// document surfaces as a parse error from `encoding/xml`, not as a
// silent fallthrough. The dispatcher will then propagate the error
// because there's no other rule for `Info.plist`.
func TestXmlInspect_MalformedXMLErrorsClean(t *testing.T) {
	t.Parallel()
	in := []byte("<?xml version=\"1.0\"?><plist><dict><key>X</key><string>broken")
	if _, err := inspectVia("Info.plist", in); err == nil {
		t.Error("expected parse error for malformed XML")
	}
}
