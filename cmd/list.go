package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/scenario"
)

// listEntry は一覧表示の 1 行分のデータを表す。
// JSON 出力にも使うため json タグを付与している。
type listEntry struct {
	ID         string   `json:"id"`
	Scenario   string   `json:"scenario,omitempty"` // runnora: ブロックの id
	Suites     []string `json:"suites,omitempty"`   // この runbook を選ぶ runnora.yaml のスイート
	Desc       string   `json:"desc,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	If         string   `json:"if,omitempty"`
	StepsCount int      `json:"steps_count"`
	Path       string   `json:"path"`
}

// newListCmd は "runnora list" (alias: "ls") サブコマンドを生成する。
//
// 指定したパスパターンにマッチする runbook のメタデータ一覧を表示する。
// runn.LoadOnly() を使うことで runbook の内容を解析するだけで実際には実行しない。
func newListCmd() *cobra.Command {
	var (
		long        bool
		format      string
		projectPath string
	)

	cmd := &cobra.Command{
		Use:     "list [PATH_PATTERN ...]",
		Short:   "runbook を一覧表示する",
		Aliases: []string{"ls"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 複数パターンを OS のパス区切り文字で連結する。
			// runn.Load はこの形式でグロブを展開する。
			pathp := strings.Join(args, string(filepath.ListSeparator))

			opts := []runn.Option{
				runn.LoadOnly(),            // 実行せずにロードだけ行う
				runn.Scopes("read:parent"), // 親ディレクトリのファイルへのアクセスを許可
			}

			op, err := runn.Load(pathp, opts...)
			if err != nil {
				return fmt.Errorf("list: load: %w", err)
			}

			// SelectedOperators は条件フィルタ後のオペレータ一覧を返す
			selected, err := op.SelectedOperators()
			if err != nil {
				return fmt.Errorf("list: select: %w", err)
			}

			// runnora.yaml があれば、各スイートが選ぶ runbook を調べる
			lp, err := loadProject(projectPath, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			suitesOf := suiteMembership(lp, cmd.ErrOrStderr())

			// オペレータから listEntry に変換する
			entries := make([]*listEntry, 0, len(selected))
			for _, o := range selected {
				e := &listEntry{
					ID:         o.ID(),
					Desc:       o.Desc(),
					Labels:     o.Labels(),
					If:         o.If(),
					StepsCount: o.NumberOfSteps(),
					Path:       o.BookPath(),
				}
				if rb, err := scenario.Read(o.BookPath(), lp.Root); err == nil {
					if rb.Meta != nil {
						e.Scenario = rb.ID
					}
					e.Suites = suitesOf[rb.Path]
				}
				entries = append(entries, e)
			}

			// JSON 形式出力
			if format == "json" {
				b, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return fmt.Errorf("list: json: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}

			// テキスト形式で出力
			w := cmd.OutOrStdout()
			if long {
				// --long: フル ID (UUID 形式 44 文字) を表示する
				fmt.Fprintf(w, "%-44s  %-16s  %-30s  %5s  %-20s  %s\n", "id", "scenario", "desc", "steps", "suites", "path")
				fmt.Fprintf(w, "%-44s  %-16s  %-30s  %5s  %-20s  %s\n",
					strings.Repeat("-", 44), strings.Repeat("-", 16), strings.Repeat("-", 30), "-----", strings.Repeat("-", 20), strings.Repeat("-", 20))
				for _, e := range entries {
					fmt.Fprintf(w, "%-44s  %-16s  %-30s  %5d  %-20s  %s\n", e.ID, e.Scenario, e.Desc, e.StepsCount, strings.Join(e.Suites, ","), e.Path)
				}
			} else {
				// 通常: ID を先頭 8 文字に短縮して表示する
				fmt.Fprintf(w, "%-8s  %-16s  %-30s  %5s  %-20s  %s\n", "id", "scenario", "desc", "steps", "suites", "path")
				fmt.Fprintf(w, "%-8s  %-16s  %-30s  %5s  %-20s  %s\n",
					"--------", strings.Repeat("-", 16), strings.Repeat("-", 30), "-----", strings.Repeat("-", 20), strings.Repeat("-", 20))
				for _, e := range entries {
					id := e.ID
					if len(id) > 8 {
						id = id[:8]
					}
					fmt.Fprintf(w, "%-8s  %-16s  %-30s  %5d  %-20s  %s\n", id, e.Scenario, e.Desc, e.StepsCount, strings.Join(e.Suites, ","), e.Path)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&long, "long", "l", false, "フル ID とパスを表示する")
	cmd.Flags().StringVar(&format, "format", "", "出力形式 (json)")
	cmd.Flags().StringVar(&projectPath, "project", "", "runnora.yaml のパス (省略時はカレントディレクトリから親へ探す)")

	return cmd
}

// suiteMembership は runbook の絶対パスごとに、それを選ぶスイート名の一覧を返す。
// スイートの条件を評価できない場合は警告を出して、そのスイートを除く。
func suiteMembership(lp *loadedProject, stderr io.Writer) map[string][]string {
	out := map[string][]string{}
	if lp.P == nil {
		return out
	}
	for _, name := range lp.P.SuiteNames() {
		rbs, err := scenario.Select(lp.Root, lp.P.Suites[name].Select)
		if err != nil {
			fmt.Fprintf(stderr, "警告: スイート %s: %v\n", name, err)
			continue
		}
		for _, rb := range rbs {
			out[rb.Path] = append(out[rb.Path], name)
		}
	}
	return out
}
