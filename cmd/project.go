package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// loadedProject は見つかった runnora.yaml と、パスの基準ディレクトリ。
type loadedProject struct {
	// P は読み込んだ runnora.yaml。見つからなかった場合は nil。
	P *project.Project
	// Root は相対パスの基準。runnora.yaml があればプロジェクトルート、なければカレントディレクトリ。
	Root string
}

// loadProject は runnora.yaml を探して読み込む。
//
// explicit (--project) が空なら、カレントディレクトリから親へ向かって探す。
// 見つからない場合はプロジェクトなしとして扱う。そのときカレントディレクトリに旧形式の
// config.yaml があれば、読み込まないことを警告する。
func loadProject(explicit string, stderr io.Writer) (*loadedProject, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, &app.AppError{ExitCode: 2, Cause: err}
	}
	path, err := project.Find(explicit, cwd)
	if err != nil {
		return nil, &app.AppError{ExitCode: 2, Cause: err}
	}
	if path == "" {
		if _, statErr := os.Stat(filepath.Join(cwd, "config.yaml")); statErr == nil {
			fmt.Fprintf(stderr, "警告: runnora.yaml が見つかりません。config.yaml は旧形式のため読み込みません (%s)\n", project.MigrateHint)
		}
		return &loadedProject{Root: cwd}, nil
	}
	p, err := project.Load(path)
	if err != nil {
		return nil, &app.AppError{ExitCode: 2, Cause: err}
	}
	return &loadedProject{P: p, Root: p.Root}, nil
}

// rejectLegacyFlags は廃止したフラグが指定されていたらエラーを返す。
// 廃止したフラグは、案内を出すために非表示のまま残している。
func rejectLegacyFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("config") {
		return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("--config は廃止されました。%s", project.MigrateHint)}
	}
	for _, name := range []string{"before-sql", "after-sql"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("--%s は廃止されました。前後処理は runbook の runnora: ブロック (before / after) か runnora.yaml の environments.<名前>.hooks に書いてください (runnora-migrate で移行できます)", name)}
		}
	}
	return nil
}

// addLegacyFlags は廃止したフラグを非表示で登録する (指定されたら案内を出すため)。
func addLegacyFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		switch name {
		case "config":
			cmd.Flags().String(name, "", "廃止 (runnora.yaml を使う)")
		default:
			cmd.Flags().StringArray(name, nil, "廃止 (runbook の runnora: ブロックを使う)")
		}
		_ = cmd.Flags().MarkHidden(name)
	}
}

// parseVars は --var NAME=VALUE の一覧を map にする。
func parseVars(vars []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range vars {
		name, value, ok := strings.Cut(v, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--var %q は NAME=VALUE の形で指定してください", v)
		}
		out[name] = value
	}
	return out, nil
}

// undefinedRunbookVars は runbook が参照している変数のうち、変数表にも OS の環境変数にもなく、
// 既定値もないものを "runbook: NAME, ..." の形で返す。
func undefinedRunbookVars(rbs []*scenario.Runbook, vars map[string]string) []string {
	var out []string
	for _, rb := range rbs {
		if names := undefinedIn(rb.Text, vars); len(names) > 0 {
			out = append(out, fmt.Sprintf("%s: %s", rb.Path, strings.Join(names, ", ")))
		}
	}
	return out
}

func undefinedIn(text string, vars map[string]string) []string {
	var names []string
	for _, ref := range project.Refs(withoutCommentLines(text)) {
		if ref.HasDefault {
			continue
		}
		if _, ok := vars[ref.Name]; ok {
			continue
		}
		if _, ok := os.LookupEnv(ref.Name); ok {
			continue
		}
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	return names
}

// withoutCommentLines は YAML の行コメント (# で始まる行) を除く。
// runn はコメントの中の ${...} を展開しないため、検査の対象から外す。
func withoutCommentLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
