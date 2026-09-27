package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/k1LoW/runn"
	"google.golang.org/grpc/status"
)

// Mode は証跡に書く範囲。
type Mode string

const (
	// ModeResponse は応答だけを書く (既定)。
	ModeResponse Mode = "response"
	// ModeFull はリクエストと応答の両方を書く。
	ModeFull Mode = "full"
)

// MaxBody は証跡に書く本文の上限 (これを超える分は切り捨てて truncated を付ける)。
const MaxBody = 1 << 20

// File は 1 ステップ分の証跡ファイルの中身。
type File struct {
	ScenarioID string   `json:"scenarioId"`
	Key        string   `json:"key"`
	Index      int      `json:"index"`
	Runner     string   `json:"runner"`
	RunnerKey  string   `json:"runnerKey,omitempty"`
	StartedAt  string   `json:"startedAt"`
	Request    any      `json:"request,omitempty"`
	Response   any      `json:"response,omitempty"`
	Masked     []string `json:"masked,omitempty"`
}

// Written は書き出した証跡ファイル 1 つ。
type Written struct {
	Key   string
	Index int
	// Path は証跡のフォルダ (evidence/) からの相対パス (/ 区切り)。
	Path string
}

// Capturer は runn の Capturer を実装し、1 つの runbook の送受信をステップごとに集める。
// runbook ごとに New で作り、実行後に Flush で書き出す。
type Capturer struct {
	scenarioID string
	mode       Mode
	mask       *Mask
	now        func() time.Time

	mu      sync.Mutex
	key     string
	index   int
	records []*record
	current *record
	errs    error
}

var _ runn.Capturer = (*Capturer)(nil)

// record は 1 ステップ (同じキーで複数回呼ばれたら 1 回分) の送受信。
type record struct {
	key       string
	index     int
	runner    string
	runnerKey string
	started   time.Time
	masked    []string

	http *httpExchange
	grpc *grpcExchange
	db   *dbExchange
	exec *execExchange
}

type httpExchange struct {
	Request  *httpRequest  `json:"request,omitempty"`
	Response *httpResponse `json:"response,omitempty"`
}

type httpRequest struct {
	Method    string              `json:"method"`
	URL       string              `json:"url"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      any                 `json:"body,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
}

type httpResponse struct {
	Status    int                 `json:"status"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      any                 `json:"body,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
}

type grpcExchange struct {
	Type     string
	Service  string
	Method   string
	ReqHdr   map[string][]string
	ReqMsgs  []any
	Status   *grpcStatus
	ResHdr   map[string][]string
	Trailers map[string][]string
	ResMsgs  []any
}

type grpcStatus struct {
	Code    int    `json:"code"`
	Name    string `json:"name"`
	Message string `json:"message,omitempty"`
}

type dbExchange struct {
	Statements []string
	Results    []dbResult
}

type dbResult struct {
	RowsAffected int64            `json:"rowsAffected"`
	LastInsertID int64            `json:"lastInsertId,omitempty"`
	Columns      []string         `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
}

type execExchange struct {
	Command string
	Shell   string
	Stdin   string
	Stdout  string
	Stderr  string
}

// New は scenarioID の runbook 用の Capturer を作る。mask が nil なら決まったヘッダだけを隠す。
func New(scenarioID string, mode Mode, mask *Mask) *Capturer {
	if mode == "" {
		mode = ModeResponse
	}
	if mask == nil {
		mask, _ = NewMask(nil, nil)
	}
	return &Capturer{scenarioID: scenarioID, mode: mode, mask: mask, now: time.Now}
}

// SetCurrentTrails は runn が実行するステップを切り替えるたびに呼ばれる。
func (c *Capturer) SetCurrentTrails(trs runn.Trails) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key, index, ok := StepKey(trs)
	if !ok {
		return
	}
	if key != c.key {
		c.current = nil
	}
	c.key, c.index = key, index
}

// begin は runner の種類 kind の送受信を記録する record を返す。
// 同じステップで同じ種類の送受信が 2 回目に始まった場合は、新しい record を作る。
func (c *Capturer) begin(kind, runnerKey string, restart bool) *record {
	if c.key == "" {
		return nil
	}
	if c.current == nil || (restart && c.current.runner == kind) || (c.current.runner != "" && c.current.runner != kind) {
		c.current = &record{key: c.key, index: c.index, runner: kind, runnerKey: runnerKey, started: c.now()}
		c.records = append(c.records, c.current)
	}
	return c.current
}

// active は送受信の途中 (リクエストの後の応答など) の record を返す。
func (c *Capturer) active(kind string) *record {
	if c.current == nil || c.current.runner != kind {
		return nil
	}
	return c.current
}

// --- HTTP ---

func (c *Capturer) CaptureHTTPRequest(name string, req *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.begin("http", name, true)
	if r == nil {
		return
	}
	r.http = &httpExchange{}
	if c.mode != ModeFull {
		return
	}
	body, truncated, err := readRequestBody(req)
	if err != nil {
		c.errs = errors.Join(c.errs, err)
	}
	r.http.Request = &httpRequest{
		Method:    req.Method,
		URL:       req.URL.String(),
		Headers:   c.mask.HTTPHeaders(req.Header, "request.headers", &r.masked),
		Body:      c.mask.Body(body, &r.masked),
		Truncated: truncated,
	}
}

func (c *Capturer) CaptureHTTPResponse(name string, res *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.active("http")
	if r == nil {
		r = c.begin("http", name, false)
		if r == nil {
			return
		}
		r.http = &httpExchange{}
	}
	var b []byte
	var err error
	b, res.Body, err = drain(res.Body)
	if err != nil {
		c.errs = errors.Join(c.errs, err)
	}
	body, truncated := decodeBody(b)
	r.http.Response = &httpResponse{
		Status:    res.StatusCode,
		Headers:   c.mask.HTTPHeaders(res.Header, "response.headers", &r.masked),
		Body:      c.mask.Body(body, &r.masked),
		Truncated: truncated,
	}
}

func readRequestBody(req *http.Request) (any, bool, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, false, nil
	}
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err != nil {
			return nil, false, err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, false, err
		}
		v, t := decodeBody(b)
		return v, t, nil
	}
	b, body, err := drain(req.Body)
	req.Body = body
	if err != nil {
		return nil, false, err
	}
	v, t := decodeBody(b)
	return v, t, nil
}

// drain は本文を読み切り、同じ内容を読み直せる ReadCloser を返す (runn が後で読むため)。
func drain(rc io.ReadCloser) ([]byte, io.ReadCloser, error) {
	if rc == nil || rc == http.NoBody {
		return nil, rc, nil
	}
	b, err := io.ReadAll(rc)
	_ = rc.Close()
	return b, io.NopCloser(bytes.NewReader(b)), err
}

// decodeBody は本文を JSON として読めればその値、読めなければ文字列にする。
// MaxBody を超える分は切り捨てる (その場合は JSON として読まない)。
func decodeBody(b []byte) (any, bool) {
	if len(b) == 0 {
		return nil, false
	}
	if len(b) > MaxBody {
		cut := b[:MaxBody]
		for !utf8.Valid(cut) && len(cut) > 0 {
			cut = cut[:len(cut)-1]
		}
		return string(cut), true
	}
	if json.Valid(b) {
		var v any
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err := dec.Decode(&v); err == nil {
			return v, false
		}
	}
	if utf8.Valid(b) {
		return string(b), false
	}
	return fmt.Sprintf("(binary %d bytes)", len(b)), false
}

// --- gRPC ---

func (c *Capturer) CaptureGRPCStart(name string, typ runn.GRPCType, service, method string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.begin("grpc", name, true)
	if r == nil {
		return
	}
	r.grpc = &grpcExchange{Type: string(typ), Service: service, Method: method}
}

func (c *Capturer) grpcExchange() *grpcExchange {
	r := c.active("grpc")
	if r == nil || r.grpc == nil {
		return nil
	}
	return r.grpc
}

func (c *Capturer) CaptureGRPCRequestHeaders(h map[string][]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil {
		g.ReqHdr = c.mask.Headers(h, "request.headers", &c.current.masked)
	}
}

func (c *Capturer) CaptureGRPCRequestMessage(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil {
		g.ReqMsgs = append(g.ReqMsgs, c.mask.Body(m, &c.current.masked))
	}
}

func (c *Capturer) CaptureGRPCResponseStatus(s *status.Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil && s != nil {
		g.Status = &grpcStatus{Code: int(s.Code()), Name: s.Code().String(), Message: s.Message()}
	}
}

func (c *Capturer) CaptureGRPCResponseHeaders(h map[string][]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil {
		g.ResHdr = c.mask.Headers(h, "response.headers", &c.current.masked)
	}
}

func (c *Capturer) CaptureGRPCResponseMessage(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil {
		g.ResMsgs = append(g.ResMsgs, c.mask.Body(m, &c.current.masked))
	}
}

func (c *Capturer) CaptureGRPCResponseTrailers(t map[string][]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.grpcExchange(); g != nil {
		g.Trailers = c.mask.Headers(t, "response.trailers", &c.current.masked)
	}
}

func (c *Capturer) CaptureGRPCClientClose()                              {}
func (c *Capturer) CaptureGRPCEnd(string, runn.GRPCType, string, string) {}

// --- DB ---

func (c *Capturer) CaptureDBStatement(name string, stmt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.active("db")
	if r == nil {
		r = c.begin("db", name, false)
		if r == nil {
			return
		}
	}
	if r.db == nil {
		r.db = &dbExchange{}
	}
	r.db.Statements = append(r.db.Statements, stmt)
}

func (c *Capturer) CaptureDBResponse(name string, res *runn.DBResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.active("db")
	if r == nil || r.db == nil || res == nil {
		return
	}
	rows := make([]map[string]any, 0, len(res.Rows))
	for _, row := range res.Rows {
		if m, ok := c.mask.Body(row, &r.masked).(map[string]any); ok {
			rows = append(rows, m)
		} else {
			rows = append(rows, row)
		}
	}
	r.db.Results = append(r.db.Results, dbResult{
		RowsAffected: res.RowsAffected, LastInsertID: res.LastInsertID, Columns: res.Columns, Rows: rows,
	})
}

// --- exec ---

func (c *Capturer) CaptureExecCommand(command, shell string, _ bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.begin("exec", "", true)
	if r == nil {
		return
	}
	r.exec = &execExchange{Command: command, Shell: shell}
}

func (c *Capturer) CaptureExecStdin(stdin string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r := c.active("exec"); r != nil && r.exec != nil {
		r.exec.Stdin = stdin
	}
}

func (c *Capturer) CaptureExecStdout(stdout string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r := c.active("exec"); r != nil && r.exec != nil {
		r.exec.Stdout = stdout
	}
}

func (c *Capturer) CaptureExecStderr(stderr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r := c.active("exec"); r != nil && r.exec != nil {
		r.exec.Stderr = stderr
	}
}

// --- 使わない通知 ---

func (c *Capturer) CaptureStart(runn.Trails, string, string)          {}
func (c *Capturer) CaptureResult(runn.Trails, *runn.RunResult)        {}
func (c *Capturer) CaptureEnd(runn.Trails, string, string)            {}
func (c *Capturer) CaptureResultByStep(runn.Trails, *runn.RunResult)  {}
func (c *Capturer) CaptureCDPStart(string)                            {}
func (c *Capturer) CaptureCDPAction(runn.CDPAction)                   {}
func (c *Capturer) CaptureCDPResponse(runn.CDPAction, map[string]any) {}
func (c *Capturer) CaptureCDPEnd(string)                              {}
func (c *Capturer) CaptureSSHCommand(string)                          {}
func (c *Capturer) CaptureSSHStdout(string)                           {}
func (c *Capturer) CaptureSSHStderr(string)                           {}
func (c *Capturer) CaptureAgentRequest(string, *runn.AgentRequest)    {}
func (c *Capturer) CaptureAgentResponse(string, *runn.AgentResponse)  {}

// Errs は runn に証跡の取得中のエラーを返す (runn はこれを実行結果のエラーにしない)。
// 取得中のエラーは Flush が返す。
func (c *Capturer) Errs() error { return nil }

// Files は集めた送受信を、書き出す形 (File) で返す。
func (c *Capturer) Files() []File {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []File
	for _, r := range c.records {
		f := File{
			ScenarioID: c.scenarioID, Key: r.key, Index: r.index, Runner: r.runner, RunnerKey: r.runnerKey,
			StartedAt: r.started.Format(time.RFC3339Nano), Masked: uniqueSorted(r.masked),
		}
		f.Request, f.Response = r.payload(c.mode)
		if f.Request == nil && f.Response == nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func (r *record) payload(mode Mode) (req, res any) {
	full := mode == ModeFull
	switch {
	case r.http != nil:
		if full && r.http.Request != nil {
			req = r.http.Request
		}
		if r.http.Response != nil {
			res = r.http.Response
		}
	case r.grpc != nil:
		g := r.grpc
		if full {
			req = map[string]any{
				"type": g.Type, "service": g.Service, "method": g.Method,
				"headers": g.ReqHdr, "messages": g.ReqMsgs,
			}
		}
		res = map[string]any{
			"status": g.Status, "headers": g.ResHdr, "trailers": g.Trailers, "messages": g.ResMsgs,
		}
	case r.db != nil:
		if full {
			req = map[string]any{"statements": r.db.Statements}
		}
		if len(r.db.Results) > 0 {
			res = map[string]any{"results": r.db.Results}
		}
	case r.exec != nil:
		if full {
			req = map[string]any{"command": r.exec.Command, "shell": r.exec.Shell, "stdin": r.exec.Stdin}
		}
		res = map[string]any{"stdout": r.exec.Stdout, "stderr": r.exec.Stderr}
	}
	return req, res
}

// Flush は集めた証跡を dir (シナリオの証跡フォルダ) に書き出す。
// base は Written.Path の基準 (証跡のフォルダ evidence/)。
func (c *Capturer) Flush(dir, base string) ([]Written, error) {
	files := c.Files()
	var written []Written
	errs := c.errs
	if len(files) > 0 {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, errors.Join(errs, err)
		}
	}
	used := map[string]int{}
	for _, f := range files {
		name := fmt.Sprintf("%02d-%s", f.Index, SafeName(f.Key))
		used[name]++
		if n := used[name]; n > 1 {
			name = fmt.Sprintf("%s.%d", name, n)
		}
		path := filepath.Join(dir, name+".json")
		b, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("evidence %s: %w", f.Key, err))
			continue
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			rel = path
		}
		written = append(written, Written{Key: f.Key, Index: f.Index, Path: filepath.ToSlash(rel)})
	}
	return written, errs
}
