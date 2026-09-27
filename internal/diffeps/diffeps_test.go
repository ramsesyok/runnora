package diffeps_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora-diff/jsondiff"

	"github.com/ramsesyok/runnora/internal/diffeps"
)

func readJSON(t *testing.T, name string) any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// testdata は runnora-diff の examples/runn (v0.2.1)。runnora-diff の CLI では
// rules.yaml ありで一致、なしで差分ありになる (examples/runn/compare.yml)。
func TestDiffEps(t *testing.T) {
	root, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	expected, actual := readJSON(t, "expected.json"), readJSON(t, "actual.json")

	tests := []struct {
		name    string
		args    []any
		want    bool
		wantErr string
	}{
		{name: "rules file (project root relative)", args: []any{expected, actual, "rules.yaml"}, want: true},
		{name: "rules file absolute", args: []any{expected, actual, filepath.Join(root, "rules.yaml")}, want: true},
		{name: "no rules", args: []any{expected, actual}, want: false},
		{name: "inline rules", args: []any{
			map[string]any{"a": 1.0, "ts": "x"}, map[string]any{"a": 1.004, "ts": "y"},
			map[string]any{"default": map[string]any{"abs": 0.01}, "ignore": []any{".ts"}},
		}, want: true},
		{name: "inline rules too strict", args: []any{
			map[string]any{"a": 1.0}, map[string]any{"a": 1.004}, map[string]any{"default": map[string]any{"abs": 0.001}},
		}, want: false},
		{name: "int and float compare as numbers", args: []any{map[string]any{"n": 3}, map[string]any{"n": 3.0}}, want: true},
		{name: "arrays keep order", args: []any{[]any{1, 2}, []any{2, 1}}, want: false},
		{name: "missing rules file", args: []any{expected, actual, "nope.yaml"}, wantErr: "許容誤差の設定を読めません"},
		{name: "unknown rules key", args: []any{1, 1, map[string]any{"defualt": 1}}, wantErr: "許容誤差の設定が正しくありません"},
		{name: "bad rules type", args: []any{1, 1, 3}, wantErr: "rules は"},
		{name: "too few args", args: []any{1}, wantErr: "引数は"},
		{name: "not JSON", args: []any{func() {}, 1}, wantErr: "expected を JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := 0
			rec := diffeps.NewRecorder(root, func() (string, int) { step++; return "check", 3 })
			got, err := rec.Func()(tt.args...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				if len(rec.Calls()) != 0 {
					t.Error("failed calls should not be recorded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("diffEps = %v, want %v", got, tt.want)
			}
			calls := rec.Calls()
			if len(calls) != 1 || calls[0].Key != "check" || calls[0].Index != 3 || calls[0].Result.Equal != tt.want {
				t.Fatalf("calls: %+v", calls)
			}
		})
	}
}

// runnora-diff の CLI と同じ比較処理なので、差分の中身も同じになる。
func TestDiffEps_SameAsRunnoraDiff(t *testing.T) {
	expectedBytes, _ := os.ReadFile(filepath.Join("testdata", "expected.json"))
	actualBytes, _ := os.ReadFile(filepath.Join("testdata", "actual.json"))
	want, err := jsondiff.CompareBytes(expectedBytes, actualBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := diffeps.NewRecorder(".", nil)
	if _, err := rec.Func()(readJSON(t, "expected.json"), readJSON(t, "actual.json")); err != nil {
		t.Fatal(err)
	}
	got := rec.Calls()[0].Result
	// 数値の書き方 (1.0 と 1) は runn が json:// を float64 で読むので残らない。値として比べる
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	var gv, wv any
	_ = json.Unmarshal(gb, &gv)
	_ = json.Unmarshal(wb, &wv)
	if !reflect.DeepEqual(gv, wv) {
		t.Fatalf("result differs from runnora-diff:\n got: %s\nwant: %s", gb, wb)
	}
	if got.Summary.Differences == 0 {
		t.Fatal("expected differences without rules")
	}
}
