package main

import "testing"

// TestNormalizeRemoteURL is the table-driven spec for the DR-0041 URL
// normalization rules. Each case documents *why* its expected (slug,
// httpsURL) pair is correct — this table IS the specification of what
// counts as a valid remote URL and how it maps to the two `vcs get`
// outputs (repository / repository-url).
func TestNormalizeRemoteURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		raw      string
		wantSlug string
		wantURL  string
		wantErr  bool
	}{
		// --- scp-style: [user@]host:path -----------------------------------
		{
			name: "scp-style with user",
			// The canonical GitHub SSH clone URL shape. DR-0041: user info
			// is stripped, host+path become the https form, trailing
			// .git is dropped.
			raw:      "git@github.com:kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		{
			name: "scp-style without user",
			// scp-style doesn't require a user segment — self-hosted
			// forges sometimes configure remotes this way when the SSH
			// config already pins the user via `Host` alias.
			raw:      "github.com:kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		{
			name: "scp-style bracketed IPv6 host with user",
			// git accepts a literal IPv6 host in scp-style form as
			// `[user@]​[v6addr]:path` (clone-tested shape). The split
			// point is the ':' right after the closing ']', NOT the
			// first ':' in the string — the address itself is full of
			// colons, so a naive first-':' split would misfire.
			raw:      "git@[2001:db8::1]:kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://[2001:db8::1]/kawaz/bump-semver",
		},
		{
			name:     "scp-style bracketed IPv6 host without user",
			raw:      "[2001:db8::1]:kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://[2001:db8::1]/kawaz/bump-semver",
		},
		// --- ssh:// ---------------------------------------------------------
		{
			name: "ssh with port",
			// DR-0041: "host の port は保持" — a self-hosted forge on a
			// non-standard port must not silently lose the port when
			// normalized to https.
			raw:      "ssh://git@example.com:2222/kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://example.com:2222/kawaz/bump-semver",
		},
		{
			name: "ssh without port",
			raw:  "ssh://git@github.com/kawaz/bump-semver.git",
			// No port in the source → none in the https form either;
			// this pins that the port-preservation logic doesn't inject
			// an empty ":" when absent.
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		// --- https:// --------------------------------------------------------
		{
			name: "https with .git suffix",
			raw:  "https://github.com/kawaz/bump-semver.git",
			// Already https — normalization should be idempotent (just
			// drop the .git suffix).
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		{
			name: "https without .git suffix",
			// Some forges (and manually-typed remotes) omit .git; the
			// slug/URL must come out identical to the .git-suffixed form.
			raw:      "https://github.com/kawaz/bump-semver",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		{
			name: "https with trailing slash",
			// DR-0041: "末尾 '/' を除去" — a redundant trailing slash
			// (common when copy-pasted from a browser address bar) must
			// not leak into the slug as an empty trailing segment.
			raw:      "https://github.com/kawaz/bump-semver/",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		{
			name: "https with userinfo stripped",
			// A basic-auth-style https remote (rare but legal git
			// syntax) must not leak the credentials into repository-url.
			raw:      "https://oauth2:TOKEN@gitlab.example.com/kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://gitlab.example.com/kawaz/bump-semver",
		},
		// --- git:// ----------------------------------------------------------
		{
			name: "git protocol",
			// DR-0041 explicitly lists git:// as an accepted scheme
			// (read-only anonymous clone URLs use it).
			raw:      "git://github.com/kawaz/bump-semver.git",
			wantSlug: "kawaz/bump-semver",
			wantURL:  "https://github.com/kawaz/bump-semver",
		},
		// --- GitLab subgroup (3+ segments) ------------------------------------
		{
			name: "subgroup path is not truncated to 2 segments",
			// DR-0041: "セグメント数を2に決め打ちしない" — GitLab
			// subgroups produce slugs with 3+ '/'-separated segments and
			// the whole path must survive intact.
			raw:      "https://gitlab.example.com/group/sub/repo.git",
			wantSlug: "group/sub/repo",
			wantURL:  "https://gitlab.example.com/group/sub/repo",
		},
		{
			name:     "scp-style subgroup path",
			raw:      "git@gitlab.example.com:group/sub/repo.git",
			wantSlug: "group/sub/repo",
			wantURL:  "https://gitlab.example.com/group/sub/repo",
		},
		// --- rejected: local filesystem remotes -------------------------------
		{
			name: "absolute local path",
			// DR-0041: local remotes have no forge host to normalize to.
			// This is the shape `setupGitRepoWithRemote` fixtures use
			// (bare.git on the local filesystem) — repository /
			// repository-url must reject it, not fabricate a bogus URL.
			raw:     "/Users/kawaz/repos/bare.git",
			wantErr: true,
		},
		{
			name:    "file:// URI",
			raw:     "file:///Users/kawaz/repos/bare.git",
			wantErr: true,
		},
		{
			name: "Windows drive letter with forward slash",
			// git's own scp-vs-local decision (url_is_local_not_ssh)
			// treats a single ASCII letter immediately before the
			// first ':' as a drive letter, not an scp host — so a
			// remote configured as `git remote add origin C:/repo`
			// (a real, if unusual, local-path form on Windows) must
			// be rejected the same way as any other local path, not
			// silently normalized into a bogus "https://C/..." URL.
			raw:     "C:/Users/kawaz/repo",
			wantErr: true,
		},
		{
			name: "Windows drive letter with backslash",
			// Same rule, backslash-separated form — no '/' appears
			// anywhere in the string, so this exercises the
			// single-letter-before-':' branch on its own (the
			// slash-before-colon branch can't fire here).
			raw:     `C:\Users\kawaz\repo`,
			wantErr: true,
		},
		{
			name: "relative path with slash before colon",
			// git's rule: a '/' occurring before the first ':' means
			// the whole string is a local path with an incidental
			// colon in it, not an scp host:path split point.
			raw:     "./sub:dir/repo",
			wantErr: true,
		},
		// --- rejected: malformed / empty ---------------------------------------
		{
			name: "empty string",
			// An empty remote URL means the remote is misconfigured
			// (e.g. `git remote add x ""`, or a jj remote list parse
			// bug upstream) — must error, not silently produce an
			// empty slug.
			raw:     "",
			wantErr: true,
		},
		{
			name: "no scheme, no colon, no slash",
			// Neither a scp host:path (no ':') nor a scheme URL (no
			// "://") — nothing to anchor a host on.
			raw:     "not-a-url-at-all",
			wantErr: true,
		},
		{
			name: "scheme URL with empty host",
			// "http:///path" parses without a Go url.Parse error but
			// yields an empty Host — must still be rejected rather than
			// emitting "https:///path".
			raw:     "http:///kawaz/bump-semver",
			wantErr: true,
		},
		{
			name: "unsupported scheme",
			// DR-0041 only accepts ssh/git/http/https (+ scp-style).
			// Anything else (e.g. a hg/svn-style scheme picked up by
			// accident) should be rejected rather than silently
			// half-normalized.
			raw:     "svn://example.com/kawaz/bump-semver",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeRemoteURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeRemoteURL(%q) = %+v, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRemoteURL(%q) unexpected error: %v", tt.raw, err)
			}
			if got.Slug != tt.wantSlug {
				t.Errorf("normalizeRemoteURL(%q).Slug = %q, want %q", tt.raw, got.Slug, tt.wantSlug)
			}
			if got.HTTPSURL != tt.wantURL {
				t.Errorf("normalizeRemoteURL(%q).HTTPSURL = %q, want %q", tt.raw, got.HTTPSURL, tt.wantURL)
			}
		})
	}
}
