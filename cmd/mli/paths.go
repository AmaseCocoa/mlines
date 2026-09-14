package main

import (
	"os"
	"path/filepath"
	"strings"
)

// dataDir は mlines 関連ファイルの集約先を返す。
// とにかく ~/.local/share/mlines 以下に集約する (XDG_DATA_HOME の影響は受けない)。
// 明示的に変えたい場合のみ MLINES_DATA_DIR で上書きできる。
func dataDir() string {
	if v := os.Getenv("MLINES_DATA_DIR"); v != "" {
		return expandPath(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mlines")
}

// reposDir は git clone した script リポジトリ群の保存先を返す。
func reposDir() string {
	return filepath.Join(dataDir(), "repos")
}

// defaultBinDir は enable した script のリンク/実体を置くディレクトリを返す。
// mlines の管理フォルダ配下 (<data>/bin) を既定とする。
// 優先度: --bin-dir フラグ > MLINES_BIN_DIR > <data>/bin
func defaultBinDir(override string) string {
	if override != "" {
		return expandPath(override)
	}
	if v := os.Getenv("MLINES_BIN_DIR"); v != "" {
		return expandPath(v)
	}
	return filepath.Join(dataDir(), "bin")
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
