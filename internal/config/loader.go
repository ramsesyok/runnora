package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// デフォルト値定数。YAML に値が書かれていない (ゼロ値の) フィールドに適用される。
const (
	defaultMaxOpenConns       = 10     // DB 最大オープン接続数
	defaultMaxIdleConns       = 2      // DB 最大アイドル接続数
	defaultConnMaxLifetimeSec = 300    // DB 接続最大寿命 (秒)
	defaultReportFormat       = "text" // レポート出力形式
	defaultDBRunnerName       = "db"   // runbook 内の DB ランナー名
)

// Load は指定パスの YAML ファイルを読み込み、デフォルト値を適用して Config を返す。
// ファイルが存在しない・読み込めない場合はエラーを返す。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return LoadWithDefaults(data)
}

// LoadWithDefaults は YAML バイト列をアンマーシャルし、デフォルト値を適用して Config を返す。
// テストから直接 YAML 文字列を渡す場合にも使用できる。
func LoadWithDefaults(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// applyDefaults はゼロ値のフィールドにデフォルト値を設定する。
// YAML で明示的に 0 を書いた場合もデフォルト値に置き換わるため注意。
func applyDefaults(cfg *Config) {
	if cfg.DB.MaxOpenConns == 0 {
		cfg.DB.MaxOpenConns = defaultMaxOpenConns
	}
	if cfg.DB.MaxIdleConns == 0 {
		cfg.DB.MaxIdleConns = defaultMaxIdleConns
	}
	if cfg.DB.ConnMaxLifetimeSec == 0 {
		cfg.DB.ConnMaxLifetimeSec = defaultConnMaxLifetimeSec
	}
	if cfg.Report.Format == "" {
		cfg.Report.Format = defaultReportFormat
	}
	if cfg.Runn.DBRunnerName == "" {
		cfg.Runn.DBRunnerName = defaultDBRunnerName
	}
}

// BuildBeforeFiles は実行順序に従って before SQL ファイルリストを組み立てる。
//
// 順序: 共通 before (config) → CLI 追加 before (--before-sql)
//
// 共通設定を先に実行することで「全テスト共通のセットアップ → テスト固有のセットアップ」
// という自然な順序になる。
func BuildBeforeFiles(cfg *Config, opts *RunOptions) []string {
	result := make([]string, 0, len(cfg.Hooks.Common.Before)+len(opts.BeforeSQLFiles))
	result = append(result, cfg.Hooks.Common.Before...)
	result = append(result, opts.BeforeSQLFiles...)
	return result
}

// BuildAfterFiles は実行順序に従って after SQL ファイルリストを組み立てる。
//
// 順序: CLI 追加 after (--after-sql) → 共通 after (config)
//
// テスト固有のクリーンアップを先に実行し、その後に共通クリーンアップを行う。
// これにより、テスト固有リソースの解放後に共通の状態リセットが走る。
func BuildAfterFiles(cfg *Config, opts *RunOptions) []string {
	result := make([]string, 0, len(opts.AfterSQLFiles)+len(cfg.Hooks.Common.After))
	result = append(result, opts.AfterSQLFiles...)
	result = append(result, cfg.Hooks.Common.After...)
	return result
}
