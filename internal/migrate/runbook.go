package migrate

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// runbookDoc は移行対象の runbook 1 つ分。
type runbookDoc struct {
	rel   string // Dir からの相対パス (/ 区切り)
	text  string
	lines []string
	root  *yaml.Node // トップレベルのマッピング
}

func parseRunbook(rel, text string) (*runbookDoc, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil || len(doc.Content) == 0 {
		return nil, false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || mapValue(root, "steps") == nil {
		return nil, false // steps を持たない YAML は runbook ではない
	}
	return &runbookDoc{rel: rel, text: text, lines: strings.Split(text, "\n"), root: root}, true
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

// runnerValue は runners に直書きされた接続先 1 つ。
type runnerValue struct {
	runner string
	kind   string // http / db / grpc
	node   *yaml.Node
	// flow はフロー形式 ({ ... }) の中の値であることを表す。${...} は引用符で囲む必要がある。
	flow bool
}

// hardcodedRunners は runners の値のうち、変数や式を使わずに直書きされた接続先を返す。
//
//	req: http://...            → http
//	req: { endpoint: http://... } → http
//	db:  oracle://...          → db
//	db:  { dsn: oracle://... } → db
//	greq: { addr: host:port }  → grpc
func (d *runbookDoc) hardcodedRunners() []runnerValue {
	runners := mapValue(d.root, "runners")
	if runners == nil || runners.Kind != yaml.MappingNode {
		return nil
	}
	var out []runnerValue
	for i := 0; i+1 < len(runners.Content); i += 2 {
		name, v := runners.Content[i].Value, runners.Content[i+1]
		flow := runners.Style&yaml.FlowStyle != 0
		switch v.Kind {
		case yaml.ScalarNode:
			if kind := scalarKind(v.Value); kind != "" {
				out = append(out, runnerValue{runner: name, kind: kind, node: v, flow: flow})
			}
		case yaml.MappingNode:
			for _, k := range []struct{ key, kind string }{{"endpoint", "http"}, {"dsn", "db"}, {"addr", "grpc"}} {
				n := mapValue(v, k.key)
				if n == nil || n.Kind != yaml.ScalarNode || isDynamic(n.Value) {
					continue
				}
				kind := k.kind
				if kind != "grpc" {
					kind = scalarKind(n.Value)
				}
				if kind != "" {
					out = append(out, runnerValue{runner: name, kind: kind, node: n, flow: flow || v.Style&yaml.FlowStyle != 0})
				}
			}
		}
	}
	return out
}

func isDynamic(v string) bool {
	return strings.Contains(v, "${") || strings.Contains(v, "{{")
}

// scalarKind は runner の値から種類を判定する。対象外なら "" を返す。
func scalarKind(v string) string {
	if isDynamic(v) {
		return ""
	}
	switch {
	case strings.HasPrefix(v, "http://"), strings.HasPrefix(v, "https://"):
		return "http"
	case strings.Contains(v, "://"):
		return "db"
	}
	return ""
}

// replaceScalar は node の値 (行と桁の位置) を newText に置き換える。
// 引用符付きのスカラーは引用符ごと、引用符なしは行末・コメント・フロー区切りまでを置き換える。
func (d *runbookDoc) replaceScalar(node *yaml.Node, newText string) error {
	li := node.Line - 1
	if li < 0 || li >= len(d.lines) {
		return fmt.Errorf("%s: 行 %d が範囲外です", d.rel, node.Line)
	}
	line := d.lines[li]
	start := node.Column - 1
	if start < 0 || start > len(line) {
		return fmt.Errorf("%s:%d: 桁 %d が範囲外です", d.rel, node.Line, node.Column)
	}
	end := len(line)
	switch node.Style {
	case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle:
		quote := line[start]
		i := start + 1
		for i < len(line) {
			if line[i] == quote {
				if quote == '\'' && i+1 < len(line) && line[i+1] == '\'' {
					i += 2
					continue
				}
				break
			}
			if quote == '"' && line[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(line) {
			return fmt.Errorf("%s:%d: 複数行の値は置き換えられません", d.rel, node.Line)
		}
		end = i + 1
	case 0:
		rest := line[start:]
		for _, stop := range []string{" #", ",", "}", "]"} {
			if j := strings.Index(rest, stop); j >= 0 && start+j < end {
				end = start + j
			}
		}
		end = start + len(strings.TrimRight(line[start:end], " \t\r"))
	default:
		return fmt.Errorf("%s:%d: ブロック形式の値は置き換えられません", d.rel, node.Line)
	}
	d.lines[li] = line[:start] + newText + line[end:]
	return nil
}

// descIDPattern は desc の先頭にあるシナリオ ID (例: "LIB-004 ...") に一致する。
var descIDPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*-[0-9]+)\b`)

var invalidIDChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// deriveID は runnora: ブロックの id の候補を返す。
// desc が "LIB-004 ..." のように ID で始まればそれを、なければファイル名 (拡張子なし) を使う。
func (d *runbookDoc) deriveID() string {
	if desc := mapValue(d.root, "desc"); desc != nil {
		if m := descIDPattern.FindStringSubmatch(strings.TrimSpace(desc.Value)); m != nil {
			return m[1]
		}
	}
	base := path.Base(d.rel)
	base = strings.TrimSuffix(base, path.Ext(base))
	return strings.Trim(invalidIDChars.ReplaceAllString(base, "-"), "-")
}

// insertBlock は runnora: ブロックを desc の直後 (desc がなければ先頭のキーの前) に挿入する。
func (d *runbookDoc) insertBlock(block []string) {
	at := -1 // 挿入する行 (0 始まり)
	keys := d.root.Content
	for i := 0; i+1 < len(keys); i += 2 {
		if keys[i].Value != "desc" {
			continue
		}
		v := keys[i+1]
		if v.Kind == yaml.ScalarNode && v.Line == keys[i].Line && (v.Style == 0 || v.Style == yaml.DoubleQuotedStyle || v.Style == yaml.SingleQuotedStyle) {
			at = keys[i].Line // desc の次の行
		} else if i+2 < len(keys) {
			at = keys[i+2].Line - 1 // 複数行の desc は次のキーの前
		}
		break
	}
	if at < 0 && len(keys) > 0 {
		at = keys[0].Line - 1
	}
	if at < 0 {
		at = 0
	}
	lines := make([]string, 0, len(d.lines)+len(block))
	lines = append(lines, d.lines[:at]...)
	for _, l := range block {
		if strings.Contains(d.text, "\r\n") {
			l += "\r" // CRLF のファイルでは追加する行も CRLF にする
		}
		lines = append(lines, l)
	}
	lines = append(lines, d.lines[at:]...)
	d.lines = lines
}

func (d *runbookDoc) content() string {
	return strings.Join(d.lines, "\n")
}

// renderBlock は runnora: ブロックの行を作る。
func renderBlock(id string, before, after []string, expect, todo string) []string {
	lines := []string{"runnora:"}
	if todo != "" {
		lines = append(lines, "  # TODO(runnora-migrate): "+todo)
	}
	lines = append(lines, "  id: "+id)
	for _, part := range []struct {
		key   string
		files []string
	}{{"before", before}, {"after", after}} {
		if len(part.files) == 0 {
			continue
		}
		lines = append(lines, "  "+part.key+":")
		for _, f := range part.files {
			lines = append(lines, "    - "+normalizeHookPath(f))
		}
	}
	if expect != "" && expect != "pass" {
		lines = append(lines, "  expect: "+expect)
	}
	return lines
}
