package main

import (
	"fmt"
	"net/url"
	"strings"
)

// remoteURLInfo is the normalized outcome of a remote URL (DR-0041 "URL
// 正規化仕様"). Both fields are non-empty on success.
type remoteURLInfo struct {
	// Slug is the forge path with the leading '/' removed and any
	// trailing '/' / '.git' stripped. GitHub always yields 2 segments
	// ("owner/repo"); GitLab subgroups yield 3+ ("group/sub/repo") — the
	// segment count is intentionally not hardcoded to 2.
	Slug string
	// HTTPSURL is the https-normalized reference form: scheme replaced
	// with "https", user info removed, host (port preserved, if any) +
	// "/" + Slug.
	HTTPSURL string
}

// normalizeRemoteURL converts a VCS remote URL — as reported verbatim by
// `git remote get-url` / `jj git remote list` — into the forge identifiers
// `vcs get repository` (Slug) and `vcs get repository-url` (HTTPSURL) need
// (DR-0041).
//
// Accepted forms:
//
//   - scp-style  [user@]host:path        (no scheme; the ':' is not
//     followed by "//" — that's what distinguishes it from a scheme URL)
//   - scp-style with a bracketed IPv6 host [user@][v6addr]:path (git
//     accepts this for `ssh` clones of hosts named by literal address)
//   - ssh://[user@]host[:port]/path
//   - git://host[:port]/path
//   - http(s)://[user[:pass]@]host[:port]/path
//
// Local filesystem remotes (`/path/to/repo`, `file://...`, a Windows
// drive-letter path such as `C:\repo` or `C:/repo`, or any relative path
// with a '/' before its first ':') have no forge host to normalize to and
// are rejected — the error names the cause and points at `git remote
// get-url <name>` as the raw-value alternative (interface-wording: cause
// + remedy, not a bare "invalid").
func normalizeRemoteURL(raw string) (remoteURLInfo, error) {
	if raw == "" {
		return remoteURLInfo{}, fmt.Errorf("remote URL is empty")
	}
	// Local filesystem remotes: an absolute path has no host segment at
	// all, and `file://` is a URI scheme but still names a local path,
	// not a forge. Both are checked before the scheme branch below so
	// they get a tailored message instead of "unsupported scheme".
	if strings.HasPrefix(raw, "/") {
		return remoteURLInfo{}, localPathError(raw)
	}

	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil {
			return remoteURLInfo{}, fmt.Errorf("remote %q is not a parseable URL: %w", raw, err)
		}
		switch u.Scheme {
		case "ssh", "git", "http", "https":
			host = u.Host // net/url already strips user info into u.User and keeps :port in u.Host
			path = u.Path
		case "file":
			return remoteURLInfo{}, fmt.Errorf(
				"remote %q is a local filesystem path (file://), not a forge URL; use `git remote get-url <name>` for the raw value", raw)
		default:
			return remoteURLInfo{}, fmt.Errorf(
				"remote %q has unsupported scheme %q (expected ssh/git/http/https, or scp-style host:path)", raw, u.Scheme)
		}
	default:
		// scp-style [user@]host:path, or [user@][v6addr]:path. The "://"
		// branch above already ruled out scheme URLs.
		if bracketIdx := strings.IndexByte(raw, '['); bracketIdx >= 0 && (bracketIdx == 0 || raw[bracketIdx-1] == '@') {
			// Bracketed IPv6 host: split on the ':' that immediately
			// follows the closing ']', not the first ':' in the string —
			// a bare first-':' split would misfire on the colons inside
			// the address itself (e.g. "git@[2001:db8::1]:kawaz/repo").
			rest := raw[bracketIdx:]
			closeIdx := strings.IndexByte(rest, ']')
			if closeIdx < 0 {
				return remoteURLInfo{}, fmt.Errorf(
					"remote %q has an unterminated '[' (expected a bracketed IPv6 host)", raw)
			}
			if closeIdx+1 >= len(rest) || rest[closeIdx+1] != ':' {
				return remoteURLInfo{}, fmt.Errorf(
					"remote %q: bracketed host must be followed by ':path'", raw)
			}
			host = rest[:closeIdx+1]
			path = rest[closeIdx+2:]
		} else {
			idx := strings.Index(raw, ":")
			if idx < 0 {
				return remoteURLInfo{}, fmt.Errorf(
					"remote %q is not a recognized URL (no scheme, no scp-style host:path)", raw)
			}
			// git's own scp-vs-local-path rule (url_is_local_not_ssh):
			// a '/' occurring before the first ':' means the whole thing
			// is a local path with an incidental colon in it (e.g.
			// "./sub:dir/repo"), and a single ASCII letter immediately
			// before the ':' is a Windows drive letter (`C:/repo`,
			// `C:\repo`) rather than an scp host. Both are local paths,
			// not scp remotes.
			slashIdx := strings.IndexByte(raw, '/')
			isDriveLetter := idx == 1 && isASCIILetter(raw[0])
			if (slashIdx >= 0 && slashIdx < idx) || isDriveLetter {
				return remoteURLInfo{}, localPathError(raw)
			}
			hostPart := raw[:idx]
			path = raw[idx+1:]
			if at := strings.LastIndex(hostPart, "@"); at >= 0 {
				hostPart = hostPart[at+1:] // drop user info
			}
			host = hostPart
		}
	}

	if host == "" || path == "" {
		return remoteURLInfo{}, fmt.Errorf(
			"remote %q has no host/path to derive a repository slug from", raw)
	}

	// DR-0041: strip trailing '/' first so "owner/repo.git/" and
	// "owner/repo.git" normalize identically, then strip trailing '.git',
	// then drop the leading '/' scheme URLs carry on their Path.
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return remoteURLInfo{}, fmt.Errorf(
			"remote %q has no path segment to derive a repository slug from", raw)
	}

	return remoteURLInfo{
		Slug:     path,
		HTTPSURL: "https://" + host + "/" + path,
	}, nil
}

// localPathError is the shared DR-0041 rejection for any remote form that
// names a local filesystem path (absolute Unix path, Windows drive letter,
// or a relative path with a '/' before its first ':') rather than a forge
// host.
func localPathError(raw string) error {
	return fmt.Errorf(
		"remote %q is a local filesystem path, not a forge URL (DR-0041: repository/repository-url only derive from remote host URLs); use `git remote get-url <name>` for the raw value", raw)
}

// isASCIILetter reports whether b is an ASCII letter (used to recognize a
// single-letter Windows drive prefix like "C:").
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
