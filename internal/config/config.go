// Package config は各コマンドが使う設定値の型を提供する。
//
// runnora.yaml 全体の読み込みと検証は internal/project が行う。
// このパッケージには、複数のパッケージから参照される設定値の型だけを置く。
//   - OracleConfig:   フック用の Oracle 接続設定 (oracle.Open が受け取る)
//   - GenerateConfig: runnora.yaml の generate セクション
//   - GenerateOptions: generate コマンドの実行オプション
package config

// OracleConfig は SQL/PLSQL フックで使う Oracle 接続設定。
// go-ora (sijms/go-ora/v2) の Pure Go ドライバを使うため Oracle Client 不要。
type OracleConfig struct {
	Driver             string // ドライバ名。現在は "oracle" のみサポート
	DSN                string // 接続文字列。例: "oracle://user:pass@host:1521/service"
	MaxOpenConns       int    // 最大オープン接続数
	MaxIdleConns       int    // 最大アイドル接続数
	ConnMaxLifetimeSec int    // 接続の最大寿命 (秒)
}

// GenerateConfig は runnora.yaml の generate セクション。
// generate コマンドのデフォルト値として使われる。
type GenerateConfig struct {
	OpenAPI        string `yaml:"openapi"`         // OpenAPI ファイルパス
	OutDir         string `yaml:"out_dir"`         // 生成物の出力基底ディレクトリ
	CaseFormat     string `yaml:"case_format"`     // case ファイル形式: "json"
	CaseStyle      string `yaml:"case_style"`      // case スタイル: "bundled"
	Mode           string `yaml:"mode"`            // 生成モード: "shallow"
	CleanGenerated bool   `yaml:"clean_generated"` // 生成前に generated/ を掃除する
	EmitManifest   bool   `yaml:"emit_manifest"`   // manifest.json を出力する
	RunnerName     string `yaml:"runner_name"`     // template runbook のランナー名
}

// GenerateOptions は CLI から generate コマンドに渡される実行オプション。
// cobra のフラグ解析と runnora.yaml のマージ後に構築する。
type GenerateOptions struct {
	ProjectPath         string   // 使った runnora.yaml のパス (なければ空)
	OpenAPIPath         string   // OpenAPI ファイルパス (--openapi)
	OutDir              string   // 出力基底ディレクトリ (--out)
	Tags                []string // フィルタ用タグ (--tags, カンマ区切り)
	OperationIDs        []string // フィルタ用 operationId (--operation-ids, カンマ区切り)
	Mode                string   // 生成モード: "shallow" (--mode)
	CaseFormat          string   // case ファイル形式 (--case-format)
	CaseStyle           string   // case スタイル (--case-style)
	Clean               bool     // generated/ を掃除する (--clean)
	Force               bool     // 既存ファイルを強制上書きする (--force)
	SkipDeprecated      bool     // deprecated な operation をスキップ (--skip-deprecated)
	Server              string   // endpoint として使う server URL (--server)
	RunnerName          string   // template runbook のランナー名 (--runner-name)
	EmitManifest        bool     // manifest.json を出力する (--emit-manifest)
	EmitResponseExample bool     // 非推奨。効果なし (--emit-response-example)
}
