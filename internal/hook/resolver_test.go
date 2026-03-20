package hook_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/hook"
)

func TestResolver_Validate_AllFilesExist(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.sql")
	f2 := filepath.Join(dir, "b.sql")
	os.WriteFile(f1, []byte("BEGIN NULL; END;"), 0o600)
	os.WriteFile(f2, []byte("BEGIN NULL; END;"), 0o600)

	r := hook.NewResolver()
	if err := r.Validate([]string{f1, f2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolver_Validate_MissingFile_ReturnsError(t *testing.T) {
	r := hook.NewResolver()
	err := r.Validate([]string{"/nonexistent/file.sql"})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestResolver_Validate_MultipleMissingFiles_AllListedInError(t *testing.T) {
	r := hook.NewResolver()
	err := r.Validate([]string{"/missing/a.sql", "/missing/b.sql"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "a.sql") {
		t.Errorf("error should contain a.sql: %v", err)
	}
	if !strings.Contains(msg, "b.sql") {
		t.Errorf("error should contain b.sql: %v", err)
	}
}

func TestResolver_Validate_EmptyList_ReturnsNil(t *testing.T) {
	r := hook.NewResolver()
	if err := r.Validate(nil); err != nil {
		t.Fatalf("unexpected error for empty list: %v", err)
	}
}
