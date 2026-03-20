// このファイルは "runnora rprof" コマンドを実装する。
//
// rprof (runbook profile) は runn が生成したプロファイルデータを人間が読みやすい
// テーブル形式で表示するツール。
//
// プロファイルデータの生成:
//   runn は実行中のスパン情報を stopw ライブラリで記録し、
//   JSON 形式でプロファイルファイルを出力する。
//   このコマンドはその JSON を読み込んで分析・表示する。
//
// stopw.Span の構造:
//   Span は木構造 (ツリー) で、各ノードが 1 つの処理単位 (ステップ等) を表す。
//   Breakdown フィールドに子スパンが入れ子で格納される。
//
// 使用例:
//
//	runnora rprof ./profile.json
//	runnora rprof --depth 2 --unit s --sort started-at ./profile.json
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/k1LoW/stopw"
	"github.com/spf13/cobra"
)

// validUnits は --unit フラグで指定できる時間単位の一覧。
// ヘルプメッセージや将来のバリデーションで使用する。
var validUnits = []string{"ns", "us", "ms", "s", "m"}

// validSorts は --sort フラグで指定できるソートキーの一覧。
var validSorts = []string{"elapsed", "started-at", "stopped-at"}

// rprofRow はテーブル表示用の 1 行データ。
// collectRows で stopw.Span ツリーをフラットなリストに変換するときに使う。
type rprofRow struct {
	label   string        // インデント付きのスパン識別子
	elapsed time.Duration // このスパンの所要時間
	started time.Time     // 開始時刻 (--sort started-at で使用)
	stopped time.Time     // 終了時刻 (--sort stopped-at で使用)
}

// newRprofCmd は "runnora rprof" (alias: "prof") サブコマンドを生成する。
//
// フラグ:
//   --depth: スパンツリーの展開深度。大きくすると詳細なブレークダウンを表示。
//   --unit:  経過時間の表示単位 (ns/us/ms/s/m)。デフォルトは ms。
//   --sort:  表示順序。elapsed (降順) / started-at / stopped-at (昇順)。
func newRprofCmd() *cobra.Command {
	var (
		depth  int
		unit   string
		sortBy string
	)

	cmd := &cobra.Command{
		Use:     "rprof [PROFILE_PATH]",
		Short:   "runbook 実行プロファイルを読み込んで表示する",
		Aliases: []string{"rrprof", "rrrprof", "prof"},
		Args:    cobra.ExactArgs(1), // プロファイルファイルパスを 1 つだけ受け取る
		RunE: func(cmd *cobra.Command, args []string) error {
			// プロファイルファイルを読み込む
			b, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("rprof: read %s: %w", args[0], err)
			}

			// JSON を stopw.Span にデシリアライズする。
			// stopw.Span は再帰的な木構造で、ルートスパンに子スパンが
			// Breakdown フィールドとして入れ子になっている。
			var s *stopw.Span
			if err := json.Unmarshal(b, &s); err != nil {
				return fmt.Errorf("rprof: parse profile: %w", err)
			}

			// Repair は欠損した時刻情報を子スパンから補完する。
			// JSON がディスクへの途中書き込みで不完全になった場合でも
			// 可能な範囲で修復してから表示する。
			s.Repair()

			// スパンツリーをフラットな行リストに変換する (depth まで展開)
			rows, err := collectRows(s, 0, depth)
			if err != nil {
				return fmt.Errorf("rprof: collect: %w", err)
			}

			// 指定されたキーで行をソートする
			sortRows(rows, sortBy)

			// テーブルヘッダーと各行を出力する
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-60s  %s\n", "label", "elapsed")
			fmt.Fprintf(w, "%-60s  %s\n", strings.Repeat("-", 60), "-------")
			for _, r := range rows {
				elapsed := formatElapsed(r.elapsed, unit)
				fmt.Fprintf(w, "%-60s  %s\n", r.label, elapsed)
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&depth, "depth", 4, "ブレークダウンの最大深度")
	cmd.Flags().StringVar(&unit, "unit", "ms", fmt.Sprintf("時間単位 (%s)", strings.Join(validUnits, "|")))
	cmd.Flags().StringVar(&sortBy, "sort", "elapsed", fmt.Sprintf("ソート順 (%s)", strings.Join(validSorts, "|")))

	return cmd
}

// collectRows はスパンツリーを深さ優先で走査し、表示用の行リストに変換する。
//
// 動作:
//   - 深さ depth がmaxDepth を超えると走査を打ち切る
//   - 各スパンの ID は fmt.Sprintf("%v", s.ID) で文字列化する
//     (stopw.Span.ID は any 型で、string/int/nil など様々な型を取る)
//   - ID が空または "<nil>" の場合は "(root)" と表示する
//   - インデントは depth に応じてスペース 2 つ × depth で付ける
//
// 例: depth=2 の場合
//
//	(root)              ... depth 0
//	  runbook1.yml      ... depth 1
//	    step:login      ... depth 2
//	    step:check      ... depth 2
func collectRows(s *stopw.Span, depth, maxDepth int) ([]rprofRow, error) {
	if s == nil || depth > maxDepth {
		return nil, nil
	}

	// stopw.Span.ID は any 型なので fmt.Sprintf で文字列に変換する。
	// string/int/nil など様々な型が入る可能性がある。
	label := fmt.Sprintf("%v", s.ID)
	if label == "" || label == "<nil>" {
		label = "(root)" // ルートスパンは ID が nil になることが多い
	}
	prefix := strings.Repeat("  ", depth) // 深さに応じてインデントを付ける

	row := rprofRow{
		label:   prefix + label,
		elapsed: s.Elapsed(),  // StartedAt - StoppedAt から計算される
		started: s.StartedAt,
		stopped: s.StoppedAt,
	}

	// まず自分自身を追加し、次に子スパンを再帰的に処理する (深さ優先)
	rows := []rprofRow{row}
	for _, child := range s.Breakdown {
		childRows, err := collectRows(child, depth+1, maxDepth)
		if err != nil {
			return nil, err
		}
		rows = append(rows, childRows...)
	}
	return rows, nil
}

// sortRows は rows を指定キーでソートする。
//
// ソートキー:
//   - "elapsed":    経過時間の降順 (最も遅いスパンが上)。デフォルト。
//   - "started-at": 開始時刻の昇順 (実行順序で表示)
//   - "stopped-at": 終了時刻の昇順 (終了順序で表示)
//
// sort.SliceStable を使って同一値の場合は元の順序を保つ (安定ソート)。
func sortRows(rows []rprofRow, by string) {
	switch by {
	case "started-at":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].started.Before(rows[j].started)
		})
	case "stopped-at":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].stopped.Before(rows[j].stopped)
		})
	default: // "elapsed": 経過時間の降順 (遅い順)
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].elapsed > rows[j].elapsed
		})
	}
}

// formatElapsed は time.Duration を指定単位の文字列に変換する。
//
// 単位と精度:
//   - "ns": ナノ秒 (整数)
//   - "us": マイクロ秒 (小数点 3 桁)
//   - "ms": ミリ秒 (小数点 3 桁) — デフォルト
//   - "s":  秒 (小数点 3 桁)
//   - "m":  分 (小数点 4 桁)
func formatElapsed(d time.Duration, unit string) string {
	switch unit {
	case "ns":
		return fmt.Sprintf("%d ns", d.Nanoseconds())
	case "us":
		return fmt.Sprintf("%.3f us", float64(d.Nanoseconds())/1e3)
	case "s":
		return fmt.Sprintf("%.3f s", d.Seconds())
	case "m":
		return fmt.Sprintf("%.4f m", d.Minutes())
	default: // "ms"
		return fmt.Sprintf("%.3f ms", float64(d.Nanoseconds())/1e6)
	}
}
