// runnora-migrate は旧形式 (runnora v0.3.0 まで) のテストプロジェクトを新形式 (version 2) に移行する。
//
//	runnora-migrate [options] [dir]
//
// 既定は dry-run で、変更の予定と TODO を表示するだけにする。--write を付けると書き込む。
// 書き込む前に、Git の作業ツリーに未コミットの変更がないことを確かめる (結果を git diff で確認し、
// 必要なら git checkout で取り消せるようにするため)。
//
// 終了コード: 0 = 成功、1 = 失敗、2 = 引数・入力の誤り
//
// 移行が済んだら役目を終えるツールなので、runnora 本体のサブコマンドにはしない。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ramsesyok/runnora/internal/migrate"
)

type envFlags []migrate.EnvSpec

func (e *envFlags) String() string { return fmt.Sprint(*e) }

func (e *envFlags) Set(v string) error {
	name, file, ok := strings.Cut(v, "=")
	if !ok || name == "" || file == "" {
		return fmt.Errorf("--env は 名前=設定ファイル の形で指定してください (例: unit=config.yaml)")
	}
	*e = append(*e, migrate.EnvSpec{Name: name, File: file})
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("runnora-migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		envs       envFlags
		scenarios  string
		write      bool
		allowDirty bool
	)
	fs.Var(&envs, "env", "旧形式の設定ファイルと環境名の対応 名前=ファイル (複数指定可。省略時は config.yaml → default、config.<名前>.yaml → <名前>)")
	fs.StringVar(&scenarios, "scenarios", "", "シナリオ対応表 (runbook ごとの前後処理 SQL と期待する結果。dir 基準)")
	fs.BoolVar(&write, "write", false, "移行結果を書き込む (省略時は dry-run)")
	fs.BoolVar(&allowDirty, "allow-dirty", false, "Git の未コミットの変更があっても書き込む")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: runnora-migrate [options] [dir]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	plan, err := migrate.Build(migrate.Options{Dir: dir, Envs: envs, ScenariosFile: scenarios})
	if err != nil {
		fmt.Fprintf(stderr, "runnora-migrate: %v\n", err)
		return 2
	}
	if write && len(plan.Changes) > 0 {
		if !allowDirty {
			if err := migrate.CheckGitClean(plan.Dir); err != nil {
				fmt.Fprintf(stderr, "runnora-migrate: 書き込みを中止しました: %v\n", err)
				if errors.Is(err, migrate.ErrDirty) {
					fmt.Fprintln(stderr, "先にコミットするか、--allow-dirty を指定してください")
				}
				return 2
			}
		}
		if err := migrate.Apply(plan); err != nil {
			fmt.Fprintf(stderr, "runnora-migrate: %v\n", err)
			return 1
		}
	}
	migrate.WriteReport(stdout, plan, !write)
	return 0
}
