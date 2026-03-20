package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/k1LoW/donegroup"
	"github.com/k1LoW/runn"
	"github.com/ryo-yamaoka/otchkiss"
	"github.com/ryo-yamaoka/otchkiss/setting"
	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
)

// newLoadtCmd は "runnora loadt" (alias: "loadtest") サブコマンドを生成する。
//
// runbook を繰り返し実行して負荷テストを行う。
// 内部では otchkiss ライブラリを使って並行実行と RPS 制御を行い、
// 結果を runn.NewLoadtResult で集計する。
//
// 実行フロー:
//  1. donegroup.WithCancel でキャンセル可能なコンテキストを生成
//  2. runn.Load で runbook をロード
//  3. setting.New で並行数・RPS・時間設定を構築
//  4. otchkiss.FromConfig で負荷テストエンジンを初期化
//  5. ot.Start で負荷テストを実行 (duration + warm-up の間)
//  6. 結果を集計し threshold チェックを行う
func newLoadtCmd() *cobra.Command {
	var (
		concurrent int
		duration   string
		warmUp     string
		maxRPS     int
		threshold  string
		format     string
	)

	cmd := &cobra.Command{
		Use:     "loadt [PATH_PATTERN ...]",
		Short:   "runbook を使って負荷テストを実行する",
		Aliases: []string{"loadtest"},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) == 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("at least one path pattern is required")}
			}

			// donegroup は goroutine のライフサイクルを管理する。
			// defer で cancel + Wait を呼ぶことで、終了時に全 goroutine の完了を保証する。
			ctx, cancel := donegroup.WithCancel(context.Background())
			defer func() {
				cancel()
				err = errors.Join(err, donegroup.Wait(ctx))
			}()

			pathp := strings.Join(args, string(filepath.ListSeparator))

			opts := []runn.Option{
				runn.Scopes("read:parent"),
			}

			op, err := runn.Load(pathp, opts...)
			if err != nil {
				return fmt.Errorf("loadt: load: %w", err)
			}

			d, err := parseDuration(duration)
			if err != nil {
				return fmt.Errorf("loadt: parse duration %q: %w", duration, err)
			}
			w, err := parseDuration(warmUp)
			if err != nil {
				return fmt.Errorf("loadt: parse warm-up %q: %w", warmUp, err)
			}

			// setting.New は otchkiss の設定を構築する。
			// concurrent: 同時実行ゴルーチン数, maxRPS: レート制限, d: テスト時間, w: ウォームアップ時間
			s, err := setting.New(concurrent, maxRPS, d, w)
			if err != nil {
				return fmt.Errorf("loadt: setting: %w", err)
			}

			selected, err := op.SelectedOperators()
			if err != nil {
				return fmt.Errorf("loadt: select operators: %w", err)
			}

			// operatorN は otchkiss.Requester インターフェースを満たす。
			// 第三引数 100_000_000 はリクエスト総数の上限 (事実上の無制限)。
			// 実際の停止は duration によって制御される。
			ot, err := otchkiss.FromConfig(op, s, 100_000_000)
			if err != nil {
				return fmt.Errorf("loadt: otchkiss config: %w", err)
			}

			if err := ot.Start(ctx); err != nil {
				return fmt.Errorf("loadt: start: %w", err)
			}

			// 集計: selected オペレータ数, ウォームアップ時間, 測定時間, 並行数, maxRPS, otchkiss の生結果
			lr, err := runn.NewLoadtResult(len(selected), w, d, concurrent, maxRPS, ot.Result)
			if err != nil {
				return fmt.Errorf("loadt: result: %w", err)
			}

			// threshold は "error_rate < 0.01" のような式で合否を判定する
			if threshold != "" {
				if err := lr.CheckThreshold(threshold); err != nil {
					return err
				}
			}

			out := cmd.OutOrStdout()
			if format == "json" {
				return lr.ReportJSON(out)
			}
			return lr.Report(out)
		},
	}

	cmd.Flags().IntVar(&concurrent, "load-concurrent", 1, "同時実行数")
	cmd.Flags().StringVar(&duration, "duration", "10s", "負荷テスト時間")
	cmd.Flags().StringVar(&warmUp, "warm-up", "5s", "ウォームアップ時間")
	cmd.Flags().IntVar(&maxRPS, "max-rps", 1, "最大 RPS")
	cmd.Flags().StringVar(&threshold, "threshold", "", "合否判定式 (例: \"error_rate < 0.01\")")
	cmd.Flags().StringVar(&format, "format", "", "出力形式 (json)")

	return cmd
}

// parseDuration は "10s" や "5sec" 形式の文字列を time.Duration に変換する。
// time.ParseDuration で解析できない場合は "%fsec" フォールバックを試みる。
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// "5sec" のような非標準表記に対応するフォールバック
	var n float64
	if _, scanErr := fmt.Sscanf(s, "%fsec", &n); scanErr == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	return 0, fmt.Errorf("invalid duration %q: %w", s, err)
}
