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

var validUnits = []string{"ns", "us", "ms", "s", "m"}
var validSorts = []string{"elapsed", "started-at", "stopped-at"}

type rprofRow struct {
	label   string
	elapsed time.Duration
	started time.Time
	stopped time.Time
}

func newRprofCmd() *cobra.Command {
	var (
		depth int
		unit  string
		sortBy string
	)

	cmd := &cobra.Command{
		Use:     "rprof [PROFILE_PATH]",
		Short:   "runbook 実行プロファイルを読み込んで表示する",
		Aliases: []string{"rrprof", "rrrprof", "prof"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("rprof: read %s: %w", args[0], err)
			}

			var s *stopw.Span
			if err := json.Unmarshal(b, &s); err != nil {
				return fmt.Errorf("rprof: parse profile: %w", err)
			}
			s.Repair()

			rows, err := collectRows(s, 0, depth)
			if err != nil {
				return fmt.Errorf("rprof: collect: %w", err)
			}

			sortRows(rows, sortBy)

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
	cmd.Flags().StringVar(&unit, "unit", "ms", "時間単位 (ns|us|ms|s|m)")
	cmd.Flags().StringVar(&sortBy, "sort", "elapsed", "ソート順 (elapsed|started-at|stopped-at)")

	return cmd
}

func collectRows(s *stopw.Span, depth, maxDepth int) ([]rprofRow, error) {
	if s == nil || depth > maxDepth {
		return nil, nil
	}

	label := fmt.Sprintf("%v", s.ID)
	if label == "" || label == "<nil>" {
		label = "(root)"
	}
	prefix := strings.Repeat("  ", depth)

	row := rprofRow{
		label:   prefix + label,
		elapsed: s.Elapsed(),
		started: s.StartedAt,
		stopped: s.StoppedAt,
	}

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
	default: // "elapsed"
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].elapsed > rows[j].elapsed
		})
	}
}

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
