// Package multipartbody builds typed multipart bodies without changing runn.
package multipartbody

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const FuncName = "multipart"

// MaxBodyBytes bounds the complete encoded body, including headers and boundary.
// The numeric representation required by runn costs substantially more memory.
const MaxBodyBytes = 4 << 20

// Builder resolves local file paths against the runnora project root.
type Builder struct{ Root string }

// Build returns body and contentType for use in a bind step. Body is a numeric
// byte list so YAML expansion preserves binary data without UTF-8 conversion.
func (b Builder) Build(parts map[string]any) (map[string]any, error) {
	return b.build(parts, MaxBodyBytes)
}

func (b Builder) build(parts map[string]any, limit int) (map[string]any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("multipart: at least one part is required")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&boundedBuffer{buf: &buf, limit: limit})
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" || !validParameter(name) {
			return nil, fmt.Errorf("multipart: invalid part name")
		}
		p, ok := parts[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("multipart %q: expected a descriptor with value or file", name)
		}
		data, filename, ct, err := b.part(p, limit-buf.Len())
		if err != nil {
			return nil, fmt.Errorf("multipart %q: %w", name, err)
		}
		// RFC 7578 forbids filename*. Use quoted UTF-8 parameters, as browsers
		// and curl do, escaping quotes and backslashes without RFC 2231 encoding.
		disposition := "form-data; name=" + quoteParameter(name)
		if filename != "" {
			disposition += "; filename=" + quoteParameter(filename)
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", disposition)
		h.Set("Content-Type", ct)
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	// runn's expression/YAML expansion handles ordinary numeric lists, while
	// []byte has special YAML encoding. Its raw encoder accepts uint64 values.
	body := make([]any, buf.Len())
	for i, v := range buf.Bytes() {
		body[i] = uint64(v)
	}
	return map[string]any{"body": body, "contentType": w.FormDataContentType()}, nil
}

func (b Builder) part(p map[string]any, remaining int) ([]byte, string, string, error) {
	for k := range p {
		switch k {
		case "value", "file", "contentType", "filename":
		default:
			return nil, "", "", fmt.Errorf("unknown property %q", k)
		}
	}
	value, hasValue := p["value"]
	file, hasFile := p["file"]
	if hasValue == hasFile {
		return nil, "", "", fmt.Errorf("specify exactly one of value or file")
	}
	ct, err := optionalString(p, "contentType")
	if err != nil {
		return nil, "", "", err
	}
	filename, err := optionalString(p, "filename")
	if err != nil {
		return nil, "", "", err
	}
	if !validParameter(filename) {
		return nil, "", "", fmt.Errorf("invalid filename")
	}
	var mediaType string
	if ct != "" {
		if strings.ContainsAny(ct, "\r\n") {
			return nil, "", "", fmt.Errorf("invalid contentType")
		}
		mediaType, _, err = mime.ParseMediaType(ct)
		if err != nil || !strings.Contains(mediaType, "/") {
			return nil, "", "", fmt.Errorf("invalid contentType %q", ct)
		}
	}
	if hasFile {
		path, ok := file.(string)
		if !ok || path == "" {
			return nil, "", "", fmt.Errorf("file must be a nonempty local path")
		}
		path = strings.TrimPrefix(path, "file://")
		if strings.Contains(path, "://") {
			return nil, "", "", fmt.Errorf("file must be a local path")
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(b.Root, filepath.FromSlash(path))
		}
		if filename == "" {
			filename = filepath.Base(path)
		}
		if !validParameter(filename) {
			return nil, "", "", fmt.Errorf("invalid filename")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, "", "", fmt.Errorf("read file: %w", err)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil, "", "", fmt.Errorf("stat file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, "", "", fmt.Errorf("file must be a regular local file")
		}
		if info.Size() > int64(remaining) {
			return nil, "", "", bodyTooLarge()
		}
		// LimitReader also bounds a file that grows after Stat.
		data, err := io.ReadAll(io.LimitReader(f, int64(remaining)+1))
		if err != nil {
			return nil, "", "", fmt.Errorf("read file: %w", err)
		}
		if len(data) > remaining {
			return nil, "", "", bodyTooLarge()
		}
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		return data, filename, ct, nil
	}
	if filename != "" {
		return nil, "", "", fmt.Errorf("filename requires file")
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		data, err := json.Marshal(value)
		if err == nil && len(data) > remaining {
			return nil, "", "", bodyTooLarge()
		}
		return data, "", ct, err
	}
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}
	switch v := value.(type) {
	case string:
		if len(v) > remaining {
			return nil, "", "", bodyTooLarge()
		}
		return []byte(v), "", ct, nil
	case bool, int, int64, uint64, float64:
		return []byte(fmt.Sprint(v)), "", ct, nil
	default:
		return nil, "", "", fmt.Errorf("structured value requires application/json or a +json contentType")
	}
}

func validParameter(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func quoteParameter(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func bodyTooLarge() error {
	return fmt.Errorf("multipart: encoded body exceeds maximum size of %d bytes (4 MiB, including headers and boundary)", MaxBodyBytes)
}

type boundedBuffer struct {
	buf   *bytes.Buffer
	limit int
}

func (w *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.buf.Len() {
		return 0, bodyTooLarge()
	}
	return w.buf.Write(p)
}

func optionalString(p map[string]any, key string) (string, error) {
	v, present := p[key]
	if !present {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("%s must be a nonempty string", key)
	}
	return s, nil
}
