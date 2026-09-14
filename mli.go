package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"golang.org/x/term"
)

const appVersion = "0.1.0"

func main() {
	app := cli.NewApp()
	app.Name = "mli"
	app.Usage = "script の有効/無効を直感的に管理する Script Manager"
	app.Version = appVersion
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "bin-dir",
			Usage:  "有効化先ディレクトリ (既定: $MLINES_BIN_DIR または <data>/bin)",
			EnvVar: "MLINES_BIN_DIR",
		},
	}
	app.Action = func(c *cli.Context) error {
		if c.NArg() == 0 {
			return runTUI(binDirOf(c), "")
		}
		return cli.ShowAppHelp(c)
	}
	app.Commands = []cli.Command{
		{
			Name:  "tui",
			Usage: "直感的TUIで有効/無効を切り替える",
			Flags: []cli.Flag{cli.StringFlag{Name: "filter", Usage: "初期フィルタ"}},
			Action: func(c *cli.Context) error {
				return runTUI(binDirOf(c), c.String("filter"))
			},
		},
		{
			Name:  "repo",
			Usage: "script リポジトリの管理",
			Subcommands: []cli.Command{
				{
					Name:      "add",
					Usage:     "リポジトリを追加する",
					ArgsUsage: "<url> <name>",
					Action: func(c *cli.Context) error {
						if c.NArg() != 2 {
							return fmt.Errorf("使い方: mli repo add <url> <name>")
						}
						url, name := c.Args().Get(0), c.Args().Get(1)
						if err := addRepo(url, name); err != nil {
							return err
						}
						fmt.Printf("追加しました: %s\n", name)
						return nil
					},
				},
				{
					Name:      "remove",
					Usage:     "リポジトリを削除する (関連リンクも掃除)",
					ArgsUsage: "<name>",
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return fmt.Errorf("使い方: mli repo remove <name>")
						}
						name := c.Args().First()
						if err := removeRepo(name, binDirOf(c)); err != nil {
							return err
						}
						fmt.Printf("削除しました: %s\n", name)
						return nil
					},
				},
				{
					Name:    "list",
					Aliases: []string{"ls"},
					Usage:   "リポジトリ一覧",
					Action: func(c *cli.Context) error {
						repos, err := listRepos()
						if err != nil {
							return err
						}
						if len(repos) == 0 {
							fmt.Println("リポジトリ未登録 (`mli repo add <url> <name>` で追加)")
							return nil
						}
						for _, r := range repos {
							n := 0
							desc := ""
							if r.Manifest != nil {
								n = len(r.Manifest.Scripts)
								desc = r.Manifest.Description
							}
							status := fmt.Sprintf("%d scripts", n)
							if r.ManifestErr != nil {
								status = "mlines.json エラー: " + r.ManifestErr.Error()
							}
							fmt.Printf("%s\t%s\t%s\n", r.Name, status, desc)
						}
						return nil
					},
				},
				{
					Name:      "sync",
					Usage:     "リポジトリを更新する (省略時は全件)",
					ArgsUsage: "[name]",
					Action: func(c *cli.Context) error {
						if c.NArg() == 0 {
							results := syncAllRepos()
							if len(results) == 0 {
								fmt.Println("リポジトリ未登録")
								return nil
							}
							failed := 0
							for name, err := range results {
								if err != nil {
									failed++
									fmt.Printf("%s: 失敗 (%v)\n", name, err)
								} else {
									fmt.Printf("%s: OK\n", name)
								}
							}
							if failed > 0 {
								return fmt.Errorf("%d件のsyncに失敗", failed)
							}
							return nil
						}
						out, err := syncRepo(c.Args().First())
						if err != nil {
							return err
						}
						fmt.Println(out)
						return nil
					},
				},
				{
					Name:  "sync-all",
					Usage: "全リポジトリを更新する (sync と同等)",
					Action: func(c *cli.Context) error {
						results := syncAllRepos()
						if len(results) == 0 {
							fmt.Println("リポジトリ未登録")
							return nil
						}
						failed := 0
						for name, err := range results {
							if err != nil {
								failed++
								fmt.Printf("%s: 失敗 (%v)\n", name, err)
							} else {
								fmt.Printf("%s: OK\n", name)
							}
						}
						if failed > 0 {
							return fmt.Errorf("%d件のsyncに失敗", failed)
						}
						return nil
					},
				},
			},
		},
		{
			Name:      "enable",
			Usage:     "script を有効化する",
			ArgsUsage: "<repo>/<script>",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "name", Usage: "別名で有効化する"},
				cli.BoolFlag{Name: "force", Usage: "上書きを許可する"},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() != 1 {
					return fmt.Errorf("使い方: mli enable <repo>/<script> [--name 別名]")
				}
				repo, name, err := parseScriptRef(c.Args().First())
				if err != nil {
					return err
				}
				dst, err := enableScript(binDirOf(c), repo, name, c.String("name"), c.Bool("force"))
				if err != nil {
					return err
				}
				fmt.Printf("有効化: %s\n", dst)
				return nil
			},
		},
		{
			Name:      "disable",
			Usage:     "script を無効化する",
			ArgsUsage: "<repo>/<script> または <alias>",
			Flags: []cli.Flag{
				cli.BoolFlag{Name: "force", Usage: "管理外ファイルの削除を許可する"},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() != 1 {
					return fmt.Errorf("使い方: mli disable <repo>/<script> または mli disable <alias>")
				}
				alias, err := disableScript(binDirOf(c), c.Args().First(), c.Bool("force"))
				if err != nil {
					return err
				}
				fmt.Printf("無効化: %s\n", alias)
				return nil
			},
		},
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "script 一覧 (状態付き)",
			Flags: []cli.Flag{
				cli.BoolFlag{Name: "json", Usage: "JSONで出力する"},
			},
			Action: func(c *cli.Context) error {
				return printList(binDirOf(c), c.Bool("json"))
			},
		},
		{
			Name:      "info",
			Usage:     "script の詳細を表示する",
			ArgsUsage: "<repo>/<script>",
			Action: func(c *cli.Context) error {
				if c.NArg() != 1 {
					return fmt.Errorf("使い方: mli info <repo>/<script>")
				}
				repo, name, err := parseScriptRef(c.Args().First())
				if err != nil {
					return err
				}
				return printInfo(binDirOf(c), repo, name)
			},
		},
		{
			Name:  "schema",
			Usage: "mlines.json の JSON Schema を出力する",
			Action: func(c *cli.Context) error {
				fmt.Println(string(manifestSchema))
				return nil
			},
		},
		{
			Name:  "doctor",
			Usage: "bin ディレクトリ・PATH・孤児リンクを診断する",
			Action: func(c *cli.Context) error {
				return runDoctor(binDirOf(c))
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "エラー: "+err.Error())
		os.Exit(1)
	}
}

func binDirOf(c *cli.Context) string {
	return defaultBinDir(c.GlobalString("bin-dir"))
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func printList(bin string, asJSON bool) error {
	items, err := collectScripts(bin)
	if err != nil {
		return err
	}
	if asJSON {
		data, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	if len(items) == 0 {
		fmt.Println("script がありません (`mli repo add` でリポジトリを追加)")
		return nil
	}
	for _, it := range items {
		mark := "○"
		extra := ""
		if it.Enabled {
			mark = "●"
			extra = " (" + it.EnabledAs + ")"
		} else if it.Conflict != "" {
			extra = " [! " + it.Conflict + "]"
		}
		fmt.Printf("%s %s [%s] %s%s\n", mark, it.ID, it.Type, it.Description, extra)
	}
	return nil
}

func printInfo(bin, repo, name string) error {
	r, se, err := loadScriptEntry(repo, name)
	if err != nil {
		return err
	}
	items, _ := collectScripts(bin)
	cur := findScriptItem(items, repo, name)
	fmt.Printf("ID:          %s/%s\n", repo, name)
	fmt.Printf("type:        %s\n", se.Type)
	fmt.Printf("description: %s\n", se.Description)
	switch se.Type {
	case "shell_script":
		fmt.Printf("path:        %s\n", se.Path)
		fmt.Printf("source:      %s\n", r.Manifest.scriptSourcePath(r.Path, name))
		if se.Sha256 != "" {
			fmt.Printf("sha256:      %s\n", strings.ToLower(se.Sha256))
		}
	case "binary":
		if se.Registry != "" {
			fmt.Printf("registry:    %s\n", se.Registry)
		} else {
			fmt.Printf("platforms:   %s\n", availablePlatforms(se.Bin))
		}
		fmt.Printf("platform:    %s\n", platformKey())
	}
	if cur != nil && cur.Enabled {
		fmt.Printf("status:      有効 (%s)\n", filepath.Join(bin, cur.EnabledAs))
	} else {
		fmt.Printf("status:      無効\n")
	}
	return nil
}

func runDoctor(bin string) error {
	failed := false
	// bin dir
	if st, err := os.Stat(bin); err != nil || !st.IsDir() {
		fmt.Printf("! bin ディレクトリがありません: %s (初回 enable 時に作成されます)\n", bin)
	} else {
		fmt.Printf("OK bin ディレクトリ: %s\n", bin)
	}
	// PATH
	pathEnv := os.Getenv("PATH")
	found := false
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if samePath(p, bin) {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("! %s が PATH に含まれていません (export PATH=\"%s:$PATH\" を追加してください)\n", bin, bin)
		failed = true
	} else {
		fmt.Printf("OK PATH に含まれています\n")
	}
	// repos
	repos, _ := listRepos()
	fmt.Printf("OK リポジトリ数: %d\n", len(repos))
	for _, r := range repos {
		if r.ManifestErr != nil {
			fmt.Printf("! %s: mlines.json エラー: %v\n", r.Name, r.ManifestErr)
			failed = true
		}
	}
	// 孤児リンク
	entries := scanBin(bin)
	for alias, e := range entries {
		if e.isSymlink && (e.target == "" || !dirExists(e.target)) {
			fmt.Printf("! 孤児リンク: %s (-> %s)\n", alias, e.target)
			failed = true
		}
		if e.meta != nil && e.path == "" {
			fmt.Printf("! 孤児メタ: %s (%s/%s)\n", alias, e.meta.Repo, e.meta.Script)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("診断で問題が見つかりました")
	}
	fmt.Println("すべて正常です")
	return nil
}

func dirExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
