package reporter

import (
	"fmt"
	"io"
	"os"
)

// Reporter は集計結果を指定された形式・出力先へ書き込む。
type Reporter interface {
	Write(*Report) error
	Close() error
}

// NewReporter は text、json、junit のいずれかを writer に出力する。
func NewReporter(format string, w io.Writer) (Reporter, error) {
	format, err := normalizeFormat(format)
	if err != nil {
		return nil, err
	}
	switch format {
	case "text":
		return NewTextReporter(w), nil
	case "json":
		return NewJSONReporter(w), nil
	case "junit":
		return NewJUnitReporter(w), nil
	default:
		panic("unreachable report format")
	}
}

func normalizeFormat(format string) (string, error) {
	if format == "" {
		return "text", nil
	}
	switch format {
	case "text", "json", "junit":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported report format %q (supported: text, json, junit)", format)
	}
}

type fileReporter struct {
	f        *os.File
	delegate Reporter
}

// NewFileReporter は形式を検証してから出力ファイルを作成する。
func NewFileReporter(format, path string) (Reporter, error) {
	format, err := normalizeFormat(format)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("reporter: create %s: %w", path, err)
	}
	delegate, err := NewReporter(format, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &fileReporter{f: f, delegate: delegate}, nil
}

func (r *fileReporter) Write(rep *Report) error { return r.delegate.Write(rep) }
func (r *fileReporter) Close() error            { return r.f.Close() }
