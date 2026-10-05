package multipartbody

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
)

func TestTypedParts(t *testing.T) {
	root := t.TempDir()
	file := []byte{0, 255, 128, 13, 10, 1}
	if err := os.WriteFile(filepath.Join(root, "data.bin"), file, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := (Builder{Root: root}).Build(map[string]any{
		"metadata": map[string]any{"contentType": "application/json", "value": map[string]any{"name": "日本語", "enabled": true}},
		"file":     map[string]any{"contentType": "text/csv", "file": "data.bin", "filename": "取込.csv"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(out["contentType"].(string))
	if err != nil {
		t.Fatal(err)
	}
	values := out["body"].([]any)
	body := make([]byte, len(values))
	for i, value := range values {
		body[i] = byte(value.(uint64))
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	p, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(p)
	if p.FormName() != "file" || p.FileName() != "取込.csv" || p.Header.Get("Content-Type") != "text/csv" || !bytes.Equal(data, file) {
		t.Fatalf("file part: %v %x", p.Header, data)
	}
	p, err = r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(p)
	if p.FormName() != "metadata" || p.Header.Get("Content-Type") != "application/json" || string(data) != `{"enabled":true,"name":"日本語"}` {
		t.Fatalf("metadata: %v %s", p.Header, data)
	}
	if _, err := r.NextPart(); err != io.EOF {
		t.Fatalf("end: %v", err)
	}
}

func TestInvalidParts(t *testing.T) {
	cases := []map[string]any{
		{}, {"p": "string"},
		{"p": map[string]any{"value": "x", "file": "x"}},
		{"p": map[string]any{"value": "x", "contentType": "text/plain\r\nBad: value"}},
		{"p": map[string]any{"value": "x", "contentType": "invalid"}},
		{"p": map[string]any{"value": map[string]any{"a": 1}}},
		{"p": map[string]any{"file": "missing.csv"}},
		{"p": map[string]any{"file": "https://example.com/data.csv"}},
		{"p": map[string]any{"value": "x", "filename": "x.csv"}},
		{"p\r\n": map[string]any{"value": "x"}},
		{"p": map[string]any{"value": "x", "contentTyp": "text/plain"}},
	}
	for i, parts := range cases {
		if _, err := (Builder{Root: t.TempDir()}).Build(parts); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}
