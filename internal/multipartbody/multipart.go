// Package multipartbody builds typed multipart bodies without changing runn.
package multipartbody

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const FuncName = "multipart"

// Builder resolves local file paths against the runnora project root.
type Builder struct{ Root string }

// Build returns body and contentType for use in a bind step. Body is a numeric
// byte list so YAML expansion preserves binary data without UTF-8 conversion.
func (b Builder) Build(parts map[string]any) (map[string]any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("multipart: at least one part is required")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" || strings.ContainsAny(name, "\r\n") {
			return nil, fmt.Errorf("multipart: invalid part name")
		}
		p, ok := parts[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("multipart %q: expected a descriptor with value or file", name)
		}
		data, filename, ct, err := b.part(p)
		if err != nil {
			return nil, fmt.Errorf("multipart %q: %w", name, err)
		}
		disposition := map[string]string{"name": name}
		if filename != "" {
			disposition["filename"] = filename
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", mime.FormatMediaType("form-data", disposition))
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

func (b Builder) part(p map[string]any) ([]byte, string, string, error) {
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
	if strings.ContainsAny(filename, "\r\n") {
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
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", "", fmt.Errorf("read file: %w", err)
		}
		if filename == "" {
			filename = filepath.Base(path)
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
		return data, "", ct, err
	}
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}
	switch v := value.(type) {
	case string:
		return []byte(v), "", ct, nil
	case bool, int, int64, uint64, float64:
		return []byte(fmt.Sprint(v)), "", ct, nil
	default:
		return nil, "", "", fmt.Errorf("structured value requires application/json or a +json contentType")
	}
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
