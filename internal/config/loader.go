package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	defaultMaxOpenConns       = 10
	defaultMaxIdleConns       = 2
	defaultConnMaxLifetimeSec = 300
	defaultReportFormat       = "text"
	defaultDBRunnerName       = "db"
)

// Load は指定パスの YAML ファイルを読み込み、デフォルト値を適用して Config を返す。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return LoadWithDefaults(data)
}

// LoadWithDefaults は YAML バイト列をアンマーシャルし、デフォルト値を適用して Config を返す。
func LoadWithDefaults(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

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
// 順序: 共通 before → CLI 追加 before
func BuildBeforeFiles(cfg *Config, opts *RunOptions) []string {
	result := make([]string, 0, len(cfg.Hooks.Common.Before)+len(opts.BeforeSQLFiles))
	result = append(result, cfg.Hooks.Common.Before...)
	result = append(result, opts.BeforeSQLFiles...)
	return result
}

// BuildAfterFiles は実行順序に従って after SQL ファイルリストを組み立てる。
// 順序: CLI 追加 after → 共通 after
func BuildAfterFiles(cfg *Config, opts *RunOptions) []string {
	result := make([]string, 0, len(opts.AfterSQLFiles)+len(cfg.Hooks.Common.After))
	result = append(result, opts.AfterSQLFiles...)
	result = append(result, cfg.Hooks.Common.After...)
	return result
}
