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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/evidence"
	"github.com/ramsesyok/runnora/internal/multipartbody"
	"github.com/ramsesyok/runnora/internal/scenario"
)

func TestRunTypedMultipartThroughInclude(t *testing.T) {
	f := newFixture(t)
	// Match CLI execution from the project root. Windows runners place the
	// checkout and t.TempDir on different drives, which runn cannot relativize.
	t.Chdir(f.root)
	binary := []byte{0, 255, 128, 13, 10, 1}
	f.write("data.bin", string(binary))
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
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
			if p.FormName() == "file" && (p.FileName() != "取込.csv" || strings.Contains(p.Header.Get("Content-Disposition"), "filename*=")) {
				t.Errorf("incorrect filename encoding: %s", p.Header.Get("Content-Disposition"))
			}
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
      upload: 'multipart({"metadata": {"contentType": "application/json", "value": {"name": vars.name}}, "file": {"contentType": "text/csv", "file": "data.bin", "filename": "取込.csv"}})'
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
	if err != nil || report.Failed != 0 || calls.Load() != 2 {
		t.Fatalf("run: %v, report=%+v, calls=%d", err, report, calls.Load())
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

func TestRunTypedMultipartRejectsOversizedFile(t *testing.T) {
	f := newFixture(t)
	t.Chdir(f.root)
	path := f.write("large.csv", "")
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(multipartbody.MaxBodyBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	runbookPath := f.write("oversized.yml", fmt.Sprintf(`runnora:
  id: TOO-LARGE
runners:
  req: %s
steps:
  prepare:
    bind:
      upload: 'multipart({"file": {"file": "large.csv", "contentType": "text/csv"}})'
  send:
    req:
      /upload:
        post:
          headers:
            Content-Type: "{{ upload.contentType }}"
          body:
            application/octet-stream: "{{ upload.body }}"
`, srv.URL))
	rb, err := scenario.Read(runbookPath, f.root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}})
	if err == nil || report == nil || report.Failed != 1 || calls.Load() != 0 ||
		len(report.Results) != 1 || !strings.Contains(report.Results[0].Error, "maximum size") {
		t.Fatalf("oversized upload: error=%v, report=%+v, HTTP calls=%d", err, report, calls.Load())
	}
}
