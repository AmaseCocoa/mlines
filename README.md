# mlines
いろいろなscriptの有効/無効を直感的に管理するためのScript Manager

## Repository
script群はgitのリポジトリとして管理されます。

```shell
mli repo add https://github.com/AmaseCocoa/gists.git cocoa-gists
```

削除する場合

```shell
mli repo remove cocoa-gists
```

## Commands

```shell
mli enable cocoa-gists/gi

# 別名でsymlinkを張る場合
mli enable cocoa-gists/gi --name gitignore
```

```shell
mli disable cocoa-gists/gi
```

```shell
mli repo sync-all

mli repo sync cocoa-gists
```

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
            "path": "gi"
        },
        "alter": {
            "type": "binary",
            "description": "gitのユーザーを切り替えるツール",
            "bin": {
                "darwin-arm64": {
                    "file": "<url>",
                    "hash": "<hash>"
                },
                ...
            }
        },
        "alter": {
            "type": "binary",
            "description": "gitのユーザーを切り替えるツール",
            "registry": "<url>"
        }
    }
}
```
- registryのファイルはbinキーの中身を外部化しただけのフォーマットになる想定
- typeがshell_scriptの場合は念のために厳密なファイル自体のチェックも行う (簡易的なマルウェア対策)
- 念のためこれらのためのjsonschemaも作っておく
