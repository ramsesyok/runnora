package oracle_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ramsesyok/runnora/internal/oracle"
)

// compile-time: stubExecutor が Executor interface を満たすことを検証
type stubExecutor struct{}

func (s *stubExecutor) ExecFile(ctx context.Context, path string) error { return nil }
func (s *stubExecutor) ExecText(ctx context.Context, sql string) error  { return nil }
func (s *stubExecutor) Close() error                                     { return nil }

var _ oracle.Executor = (*stubExecutor)(nil)

func TestExecutorInterface_CanBeImplementedByStub(t *testing.T) {
	t.Log("stubExecutor satisfies oracle.Executor interface")
}

func TestOracleExecutor_ExecText_CallsExecContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("BEGIN NULL; END;").WillReturnResult(sqlmock.NewResult(0, 0))

	exec := oracle.NewOracleExecutor(db)
	if err := exec.ExecText(context.Background(), "BEGIN NULL; END;"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestOracleExecutor_ExecText_DBError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INVALID").WillReturnError(errors.New("ORA-00900: invalid SQL"))

	exec := oracle.NewOracleExecutor(db)
	if err := exec.ExecText(context.Background(), "INVALID"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOracleExecutor_ExecFile_ReadsFileAndExecutes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	sqlFile := filepath.Join(dir, "test.sql")
	sqlContent := "BEGIN NULL; END;"
	if err := os.WriteFile(sqlFile, []byte(sqlContent), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mock.ExpectExec("BEGIN NULL").WillReturnResult(driver.ResultNoRows)

	exec := oracle.NewOracleExecutor(db)
	if err := exec.ExecFile(context.Background(), sqlFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOracleExecutor_ExecFile_MissingFile_ReturnsError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	exec := oracle.NewOracleExecutor(db)
	if err := exec.ExecFile(context.Background(), "/nonexistent/file.sql"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
