package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed schema/mlines.schema.json
var manifestSchema []byte

// Manifest は各 script リポジトリ直下の mlines.json に対応する。
type Manifest struct {
	Description string                 `json:"description"`
	ScriptRoot  string                 `json:"script_root"`
	Scripts     map[string]ScriptEntry `json:"scripts"`
}

// ScriptEntry は1つの script の定義。
type ScriptEntry struct {
	Type        string              `json:"type"` // shell_script | binary
	Description string              `json:"description"`
	Path        string              `json:"path,omitempty"`     // shell_script用
	Sha256      string              `json:"sha256,omitempty"`   // shell_script用 (任意・推奨)
	Bin         map[string]BinAsset `json:"bin,omitempty"`      // binary用 (inline)
	Registry    string              `json:"registry,omitempty"` // binary用 (外部URL)
}

// BinAsset は特定プラットフォーム向けバイナリの取得先。
type BinAsset struct {
	File string `json:"file"`
	Hash string `json:"hash"`
}

// platformKey は runtime 由来の "<goos>-<goarch>" を返す。
func platformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// defaultScriptRoot は script_root 未指定時の既定値。
const defaultScriptRoot = "./scripts"

func manifestPath(repoPath string) string {
	return filepath.Join(repoPath, "mlines.json")
}

// loadManifest は mlines.json を読み、既定値補完と検証を行う。
func loadManifest(repoPath string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath(repoPath))
	if err != nil {
		return nil, fmt.Errorf("mlines.json が読めません: %w", err)
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("mlines.json の解析に失敗: %w", err)
	}
	if strings.TrimSpace(m.ScriptRoot) == "" {
		m.ScriptRoot = defaultScriptRoot
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate は簡易的なスキーマ検証を行う。
// 厳密な JSON Schema 検証が必要な場合は `mli schema` の出力を ajv 等に渡す。
func (m *Manifest) Validate() error {
	if m.Scripts == nil || len(m.Scripts) == 0 {
		return fmt.Errorf("scripts が空です (1件以上必要)")
	}
	for name, s := range m.Scripts {
		if err := validScriptName(name); err != nil {
			return fmt.Errorf("script %q: %w", name, err)
		}
		switch s.Type {
		case "shell_script":
			if strings.TrimSpace(s.Path) == "" {
				return fmt.Errorf("script %q: shell_script には path が必須です", name)
			}
			if filepath.IsAbs(s.Path) || strings.Contains(s.Path, "..") {
				return fmt.Errorf("script %q: path はリポジトリ相対で指定してください", name)
			}
			if s.Sha256 != "" && !isHex64(s.Sha256) {
				return fmt.Errorf("script %q: sha256 は64桁の16進数で指定してください", name)
			}
			if len(s.Bin) > 0 || s.Registry != "" {
				return fmt.Errorf("script %q: shell_script に bin/registry は指定できません", name)
			}
		case "binary":
			hasBin := len(s.Bin) > 0
			hasReg := strings.TrimSpace(s.Registry) != ""
			if !hasBin && !hasReg {
				return fmt.Errorf("script %q: binary には bin か registry のどちらかが必要です", name)
			}
			if hasBin && hasReg {
				return fmt.Errorf("script %q: bin と registry は同時に指定できません", name)
			}
			if s.Path != "" {
				return fmt.Errorf("script %q: binary に path は指定できません", name)
			}
			for plat, a := range s.Bin {
				if strings.TrimSpace(a.File) == "" {
					return fmt.Errorf("script %q: bin[%s] の file が空です", name, plat)
				}
				if a.Hash != "" && !isHex64(a.Hash) {
					return fmt.Errorf("script %q: bin[%s] の hash は64桁の16進数で指定してください", name, plat)
				}
			}
		case "":
			return fmt.Errorf("script %q: type が必須です (shell_script|binary)", name)
		default:
			return fmt.Errorf("script %q: 不明な type %q (shell_script|binary)", name, s.Type)
		}
	}
	return nil
}

// scriptSourcePath は shell_script の実体パス (絶対パス) を返す。
func (m *Manifest) scriptSourcePath(repoPath, name string) string {
	s := m.Scripts[name]
	rel := filepath.FromSlash(s.Path)
	return filepath.Join(repoPath, filepath.FromSlash(m.ScriptRoot), rel)
}

// resolveBinaryAsset は現行プラットフォーム向けアセットを返す。
// registry 指定時は外部JSONを取得する。
func (s *ScriptEntry) resolveBinaryAsset(scriptID string) (BinAsset, string, error) {
	plat := platformKey()
	if len(s.Bin) > 0 {
		a, ok := s.Bin[plat]
		if !ok {
			return BinAsset{}, "", fmt.Errorf("%s: プラットフォーム %s 向けの bin 定義がありません (利用可能: %s)",
				scriptID, plat, availablePlatforms(s.Bin))
		}
		return a, "", nil
	}
	reg, err := fetchRegistry(s.Registry)
	if err != nil {
		return BinAsset{}, "", fmt.Errorf("%s: registry の取得に失敗: %w", scriptID, err)
	}
	a, ok := reg[plat]
	if !ok {
		return BinAsset{}, "", fmt.Errorf("%s: registry にプラットフォーム %s がありません (利用可能: %s)",
			scriptID, plat, availablePlatforms(reg))
	}
	return a, s.Registry, nil
}

func availablePlatforms(m map[string]BinAsset) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return "(なし)"
	}
	// 決定的な順序にする
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return strings.Join(keys, ", ")
}

// fetchRegistry は bin マップを外部化した JSON を取得する。
func fetchRegistry(url string) (map[string]BinAsset, error) {
	if strings.HasPrefix(url, "file://") {
		data, err := os.ReadFile(strings.TrimPrefix(url, "file://"))
		if err != nil {
			return nil, err
		}
		var m map[string]BinAsset
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return m, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	var m map[string]BinAsset
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// sha256File はファイルの SHA-256 hex を返す。
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyShellScript はマニフェストに sha256 がある場合に内容を検証する。
// (README: shell_script は念のため厳密なファイルチェックを行う)
func verifyShellScript(m *Manifest, repoPath, name string) error {
	want := strings.ToLower(m.Scripts[name].Sha256)
	if want == "" {
		return nil
	}
	got, err := sha256File(m.scriptSourcePath(repoPath, name))
	if err != nil {
		return fmt.Errorf("ハッシュ検証のための読み取りに失敗: %w", err)
	}
	if got != want {
		return fmt.Errorf("ハッシュ不一致: 想定 %s 実際 %s (改ざんの可能性。無視する場合はマニフェストの sha256 を更新してください)", want, got)
	}
	return nil
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func validScriptName(name string) error {
	if name == "" {
		return fmt.Errorf("空の名前は使えません")
	}
	for _, c := range name {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-'
		if !ok {
			return fmt.Errorf("名前 %q に使えない文字 %q が含まれています (英数字と . _ - のみ)", name, c)
		}
	}
	return nil
}
