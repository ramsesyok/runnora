// Package migrate は旧形式 (runnora v0.3.0 まで) のテストプロジェクトを新形式 (version 2) に移行する。
//
// 移行する内容:
//   - config.yaml / config.<名前>.yaml → runnora.yaml の environments
//   - runbook の runners に直書きした接続先 (HTTP の endpoint、DB の DSN、gRPC の addr) → ${VAR}
//   - 実行の入口になる runbook (include されない runbook) → runnora: ブロックを追加
//     (前後処理の SQL と期待する結果は、シナリオ対応表 (--scenarios) から入れる)
//
// 自動で移行できないもの (スクリプトの --before-sql などの指定、環境ごとに違う値) は TODO として報告する。
// 再生成する runbooks/generated/ は変更しない。
package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/internal/project"
)

// Options は移行の入力。
type Options struct {
	// Dir は移行するプロジェクトのディレクトリ (runnora.yaml を作る場所)。
	Dir string
	// Envs は旧形式の設定ファイルと環境名の対応。空なら DiscoverEnvs で探す。
	Envs []EnvSpec
	// ScenariosFile はシナリオ対応表 (Dir 基準、空なら使わない)。
	ScenariosFile string
}

// Change はファイル 1 つへの変更。
type Change struct {
	Path    string // Dir からの相対パス (/ 区切り)
	Action  string // create / modify / delete
	Content string // create / modify の新しい内容
	Notes   []string
}

// Todo は自動で移行できず、人が確認・対応するもの。
type Todo struct {
	Where   string
	Message string
}

// Plan は移行の計画 (まだファイルは変更していない)。
type Plan struct {
	Dir     string
	Changes []Change
	Todos   []Todo
}

// ScenarioEntry はシナリオ対応表の 1 行。スクリプトで runbook ごとに指定していた内容を書く。
type ScenarioEntry struct {
	// Runbook は Dir 基準のパス (glob 可)。
	Runbook string   `yaml:"runbook"`
	ID      string   `yaml:"id"`
	Before  []string `yaml:"before"`
	After   []string `yaml:"after"`
	Expect  string   `yaml:"expect"`
	// ExpectExit は旧形式の期待する終了コード (0 → pass、1 → fail、4 → hookFail)。
	ExpectExit *int `yaml:"expectExit"`
}

// scenarioFile はシナリオ対応表のファイル。スクリプトに書かれていた内容を人が書き写す。
//   - scenarios:    runbook ごとの前後処理の SQL と期待する結果
//   - environments: スクリプトが環境変数で渡していた値 (環境ごとの vars に加える)
//   - suites:       スクリプトで runbook をまとめて実行していた単位 (runnora.yaml の suites と同じ形。そのまま写す)
type scenarioFile struct {
	Scenarios    []ScenarioEntry       `yaml:"scenarios"`
	Environments map[string]envOverlay `yaml:"environments"`
	Suites       yaml.Node             `yaml:"suites"`
}

type envOverlay struct {
	Vars map[string]string `yaml:"vars"`
}

// generatedDir は runnora generate の出力で、移行せずに再生成するディレクトリ名。
const generatedDir = "generated"

// Build は移行の計画を作る。ファイルは変更しない。
func Build(opts Options) (*Plan, error) {
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Dir: dir}

	envs, err := loadEnvironments(dir, opts.Envs, plan)
	if err != nil {
		return nil, err
	}
	sf, err := loadScenarios(dir, opts.ScenariosFile)
	if err != nil {
		return nil, err
	}
	entries := sf.Scenarios

	runbooks, generated, err := findRunbooks(dir)
	if err != nil {
		return nil, err
	}
	if len(generated) > 0 {
		plan.todo("runbooks/"+generatedDir, "runnora generate の出力 (%d ファイル) は移行しません。新しい runnora で再生成してください", len(generated))
	}

	included := includedRunbooks(dir, runbooks)
	names := newVarNames(envs)
	usedIDs := map[string]string{}
	matched := map[int]bool{}

	for _, d := range runbooks {
		var notes []string
		changed := false

		// 1. runners の直書きの接続先を変数にする。変数名は書かれた順に決め、
		//    置き換えは位置がずれないように後ろの値から行う。
		values := d.hardcodedRunners()
		varName := map[*yaml.Node]string{}
		for _, v := range values {
			varName[v.node] = names.nameFor(v.kind, v.runner, v.node.Value)
		}
		sort.Slice(values, func(i, j int) bool {
			if values[i].node.Line != values[j].node.Line {
				return values[i].node.Line > values[j].node.Line
			}
			return values[i].node.Column > values[j].node.Column
		})
		var replaced []string
		for _, v := range values {
			name := varName[v.node]
			ref := "${" + name + "}"
			if v.flow {
				ref = q(ref) // フロー形式の中では { } を含む値を引用符で囲む
			}
			if err := d.replaceScalar(v.node, ref); err != nil {
				plan.todo(d.rel, "runners.%s の値を変数にできませんでした: %v", v.runner, err)
				continue
			}
			replaced = append(replaced, name)
			changed = true
		}
		if len(replaced) > 0 {
			sort.Strings(replaced)
			notes = append(notes, "runners の直書きの値を変数に置き換え: "+strings.Join(uniq(replaced), ", "))
		}

		// 2. 実行の入口になる runbook に runnora: ブロックを追加する
		if mapKey(d.root, "runnora") == nil && !included[d.rel] {
			entry, idx := matchScenario(entries, d.rel)
			if idx >= 0 {
				matched[idx] = true
			}
			id := entry.ID
			if id == "" {
				id = d.deriveID()
			}
			if prev, ok := usedIDs[id]; ok {
				newID := uniqueID(id, usedIDs)
				plan.todo(d.rel, "シナリオ ID %q が %s と重複するため %q にしました。適切な ID に直してください", id, prev, newID)
				id = newID
			}
			usedIDs[id] = d.rel
			expect, err := entry.expect()
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", opts.ScenariosFile, entry.Runbook, err)
			}
			todo := ""
			if idx < 0 {
				todo = "前後処理 (before / after) と期待する結果 (expect) を確認する"
				plan.todo(d.rel, "シナリオ対応表にないため、前後処理と期待する結果を入れていません。スクリプトで指定していた場合は runnora: ブロックに書いてください")
			}
			d.insertBlock(renderBlock(id, entry.Before, entry.After, expect, todo))
			notes = append(notes, "runnora: ブロックを追加 (id: "+id+")")
			changed = true
		} else if mapKey(d.root, "runnora") != nil {
			usedIDs[blockID(d)] = d.rel
		}

		if changed {
			plan.Changes = append(plan.Changes, Change{Path: d.rel, Action: "modify", Content: d.content(), Notes: notes})
		}
	}
	for i, e := range entries {
		if !matched[i] {
			plan.todo(opts.ScenariosFile, "対応表の runbook %q に当てはまる runbook がありません (include される部品か、すでに runnora: ブロックがあります)", e.Runbook)
		}
	}

	// 3. runnora.yaml を作る (対応表の環境の変数と、runbook から取り出した変数を環境に加える)
	for name, ov := range sf.Environments {
		e := findEnv(envs, name)
		if e == nil {
			return nil, fmt.Errorf("%s: environments.%s: 環境がありません (%s)", opts.ScenariosFile, name, envNames(envs))
		}
		for k, v := range ov.Vars {
			e.vars[k] = v
			e.varsOf[k] = "シナリオ対応表から (スクリプトが渡していた値)"
		}
	}
	if len(envs) > 0 {
		for _, v := range names.extracted {
			var filled []string
			for _, e := range envs {
				if _, ok := e.vars[v.name]; ok {
					continue
				}
				e.vars[v.name] = v.value
				e.varsOf[v.name] = "runbook から取り出した値。環境に合わせて確認する"
				filled = append(filled, e.spec.Name)
			}
			if len(filled) > 0 {
				plan.todo("runnora.yaml", "%s は runbook に直書きされていた値 (%s) を環境 %s に入れました。環境ごとに正しい値か確認してください", v.name, v.value, strings.Join(filled, ", "))
			}
		}
		suites, err := renderSuites(&sf.Suites)
		if err != nil {
			return nil, fmt.Errorf("%s: suites: %w", opts.ScenariosFile, err)
		}
		plan.Changes = append([]Change{{Path: project.FileName, Action: "create", Content: renderProject(envs, suites),
			Notes: []string{"環境: " + envNames(envs)}}}, plan.Changes...)
		for _, e := range envs {
			plan.Changes = append(plan.Changes, Change{Path: filepath.ToSlash(e.spec.File), Action: "delete",
				Notes: []string{"runnora.yaml の環境 " + e.spec.Name + " に移行"}})
		}
	} else if len(names.extracted) > 0 {
		for _, v := range names.extracted {
			plan.todo("runnora.yaml", "%s を environments.<名前>.vars に定義してください (runbook に直書きされていた値: %s)", v.name, v.value)
		}
	}

	// 4. 未定義の変数 (スクリプトが環境変数で渡していた値など) を報告する
	defined := map[string]bool{}
	for _, e := range envs {
		for k := range e.vars {
			defined[k] = true
		}
	}
	for _, name := range undefinedVars(runbooks, defined) {
		plan.todo("runnora.yaml", "runbook が参照する %s がどの環境にも定義されていません。スクリプトで環境変数として渡していた値を environments.<名前>.vars に書いてください", name)
	}

	// 5. スクリプトと runbook のコメントにある旧形式の指定を報告する
	if err := scanScripts(dir, plan); err != nil {
		return nil, err
	}
	for _, d := range runbooks {
		for i, line := range d.lines { // 移行後の行番号で報告する
			if strings.HasPrefix(strings.TrimSpace(line), "#") && legacyScriptPattern.MatchString(line) {
				plan.todo(fmt.Sprintf("%s:%d", d.rel, i+1), "コメントに旧形式の実行方法が書かれています。新形式 (runnora run --suite / runnora: ブロック) に合わせて直してください")
			}
		}
	}
	return plan, nil
}

func (p *Plan) todo(where, format string, args ...any) {
	p.Todos = append(p.Todos, Todo{Where: where, Message: fmt.Sprintf(format, args...)})
}

func loadEnvironments(dir string, specs []EnvSpec, plan *Plan) ([]*environment, error) {
	if _, err := os.Stat(filepath.Join(dir, project.FileName)); err == nil {
		if _, err := project.Load(filepath.Join(dir, project.FileName)); err == nil {
			if len(specs) > 0 {
				return nil, fmt.Errorf("%s はすでに新形式の runnora.yaml があります", dir)
			}
			return nil, nil // 移行済み。runbook だけ確認する
		}
		return nil, fmt.Errorf("%s に新形式ではない runnora.yaml があります。移動してから実行してください", dir)
	}
	if len(specs) == 0 {
		var err error
		specs, err = DiscoverEnvs(dir)
		if err != nil {
			return nil, err
		}
	}
	var envs []*environment
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.Name] {
			return nil, fmt.Errorf("環境名 %q が重複しています", s.Name)
		}
		seen[s.Name] = true
		cfg, err := loadLegacy(filepath.Join(dir, filepath.FromSlash(s.File)))
		if err != nil {
			return nil, err
		}
		e := &environment{spec: s, cfg: cfg, vars: map[string]string{}, varsOf: map[string]string{}}
		if cfg.Oracle.DSN != "" {
			e.vars["ORACLE_DSN"] = cfg.Oracle.DSN
			e.varsOf["ORACLE_DSN"] = "パスワードは ${ORACLE_PASSWORD} のように OS の環境変数から渡すことを推奨"
		}
		if cfg.Runn.DBRunnerName != "" && cfg.Runn.DBRunnerName != "db" {
			plan.todo(s.File, "runn.db_runner_name (%s) は新形式にありません (もともと使われていない設定です)", cfg.Runn.DBRunnerName)
		}
		envs = append(envs, e)
	}
	return envs, nil
}

func loadScenarios(dir, file string) (*scenarioFile, error) {
	var sf scenarioFile
	if file == "" {
		return &sf, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&sf); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for _, e := range sf.Scenarios {
		if _, err := path.Match(e.Runbook, ""); err != nil {
			return nil, fmt.Errorf("%s: runbook %q: %w", file, e.Runbook, err)
		}
	}
	return &sf, nil
}

// renderSuites は対応表の suites を runnora.yaml に書く形 (2 字下げ) にする。なければ "" を返す。
func renderSuites(n *yaml.Node) (string, error) {
	if n.Kind == 0 {
		return "", nil
	}
	if n.Kind != yaml.MappingNode {
		return "", fmt.Errorf("スイート名をキーにしたマッピングで書いてください")
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("suites:\n")
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	return b.String(), nil
}

func matchScenario(entries []ScenarioEntry, rel string) (ScenarioEntry, int) {
	for i, e := range entries {
		if ok, _ := path.Match(strings.TrimPrefix(e.Runbook, "./"), rel); ok {
			return e, i
		}
	}
	return ScenarioEntry{}, -1
}

func (e ScenarioEntry) expect() (string, error) {
	if e.Expect != "" {
		switch e.Expect {
		case "pass", "fail", "hookFail":
			return e.Expect, nil
		}
		return "", fmt.Errorf("expect %q は使えません (pass / fail / hookFail)", e.Expect)
	}
	if e.ExpectExit == nil {
		return "", nil
	}
	switch *e.ExpectExit {
	case 0:
		return "pass", nil
	case 1:
		return "fail", nil
	case 4:
		return "hookFail", nil
	}
	return "", fmt.Errorf("expectExit %d は変換できません (0 / 1 / 4)", *e.ExpectExit)
}

// findRunbooks は dir 以下の runbook (steps を持つ YAML) を読む。
// generated/ の下は移行しないので、別に数だけ返す。
func findRunbooks(dir string) ([]*runbookDoc, []string, error) {
	var docs []*runbookDoc
	var generated []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(p)
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		doc, ok := parseRunbook(rel, string(data))
		if !ok {
			return nil
		}
		if strings.Contains("/"+rel, "/"+generatedDir+"/") {
			generated = append(generated, rel)
			return nil
		}
		docs = append(docs, doc)
		return nil
	})
	sort.Slice(docs, func(i, j int) bool { return docs[i].rel < docs[j].rel })
	return docs, generated, err
}

// includedRunbooks は他の runbook から include されている runbook (部品) の集合を返す。
func includedRunbooks(dir string, docs []*runbookDoc) map[string]bool {
	out := map[string]bool{}
	for _, d := range docs {
		for _, inc := range includes(d) {
			p := path.Clean(path.Join(path.Dir(d.rel), inc))
			out[p] = true
		}
	}
	return out
}

func includes(d *runbookDoc) []string {
	steps := mapValue(d.root, "steps")
	if steps == nil {
		return nil
	}
	var stepNodes []*yaml.Node
	switch steps.Kind {
	case yaml.MappingNode:
		for i := 1; i < len(steps.Content); i += 2 {
			stepNodes = append(stepNodes, steps.Content[i])
		}
	case yaml.SequenceNode:
		stepNodes = steps.Content
	}
	var out []string
	for _, st := range stepNodes {
		if st.Kind != yaml.MappingNode {
			continue
		}
		inc := mapValue(st, "include")
		if inc == nil {
			continue
		}
		p := inc.Value
		if inc.Kind == yaml.MappingNode {
			if pn := mapValue(inc, "path"); pn != nil {
				p = pn.Value
			}
		}
		if p != "" && !isDynamic(p) {
			out = append(out, filepath.ToSlash(p))
		}
	}
	return out
}

func blockID(d *runbookDoc) string {
	if b := mapValue(d.root, "runnora"); b != nil {
		if id := mapValue(b, "id"); id != nil {
			return id.Value
		}
	}
	return d.rel
}

func uniqueID(id string, used map[string]string) string {
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", id, i)
		if _, ok := used[c]; !ok {
			return c
		}
	}
}

func undefinedVars(docs []*runbookDoc, defined map[string]bool) []string {
	seen := map[string]bool{}
	for _, d := range docs {
		for _, ref := range project.Refs(d.content()) {
			if !ref.HasDefault && !defined[ref.Name] {
				seen[ref.Name] = true
			}
		}
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// legacyScriptPattern はスクリプト中の旧形式の指定に一致する。
var legacyScriptPattern = regexp.MustCompile(`--config\b|--before-sql\b|--after-sql\b|--scopes\b`)

// scanScripts はスクリプトの旧形式の指定 (--config / --before-sql / --after-sql / --scopes) を報告する。
func scanScripts(dir string, plan *Plan) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".ps1", ".psm1", ".sh", ".bash", ".bat", ".cmd":
		default:
			if d.Name() != "Makefile" {
				return nil
			}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			where := fmt.Sprintf("%s:%d", rel, i+1)
			for _, m := range uniqSorted(legacyScriptPattern.FindAllString(line, -1)) {
				reportScriptFlag(plan, where, m)
			}
		}
		return nil
	})
}

// reportScriptFlag はスクリプトの旧形式の指定 1 つを TODO にする。
func reportScriptFlag(plan *Plan, where, m string) {
	switch m {
	case "--config":
		plan.todo(where, "--config は廃止されました。runnora.yaml を使うので指定を外してください (環境は --env で選ぶ)")
	case "--scopes":
		plan.todo(where, "--scopes で許可していたスコープは runnora.yaml の runn.scopes に書けます")
	default:
		plan.todo(where, "%s は廃止されました。指定していた SQL を runbook の runnora: ブロック (before / after) に移し、指定を外してください", m)
	}
}

// uniqSorted は出現順を保ったまま重複を除く。
func uniqSorted(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func findEnv(envs []*environment, name string) *environment {
	for _, e := range envs {
		if e.spec.Name == name {
			return e
		}
	}
	return nil
}

func envNames(envs []*environment) string {
	names := make([]string, 0, len(envs))
	for _, e := range envs {
		names = append(names, e.spec.Name+" ← "+e.spec.File)
	}
	return strings.Join(names, ", ")
}

func uniq(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// ErrDirty は Git の作業ツリーに未コミットの変更があるため書き込まないことを表す。
var ErrDirty = errors.New("未コミットの変更があります")
