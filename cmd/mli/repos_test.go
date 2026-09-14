package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.email=t@t", "-c", "user.name=t",
		"-c", "init.defaultBranch=main"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	// テスト実行者のグローバル設定に左右されないようにする
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func makeOrigin(t *testing.T, content string) string {
	t.Helper()
	origin := t.TempDir()
	if err := os.MkdirAll(filepath.Join(origin, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, origin, "init")
	if err := os.WriteFile(filepath.Join(origin, "mlines.json"),
		[]byte(`{"scripts": {"s": {"type": "shell_script", "path": "s"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origin, "scripts", "s"), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, origin, "add", "-A")
	gitRun(t, origin, "commit", "-qm", "init")
	return origin
}

func commitFile(t *testing.T, dir, rel, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", msg)
}

// TestResetRepo は diverge 後の pull 失敗 → reset で upstream に合う流れを検証する。
func TestResetRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git がない")
	}
	origin := makeOrigin(t, "v1\n")
	t.Setenv("MLINES_DATA_DIR", t.TempDir())

	if err := addRepo(origin, "t"); err != nil {
		t.Fatalf("addRepo: %v", err)
	}
	// 正常系: ff pull が通る
	commitFile(t, origin, "scripts/s", "v2\n", "bump")
	if _, err := pullRepo("t"); err != nil {
		t.Fatalf("pullRepo: %v", err)
	}
	// diverge させる: 両方で別コミット
	clone := repoPath("t")
	commitFile(t, clone, "scripts/s", "local\n", "local change")
	commitFile(t, origin, "scripts/s", "v3\n", "bump2")
	if _, err := pullRepo("t"); err == nil {
		t.Fatalf("diverge 後は pull が失敗するべき")
	}
	// reset で upstream に合う
	msg, err := resetRepo("t")
	if err != nil {
		t.Fatalf("resetRepo: %v", err)
	}
	t.Logf("reset: %s", msg)
	want := gitRun(t, origin, "rev-parse", "HEAD")
	got := gitRun(t, clone, "rev-parse", "HEAD")
	if want != got {
		t.Errorf("HEAD不一致: clone=%s origin=%s", got, want)
	}
	data, _ := os.ReadFile(filepath.Join(clone, "scripts", "s"))
	if string(data) != "v3\n" {
		t.Errorf("内容が upstream に戻っていない: %q", data)
	}
}

func TestResetRepoErrors(t *testing.T) {
	t.Setenv("MLINES_DATA_DIR", t.TempDir())
	if _, err := resetRepo("nonexistent"); err == nil {
		t.Errorf("存在しないリポジトリはエラーになるべき")
	}
}
