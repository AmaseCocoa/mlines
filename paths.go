package main

import (
	"os"
	"path/filepath"
	"strings"
)

// dataDir はリポジトリ群の保存先を返す。
// XDG_DATA_HOME/mlines を優先し、なければ ~/.local/share/mlines を使う。
func dataDir() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "mlines")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mlines")
}

// reposDir は git clone した script リポジトリ群の保存先を返す。
func reposDir() string {
	return filepath.Join(dataDir(), "repos")
}

// defaultBinDir は enable した script のリンク/実体を置くディレクトリを返す。
// 優先度: --bin-dir フラグ > MLINES_BIN_DIR > ~/.local/bin
func defaultBinDir(override string) string {
	if override != "" {
		return expandPath(override)
	}
	if v := os.Getenv("MLINES_BIN_DIR"); v != "" {
		return expandPath(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}
