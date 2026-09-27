package migrate

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// 証跡の自動保存と diffEps() に合わせた runbook の書き換え (docs/design/evidence-report.md 10 章)。
//   - RUNNORA_EVIDENCE_DIR の下に書く dump ステップ → 削除 (runnora が応答を自動で保存する)
//   - exec で runnora-diff を呼んで比較するステップ → test: diffEps(...)
//
// 当てはまらない dump と exec: runnora-diff は変更せず TODO として報告する。

// evidenceEnv は旧来の dump ステップが証跡の保存先として参照する環境変数。
const evidenceEnv = "RUNNORA_EVIDENCE_DIR"

// lineEdit は runbook の行 [start, end) を lines に置き換える編集 (0 始まり)。
type lineEdit struct {
	start, end int
	lines      []string
}

// evidenceResult は 1 つの runbook の書き換えの結果。
type evidenceResult struct {
	removedDumps []string
	converted    []string
}

// migrateEvidence は dump ステップの削除と runnora-diff の diffEps() への書き換えを d に適用する。
// 書き換えた場合は d を読み直す。
func migrateEvidence(d *runbookDoc, plan *Plan) (evidenceResult, error) {
	var res evidenceResult
	steps := mapValue(d.root, "steps")
	if steps == nil {
		return res, nil
	}
	if steps.Kind == yaml.SequenceNode {
		for i, st := range steps.Content {
			if st.Kind == yaml.MappingNode && (mapValue(st, "dump") != nil || diffExec(st) != nil) {
				plan.todo(fmt.Sprintf("%s:%d", d.rel, st.Line), "steps[%d]: リスト形式の steps は番号がずれるため書き換えません。dump ステップの削除と runnora-diff の diffEps() への置き換えを手で行ってください", i)
			}
		}
		return res, nil
	}
	if steps.Kind != yaml.MappingNode || steps.Style&yaml.FlowStyle != 0 {
		return res, nil
	}

	var edits []lineEdit
	vars := newDiffVars(d)
	for i := 0; i+1 < len(steps.Content); i += 2 {
		keyNode, st := steps.Content[i], steps.Content[i+1]
		if st.Kind != yaml.MappingNode {
			continue
		}
		key := keyNode.Value
		where := fmt.Sprintf("%s:%d", d.rel, keyNode.Line)
		start, end := d.stepRange(steps, i)
		switch {
		case mapValue(st, "dump") != nil:
			if msg := removableDump(d, key, st); msg != "" {
				plan.todo(where, "steps.%s: %s", key, msg)
				continue
			}
			edits = append(edits, lineEdit{start: start, end: d.trailingBlank(start, end)})
			res.removedDumps = append(res.removedDumps, key)
		case diffExec(st) != nil:
			call, msg := parseDiffExec(st)
			if msg != "" {
				plan.todo(where, "steps.%s: runnora-diff の呼び出しを diffEps() に置き換えられません (%s)。手で書き換えてください", key, msg)
				continue
			}
			name, ok := vars.nameFor(call.expected)
			if !ok {
				plan.todo(where, "steps.%s: vars がフロー形式のため期待ファイルの変数を追加できません。手で diffEps() に書き換えてください", key)
				continue
			}
			edits = append(edits, d.diffEpsEdits(st, start, end, call.test(name))...)
			res.converted = append(res.converted, key)
		}
	}
	if len(edits) == 0 {
		return res, nil
	}
	edits = append(edits, vars.edits()...)
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	lines := d.lines
	for _, e := range edits {
		next := append([]string{}, lines[:e.start]...)
		next = append(next, e.lines...)
		lines = append(next, lines[e.end:]...)
	}
	nd, ok := parseRunbook(d.rel, strings.Join(lines, "\n"))
	if !ok {
		return res, fmt.Errorf("%s: 書き換え後の runbook を読めません", d.rel)
	}
	*d = *nd
	return res, nil
}

// removableDump は dump ステップを削除できない理由を返す。削除できる場合は ""。
func removableDump(d *runbookDoc, key string, st *yaml.Node) string {
	dump := mapValue(st, "dump")
	out := ""
	if dump.Kind == yaml.MappingNode {
		if o := mapValue(dump, "out"); o != nil {
			out = o.Value
		}
	}
	if !strings.Contains(out, evidenceEnv) {
		return fmt.Sprintf("dump の出力先が %s の下ではないため残しました。証跡のためなら削除してください (応答は runnora が自動で保存します)", evidenceEnv)
	}
	for j := 0; j+1 < len(st.Content); j += 2 {
		switch st.Content[j].Value {
		case "dump", "desc", "if":
		default:
			return fmt.Sprintf("dump と一緒に %s があるため残しました。dump だけ削除してください", st.Content[j].Value)
		}
	}
	if stepReferenced(d.text, key) {
		return "ほかのステップから参照されているため残しました"
	}
	return ""
}

func stepReferenced(text, key string) bool {
	q := regexp.QuoteMeta(key)
	return regexp.MustCompile(`steps\.` + q + `\b|steps\[["']` + q + `["']\]`).MatchString(text)
}

// stepRange は steps の i 番目 (キーの位置) のステップの行範囲 [start, end) を返す。
// end は次のステップのキー (なければ steps の次のトップレベルのキー、ファイルの終わり) の直前で、
// 次のステップの前に書かれたコメント (ステップのキーと同じ字下げ) と空行は含めない。
func (d *runbookDoc) stepRange(steps *yaml.Node, i int) (int, int) {
	keyNode := steps.Content[i]
	start := keyNode.Line - 1
	limit := len(d.lines)
	if i+2 < len(steps.Content) {
		limit = steps.Content[i+2].Line - 1
	} else if next := nextTopLevelKey(d.root, "steps"); next != nil {
		limit = next.Line - 1
	}
	indent := keyNode.Column - 1
	end := start + 1
	for j := start + 1; j < limit; j++ {
		t := strings.TrimSpace(d.lines[j])
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") && leadingSpaces(d.lines[j]) <= indent {
			continue
		}
		end = j + 1
	}
	return start, end
}

// trailingBlank は、削除する範囲 [start, end) の後ろの空行を、前にも空行があれば含めて返す (空行が重ならないように)。
func (d *runbookDoc) trailingBlank(start, end int) int {
	if start == 0 || strings.TrimSpace(d.lines[start-1]) != "" {
		return end
	}
	for end < len(d.lines) && strings.TrimSpace(d.lines[end]) == "" && end+1 < len(d.lines) {
		end++
	}
	return end
}

func nextTopLevelKey(root *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key && i+2 < len(root.Content) {
			return root.Content[i+2]
		}
	}
	return nil
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// diffCommandNames は runnora-diff の実行ファイル名 (拡張子なし、小文字)。
var diffCommandNames = map[string]bool{"runnora-diff": true, "jsondiff-eps": true}

// diffExec は、ステップが exec で runnora-diff を呼んでいれば exec のノードを返す。
func diffExec(st *yaml.Node) *yaml.Node {
	ex := mapValue(st, "exec")
	if ex == nil || ex.Kind != yaml.MappingNode {
		return nil
	}
	cmd := mapValue(ex, "command")
	if cmd == nil || cmd.Kind != yaml.ScalarNode {
		return nil
	}
	args := splitCommand(cmd.Value)
	if len(args) == 0 || !diffCommandNames[commandName(args[0])] {
		return nil
	}
	return ex
}

func commandName(p string) string {
	p = p[strings.LastIndexAny(p, `/\`)+1:]
	return strings.TrimSuffix(strings.ToLower(p), ".exe")
}

// splitCommand はコマンドラインを空白で区切る (引用符 '…' "…" で囲んだ部分は 1 つ)。
func splitCommand(s string) []string {
	var out []string
	var cur strings.Builder
	quote := rune(0)
	inArg := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if inArg {
		out = append(out, cur.String())
	}
	return out
}

// diffCall は diffEps() に置き換える runnora-diff の呼び出し。
type diffCall struct {
	expected string // 期待ファイル (プロジェクトルート基準)
	actual   string // 実際の値の式 (stdin の toJSON の中身)
	config   string // --config のパス (プロジェクトルート基準)。なければ ""
	equal    bool   // 差分がないことを期待するか
}

var stdinToJSON = regexp.MustCompile(`^\s*\{\{\s*toJSON\((.+)\)\s*\}\}\s*$`)

// parseDiffExec は exec: runnora-diff のステップを読む。置き換えられない場合は理由を返す。
func parseDiffExec(st *yaml.Node) (diffCall, string) {
	var call diffCall
	for j := 0; j+1 < len(st.Content); j += 2 {
		switch st.Content[j].Value {
		case "exec", "test", "desc", "if":
		default:
			return call, st.Content[j].Value + " があります"
		}
	}
	ex := mapValue(st, "exec")
	for j := 0; j+1 < len(ex.Content); j += 2 {
		switch ex.Content[j].Value {
		case "command", "stdin", "shell":
		default:
			return call, "exec." + ex.Content[j].Value + " があります"
		}
	}

	args := splitCommand(mapValue(ex, "command").Value)[1:]
	var positional []string
	for k := 0; k < len(args); k++ {
		a := args[k]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		switch {
		case !strings.HasPrefix(a, "-") || a == "-":
			positional = append(positional, a)
			continue
		case name != "format" && name != "config":
			return call, "オプション " + a + " は変換できません"
		case !hasValue:
			if k+1 >= len(args) {
				return call, a + " の値がありません"
			}
			k++
			value = args[k]
		}
		if name == "config" {
			call.config = value
		}
	}
	if len(positional) != 2 || positional[1] != "-" || positional[0] == "-" {
		return call, "引数が「期待ファイル -」の形ではありません"
	}
	call.expected = positional[0]
	if strings.ContainsAny(call.expected+call.config, "{}$") {
		return call, "ファイルのパスに変数や式を使っています"
	}

	stdin := mapValue(ex, "stdin")
	m := []string(nil)
	if stdin != nil {
		m = stdinToJSON.FindStringSubmatch(stdin.Value)
	}
	if m == nil {
		return call, "stdin が {{ toJSON(<式>) }} の形ではありません"
	}
	call.actual = strings.TrimSpace(m[1])

	test := mapValue(st, "test")
	if test == nil || test.Kind != yaml.ScalarNode {
		return call, "test がありません"
	}
	equal, ok := diffExpectation(test.Value)
	if !ok {
		return call, "test が終了コードか summary.differences だけを見る形ではありません"
	}
	call.equal = equal
	return call, ""
}

var (
	exitCodeCond    = regexp.MustCompile(`^current\.exit_code\s*(==|!=)\s*(\d+)$`)
	emptyOutputCond = regexp.MustCompile(`^current\.(stdout|stderr)\s*==\s*(""|'')$`)
	differencesCond = regexp.MustCompile(`^fromJSON\(current\.stdout\)\.summary\.differences\s*(==|>|!=)\s*0$`)
	equalFieldCond  = regexp.MustCompile(`^(!?)fromJSON\(current\.stdout\)\.equal$`)
)

// diffExpectation は runnora-diff の結果を見る test から、差分がないことを期待しているかを返す。
func diffExpectation(test string) (equal, ok bool) {
	test = strings.Join(strings.Fields(test), " ")
	if strings.Contains(test, "||") {
		return false, false
	}
	decided := false
	set := func(v bool) bool {
		if decided && equal != v {
			return false
		}
		equal, decided = v, true
		return true
	}
	for _, c := range strings.Split(test, "&&") {
		c = strings.TrimSpace(c)
		switch {
		case emptyOutputCond.MatchString(c):
		case exitCodeCond.MatchString(c):
			m := exitCodeCond.FindStringSubmatch(c)
			switch {
			case m[1] == "==" && m[2] == "0":
				ok = set(true)
			case m[1] == "==" && m[2] == "1", m[1] == "!=" && m[2] == "0":
				ok = set(false)
			default:
				return false, false
			}
			if !ok {
				return false, false
			}
		case differencesCond.MatchString(c):
			if !set(differencesCond.FindStringSubmatch(c)[1] == "==") {
				return false, false
			}
		case equalFieldCond.MatchString(c):
			if !set(equalFieldCond.FindStringSubmatch(c)[1] == "") {
				return false, false
			}
		default:
			return false, false
		}
	}
	return equal, decided
}

// test は置き換え後の test の式を返す。
func (c diffCall) test(expectedVar string) string {
	expr := "diffEps(vars." + expectedVar + ", " + c.actual
	if c.config != "" {
		expr += `, "` + filepath.ToSlash(c.config) + `"`
	}
	expr += ")"
	if !c.equal {
		expr = "!" + expr
	}
	return expr
}

// diffEpsEdits は exec: runnora-diff のステップの exec と test を test: diffEps(...) 1 行にする編集を返す。
// desc と if はそのまま残す。
func (d *runbookDoc) diffEpsEdits(st *yaml.Node, start, end int, expr string) []lineEdit {
	type part struct {
		key        string
		start, end int
	}
	var parts []part
	for j := 0; j+1 < len(st.Content); j += 2 {
		parts = append(parts, part{key: st.Content[j].Value, start: st.Content[j].Line - 1})
	}
	for k := range parts {
		parts[k].end = end
		if k+1 < len(parts) {
			parts[k].end = parts[k+1].start
		}
	}
	indent := strings.Repeat(" ", st.Content[0].Column-1)
	crlf := strings.HasSuffix(d.lines[start], "\r")
	line := indent + "test: '" + strings.ReplaceAll(expr, "'", "''") + "'"
	if crlf {
		line += "\r"
	}
	var edits []lineEdit
	inserted := false
	for _, p := range parts {
		if p.key != "exec" && p.key != "test" {
			continue
		}
		e := lineEdit{start: p.start, end: p.end}
		if !inserted {
			e.lines = []string{line}
			inserted = true
		}
		// 次の部分との間の空行・コメントは残す
		for e.end > e.start+1 && strings.TrimSpace(d.lines[e.end-1]) == "" {
			e.end--
		}
		edits = append(edits, e)
	}
	return edits
}

// diffVars は期待ファイルを json:// で読む vars の変数を決める。
type diffVars struct {
	d        *runbookDoc
	vars     *yaml.Node
	existing map[string]string // 変数名 → 値
	added    []string          // 追加する変数名 (追加順)
	values   map[string]string // 追加する変数名 → 値
}

func newDiffVars(d *runbookDoc) *diffVars {
	v := &diffVars{d: d, vars: mapValue(d.root, "vars"), existing: map[string]string{}, values: map[string]string{}}
	if v.vars != nil && v.vars.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(v.vars.Content); i += 2 {
			v.existing[v.vars.Content[i].Value] = v.vars.Content[i+1].Value
		}
	}
	return v
}

// nameFor は期待ファイル (プロジェクトルート基準) を読む変数名を返す。同じファイルの変数があれば使い回す。
func (v *diffVars) nameFor(expected string) (string, bool) {
	value := "json://" + relFromRunbook(v.d.rel, expected)
	for _, name := range v.added {
		if v.values[name] == value {
			return name, true
		}
	}
	names := make([]string, 0, len(v.existing))
	for name := range v.existing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v.existing[name] == value {
			return name, true
		}
	}
	if v.vars != nil && (v.vars.Kind != yaml.MappingNode || v.vars.Style&yaml.FlowStyle != 0) {
		return "", false
	}
	name := "expected"
	for i := 2; v.taken(name); i++ {
		name = fmt.Sprintf("expected%d", i)
	}
	v.added = append(v.added, name)
	v.values[name] = value
	return name, true
}

func (v *diffVars) taken(name string) bool {
	_, ok := v.existing[name]
	_, added := v.values[name]
	return ok || added
}

// edits は追加する変数の行を vars の最後 (vars がなければ steps の前) に入れる編集を返す。
func (v *diffVars) edits() []lineEdit {
	if len(v.added) == 0 {
		return nil
	}
	d := v.d
	crlf := ""
	if strings.Contains(d.text, "\r\n") {
		crlf = "\r"
	}
	var lines []string
	indent := "  "
	at := 0
	if v.vars == nil {
		stepsKey := mapKey(d.root, "steps")
		at = stepsKey.Line - 1
		lines = append(lines, "vars:"+crlf)
	} else {
		if len(v.vars.Content) > 0 {
			indent = strings.Repeat(" ", v.vars.Content[0].Column-1)
		}
		varsKey := mapKey(d.root, "vars")
		limit := len(d.lines)
		if next := nextTopLevelKey(d.root, "vars"); next != nil {
			limit = next.Line - 1
		}
		at = varsKey.Line
		for j := varsKey.Line; j < limit; j++ {
			t := strings.TrimSpace(d.lines[j])
			if t != "" && (!strings.HasPrefix(t, "#") || leadingSpaces(d.lines[j]) > 0) {
				at = j + 1
			}
		}
	}
	for _, name := range v.added {
		lines = append(lines, indent+name+": "+v.values[name]+crlf)
	}
	if v.vars == nil && at > 0 && strings.TrimSpace(d.lines[at-1]) == "" {
		lines = append(lines, crlf) // steps の前の空行をそろえる
	}
	return []lineEdit{{start: at, end: at, lines: lines}}
}

// relFromRunbook はプロジェクトルート基準のパスを runbook のディレクトリからの相対パスにする。
func relFromRunbook(rel, p string) string {
	p = path.Clean(filepath.ToSlash(p))
	if path.IsAbs(p) || filepath.IsAbs(p) {
		return p
	}
	dir := path.Dir(rel)
	if dir == "." {
		return p
	}
	up := strings.Repeat("../", strings.Count(dir, "/")+1)
	return up + p
}
