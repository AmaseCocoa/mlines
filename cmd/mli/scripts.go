package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// linkMeta は binary 導入時に bin/<alias>.mlines.json として残す管理メタデータ。
// symlink と違い実体コピーのため、出所を逆引きするために使う。
type linkMeta struct {
	Repo     string `json:"repo"`
	Script   string `json:"script"`
	Alias    string `json:"alias"`
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Platform string `json:"platform,omitempty"`
}

func metaPath(bin, alias string) string {
	return filepath.Join(bin, alias+".mlines.json")
}

// ScriptItem は TUI/list 共通の表示・操作単位。
type ScriptItem struct {
	ID          string // "repo/name"
	Repo        string
	Name        string
	Type        string
	Description string
	Enabled     bool
	EnabledAs   string // bin ディレクトリ上の名前
	Conflict    string // 有効化されていないが同名ファイルが存在する旨
}

type binEntry struct {
	path      string
	isSymlink bool
	target    string // symlink の解決済み絶対パス
	meta      *linkMeta
	isMeta    bool
}

// scanBin は bin ディレクトリを1回走査する。
func scanBin(bin string) map[string]*binEntry {
	entries := map[string]*binEntry{}
	files, err := os.ReadDir(bin)
	if os.IsNotExist(err) {
		return entries
	}
	if err != nil {
		return entries
	}
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".mlines.json") {
			continue
		}
		full := filepath.Join(bin, name)
		e := &binEntry{path: full}
		if st, err := os.Lstat(full); err == nil && st.Mode()&os.ModeSymlink != 0 {
			e.isSymlink = true
			if t, err := filepath.EvalSymlinks(full); err == nil {
				e.target = t
			} else if link, err := os.Readlink(full); err == nil {
				if !filepath.IsAbs(link) {
					link = filepath.Join(bin, link)
				}
				e.target = link
			}
		}
		entries[name] = e
	}
	// sidecar を紐付け
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".mlines.json") {
			continue
		}
		alias := strings.TrimSuffix(name, ".mlines.json")
		data, err := os.ReadFile(filepath.Join(bin, name))
		if err != nil {
			continue
		}
		var m linkMeta
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		e, ok := entries[alias]
		if !ok {
			e = &binEntry{}
			entries[alias] = e
		}
		e.meta = &m
	}
	return entries
}

// collectScripts は全リポジトリの script 一覧を、bin 側の状態付きで返す。
func collectScripts(bin string) ([]ScriptItem, error) {
	repos, err := listRepos()
	if err != nil {
		return nil, err
	}
	entries := scanBin(bin)
	var items []ScriptItem
	for _, r := range repos {
		if r.Manifest == nil {
			continue
		}
		names := make([]string, 0, len(r.Manifest.Scripts))
		for n := range r.Manifest.Scripts {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			se := r.Manifest.Scripts[n]
			it := ScriptItem{
				ID:          r.Name + "/" + n,
				Repo:        r.Name,
				Name:        n,
				Type:        se.Type,
				Description: se.Description,
			}
			// 1. symlink が指す実体で逆引き (shell_script)
			if se.Type == "shell_script" {
				src := r.Manifest.scriptSourcePath(r.Path, n)
				for alias, e := range entries {
					if e.isSymlink && e.target != "" && samePath(e.target, src) {
						it.Enabled = true
						it.EnabledAs = alias
						break
					}
				}
			}
			// 2. sidecar で逆引き (binary。alias 変更にも追従)
			if !it.Enabled {
				for alias, e := range entries {
					if e.meta != nil && e.meta.Repo == r.Name && e.meta.Script == n {
						it.Enabled = true
						it.EnabledAs = alias
						break
					}
				}
			}
			// 3. 同名ファイルの衝突検出
			if !it.Enabled {
				if e, ok := entries[n]; ok && e.path != "" {
					it.Conflict = describeConflict(e)
				}
			}
			items = append(items, it)
		}
	}
	return items, nil
}

func describeConflict(e *binEntry) string {
	if e.isSymlink {
		return fmt.Sprintf("同名リンクあり (-> %s)", e.target)
	}
	if e.meta != nil {
		return fmt.Sprintf("同名ファイルあり (%s/%s のもの)", e.meta.Repo, e.meta.Script)
	}
	return "同名ファイルあり (mlines管理外)"
}

func samePath(a, b string) bool {
	// macOS の /tmp -> /private/tmp のような symlink 解決差を吸収する
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return ca == cb
}

func findScriptItem(items []ScriptItem, repo, name string) *ScriptItem {
	for i := range items {
		if items[i].Repo == repo && items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

// parseScriptRef は "repo/name" を分解する。
func parseScriptRef(ref string) (repo, name string, err error) {
	parts := strings.SplitN(strings.TrimSpace(ref), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%q は <repo>/<script> の形式で指定してください (例: cocoa-gists/gi)", ref)
	}
	if err := validRepoName(parts[0]); err != nil {
		return "", "", err
	}
	if err := validScriptName(parts[1]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}

func validAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias が空です")
	}
	if strings.Contains(alias, "/") || alias == "." || alias == ".." {
		return fmt.Errorf("alias %q には / を含められません", alias)
	}
	if strings.HasSuffix(alias, ".mlines.json") {
		return fmt.Errorf("alias %q は予約接尾辞です", alias)
	}
	return validScriptName(alias)
}

// loadScriptEntry は repo/name の定義とリポジトリを返す。
func loadScriptEntry(repo, name string) (*Repo, ScriptEntry, error) {
	r, err := findRepo(repo)
	if err != nil {
		return nil, ScriptEntry{}, err
	}
	if r.Manifest == nil {
		return nil, ScriptEntry{}, fmt.Errorf("リポジトリ %q の mlines.json が読めません: %v", repo, r.ManifestErr)
	}
	se, ok := r.Manifest.Scripts[name]
	if !ok {
		return nil, ScriptEntry{}, fmt.Errorf("%s/%s が見つかりません", repo, name)
	}
	return r, se, nil
}

// enableScript は script を bin ディレクトリに有効化する。
func enableScript(bin, repo, name, alias string, force bool) (string, error) {
	r, se, err := loadScriptEntry(repo, name)
	if err != nil {
		return "", err
	}
	if alias == "" {
		alias = name
	}
	if err := validAlias(alias); err != nil {
		return "", err
	}
	if err := ensureDir(bin); err != nil {
		return "", fmt.Errorf("bin ディレクトリの作成に失敗: %w", err)
	}
	dest := filepath.Join(bin, alias)
	items, _ := collectScripts(bin)
	if cur := findScriptItem(items, repo, name); cur != nil && cur.Enabled {
		if cur.EnabledAs == alias {
			return "", fmt.Errorf("%s/%s は既に %s として有効です", repo, name, alias)
		}
		// 別名で有効化済み: 既定では案内して終了、force なら両立を許す (別名なので共存可能)
		if !force {
			return "", fmt.Errorf("%s/%s は既に %s として有効です (別名 %s で追加するには --force を付けてください)",
				repo, name, cur.EnabledAs, alias)
		}
	}
	if st, err := os.Lstat(dest); err == nil {
		_ = st
		if !force {
			return "", fmt.Errorf("%s は既に存在します (--name で別名指定、または --force で上書き)", dest)
		}
		os.Remove(dest)
		os.Remove(metaPath(bin, alias))
	}
	scriptID := repo + "/" + name
	switch se.Type {
	case "shell_script":
		src := r.Manifest.scriptSourcePath(r.Path, name)
		if _, err := os.Stat(src); err != nil {
			return "", fmt.Errorf("実体が見つかりません: %s", src)
		}
		if err := verifyShellScript(r.Manifest, r.Path, name); err != nil {
			return "", err
		}
		if err := os.Symlink(src, dest); err != nil {
			return "", fmt.Errorf("symlink 作成に失敗: %w", err)
		}
		// clone で実行ビットが落ちていると動かないため付与 (失敗しても致命傷ではない)
		_ = os.Chmod(src, 0o755)
		return fmt.Sprintf("%s -> %s", dest, src), nil
	case "binary":
		asset, regURL, err := se.resolveBinaryAsset(scriptID)
		if err != nil {
			return "", err
		}
		if err := installBinary(dest, asset); err != nil {
			return "", err
		}
		meta := linkMeta{Repo: repo, Script: name, Alias: alias, Type: "binary",
			URL: asset.File, Hash: strings.ToLower(asset.Hash), Platform: platformKey()}
		if regURL != "" {
			meta.URL = regURL + " [" + asset.File + "]"
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		_ = os.WriteFile(metaPath(bin, alias), data, 0o644)
		return dest, nil
	default:
		return "", fmt.Errorf("不明な type %q", se.Type)
	}
}

// disableScript は bin から取り除く。target は alias か repo/script。
func disableScript(bin, target string, force bool) (string, error) {
	var alias string
	if strings.Contains(target, "/") {
		repo, name, err := parseScriptRef(target)
		if err != nil {
			return "", err
		}
		items, _ := collectScripts(bin)
		cur := findScriptItem(items, repo, name)
		if cur == nil || !cur.Enabled {
			return "", fmt.Errorf("%s/%s は有効化されていません", repo, name)
		}
		alias = cur.EnabledAs
	} else {
		alias = strings.TrimSpace(target)
		if err := validAlias(alias); err != nil {
			return "", err
		}
	}
	dest := filepath.Join(bin, alias)
	st, err := os.Lstat(dest)
	if err != nil {
		return "", fmt.Errorf("%s は存在しません (有効化されていません)", alias)
	}
	_ = st
	// 管理外ファイルの保護
	entries := scanBin(bin)
	if e := entries[alias]; e != nil && !e.isSymlink && e.meta == nil && !force {
		return "", fmt.Errorf("%s は mlines管理外のファイルです (消す場合は --force)", dest)
	}
	// リポジトリ外を指す symlink の保護
	if e := entries[alias]; e != nil && e.isSymlink && e.target != "" && !underDir(e.target, reposDir()) && !force {
		return "", fmt.Errorf("%s はリポジトリ外 (-> %s) を指しています (消す場合は --force)", dest, e.target)
	}
	os.Remove(dest)
	os.Remove(metaPath(bin, alias))
	return alias, nil
}

// sweepRepoLinks は指定リポジトリ配下を指す symlink・sidecar と
// 対応する binary 実体を掃除する。
func sweepRepoLinks(repoName, repoPath, bin string) ([]string, error) {
	entries := scanBin(bin)
	var cleaned []string
	for alias, e := range entries {
		if e.isSymlink && e.target != "" && underDir(e.target, repoPath) {
			if err := os.Remove(e.path); err == nil {
				cleaned = append(cleaned, alias)
			}
			continue
		}
		if e.meta != nil && e.meta.Repo == repoName && e.path != "" {
			os.Remove(e.path)
			os.Remove(metaPath(bin, alias))
			cleaned = append(cleaned, alias)
		}
	}
	// 孤児 sidecar の掃除
	files, _ := os.ReadDir(bin)
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".mlines.json") {
			continue
		}
		alias := strings.TrimSuffix(name, ".mlines.json")
		if _, ok := entries[alias]; !ok || entries[alias].path == "" {
			os.Remove(filepath.Join(bin, name))
			cleaned = append(cleaned, alias+"(meta)")
		}
	}
	sort.Strings(cleaned)
	return cleaned, nil
}

func underDir(target, dir string) bool {
	if rt, err := filepath.EvalSymlinks(target); err == nil {
		target = rt
	}
	if rd, err := filepath.EvalSymlinks(dir); err == nil {
		dir = rd
	}
	ta, err1 := filepath.Abs(target)
	da, err2 := filepath.Abs(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(da, ta)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// installBinary は URL (https/http/file) から取得し hash 検証の上で配置する。
func installBinary(dest string, asset BinAsset) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".mlines-dl-*")
	if err != nil {
		return fmt.Errorf("一時ファイルの作成に失敗: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := downloadTo(asset.File, tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if strings.TrimSpace(asset.Hash) != "" {
		got, err := sha256File(tmpName)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, asset.Hash) {
			return fmt.Errorf("ハッシュ不一致: 想定 %s 実際 %s (中断しました)", strings.ToLower(asset.Hash), got)
		}
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("配置に失敗: %w", err)
	}
	return nil
}

func downloadTo(url string, w io.Writer) error {
	if strings.HasPrefix(url, "file://") {
		f, err := os.Open(strings.TrimPrefix(url, "file://"))
		if err != nil {
			return fmt.Errorf("ローカルファイルの読み取りに失敗: %w", err)
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("ダウンロードに失敗: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ダウンロードに失敗: HTTP %s", resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
