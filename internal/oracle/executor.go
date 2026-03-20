package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

// Executor は Oracle DB への SQL 実行インターフェース。
type Executor interface {
	ExecFile(ctx context.Context, path string) error
	ExecText(ctx context.Context, sqlText string) error
	Close() error
}

// OracleExecutor は *sql.DB を使って SQL を実行する。
type OracleExecutor struct {
	db *sql.DB
}

// NewOracleExecutor は OracleExecutor を生成する。
func NewOracleExecutor(db *sql.DB) *OracleExecutor {
	return &OracleExecutor{db: db}
}

// ExecText は SQL テキストを直接実行する。
func (e *OracleExecutor) ExecText(ctx context.Context, sqlText string) error {
	if _, err := e.db.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("oracle: exec: %w", err)
	}
	return nil
}

// ExecFile はファイルを読み込んで SQL を実行する。
func (e *OracleExecutor) ExecFile(ctx context.Context, path string) error {
	sqlText, err := readSQLFile(path)
	if err != nil {
		return err
	}
	return e.ExecText(ctx, sqlText)
}

// Close は DB 接続を閉じる。
func (e *OracleExecutor) Close() error {
	return e.db.Close()
}

func readSQLFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("oracle: read file %s: %w", path, err)
	}
	return string(data), nil
}
