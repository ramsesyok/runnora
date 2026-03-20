// Package oracle は Oracle DB への接続と SQL/PL/SQL 実行機能を提供する。
//
// テスト容易性のために Executor インターフェースを定義し、
// hook パッケージや app パッケージはこのインターフェースを通じて DB 操作を行う。
// これにより、テスト時は stub/mock に差し替えることができ、実 Oracle DB が不要になる。
package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

// Executor は Oracle DB への SQL 実行を抽象化するインターフェース。
//
// このインターフェースが「テスト容易性の境界 (isolation seam)」として機能する。
// - 本番: OracleExecutor (実 Oracle DB に接続)
// - テスト: stub/mock (DB 接続不要)
type Executor interface {
	// ExecFile は指定パスの SQL/PL/SQL ファイルを読み込んで実行する。
	ExecFile(ctx context.Context, path string) error
	// ExecText は SQL テキストを直接実行する。
	ExecText(ctx context.Context, sqlText string) error
	// Close は DB 接続を閉じる。
	Close() error
}

// OracleExecutor は *sql.DB を通じて Oracle に SQL/PL/SQL を実行する。
// go-ora (sijms/go-ora/v2) の Pure Go ドライバを使うため Oracle Client が不要。
//
// PL/SQL ブロックの実行:
//   - ExecText に "BEGIN ... END;" のブロックをそのまま渡せる
//   - ファイル全体を 1 つの SQL 文として ExecContext に渡すため、
//     セミコロン終端の DDL (例: CREATE TABLE ...;) は go-ora がエラーにすることがある
//   - DDL は PL/SQL の EXECUTE IMMEDIATE でラップするか、セミコロンを省略する
type OracleExecutor struct {
	db *sql.DB
}

// NewOracleExecutor は既存の *sql.DB から OracleExecutor を生成する。
func NewOracleExecutor(db *sql.DB) *OracleExecutor {
	return &OracleExecutor{db: db}
}

// ExecText は SQL テキストを直接実行する。
// 結果セットは使用しない (INSERT/UPDATE/DELETE/DDL/PL/SQL 向け)。
func (e *OracleExecutor) ExecText(ctx context.Context, sqlText string) error {
	if _, err := e.db.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("oracle: exec: %w", err)
	}
	return nil
}

// ExecFile はファイルを読み込んで SQL を実行する。
// ファイルの内容全体を 1 つの SQL 文として実行するため、
// 複数ステートメントを含む場合は PL/SQL の BEGIN...END ブロックで包む。
func (e *OracleExecutor) ExecFile(ctx context.Context, path string) error {
	sqlText, err := readSQLFile(path)
	if err != nil {
		return err
	}
	return e.ExecText(ctx, sqlText)
}

// Close は DB 接続プールを閉じる。
// defer で呼び出すことで接続リークを防ぐ。
func (e *OracleExecutor) Close() error {
	return e.db.Close()
}

// readSQLFile はファイルを読み込んで文字列として返す。
func readSQLFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("oracle: read file %s: %w", path, err)
	}
	return string(data), nil
}
