// Package config はアプリケーション設定の構造体定義とデフォルト値適用を提供する。
//
// 設定ファイル (YAML) の構造例:
//
//	app:
//	  name: runnora
//	db:
//	  driver: oracle
//	  dsn: "oracle://user:pass@host:1521/service"
//	hooks:
//	  common:
//	    before: ["./sql/before.sql"]
//	    after:  ["./sql/after.sql"]
//	report:
//	  format: "text"
package config

// Config はアプリケーション全体の設定を保持する。
// yaml タグにより YAML ファイルのキー名と対応している。
type Config struct {
	App    AppConfig    `yaml:"app"`
	DB     DBConfig     `yaml:"db"`
	Runn   RunnConfig   `yaml:"runn"`
	Hooks  HooksConfig  `yaml:"hooks"`
	Report ReportConfig `yaml:"report"`
}

// AppConfig はアプリケーション基本設定。
type AppConfig struct {
	Name string `yaml:"name"` // アプリケーション名 (ログ・レポートに使用)
}

// DBConfig は Oracle DB 接続設定。
// go-ora (sijms/go-ora/v2) の Pure Go ドライバを使うため Oracle Client 不要。
type DBConfig struct {
	Driver             string `yaml:"driver"`                // ドライバ名。現在は "oracle" のみサポート
	DSN                string `yaml:"dsn"`                   // 接続文字列。例: "oracle://user:pass@host:1521/service"
	MaxOpenConns       int    `yaml:"max_open_conns"`        // 最大オープン接続数。0 の場合はデフォルト (10) が適用される
	MaxIdleConns       int    `yaml:"max_idle_conns"`        // 最大アイドル接続数。0 の場合はデフォルト (2) が適用される
	ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"` // 接続の最大寿命 (秒)。0 の場合はデフォルト (300) が適用される
}

// RunnConfig は runn エンジン固有の設定。
type RunnConfig struct {
	DBRunnerName string `yaml:"db_runner_name"` // runbook 内での DB ランナー名。デフォルト: "db"
	Trace        bool   `yaml:"trace"`          // true にすると runn のトレースログを出力する
}

// HooksConfig はフック設定のコンテナ。
// 将来的にテスト固有フックなどを追加できるよう入れ子構造にしている。
type HooksConfig struct {
	Common CommonHooks `yaml:"common"` // 全 runbook に共通して適用されるフック
}

// CommonHooks は共通 before/after フックで実行する SQL ファイルリスト。
//
// 実行順序:
//
//	Before: config の common.before → CLI --before-sql
//	After:  CLI --after-sql         → config の common.after
//
// この順序により、共通のセットアップ後にテスト固有のセットアップを追加でき、
// テスト固有のクリーンアップ後に共通クリーンアップが走るようになっている。
type CommonHooks struct {
	Before []string `yaml:"before"` // runbook 実行前に必ず実行する SQL/PL/SQL ファイルパスのリスト
	After  []string `yaml:"after"`  // runbook 実行後に必ず実行する SQL/PL/SQL ファイルパスのリスト
}

// ReportConfig はレポート出力設定。
type ReportConfig struct {
	Format string `yaml:"format"` // 出力形式: "text" | "json" | "junit"。デフォルト: "text"
	Output string `yaml:"output"` // ファイル出力先。空の場合は標準出力
}

// RunOptions は CLI から run コマンドに渡される実行オプション。
// cobra のフラグ解析後に構築し、app.Runner.Run に渡す。
type RunOptions struct {
	ConfigPath     string   // 設定ファイルパス (--config)
	BeforeSQLFiles []string // 実行前 SQL ファイル (--before-sql)
	AfterSQLFiles  []string // 実行後 SQL ファイル (--after-sql)
	RunbookPaths   []string // 実行する runbook のパスリスト (位置引数)
	ReportFormat   string   // レポート形式 (--report-format)
	ReportOutput   string   // レポートファイルパス (--report-out)
	Trace          bool     // トレースモード有効フラグ (--trace)
	FailFast       bool     // 最初の失敗で停止するフラグ (--fail-fast)
}
