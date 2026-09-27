package migrate

import (
	"strings"
	"testing"
)

func TestMigrateEvidence(t *testing.T) {
	const diffStep = `  compare:
    exec:
      command: ./bin/runnora-diff.exe --config cases/tol.yaml cases/expected.json -
      shell: pwsh -NoProfile -Command {0}
      stdin: '{{ toJSON(steps.call.res.message) }} '
    test: current.exit_code == 0
`
	tests := []struct {
		name     string
		in       string
		want     string // 空なら変更なし
		wantTodo string
	}{
		{
			name: "remove evidence dump keeping one blank line",
			in: `steps:
  call:
    req: {}

  dump_call:
    dump:
      expr: steps.call.res
      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/call.json'

  check:
    test: true
`,
			want: `steps:
  call:
    req: {}

  check:
    test: true
`,
		},
		{
			name: "last step and comment of the next key",
			in: `steps:
  call:
    req: {}
  dump_call:
    desc: 証跡
    dump:
      expr: steps.call.res
      # 保存先
      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/call.json'
# 末尾のコメント
after: 1
`,
			want: `steps:
  call:
    req: {}
# 末尾のコメント
after: 1
`,
		},
		{
			name:     "dump to another place is kept",
			in:       "steps:\n  d:\n    dump:\n      expr: steps\n      out: out/x.json\n",
			wantTodo: "dump の出力先が RUNNORA_EVIDENCE_DIR の下ではない",
		},
		{
			name:     "dump to stdout is kept",
			in:       "steps:\n  d:\n    dump: steps\n",
			wantTodo: "dump の出力先が RUNNORA_EVIDENCE_DIR の下ではない",
		},
		{
			name:     "dump with test is kept",
			in:       "steps:\n  d:\n    dump:\n      expr: steps\n      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/x.json'\n    test: true\n",
			wantTodo: "dump と一緒に test がある",
		},
		{
			name:     "referenced dump is kept",
			in:       "steps:\n  d:\n    dump:\n      expr: steps\n      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/x.json'\n  c:\n    test: steps.d != null\n",
			wantTodo: "参照されている",
		},
		{
			name:     "list steps are reported",
			in:       "steps:\n  - req: {}\n  - dump:\n      expr: steps[0]\n      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/x.json'\n",
			wantTodo: "リスト形式の steps",
		},
		{
			name: "diffEps with a new vars block",
			in:   "desc: x\n\nsteps:\n" + diffStep,
			want: `desc: x

vars:
  expected: json://../cases/expected.json

steps:
  compare:
    test: 'diffEps(vars.expected, steps.call.res.message, "cases/tol.yaml")'
`,
		},
		{
			name: "diffEps reuses the var and keeps desc",
			in: `vars:
  exp: json://../cases/expected.json
  # コメント
steps:
  compare:
    desc: 差分があること
    exec:
      command: runnora-diff --format=json cases/expected.json -
      stdin: '{{ toJSON(steps.call.res) }}'
    test: |
      current.exit_code == 1 &&
      fromJSON(current.stdout).summary.differences > 0
`,
			want: `vars:
  exp: json://../cases/expected.json
  # コメント
steps:
  compare:
    desc: 差分があること
    test: '!diffEps(vars.exp, steps.call.res)'
`,
		},
		{
			name: "expected is taken",
			in:   "vars:\n  expected: 1\nsteps:\n" + diffStep,
			want: `vars:
  expected: 1
  expected2: json://../cases/expected.json
steps:
  compare:
    test: 'diffEps(vars.expected2, steps.call.res.message, "cases/tol.yaml")'
`,
		},
		{
			name:     "flow vars",
			in:       "vars: {}\nsteps:\n" + diffStep,
			wantTodo: "vars がフロー形式",
		},
		{
			name:     "unknown option",
			in:       "steps:\n  c:\n    exec:\n      command: runnora-diff --ignore .a e.json -\n      stdin: '{{ toJSON(x) }}'\n    test: current.exit_code == 0\n",
			wantTodo: "オプション --ignore は変換できません",
		},
		{
			name:     "test looks at the output",
			in:       "steps:\n  c:\n    exec:\n      command: runnora-diff e.json -\n      stdin: '{{ toJSON(x) }}'\n    test: current.exit_code == 0 || current.stdout != \"\"\n",
			wantTodo: "test が終了コードか summary.differences だけを見る形ではありません",
		},
		{
			name:     "actual from a file",
			in:       "steps:\n  c:\n    exec:\n      command: jsondiff-eps e.json a.json\n    test: current.exit_code == 0\n",
			wantTodo: "引数が「期待ファイル -」の形ではありません",
		},
		{
			name: "other exec is untouched",
			in:   "steps:\n  c:\n    exec:\n      command: echo hi\n",
		},
		{
			name: "CRLF",
			in:   strings.ReplaceAll("vars:\n  a: 1\nsteps:\n  d:\n    dump:\n      expr: x\n      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/x.json'\n"+diffStep, "\n", "\r\n"),
			want: strings.ReplaceAll("vars:\n  a: 1\n  expected: json://../cases/expected.json\nsteps:\n  compare:\n    test: 'diffEps(vars.expected, steps.call.res.message, \"cases/tol.yaml\")'\n", "\n", "\r\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := parseRunbook("runbooks/a.yml", tt.in)
			if !ok {
				t.Fatal("not a runbook")
			}
			plan := &Plan{}
			if _, err := migrateEvidence(d, plan); err != nil {
				t.Fatal(err)
			}
			want := tt.want
			if want == "" {
				want = tt.in
			}
			if got := d.content(); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
			var todos []string
			for _, td := range plan.Todos {
				todos = append(todos, td.Where+": "+td.Message)
			}
			all := strings.Join(todos, "\n")
			if tt.wantTodo == "" && len(todos) > 0 {
				t.Errorf("unexpected TODO: %s", all)
			}
			if tt.wantTodo != "" && !strings.Contains(all, tt.wantTodo) {
				t.Errorf("TODO = %q, want containing %q", all, tt.wantTodo)
			}
		})
	}
}

func TestDiffExpectation(t *testing.T) {
	tests := []struct {
		test      string
		equal, ok bool
	}{
		{test: "current.exit_code == 0", equal: true, ok: true},
		{test: `current.stdout == "" && current.stderr == "" && current.exit_code == 0`, equal: true, ok: true},
		{test: "current.exit_code == 1 && fromJSON(current.stdout).summary.differences > 0", equal: false, ok: true},
		{test: "current.exit_code != 0", equal: false, ok: true},
		{test: "fromJSON(current.stdout).summary.differences == 0", equal: true, ok: true},
		{test: "fromJSON(current.stdout).equal", equal: true, ok: true},
		{test: "!fromJSON(current.stdout).equal", equal: false, ok: true},
		{test: "current.exit_code == 0 && fromJSON(current.stdout).summary.differences > 0"},
		{test: "current.exit_code == 2"},
		{test: `current.stdout == ""`},
		{test: "current.exit_code == 0 || true"},
		{test: "len(fromJSON(current.stdout).differences) == 3"},
	}
	for _, tt := range tests {
		equal, ok := diffExpectation(tt.test)
		if ok != tt.ok || ok && equal != tt.equal {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", tt.test, equal, ok, tt.equal, tt.ok)
		}
	}
}

func TestRelFromRunbook(t *testing.T) {
	for _, tt := range []struct{ rel, p, want string }{
		{"a.yml", "cases/e.json", "cases/e.json"},
		{"runbooks/a.yml", "./cases/e.json", "../cases/e.json"},
		{"runbooks/scenarios/a.yml", "cases/e.json", "../../cases/e.json"},
		{"runbooks/a.yml", "/abs/e.json", "/abs/e.json"},
	} {
		if got := relFromRunbook(tt.rel, tt.p); got != tt.want {
			t.Errorf("relFromRunbook(%q, %q) = %q, want %q", tt.rel, tt.p, got, tt.want)
		}
	}
}
