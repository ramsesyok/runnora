package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"
)

type listEntry struct {
	ID         string   `json:"id"`
	Desc       string   `json:"desc,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	If         string   `json:"if,omitempty"`
	StepsCount int      `json:"steps_count"`
	Path       string   `json:"path"`
}

func newListCmd() *cobra.Command {
	var (
		long   bool
		format string
	)

	cmd := &cobra.Command{
		Use:     "list [PATH_PATTERN ...]",
		Short:   "runbook を一覧表示する",
		Aliases: []string{"ls"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathp := strings.Join(args, string(filepath.ListSeparator))

			opts := []runn.Option{
				runn.LoadOnly(),
				runn.Scopes("read:parent"),
			}

			op, err := runn.Load(pathp, opts...)
			if err != nil {
				return fmt.Errorf("list: load: %w", err)
			}

			selected, err := op.SelectedOperators()
			if err != nil {
				return fmt.Errorf("list: select: %w", err)
			}

			entries := make([]*listEntry, 0, len(selected))
			for _, o := range selected {
				entries = append(entries, &listEntry{
					ID:         o.ID(),
					Desc:       o.Desc(),
					Labels:     o.Labels(),
					If:         o.If(),
					StepsCount: o.NumberOfSteps(),
					Path:       o.BookPath(),
				})
			}

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
				fmt.Fprintf(w, "%-44s  %-30s  %5s  %s\n", "id", "desc", "steps", "path")
				fmt.Fprintf(w, "%-44s  %-30s  %5s  %s\n",
					strings.Repeat("-", 44), strings.Repeat("-", 30), "-----", strings.Repeat("-", 20))
				for _, e := range entries {
					fmt.Fprintf(w, "%-44s  %-30s  %5d  %s\n", e.ID, e.Desc, e.StepsCount, e.Path)
				}
			} else {
				fmt.Fprintf(w, "%-8s  %-30s  %5s  %s\n", "id", "desc", "steps", "path")
				fmt.Fprintf(w, "%-8s  %-30s  %5s  %s\n",
					"--------", strings.Repeat("-", 30), "-----", strings.Repeat("-", 20))
				for _, e := range entries {
					id := e.ID
					if len(id) > 8 {
						id = id[:8]
					}
					fmt.Fprintf(w, "%-8s  %-30s  %5d  %s\n", id, e.Desc, e.StepsCount, e.Path)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&long, "long", "l", false, "フル ID とパスを表示する")
	cmd.Flags().StringVar(&format, "format", "", "出力形式 (json)")

	return cmd
}
