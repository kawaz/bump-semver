package main

import "testing"

func TestDetectHandler(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want string
	}{
		{"Cargo.toml", "cargo"},
		{"path/to/Cargo.toml", "cargo"},
		{"VERSION", "version"},
		{"sub/VERSION", "version"},
		{"package.json", "json"},
		{".claude-plugin/plugin.json", "json"},
		{"moon.mod.json", "json"},
	}
	for _, tc := range cases {
		h, err := detectHandler(tc.path)
		if err != nil {
			t.Errorf("detectHandler(%q) error: %v", tc.path, err)
			continue
		}
		var name string
		switch h.(type) {
		case cargoHandler:
			name = "cargo"
		case jsonHandler:
			name = "json"
		case versionHandler:
			name = "version"
		default:
			name = "?"
		}
		if name != tc.want {
			t.Errorf("detectHandler(%q) = %s, want %s", tc.path, name, tc.want)
		}
	}
	if _, err := detectHandler("readme.md"); err == nil {
		t.Error("detectHandler(readme.md) expected error, got nil")
	}
	if _, err := detectHandler("Version"); err == nil {
		t.Error("detectHandler(Version) expected error (case-sensitive), got nil")
	}
}
