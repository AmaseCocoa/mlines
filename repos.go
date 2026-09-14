package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Repo は管理下の1リポジトリを表す。
type Repo struct {
	Name        string
	Path        string
	Manifest    *Manifest
	ManifestErr error
}

func repoPath(name string) string {
	return filepath.Join(reposDir(), name)
}

func validRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("リポジトリ名が空です")
	}
	// alias との衝突やパストラバーサルを避ける
	if strings.Contains(name, "/") || name == "." || name == ".." {
		return fmt.Errorf("リポジトリ名 %q には / を含められません", name)
	}
	return validScriptName(name)
}

// listRepos は管理下のリポジトリを名前順で返す。
func listRepos() ([]Repo, error) {
	root := reposDir()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var repos []Repo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		r := Repo{Name: e.Name(), Path: p}
		m, err := loadManifest(p)
		if err != nil {
			r.ManifestErr = err
		} else {
			r.Manifest = m
		}
		repos = append(repos, r)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos, nil
}

func findRepo(name string) (*Repo, error) {
	if err := validRepoName(name); err != nil {
		return nil, err
	}
	p := repoPath(name)
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("リポジトリ %q が見つかりません", name)
	}
	r := &Repo{Name: name, Path: p}
	m, err := loadManifest(p)
	if err != nil {
		r.ManifestErr = err
	} else {
		r.Manifest = m
	}
	return r, nil
}

// addRepo は git リポジトリを clone して管理下に加える。
func addRepo(url, name string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("URL が空です")
	}
	if err := validRepoName(name); err != nil {
		return err
	}
	dest := repoPath(name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("リポジトリ %q は既に存在します", name)
	}
	if err := ensureDir(reposDir()); err != nil {
		return err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git が見つかりません")
	}
	cmd := exec.Command("git", "clone", url, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone に失敗: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(manifestPath(dest)); os.IsNotExist(err) {
		fmt.Printf("注意: %s に mlines.json がありません (list には出ますが script は0件です)\n", name)
	}
	return nil
}

// removeRepo は管理下のリポジトリを削除する。
// ついでに当該リポジトリを指す有効なリンクも掃除する。
func removeRepo(name, bin string) error {
	r, err := findRepo(name)
	if err != nil {
		return err
	}
	cleaned, _ := sweepRepoLinks(r.Name, r.Path, bin)
	if err := os.RemoveAll(r.Path); err != nil {
		return fmt.Errorf("削除に失敗: %w", err)
	}
	if len(cleaned) > 0 {
		fmt.Printf("関連リンクを掃除しました: %s\n", strings.Join(cleaned, ", "))
	}
	return nil
}

// syncRepo は git pull --ff-only で更新する。
func syncRepo(name string) (string, error) {
	r, err := findRepo(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil {
		return "", fmt.Errorf("リポジトリ %q は git 管理ではありません", name)
	}
	cmd := exec.Command("git", "-C", r.Path, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return msg, fmt.Errorf("sync に失敗: %w\n%s", err, msg)
	}
	return msg, nil
}

// syncAllRepos は全リポジトリを更新する。結果は名前順。
func syncAllRepos() map[string]error {
	repos, _ := listRepos()
	results := make(map[string]error, len(repos))
	for _, r := range repos {
		_, err := syncRepo(r.Name)
		results[r.Name] = err
	}
	return results
}
