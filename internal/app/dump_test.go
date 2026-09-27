package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
)

func TestDumpWarning(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{path: filepath.Join("runbooks", "scenarios", "a.yml"), want: "runnora-migrate で削除できます"},
		{path: filepath.Join("runbooks", "generated", "books", "get.template.yml"), want: "runnora generate で作り直してください"},
		{path: "generated/a.yml", want: "runnora generate で作り直してください"},
	}
	for _, tt := range tests {
		if got := app.DumpWarning(tt.path); !strings.Contains(got, tt.want) {
			t.Errorf("DumpWarning(%q) = %q, want containing %q", tt.path, got, tt.want)
		}
	}
}
