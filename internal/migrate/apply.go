package migrate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Apply は計画どおりにファイルを作成・変更・削除する。
func Apply(plan *Plan) error {
	for _, c := range plan.Changes {
		p := filepath.Join(plan.Dir, filepath.FromSlash(c.Path))
		switch c.Action {
		case "create", "modify":
			if c.Action == "create" {
				if _, err := os.Stat(p); err == nil {
					return fmt.Errorf("%s はすでにあります", c.Path)
				}
			}
			mode := os.FileMode(0o644)
			if info, err := os.Stat(p); err == nil {
				mode = info.Mode().Perm()
			}
			if err := os.WriteFile(p, []byte(c.Content), mode); err != nil {
				return err
			}
		case "delete":
			if err := os.Remove(p); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown action %q", c.Action)
		}
	}
	return nil
}

// CheckGitClean は dir が Git の作業ツリーの中にあり、dir 以下に未コミットの変更がないことを確かめる。
// 移行の結果は Git で確認・取り消しする前提なので、変更がある状態では書き込まない。
func CheckGitClean(dir string) error {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--", ".").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Git の作業ツリーではないか、git を実行できません: %s", strings.TrimSpace(string(out)))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		return fmt.Errorf("%w:\n%s", ErrDirty, s)
	}
	return nil
}

// WriteReport は計画 (変更と TODO) を人向けのテキストで出力する。
func WriteReport(w io.Writer, plan *Plan, dryRun bool) {
	fmt.Fprintf(w, "runnora-migrate: %s\n", filepath.ToSlash(plan.Dir))
	if len(plan.Changes) == 0 {
		fmt.Fprintln(w, "\n変更はありません (移行済み)")
	} else {
		fmt.Fprintf(w, "\n変更 (%d ファイル):\n", len(plan.Changes))
		for _, c := range plan.Changes {
			fmt.Fprintf(w, "  %-6s  %s\n", c.Action, c.Path)
			for _, n := range c.Notes {
				fmt.Fprintf(w, "          - %s\n", n)
			}
		}
	}
	if len(plan.Todos) > 0 {
		fmt.Fprintf(w, "\nTODO (%d 件):\n", len(plan.Todos))
		for _, t := range plan.Todos {
			fmt.Fprintf(w, "  - %s: %s\n", t.Where, t.Message)
		}
	}
	fmt.Fprintln(w)
	switch {
	case len(plan.Changes) == 0:
	case dryRun:
		fmt.Fprintln(w, "dry-run のため書き込んでいません。--write を付けると書き込みます。")
	default:
		fmt.Fprintln(w, "書き込みました。git diff で確認し、TODO に対応したあと runnora validate を実行してください。")
	}
}
