package scenario

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/internal/project"
)

// Select はスイートの条件に合う runbook を返す。
//
//   - paths の glob (root 基準、** 可) に一致し、runnora: ブロックを持つ runbook だけを対象にする。
//     ブロックのない runbook は include される部品とみなして除く。
//   - labels を指定すると、どれか 1 つを持つものに絞る。
//   - ids を指定すると、その ID のものに絞り ids の順に並べる。見つからない ID はエラー。
//   - ids がなければパスの順に並べる。
//   - 同じ ID の runbook が複数あればエラー。
func Select(root string, sel project.Selection) ([]*Runbook, error) {
	var matchers []*regexp.Regexp
	for _, p := range sel.Paths {
		re, err := globRegexp(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./"))
		if err != nil {
			return nil, fmt.Errorf("select.paths %q: %w", p, err)
		}
		matchers = append(matchers, re)
	}

	var candidates []*Runbook
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !matchAny(matchers, rel) {
			return nil
		}
		rb, err := Read(path, root)
		if err != nil {
			return err
		}
		if rb.Meta != nil && hasAnyLabel(rb.Labels, sel.Labels) {
			candidates = append(candidates, rb)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := CheckDuplicateIDs(candidates); err != nil {
		return nil, err
	}

	if len(sel.IDs) == 0 {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
		return candidates, nil
	}
	byID := map[string]*Runbook{}
	for _, rb := range candidates {
		byID[rb.ID] = rb
	}
	var (
		selected []*Runbook
		missing  []string
	)
	for _, id := range sel.IDs {
		rb, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		selected = append(selected, rb)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("select.ids: 条件に合う runbook が見つからない ID があります: %s", strings.Join(missing, ", "))
	}
	return selected, nil
}

// FromArgs はコマンドラインで指定された runbook (glob 可) を読み込む。
// runnora: ブロックの有無にかかわらず、指定されたものをすべて返す。
// base はブロックのない runbook の ID を作るときの基準ディレクトリ。
func FromArgs(args []string, base string) ([]*Runbook, error) {
	var paths []string
	for _, arg := range args {
		matched, err := expandArg(arg)
		if err != nil {
			return nil, err
		}
		if len(matched) == 0 {
			return nil, fmt.Errorf("runbook が見つかりません: %s", arg)
		}
		paths = append(paths, matched...)
	}
	var rbs []*Runbook
	seen := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		rb, err := Read(abs, base)
		if err != nil {
			return nil, err
		}
		rbs = append(rbs, rb)
	}
	if err := CheckDuplicateIDs(rbs); err != nil {
		return nil, err
	}
	return rbs, nil
}

// expandArg は glob 文字を含む引数を展開する。含まなければそのまま返す。
func expandArg(arg string) ([]string, error) {
	if !strings.ContainsAny(arg, "*?[") {
		if _, err := os.Stat(arg); err != nil {
			return nil, fmt.Errorf("runbook が見つかりません: %s", arg)
		}
		return []string{arg}, nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return nil, err
	}
	pattern := filepath.ToSlash(abs)
	// glob 文字を含まない先頭のディレクトリから探す。
	baseDir := pattern
	if i := strings.IndexAny(pattern, "*?["); i >= 0 {
		baseDir = pattern[:strings.LastIndex(pattern[:i], "/")+1]
	}
	re, err := globRegexp(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", arg, err)
	}
	var out []string
	err = filepath.WalkDir(filepath.FromSlash(baseDir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() && re.MatchString(filepath.ToSlash(path)) {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// globRegexp は glob (/ 区切り) を正規表現に変換する。
// * は / 以外の任意の文字列、? は / 以外の 1 文字、** は任意の階層 (0 階層を含む)。
func globRegexp(glob string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case c == '*' && strings.HasPrefix(glob[i:], "**/"):
			sb.WriteString("(?:.*/)?")
			i += 2
		case c == '*' && strings.HasPrefix(glob[i:], "**"):
			sb.WriteString(".*")
			i++
		case c == '*':
			sb.WriteString("[^/]*")
		case c == '?':
			sb.WriteString("[^/]")
		case c == '[':
			end := strings.IndexByte(glob[i:], ']')
			if end < 0 {
				return nil, errors.New("[ が閉じていません")
			}
			class := glob[i+1 : i+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + class + "]")
			i += end
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func hasAnyLabel(labels, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		for _, l := range labels {
			if l == w {
				return true
			}
		}
	}
	return false
}

// Includes は runbook のステップが include している runbook のパス (絶対パス) を返す。
// パスに変数や式 ({{ }}) を含むものは解決できないので除く。
func Includes(rb *Runbook) []string {
	var doc struct {
		Steps yaml.Node `yaml:"steps"`
	}
	if err := yaml.Unmarshal([]byte(rb.Text), &doc); err != nil {
		return nil
	}
	var steps []*yaml.Node
	switch doc.Steps.Kind {
	case yaml.MappingNode:
		for i := 1; i < len(doc.Steps.Content); i += 2 {
			steps = append(steps, doc.Steps.Content[i])
		}
	case yaml.SequenceNode:
		steps = doc.Steps.Content
	}
	dir := filepath.Dir(rb.Path)
	var out []string
	for _, st := range steps {
		if st.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(st.Content); i += 2 {
			if st.Content[i].Value != "include" {
				continue
			}
			v := st.Content[i+1]
			path := ""
			switch v.Kind {
			case yaml.ScalarNode:
				path = v.Value
			case yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					if v.Content[j].Value == "path" {
						path = v.Content[j+1].Value
					}
				}
			}
			if path == "" || strings.Contains(path, "{{") || strings.Contains(path, "${") || strings.Contains(path, "://") {
				continue
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(dir, filepath.FromSlash(path))
			}
			out = append(out, path)
		}
	}
	return out
}
