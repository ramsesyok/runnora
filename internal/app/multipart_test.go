package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/evidence"
	"github.com/ramsesyok/runnora/internal/scenario"
)

func TestRunTypedMultipartThroughInclude(t *testing.T) {
	f := newFixture(t)
	binary := []byte{0, 255, 128, 13, 10, 1}
	f.write("data.bin", string(binary))
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("multipart: %v", err)
			w.WriteHeader(400)
			return
		}
		got := map[string]string{}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			data, _ := io.ReadAll(p)
			got[p.FormName()] = p.Header.Get("Content-Type")
			if p.FormName() == "file" && !bytes.Equal(data, binary) {
				t.Errorf("binary changed: %x", data)
			}
			if p.FormName() == "metadata" && string(data) != `{"name":"from-bind"}` {
				t.Errorf("metadata: %s", data)
			}
		}
		if got["metadata"] != "application/json" || got["file"] != "text/csv" {
			t.Errorf("part headers: %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	f.write("runbooks/part.yml", fmt.Sprintf(`runners:
  req: %s
steps:
  prepare:
    bind:
      upload: 'multipart({"metadata": {"contentType": "application/json", "value": {"name": vars.name}}, "file": {"contentType": "text/csv", "file": "data.bin"}})'
  send:
    req:
      /upload:
        post:
          headers:
            Accept: application/json
            Content-Type: "{{ upload.contentType }}"
          body:
            application/octet-stream: "{{ upload.body }}"
    test: current.res.status == 200 && current.res.body.ok == true
`, srv.URL))
	p := f.write("runbooks/main.yml", `runnora:
  id: MULTIPART
steps:
  value:
    bind:
      name: '"from-bind"'
  nested:
    loop:
      count: 2
    include:
      path: part.yml
      vars:
        name: "{{ name }}"
`)
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	ev := filepath.Join(t.TempDir(), "evidence")
	report, err := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}, EvidenceDir: ev, EvidenceMode: evidence.ModeFull})
	if err != nil || report.Failed != 0 || calls != 2 {
		t.Fatalf("run: %v, report=%+v, calls=%d", err, report, calls)
	}
	files, err := filepath.Glob(filepath.Join(ev, "MULTIPART", "*.json"))
	if err != nil || len(files) != 2 {
		t.Fatalf("evidence: %v %v", files, err)
	}
	for _, file := range files {
		data, _ := os.ReadFile(file)
		var captured struct {
			Request struct {
				Headers map[string][]string `json:"headers"`
			} `json:"request"`
		}
		if err := json.Unmarshal(data, &captured); err != nil {
			t.Fatal(err)
		}
		if len(captured.Request.Headers["Content-Type"]) == 0 {
			t.Fatalf("missing request Content-Type: %s", data)
		}
	}
}
