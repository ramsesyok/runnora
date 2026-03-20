package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/sijms/go-ora/v2"

	"github.com/ramsesyok/runnora/internal/config"
)

// ErrEmptyDSN は DSN が空のときに返るエラー。
var ErrEmptyDSN = errors.New("oracle: DSN must not be empty")

// Open は config.DBConfig をもとに Oracle DB 接続を開く。
func Open(cfg *config.DBConfig) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, ErrEmptyDSN
	}

	db, err := sql.Open("oracle", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("oracle: open: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetimeSec > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)
	}

	return db, nil
}
