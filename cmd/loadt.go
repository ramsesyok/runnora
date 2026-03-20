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

			s, err := setting.New(concurrent, maxRPS, d, w)
			if err != nil {
				return fmt.Errorf("loadt: setting: %w", err)
			}

			selected, err := op.SelectedOperators()
			if err != nil {
				return fmt.Errorf("loadt: select operators: %w", err)
			}

			// operatorN は otchkiss.Requester インターフェースを満たす
			ot, err := otchkiss.FromConfig(op, s, 100_000_000)
			if err != nil {
				return fmt.Errorf("loadt: otchkiss config: %w", err)
			}

			if err := ot.Start(ctx); err != nil {
				return fmt.Errorf("loadt: start: %w", err)
			}

			lr, err := runn.NewLoadtResult(len(selected), w, d, concurrent, maxRPS, ot.Result)
			if err != nil {
				return fmt.Errorf("loadt: result: %w", err)
			}

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
	cmd.Flags().StringVar(&threshold, "threshold", "", "合否判定式")
	cmd.Flags().StringVar(&format, "format", "", "出力形式 (json)")

	return cmd
}

// parseDuration は "10s" や "5sec" 形式の文字列を time.Duration に変換する。
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	var n float64
	if _, scanErr := fmt.Sscanf(s, "%fsec", &n); scanErr == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	return 0, fmt.Errorf("invalid duration %q: %w", s, err)
}
