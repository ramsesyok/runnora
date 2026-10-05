package multipartbody

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
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
	if strings.Contains(p.Header.Get("Content-Disposition"), "filename*") {
		t.Fatal("RFC 7578 forbids filename* in form-data")
	}
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
		{"p": map[string]any{"file": "x", "filename": "bad\x00.csv"}},
		{"p": map[string]any{"file": "x", "filename": "bad\r\n.csv"}},
		{"p\t": map[string]any{"value": "x"}},
		{string([]byte{0xff}): map[string]any{"value": "x"}},
	}
	for i, parts := range cases {
		if _, err := (Builder{Root: t.TempDir()}).Build(parts); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestDispositionParameters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "取込.csv"), []byte("csv"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"", `取込"稿\名.csv`} {
		name := `添付"\`
		desc := map[string]any{"file": "取込.csv", "contentType": "text/csv"}
		if filename != "" {
			desc["filename"] = filename
		} else {
			filename = "取込.csv"
		}
		out, err := (Builder{Root: root}).Build(map[string]any{name: desc})
		if err != nil {
			t.Fatal(err)
		}
		_, params, _ := mime.ParseMediaType(out["contentType"].(string))
		values := out["body"].([]any)
		data := make([]byte, len(values))
		for i, v := range values {
			data[i] = byte(v.(uint64))
		}
		part, err := multipart.NewReader(bytes.NewReader(data), params["boundary"]).NextPart()
		if err != nil {
			t.Fatal(err)
		}
		header := part.Header.Get("Content-Disposition")
		_, disposition, err := mime.ParseMediaType(header)
		if err != nil || disposition["name"] != name || disposition["filename"] != filename ||
			strings.Contains(header, "filename*=") || strings.Contains(header, "name*=") {
			t.Fatalf("incorrect quoted UTF-8 parameters: %q, %v", header, err)
		}
	}
}

func TestEncodedBodyLimit(t *testing.T) {
	b := Builder{Root: t.TempDir()}
	parts := map[string]any{"p": map[string]any{"value": strings.Repeat("x", 128)}}
	out, err := b.Build(parts)
	if err != nil {
		t.Fatal(err)
	}
	// Writer boundaries have a fixed length. Include all headers and the closing
	// boundary in the limit, and check both sides of the exact byte boundary.
	size := len(out["body"].([]any))
	if _, err := b.build(parts, size); err != nil {
		t.Fatalf("exact limit rejected: %v", err)
	}
	if out, err := b.build(parts, size-1); err == nil || out != nil {
		t.Fatal("closing boundary must count toward the limit")
	}
	for _, parts := range []map[string]any{
		{"p": map[string]any{"value": strings.Repeat("x", 1025)}},
		{"p": map[string]any{"contentType": "application/json", "value": map[string]any{"a": strings.Repeat("x", 1024)}}},
		{"a": map[string]any{"value": strings.Repeat("x", 400)}, "b": map[string]any{"value": strings.Repeat("x", 400)}},
	} {
		if out, err := b.build(parts, 1024); err == nil || out != nil || !strings.Contains(err.Error(), "maximum size") {
			t.Fatalf("oversized body accepted: %v", err)
		}
	}
}

func TestOversizedFileRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxBodyBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := (Builder{Root: root}).Build(map[string]any{"file": map[string]any{"file": "large.csv"}})
	if err == nil || out != nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("oversized file accepted: %v", err)
	}
}
