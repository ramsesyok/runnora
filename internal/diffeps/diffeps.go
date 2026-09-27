// Package diffeps は runbook の式から使う組み込み関数 diffEps() を提供する。
//
// diffEps は runnora-diff の比較処理 (jsondiff) で、2 つの JSON の値を許容誤差付きで比べる。
//
//	test: diffEps(vars.expected, current.res.message, "cases/series-analysis/tolerances.yaml")
//
// 差分がなければ true、あれば false を返す。差分の中身は、呼ばれたステップのキーと一緒に
// Recorder に記録し、runnora がレポートと証跡に書く (docs/design/evidence-report.md の 5 章)。
package diffeps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/ramsesyok/runnora-diff/jsondiff"
)

// FuncName は runbook の式で使う関数名。
const FuncName = "diffEps"

// Call は diffEps の呼び出し 1 回分の結果。
type Call struct {
	// Key と Index は呼ばれたステップ (証跡と report.json のキー)。
	Key   string
	Index int
	// Result は比較結果。
	Result *jsondiff.Result
}

// Recorder は 1 つの runbook の実行中の diffEps の呼び出しを記録する。
type Recorder struct {
	root    string
	current func() (key string, index int)

	mu    sync.Mutex
	calls []Call
}

// NewRecorder は Recorder を作る。root は rules に書いたパスの基準 (プロジェクトルート)、
// current は実行中のステップのキーと番号を返す関数 (evidence.Capturer.CurrentStep)。
func NewRecorder(root string, current func() (string, int)) *Recorder {
	return &Recorder{root: root, current: current}
}

// Calls は記録した呼び出しを呼ばれた順に返す。
func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Call(nil), r.calls...)
}

// Func は runn.Func に登録する関数を返す。
//
//	diffEps(expected, actual)          許容誤差なし (完全一致)
//	diffEps(expected, actual, rules)   rules は runnora-diff の設定ファイルのパス (プロジェクトルート基準) か、設定そのもの (map)
func (r *Recorder) Func() func(args ...any) (bool, error) {
	return func(args ...any) (bool, error) {
		if len(args) < 2 || len(args) > 3 {
			return false, fmt.Errorf("%s: 引数は (expected, actual) か (expected, actual, rules) です (%d 個指定されています)", FuncName, len(args))
		}
		var rules any
		if len(args) == 3 {
			rules = args[2]
		}
		opts, err := r.options(rules)
		if err != nil {
			return false, fmt.Errorf("%s: %w", FuncName, err)
		}
		expected, err := normalize(args[0])
		if err != nil {
			return false, fmt.Errorf("%s: expected を JSON として扱えません: %w", FuncName, err)
		}
		actual, err := normalize(args[1])
		if err != nil {
			return false, fmt.Errorf("%s: actual を JSON として扱えません: %w", FuncName, err)
		}
		res, err := jsondiff.Compare(expected, actual, opts)
		if err != nil {
			return false, fmt.Errorf("%s: %w", FuncName, err)
		}
		key, index := "", 0
		if r.current != nil {
			key, index = r.current()
		}
		r.mu.Lock()
		r.calls = append(r.calls, Call{Key: key, Index: index, Result: res})
		r.mu.Unlock()
		return res.Equal, nil
	}
}

// configCache は読み込んだ設定ファイル (絶対パス → 設定)。同じファイルを何度も読まない。
var configCache sync.Map

func (r *Recorder) options(rules any) (*jsondiff.Options, error) {
	switch v := rules.(type) {
	case nil:
		return nil, nil
	case string:
		path := v
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.root, filepath.FromSlash(path))
		}
		if cached, ok := configCache.Load(path); ok {
			return cached.(*jsondiff.Options), nil
		}
		opts, err := jsondiff.LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("許容誤差の設定を読めません: %w", err)
		}
		configCache.Store(path, opts)
		return opts, nil
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		opts, err := jsondiff.ParseJSONConfig(b)
		if err != nil {
			return nil, fmt.Errorf("許容誤差の設定が正しくありません: %w", err)
		}
		return opts, nil
	default:
		return nil, errors.New("rules は設定ファイルのパス (文字列) か、設定 (map) で指定してください")
	}
}

// normalize は runn の値を JSON の値 (数値は json.Number) にそろえる。
// runn の値は int や float64、gRPC のメッセージの map などが混ざるので、JSON を経由する。
func normalize(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsondiff.DecodeJSON(bytes.NewReader(b))
}
