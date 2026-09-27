package evidence

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
)

// Masked は隠した値の置き換え先。
const Masked = "***"

// alwaysMaskedHeaders は設定によらず必ず隠すヘッダ (小文字)。gRPC のメタデータにも使う。
var alwaysMaskedHeaders = []string{"authorization", "proxy-authorization", "cookie", "set-cookie"}

// Mask は証跡に書く前に秘密情報を隠す規則。
type Mask struct {
	headers map[string]bool
	paths   []maskPath
}

type maskPath struct {
	expr  string
	query *gojq.Code
}

// NewMask は、必ず隠すヘッダに headers を加え、本文の JSON で隠す場所 paths (jq 形式) を解釈する。
func NewMask(headers, paths []string) (*Mask, error) {
	m := &Mask{headers: map[string]bool{}}
	for _, h := range alwaysMaskedHeaders {
		m.headers[h] = true
	}
	for _, h := range headers {
		m.headers[strings.ToLower(h)] = true
	}
	for _, p := range paths {
		code, err := compileMaskPath(p)
		if err != nil {
			return nil, err
		}
		m.paths = append(m.paths, maskPath{expr: p, query: code})
	}
	return m, nil
}

// ValidatePaths は evidence.mask.paths の書き方を確かめる (runnora validate 用)。
func ValidatePaths(paths []string) error {
	for _, p := range paths {
		if _, err := compileMaskPath(p); err != nil {
			return err
		}
	}
	return nil
}

func compileMaskPath(p string) (*gojq.Code, error) {
	// 値がある場所だけを列挙する (存在しないキーに *** を作らないため)
	src := fmt.Sprintf("[path(%s) as $p | select(getpath($p) != null) | $p]", p)
	q, err := gojq.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("evidence.mask.paths %q: %w", p, err)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, fmt.Errorf("evidence.mask.paths %q: %w", p, err)
	}
	return code, nil
}

// Headers はヘッダの写しを返す。隠すヘッダの値は *** にし、隠した名前を masked に加える。
func (m *Mask) Headers(h map[string][]string, where string, masked *[]string) map[string][]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string][]string, len(h))
	for k, v := range h {
		if m.headers[strings.ToLower(k)] {
			vv := make([]string, len(v))
			for i := range vv {
				vv[i] = Masked
			}
			out[k] = vv
			*masked = append(*masked, where+"."+k)
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// HTTPHeaders は http.Header を Headers で隠す。
func (m *Mask) HTTPHeaders(h http.Header, where string, masked *[]string) map[string][]string {
	return m.Headers(map[string][]string(h), where, masked)
}

// Body は JSON の値 v の中の、evidence.mask.paths に当たる値を *** にした写しを返す。
// JSON でない値 (文字列の本文など) や、パスが当てはまらない値はそのまま返す。
func (m *Mask) Body(v any, masked *[]string) any {
	if len(m.paths) == 0 || v == nil {
		return v
	}
	norm, err := normalize(v)
	if err != nil {
		return v
	}
	for _, p := range m.paths {
		iter := p.query.Run(norm)
		res, ok := iter.Next()
		if !ok {
			continue
		}
		paths, ok := res.([]any)
		if !ok || len(paths) == 0 {
			// 型が合わない (オブジェクトでない本文に .key など) ものはエラーになるので無視する
			continue
		}
		for _, raw := range paths {
			path, ok := raw.([]any)
			if !ok {
				continue
			}
			updated, err := setPath(norm, path)
			if err != nil {
				continue
			}
			norm = updated
			*masked = append(*masked, p.expr)
		}
	}
	return norm
}

// normalize は gojq が扱える形 (map[string]any / []any / float64 など) に変換する。
func normalize(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func setPath(v any, path []any) (any, error) {
	iter := setPathQuery.Run(v, path)
	res, ok := iter.Next()
	if !ok {
		return v, nil
	}
	if err, ok := res.(error); ok {
		return nil, err
	}
	return res, nil
}

// setPathQuery は path の値を *** に置き換える。
var setPathQuery = mustCompile(`setpath($p; "`+Masked+`")`, "$p")

func mustCompile(src string, vars ...string) *gojq.Code {
	q, err := gojq.Parse(src)
	if err != nil {
		panic(err)
	}
	code, err := gojq.Compile(q, gojq.WithVariables(vars))
	if err != nil {
		panic(err)
	}
	return code
}

// uniqueSorted は重複を除いて並べる (masked の記録用)。
func uniqueSorted(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
