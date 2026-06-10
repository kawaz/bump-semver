package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestValidateUserRev_LeadingDash: ユーザ由来の rev が `-` 始まりなら
// reject、正当な rev は通す (引数インジェクション対策, C-1)。
func TestValidateUserRev_LeadingDash(t *testing.T) {
	t.Parallel()
	reject := []string{"-d", "--output=x", "-"}
	for _, rev := range reject {
		if err := validateUserRev(rev); err == nil {
			t.Errorf("validateUserRev(%q): expected error, got nil", rev)
		}
	}
	accept := []string{"", "HEAD", "HEAD~3", "@-", "main@origin", "origin/main", "v1.2.3", "abc1234", "feature/x"}
	for _, rev := range accept {
		if err := validateUserRev(rev); err != nil {
			t.Errorf("validateUserRev(%q): expected nil, got %v", rev, err)
		}
	}
}

// TestValidTagName_LeadingDash: tag NAME が `-` 始まりなら reject
// (`git tag -d <sha>` 等のフラグ注入対策, C-1)。
func TestValidTagName_LeadingDash(t *testing.T) {
	t.Parallel()
	reject := []string{"-d", "-f", "--delete"}
	for _, name := range reject {
		if err := validTagName(name); err == nil {
			t.Errorf("validTagName(%q): expected error, got nil", name)
		}
	}
	accept := []string{"v1.0.0", "release-1", "@v1"}
	for _, name := range accept {
		if err := validTagName(name); err != nil {
			t.Errorf("validTagName(%q): expected nil, got %v", name, err)
		}
	}
}

// TestValidateRemote_LeadingDash: remote 名が空 or `-` 始まり or 空白含みなら
// reject (`git fetch -X` 等のフラグ注入対策, C-1)。
func TestValidateRemote_LeadingDash(t *testing.T) {
	t.Parallel()
	reject := []string{"", "-X", "--upload-pack=x", "a b"}
	for _, r := range reject {
		if err := validateRemote(r); err == nil {
			t.Errorf("validateRemote(%q): expected error, got nil", r)
		}
	}
	accept := []string{"origin", "upstream", "my-remote"}
	for _, r := range accept {
		if err := validateRemote(r); err != nil {
			t.Errorf("validateRemote(%q): expected nil, got %v", r, err)
		}
	}
}

// TestExpandRepoArg_RejectLeadingDash: `--repository -X` のような `-` 始まり
// repository は expandRepoArg が error で reject (C-1)。正当 URL / owner/repo は通す。
func TestExpandRepoArg_RejectLeadingDash(t *testing.T) {
	t.Parallel()
	reject := []string{"-X", "--upload-pack=x"}
	for _, s := range reject {
		if _, err := expandRepoArg(s); err == nil {
			t.Errorf("expandRepoArg(%q): expected error, got nil", s)
		}
	}
	accept := map[string]string{
		"":                             "",
		"kawaz/pkf-tasks":              "https://github.com/kawaz/pkf-tasks",
		"https://github.com/kawaz/x":   "https://github.com/kawaz/x",
		"git@github.com:kawaz/x.git":   "git@github.com:kawaz/x.git",
		"ssh://git@github.com/kawaz/x": "ssh://git@github.com/kawaz/x",
	}
	for in, want := range accept {
		got, err := expandRepoArg(in)
		if err != nil {
			t.Errorf("expandRepoArg(%q): expected nil error, got %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("expandRepoArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestValidateGhRepo: `gh -R <repo>` に渡す repo は owner/repo 形式必須。
// `-` 始まり / slash 数違い / 空白含みは reject (C-1)。
func TestValidateGhRepo(t *testing.T) {
	t.Parallel()
	reject := []string{"-X", "--foo", "a b/c", "a/b/c", "noslash", "/leading", "trailing/"}
	for _, repo := range reject {
		if err := validateGhRepo(repo); err == nil {
			t.Errorf("validateGhRepo(%q): expected error, got nil", repo)
		}
	}
	accept := []string{"", "owner/repo", "kawaz/bump-semver"}
	for _, repo := range accept {
		if err := validateGhRepo(repo); err != nil {
			t.Errorf("validateGhRepo(%q): expected nil, got %v", repo, err)
		}
	}
}

// TestRun_VcsDiff_RejectDashRev: 実害回帰テスト (最重要)。
// `vcs diff -- --output=<tmp>/PWNED` は `--` で rev 位置に `--output=...` を
// 注入できるが、(a) exit 2 で reject され、(b) ファイルが生成されないこと。
func TestRun_VcsDiff_RejectDashRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	pwned := filepath.Join(dir, "PWNED")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "--", "--output=" + pwned},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for leading-dash rev, got nil")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2 (usage), got: %v", err)
		}
		if _, serr := os.Stat(pwned); serr == nil {
			t.Errorf("PWNED file was created (arg injection succeeded): %s", pwned)
		}
	})
}

// TestRun_VcsGetCommitID_RejectDashRev: `vcs get commit-id --rev --output=...`
// が backend 到達前に usage error で弾かれること (C-1)。
func TestRun_VcsGetCommitID_RejectDashRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	pwned := filepath.Join(dir, "PWNEDcid")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "get", "commit-id", "--rev", "--output=" + pwned},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2 (usage), got: %v", err)
		}
		if _, serr := os.Stat(pwned); serr == nil {
			t.Errorf("PWNEDcid file created (injection succeeded): %s", pwned)
		}
	})
}

// TestRun_VcsTagDelete_RejectDashName: `vcs tag delete -- -d` が usage error。
func TestRun_VcsTagDelete_RejectDashName(t *testing.T) {
	t.Parallel()
	err := run([]string{"vcs", "tag", "delete", "--", "-d"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2 (usage), got: %v", err)
	}
}

// TestRun_VcsTagPush_RejectDashName: `vcs tag push --rev HEAD -- -d` が usage error。
func TestRun_VcsTagPush_RejectDashName(t *testing.T) {
	t.Parallel()
	err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "--", "-d"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2 (usage), got: %v", err)
	}
}

// TestRun_VcsGetLatestTag_RejectDashRepository: `vcs get latest-tag --repository -X`
// が backend 到達前に usage error で弾かれること (C-1)。
func TestRun_VcsGetLatestTag_RejectDashRepository(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "get", "latest-tag", "--repository", "-X"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2 (usage), got: %v", err)
		}
	})
}

// TestRun_VcsGetLatestRelease_RejectDashRepository: `vcs get latest-release
// --repository --foo` が gh 実呼び前に usage error で弾かれること。ghRunner は
// 呼ばれてはならない (stub で監視)。
func TestRun_VcsGetLatestRelease_RejectDashRepository(t *testing.T) {
	origRunner := ghRunner
	origLook := ghLookPath
	t.Cleanup(func() { ghRunner = origRunner; ghLookPath = origLook })
	called := false
	ghLookPath = func() error { return nil }
	ghRunner = func(args ...string) ([]byte, error) {
		called = true
		return []byte("[]"), nil
	}
	err := run([]string{"vcs", "get", "latest-release", "--repository", "--foo"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2 (usage), got: %v", err)
	}
	if called {
		t.Error("ghRunner was called despite invalid repository (backend reached before validation)")
	}
}

// TestRun_VcsInputMode_RejectDashRev: `vcs:REV` 入力モード (CLI 引数パーサ非経由)
// でも rev の leading-dash が reject されること (対極漏れ防止)。
func TestRun_VcsInputMode_RejectDashRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	pwned := filepath.Join(dir, "PWNEDinput")
	withCwd(t, dir, func() {
		err := run([]string{"get", "vcs:--output=" + pwned + ":VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for leading-dash rev in vcs: input mode")
		}
		if _, serr := os.Stat(pwned); serr == nil {
			t.Errorf("PWNEDinput file created (injection via vcs: input mode): %s", pwned)
		}
	})
}
