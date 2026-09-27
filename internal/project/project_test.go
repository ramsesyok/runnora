package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sampleYAML = `version: 2
project:
  name: library-api-test
defaults:
  env: unit
environments:
  mock:
    vars:
      API_URL: http://127.0.0.1:18080
  unit:
    description: API 単体
    vars:
      API_URL: http://127.0.0.1:18081
      ORACLE_DSN: oracle://libapp:${LIBAPP_PASSWORD:-libapp_pw}@127.0.0.1:1522/FREEPDB1
    oracle:
      dsn: ${ORACLE_DSN}
      max_open_conns: 5
    hooks:
      before: [sql/common/00_reset.sql]
      after: [sql/common/90_verify.sql]
    backends:
      calc: { mode: stub, note: API 内蔵スタブ }
  integration:
    vars:
      API_URL: ${INT_API_URL}
suites:
  scenarios:
    select:
      paths: [runbooks/scenarios/*.yml]
  integration:
    env: integration
    select:
      paths: [runbooks/scenarios/*.yml]
      ids: [LIB-001]
    vars:
      TOLERANCE_RULES: rules/integration.yaml
report:
  format: json
  output: ${REPORT_DIR:-reports}/result.json
`

func mustParse(t *testing.T, data string) *Project {
	t.Helper()
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p.Root = filepath.FromSlash("/proj")
	return p
}

func noEnv(string) (string, bool) { return "", false }

func envOf(m map[string]string) Lookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantLegacy bool
		wantErr    string
	}{
		{name: "valid", yaml: sampleYAML},
		{name: "old config.yaml", yaml: "app:\n  name: x\noracle:\n  dsn: \"\"\nhooks:\n  common:\n    before: []\n", wantLegacy: true},
		{name: "version missing", yaml: "environments: {}\n", wantLegacy: true},
		{name: "unsupported version", yaml: "version: 3\n", wantLegacy: true},
		{name: "unknown key", yaml: "version: 2\nenvironmnets: {}\n", wantErr: "environmnets"},
		{name: "defaults.env unknown", yaml: "version: 2\ndefaults:\n  env: dev\n", wantErr: "defaults.env"},
		{name: "suite env unknown", yaml: "version: 2\nsuites:\n  s:\n    env: dev\n    select:\n      paths: [a.yml]\n", wantErr: "suites.s.env"},
		{name: "suite without paths", yaml: "version: 2\nsuites:\n  s:\n    select: {}\n", wantErr: "suites.s.select.paths"},
		{name: "backend mode invalid", yaml: "version: 2\nenvironments:\n  e:\n    backends:\n      calc: { mode: fake }\n", wantErr: "backends.calc.mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			var legacy *LegacyError
			switch {
			case tt.wantLegacy:
				if !errors.As(err, &legacy) {
					t.Fatalf("want LegacyError, got %v", err)
				}
				if !strings.Contains(err.Error(), "runnora-migrate") {
					t.Errorf("legacy error should mention runnora-migrate: %v", err)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSelectEnv(t *testing.T) {
	p := mustParse(t, sampleYAML)
	single := mustParse(t, "version: 2\nenvironments:\n  only: {}\n")
	multi := mustParse(t, "version: 2\nenvironments:\n  a: {}\n  b: {}\n")
	none := mustParse(t, "version: 2\n")
	tests := []struct {
		name    string
		p       *Project
		flag    string
		suite   string
		want    string
		wantErr string
	}{
		{name: "defaults.env", p: p, want: "unit"},
		{name: "flag wins over defaults", p: p, flag: "mock", want: "mock"},
		{name: "suite env", p: p, suite: "integration", want: "integration"},
		{name: "suite without env uses defaults", p: p, suite: "scenarios", want: "unit"},
		{name: "flag matches suite env", p: p, flag: "integration", suite: "integration", want: "integration"},
		{name: "flag conflicts with suite env", p: p, flag: "unit", suite: "integration", wantErr: "--env unit"},
		{name: "unknown flag env", p: p, flag: "dev", wantErr: "環境 \"dev\""},
		{name: "unknown suite", p: p, suite: "nope", wantErr: "スイート \"nope\""},
		{name: "single env", p: single, want: "only"},
		{name: "multiple envs without default", p: multi, wantErr: "環境が複数"},
		{name: "no envs", p: none, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.p.SelectEnv(tt.flag, tt.suite)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v (env %q)", tt.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	p := mustParse(t, sampleYAML)
	tests := []struct {
		name          string
		env           string
		suite         string
		cli           map[string]string
		os            map[string]string
		wantVars      map[string]string
		wantOverrides []string
		wantDSN       string
		wantErr       string
	}{
		{
			name: "yaml values with default in nested reference",
			env:  "unit",
			wantVars: map[string]string{
				"API_URL":    "http://127.0.0.1:18081",
				"ORACLE_DSN": "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1",
			},
			wantDSN: "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1",
		},
		{
			name: "OS value used inside yaml value",
			env:  "unit",
			os:   map[string]string{"LIBAPP_PASSWORD": "secret"},
			wantVars: map[string]string{
				"API_URL":    "http://127.0.0.1:18081",
				"ORACLE_DSN": "oracle://libapp:secret@127.0.0.1:1522/FREEPDB1",
			},
			wantDSN: "oracle://libapp:secret@127.0.0.1:1522/FREEPDB1",
		},
		{
			name:          "OS overrides yaml var and is recorded",
			env:           "unit",
			os:            map[string]string{"API_URL": "http://os:1"},
			wantVars:      map[string]string{"API_URL": "http://os:1", "ORACLE_DSN": "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1"},
			wantOverrides: []string{"API_URL"},
			wantDSN:       "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1",
		},
		{
			name:          "--var overrides OS and yaml",
			env:           "unit",
			os:            map[string]string{"API_URL": "http://os:1"},
			cli:           map[string]string{"API_URL": "http://cli:2", "EXTRA": "x"},
			wantVars:      map[string]string{"API_URL": "http://cli:2", "EXTRA": "x", "ORACLE_DSN": "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1"},
			wantOverrides: []string{"API_URL"},
			wantDSN:       "oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1",
		},
		{
			name:     "suite vars are added",
			env:      "integration",
			suite:    "integration",
			os:       map[string]string{"INT_API_URL": "http://int"},
			wantVars: map[string]string{"API_URL": "http://int", "TOLERANCE_RULES": "rules/integration.yaml"},
		},
		{
			name:    "undefined without default",
			env:     "integration",
			wantErr: "environments.integration.vars.API_URL: 未定義の変数 INT_API_URL",
		},
		{
			name:     "no environment",
			env:      "",
			cli:      map[string]string{"A": "1"},
			wantVars: map[string]string{"A": "1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := p.Resolve(tt.env, tt.suite, tt.cli, envOf(tt.os))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(res.Vars, tt.wantVars) {
				t.Errorf("vars = %v, want %v", res.Vars, tt.wantVars)
			}
			if !reflect.DeepEqual(res.Overrides, tt.wantOverrides) {
				t.Errorf("overrides = %v, want %v", res.Overrides, tt.wantOverrides)
			}
			if res.Oracle.DSN != tt.wantDSN {
				t.Errorf("dsn = %q, want %q", res.Oracle.DSN, tt.wantDSN)
			}
		})
	}
}

func TestResolveOracleHooksAndReport(t *testing.T) {
	p := mustParse(t, sampleYAML)
	res, err := p.Resolve("unit", "", nil, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasOracle || res.Oracle.MaxOpenConns != 5 || res.Oracle.MaxIdleConns != 2 || res.Oracle.ConnMaxLifetimeSec != 300 {
		t.Errorf("oracle defaults not applied: %+v", res.Oracle)
	}
	wantBefore := []string{filepath.Join(p.Root, "sql", "common", "00_reset.sql")}
	if !reflect.DeepEqual(res.Before, wantBefore) {
		t.Errorf("before = %v, want %v", res.Before, wantBefore)
	}
	if res.ReportOutput != "reports/result.json" {
		t.Errorf("report output = %q", res.ReportOutput)
	}
	if res.Backends["calc"].Mode != "stub" {
		t.Errorf("backends = %v", res.Backends)
	}

	mock, err := p.Resolve("mock", "", nil, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if mock.HasOracle || len(mock.Before) != 0 {
		t.Errorf("mock env should have no oracle or hooks: %+v", mock)
	}
}

func TestExpandAndRefs(t *testing.T) {
	lookup := envOf(map[string]string{"SET": "v", "EMPTY": ""})
	tests := []struct {
		in            string
		want          string
		wantUndefined []string
	}{
		{in: "${SET}", want: "v"},
		{in: "a-${SET}-b", want: "a-v-b"},
		{in: "${MISSING:-d}", want: "d"},
		{in: "${MISSING-d}", want: "d"},
		{in: "${EMPTY:-d}", want: "d"},
		{in: "${EMPTY-d}", want: ""},
		{in: "${MISSING}", want: "", wantUndefined: []string{"MISSING"}},
		{in: "{{ vars.x }} $NOT_BRACED", want: "{{ vars.x }} $NOT_BRACED"},
	}
	for _, tt := range tests {
		got, undefined := Expand(tt.in, lookup)
		if got != tt.want || !reflect.DeepEqual(undefined, tt.wantUndefined) {
			t.Errorf("Expand(%q) = %q %v, want %q %v", tt.in, got, undefined, tt.want, tt.wantUndefined)
		}
	}

	refs := Refs("${A} ${B:-x} ${A} ${C:?msg}")
	want := []Ref{{Name: "A"}, {Name: "B", HasDefault: true}, {Name: "C"}}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("Refs = %v, want %v", refs, want)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "runbooks", "scenarios")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()

	tests := []struct {
		name     string
		explicit string
		dir      string
		want     string
		wantErr  bool
	}{
		{name: "found in dir", dir: root, want: filepath.Join(root, FileName)},
		{name: "found in parent", dir: nested, want: filepath.Join(root, FileName)},
		{name: "explicit", explicit: filepath.Join(root, FileName), dir: other, want: filepath.Join(root, FileName)},
		{name: "explicit missing", explicit: filepath.Join(other, FileName), dir: other, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Find(tt.explicit, tt.dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	// 見つからない場合 (一時ディレクトリの親に runnora.yaml がないことを前提にする)
	if got, err := Find("", other); err != nil || (got != "" && !strings.HasPrefix(got, filepath.Dir(other))) {
		t.Errorf("not found case: got %q, %v", got, err)
	}
}
