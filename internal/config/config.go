package config

// Config はアプリケーション全体の設定を保持する。
type Config struct {
	App    AppConfig    `yaml:"app"`
	DB     DBConfig     `yaml:"db"`
	Runn   RunnConfig   `yaml:"runn"`
	Hooks  HooksConfig  `yaml:"hooks"`
	Report ReportConfig `yaml:"report"`
}

// AppConfig はアプリケーション基本設定。
type AppConfig struct {
	Name string `yaml:"name"`
}

// DBConfig は Oracle DB 接続設定。
type DBConfig struct {
	Driver             string `yaml:"driver"`
	DSN                string `yaml:"dsn"`
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"`
}

// RunnConfig は runn エンジン設定。
type RunnConfig struct {
	DBRunnerName string `yaml:"db_runner_name"`
	Trace        bool   `yaml:"trace"`
}

// HooksConfig はフック設定。
type HooksConfig struct {
	Common CommonHooks `yaml:"common"`
}

// CommonHooks は共通 before/after フック設定。
type CommonHooks struct {
	Before []string `yaml:"before"`
	After  []string `yaml:"after"`
}

// ReportConfig はレポート出力設定。
type ReportConfig struct {
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// RunOptions は CLI から渡される実行オプション。
type RunOptions struct {
	ConfigPath     string
	BeforeSQLFiles []string
	AfterSQLFiles  []string
	RunbookPaths   []string
	ReportFormat   string
	ReportOutput   string
	Trace          bool
	FailFast       bool
}
