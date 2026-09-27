package evidence_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/k1LoW/runn"

	"github.com/ramsesyok/runnora/internal/evidence"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": r.URL.Path, "sent": string(body), "token": "t-123", "user": map[string]any{"password": "p"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func write(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// setup は include・loop・if・exec を含む runbook を作る。
func setup(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "part.yml", `desc: part
runners:
  req: `+url+`
steps:
  call:
    req:
      /part:
        get:
          body: null
`)
	return write(t, dir, "main.yml", `desc: main
runners:
  req: `+url+`
steps:
  first:
    req:
      /first:
        post:
          headers:
            Authorization: Bearer abc
          body:
            application/json:
              name: taro
    test: current.res.status == 200
  only_test:
    test: true
  poll:
    loop:
      count: 2
    req:
      /poll:
        get:
          body: null
  inc:
    include:
      path: part.yml
  skipped:
    if: 'len("x") == 0'
    req:
      /never:
        get:
          body: null
  cmd:
    exec:
      command: echo hello
`)
}

func run(t *testing.T, book string, c *evidence.Capturer) {
	t.Helper()
	op, err := runn.Load(book, runn.Capture(c), runn.Scopes("read:parent", "run:exec"))
	if err != nil {
		t.Fatal(err)
	}
	if err := op.RunN(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCapturer_KeysAndFiles(t *testing.T) {
	srv := server(t)
	book := setup(t, srv.URL)

	for _, tc := range []struct {
		mode        evidence.Mode
		wantRequest bool
	}{
		{evidence.ModeResponse, false},
		{evidence.ModeFull, true},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			mask, err := evidence.NewMask([]string{"X-Unused"}, []string{".token", ".user.password", ".missing"})
			if err != nil {
				t.Fatal(err)
			}
			c := evidence.New("LIB-001", tc.mode, mask)
			run(t, book, c)

			out := t.TempDir()
			dir := filepath.Join(out, "LIB-001")
			written, err := c.Flush(dir, out)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, w := range written {
				got = append(got, w.Path)
			}
			want := []string{
				"LIB-001/01-first.json",
				"LIB-001/03-poll[0].json",
				"LIB-001/03-poll[1].json",
				"LIB-001/04-inc.call.json",
				"LIB-001/06-cmd.json",
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("files:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}

			var first evidence.File
			readJSON(t, filepath.Join(dir, "01-first.json"), &first)
			if first.ScenarioID != "LIB-001" || first.Key != "first" || first.Index != 1 || first.Runner != "http" || first.RunnerKey != "req" {
				t.Errorf("first: %+v", first)
			}
			res := first.Response.(map[string]any)
			if res["status"].(float64) != 200 {
				t.Errorf("status: %v", res["status"])
			}
			body := res["body"].(map[string]any)
			if body["token"] != evidence.Masked || body["user"].(map[string]any)["password"] != evidence.Masked || body["path"] != "/first" {
				t.Errorf("body not masked as expected: %v", body)
			}
			if _, ok := body["missing"]; ok {
				t.Error("mask created a missing key")
			}
			if h := res["headers"].(map[string]any)["Set-Cookie"].([]any); h[0] != evidence.Masked {
				t.Errorf("Set-Cookie: %v", h)
			}
			if (first.Request != nil) != tc.wantRequest {
				t.Fatalf("request present = %v, want %v", first.Request != nil, tc.wantRequest)
			}
			if tc.wantRequest {
				req := first.Request.(map[string]any)
				if req["method"] != "POST" || !strings.HasSuffix(req["url"].(string), "/first") {
					t.Errorf("request: %v", req)
				}
				if req["headers"].(map[string]any)["Authorization"].([]any)[0] != evidence.Masked {
					t.Errorf("Authorization not masked: %v", req["headers"])
				}
				if req["body"].(map[string]any)["name"] != "taro" {
					t.Errorf("request body: %v", req["body"])
				}
			}
			wantMasked := []string{".token", ".user.password", "response.headers.Set-Cookie"}
			if tc.wantRequest {
				wantMasked = append(wantMasked, "request.headers.Authorization")
				sort.Strings(wantMasked)
			}
			if strings.Join(first.Masked, ",") != strings.Join(wantMasked, ",") {
				t.Errorf("masked = %v, want %v", first.Masked, wantMasked)
			}

			var cmd evidence.File
			readJSON(t, filepath.Join(dir, "06-cmd.json"), &cmd)
			if cmd.Runner != "exec" || !strings.Contains(cmd.Response.(map[string]any)["stdout"].(string), "hello") {
				t.Errorf("exec: %+v", cmd)
			}
		})
	}
}

func TestStepKey(t *testing.T) {
	i0, i3, l2 := 0, 3, 2
	tests := []struct {
		name      string
		trs       runn.Trails
		wantKey   string
		wantIndex int
	}{
		{"top", runn.Trails{{Type: runn.TrailTypeRunbook}, {Type: runn.TrailTypeStep, StepKey: "a", StepIndex: &i3}}, "a", 4},
		{"loop", runn.Trails{{Type: runn.TrailTypeRunbook}, {Type: runn.TrailTypeStep, StepKey: "a", StepIndex: &i0}, {Type: runn.TrailTypeLoop, LoopIndex: &l2, StepKey: "a"}}, "a[2]", 1},
		{"include", runn.Trails{
			{Type: runn.TrailTypeRunbook}, {Type: runn.TrailTypeStep, StepKey: "inc", StepIndex: &i3},
			{Type: runn.TrailTypeRunbook}, {Type: runn.TrailTypeStep, StepKey: "call", StepIndex: &i0},
		}, "inc.call", 4},
		{"list steps", runn.Trails{{Type: runn.TrailTypeRunbook}, {Type: runn.TrailTypeStep, StepIndex: &i3}}, "3", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, index, ok := evidence.StepKey(tt.trs)
			if !ok || key != tt.wantKey || index != tt.wantIndex {
				t.Fatalf("StepKey = %q, %d, %v; want %q, %d", key, index, ok, tt.wantKey, tt.wantIndex)
			}
		})
	}
	if _, _, ok := evidence.StepKey(runn.Trails{{Type: runn.TrailTypeRunbook}}); ok {
		t.Error("no step trail should not be ok")
	}
}

func TestNewMask_InvalidPath(t *testing.T) {
	if _, err := evidence.NewMask(nil, []string{".a["}); err == nil {
		t.Fatal("expected error")
	}
	if err := evidence.ValidatePaths([]string{".a", ".. | .token?"}); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
