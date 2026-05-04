// このファイルは Oracle DB への接続確立を担当する。
//
// ドライバの登録:
//   - _ "github.com/sijms/go-ora/v2" の blank import によって、
//     go-ora ドライバが database/sql に "oracle" という名前で自動登録される。
//   - これにより sql.Open("oracle", dsn) でドライバを指定できる。
//   - go-ora は Pure Go 実装なので Oracle Instant Client が不要。
//
// DSN フォーマット例:
//
//	oracle://user:pass@host:1521/service
//	oracle://user:pass%21@host:1521/FREEPDB1  // パスワードに '!' を含む場合は URL エンコード
//
// 注意: go-ora の DSN はテンプレート展開されない。
// runn.Load 時に store が nil のため、{{ env.DB_DSN }} のようなテンプレートは
// 展開されず文字列そのままになる。設定ファイルには必ずリテラル DSN を記述すること。
package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/sijms/go-ora/v2" // "oracle" ドライバを database/sql に登録する副作用インポート

	"github.com/ramsesyok/runnora/internal/config"
)

// ErrEmptyDSN は DSN が空のときに返るエラー。
// errors.Is(err, oracle.ErrEmptyDSN) で検査できる。
var ErrEmptyDSN = errors.New("oracle: DSN must not be empty")

// Open は config.OracleConfig をもとに Oracle DB への接続プールを開く。
//
// 処理の流れ:
//  1. DSN の空チェック (ErrEmptyDSN を返す)
//  2. sql.Open("oracle", dsn) で接続プールを初期化
//     ※ この時点では実際に接続を試みない (lazy connect)
//  3. 接続プールのパラメータを設定
//
// 接続プール設定:
//   - MaxOpenConns: 同時に開くことができる最大接続数。サーバー側のセッション数に合わせて設定。
//   - MaxIdleConns: アイドル状態で保持する接続数。頻繁な接続/切断を防ぎパフォーマンスを向上。
//   - ConnMaxLifetime: 接続を再利用できる最大時間。Oracle 側の接続タイムアウトより短く設定推奨。
//
// 返り値の *sql.DB は defer exec.Close() で必ず閉じること。
func Open(cfg *config.OracleConfig) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, ErrEmptyDSN
	}

	// sql.Open は接続プールの設定のみで、実際の接続は最初のクエリ時に確立される (lazy connect)。
	// そのため、この時点ではネットワークエラーは発生しない。
	db, err := sql.Open("oracle", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("oracle: open: %w", err)
	}

	// 接続プールのパラメータを設定する。
	// cfg の値が 0 の場合は config.applyDefaults でデフォルト値が適用されているため、
	// ここでは > 0 チェックで十分 (明示的に 0 を指定したケースでも安全)。
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetimeSec > 0 {
		// int (秒) を time.Duration (ナノ秒) に変換して設定する。
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)
	}

	return db, nil
}
