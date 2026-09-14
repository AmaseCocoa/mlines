# mlines
いろいろなscriptの有効/無効を直感的に管理するためのScript Manager

## Install

```shell
go build -o ~/.local/bin/mli .
```

有効化した script の置き場 (`~/.local/share/mlines/bin`) を `PATH` に入れておく:

```shell
export PATH="$HOME/.local/share/mlines/bin:$PATH"
```

`mli doctor` で `bin` ディレクトリと `PATH` を診断できる。

## TUI (推奨)

引数なし、または `tui` サブコマンドで直感的TUIが起動する。
`mli enable` / `mli disable` を覚える必要はない。

```shell
mli
mli tui
mli tui --filter gi
```

| キー | 動作 |
| ---- | ---- |
| `↑↓` / `j k` | 移動 (`g`/`G` で先頭/末尾) |
| `space` / `enter` | 有効/無効を切り替え |
| `/` | 絞り込み (enterで確定・escでクリア) |
| `a` | 別名で有効化 |
| `r` / `R` | このrepoをsync / 全repoをsync |
| `?` | ヘルプ表示 |
| `q` | 終了 |

非TTY (パイプ時) では `tui` は一覧表示にフォールバックする。

## Commands

CLIから直接操作もできる。

```shell
mli enable cocoa-gists/gi

# 別名でsymlinkを張る場合
mli enable cocoa-gists/gi --name gitignore
```

```shell
mli disable cocoa-gists/gi
# または alias 指定
mli disable gitignore
```

```shell
mli list
mli info cocoa-gists/gi
```

## Repository
script群はgitのリポジトリとして管理されます。

```shell
mli repo add https://github.com/AmaseCocoa/gists.git cocoa-gists
```

削除する場合 (関連リンクも自動で掃除される):

```shell
mli repo remove cocoa-gists
```

```shell
mli repo list

mli repo sync-all

mli repo sync cocoa-gists
```

## 保存先

| 用途 | 場所 |
| ---- | ---- |
| リポジトリ群 | `$XDG_DATA_HOME/mlines/repos/<name>` (既定 `~/.local/share/mlines/repos`) |
| 有効化先 | `$MLINES_BIN_DIR` または `--bin-dir` (既定 `~/.local/share/mlines/bin`) |

- `shell_script` は有効化先に symlink が張られる
- `binary` はダウンロード実体 + `<alias>.mlines.json` (管理メタ) が置かれる

## Architecture
```
repository/
    scripts/
        gi
        ...
    mlines.json
    README.md
```

mlines.json:
```json
{
    "description": "",
    "script_root": "./scripts",
    "scripts": {
        "gi": {
            "type": "shell_script",
            "description": "gitignoreを自動生成するscript",
            "path": "gi",
            "sha256": "<sha256 (任意・推奨)>"
        },
        "alter": {
            "type": "binary",
            "description": "gitのユーザーを切り替えるツール",
            "bin": {
                "darwin-arm64": {
                    "file": "<url>",
                    "hash": "<hash>"
                }
            }
        },
        "toolreg": {
            "type": "binary",
            "description": "gitのユーザーを切り替えるツール",
            "registry": "<url>"
        }
    }
}
```
- registryのファイルはbinキーの中身を外部化しただけのフォーマットになる想定
- typeがshell_scriptの場合は念のために厳密なファイル自体のチェックも行う (簡易的なマルウェア対策: `sha256` 指定時は enable 時に検証し、不一致なら中断する)
- binary の `hash` 指定時も同様にダウンロード検証を行い、不一致なら中断する
- JSON Schema は `schema/mlines.schema.json` (`mli schema` で出力も可能)
