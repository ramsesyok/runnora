# runnora 基本設計書

## 1. 文書目的

本書は、WebAPI および gRPC のシナリオテストツール `runnora` の基本設計を定義するものである。
`runnora` は `runn` を Go パッケージとして組み込み、HTTP リクエスト、gRPC リクエスト、DB クエリを runbook で実行しつつ、Oracle 向けの共通 PL/SQL と実行時追加 PL/SQL を runbook の前後で実行することを目的とする。`runn` は Go パッケージ／CLI の両方として提供され、HTTP, gRPC, DB query を扱える。`BeforeFunc`、`AfterFunc`、`DBRunner` も公開されている。 ([Go Packages][1])

## 2. 設計方針

### 2.1 基本方針

`runnora` の基本方針は以下とする。

* テスト実行エンジンは `runn`
* Oracle 接続は `go-ora/v2`
* CLI 基盤は `spf13/cobra`
* 雛形生成は `cobra-cli`
* 前処理・後処理の PL/SQL 実行は `runn.BeforeFunc` / `runn.AfterFunc`
* runbook 内で DB 検証が必要な場合のみ `runn.DBRunner("db", db)` を利用

`go-ora` は pure Go の Oracle クライアントで、v2 は `github.com/sijms/go-ora/v2` を import して `database/sql` 経由で `sql.Open("oracle", ...)` により接続できる。Oracle の解説記事でも、go-ora は Oracle Client ライブラリ不要の pure driver として説明されている。 ([GitHub][2])

### 2.2 ツール名

コマンド名は `runnora` とする。
CLI の基本構文は以下とする。

```text
runnora [command] [options]
```

主コマンドは `run` とする。

```text
runnora run [options] <runbook...>
```

### 2.3 Cobra 採用方針

CLI 構成には `spf13/cobra` を採用する。Cobra はサブコマンドベースの CLI、短縮／長形式のフラグ、自動 help、サジェスト、シェル補完、man page 生成などを提供する。`cobra-cli` は Go module 内で `cobra-cli init` により初期構成を生成でき、標準的には `main.go` と `cmd/root.go` を作成する。 ([GitHub][3])

本設計では、`cobra-cli` で基礎構成を生成し、その上に `run` コマンド、将来用の `list` コマンド、`version` コマンドを追加する。

## 3. 適用範囲

### 3.1 対象

* HTTP API テスト
* gRPC テスト
* runbook 実行前の共通 PL/SQL 実行
* runbook 実行前の追加 PL/SQL 実行
* runbook 実行後の追加 PL/SQL 実行
* runbook 実行後の共通 PL/SQL 実行
* Oracle DB への接続
* CLI 実行
* CI 実行

### 3.2 対象外

* SQL Plus 依存の実行方式
* GUI
* DB マイグレーション管理
* Oracle の高度な型変換や複雑な OUT パラメータ処理の網羅
* runbook 文法そのものの改変

## 4. 要件整理

### 4.1 機能要件

1. runbook により HTTP / gRPC シナリオを実行できること
2. 共通 PL/SQL を設定ファイルで定義できること
3. 実行時追加の PL/SQL ファイルをコマンドライン引数で指定できること
4. 前処理と後処理の両方に SQL / PL/SQL を指定できること
5. Oracle DB 接続情報を設定ファイルで定義できること
6. 1 回の実行で 1 個以上の runbook を指定できること
7. runbook 内で DB 検証 step を使えること
8. 標準出力とファイルへの結果出力ができること
9. CI で非対話実行できること

### 4.2 非機能要件

1. SQL Plus 未導入環境で動作できること
2. Go バイナリとして配布しやすいこと
3. Linux / Windows で実行しやすいこと
4. ログに機密情報を出力しないこと
5. エラー箇所を特定しやすいこと
6. 将来、機能拡張しやすいこと

## 5. 全体構成

### 5.1 システム構成

```text
+-----------------------------+
| runnora CLI                 |
|  - root command             |
|  - run command              |
+-------------+---------------+
              |
              v
+-----------------------------+
| Application Layer           |
|  - config loader            |
|  - option builder           |
|  - execution controller     |
|  - result reporter          |
+------+------+---------------+
       |      |
       |      +------------------------------+
       |                                     |
       v                                     v
+--------------+                    +----------------------+
| runn Engine  |                    | Oracle Hook Executor |
| - HTTP       |                    | - common before      |
| - gRPC       |                    | - cli before         |
| - db runner  |                    | - cli after          |
|              |                    | - common after       |
+--------------+                    +----------------------+
       |                                     |
       +-------------------+-----------------+
                           |
                           v
                       Oracle DB
```

### 5.2 コンポーネント一覧

#### A. CLI 層

* Cobra によるコマンド定義
* フラグ解析
* 引数解析
* help / usage 表示
* 実行開始

#### B. Config Loader

* YAML 設定ファイルの読込
* デフォルト値適用
* CLI 引数による追加指定の取り込み
* 実行構成オブジェクトの生成

#### C. Execution Controller

* Oracle 接続生成
* `runn.Option` 構築
* `BeforeFunc` / `AfterFunc` 登録
* `DBRunner` 登録
* runbook 実行
* 結果集約

#### D. Oracle Hook Executor

* 共通 before SQL / PL/SQL 実行
* CLI 追加 before SQL / PL/SQL 実行
* CLI 追加 after SQL / PL/SQL 実行
* 共通 after SQL / PL/SQL 実行
* ログ出力
* エラー返却

#### E. Reporter

* 実行結果の要約表示
* 失敗箇所表示
* JSON / JUnit XML 出力

## 6. 実行方式

### 6.1 実行コマンド

標準的な実行は以下とする。

```bash
runnora run \
  --config ./config.yaml \
  --before-sql ./sql/tmp/setup_case_001.sql \
  --after-sql  ./sql/tmp/cleanup_case_001.sql \
  hello_world.yml
```

複数 runbook を指定する場合は以下とする。

```bash
runnora run \
  --config ./config.yaml \
  hello_world.yml \
  user_create.yml
```

### 6.2 コマンド意味

* `--config`: 設定ファイル
* `--before-sql`: 実行時追加の事前 SQL / PL/SQL ファイル。複数指定可
* `--after-sql`: 実行時追加の事後 SQL / PL/SQL ファイル。複数指定可
* `<runbook...>`: 実行対象 runbook。1 個以上指定可能

### 6.3 実行シーケンス

1. `runnora run` が起動される
2. Cobra が CLI 引数を解析する
3. 設定ファイルを読み込む
4. Oracle 接続を生成する
5. runbook 一覧を確定する
6. `runn.Option` を生成する
7. `BeforeFunc` を登録する
8. 必要なら `DBRunner("db", db)` を登録する
9. `AfterFunc` を登録する
10. `runn.Load(...).RunN(ctx)` または `runn.New(...).Run(ctx)` を実行する
11. 結果を集約して終了コードを返す

`runn` のパッケージ API では、`Load(..., opts...)` と `RunN(ctx)`、単一 runbook 向けには `New(opts...)` と `Run(ctx)` が用意されている。`DBRunner(name, client Querier)`、`BeforeFunc(fn)`、`AfterFunc(fn)` も公開されている。 ([Go Packages][1])

## 7. PL/SQL フック設計

### 7.1 フックの基本思想

PL/SQL は runbook に直接書かず、`runnora` 側で管理する。
前処理・後処理は Go から `BeforeFunc` / `AfterFunc` として登録し、runbook 実行の外側で制御する。`BeforeFunc` は runbook 実行前、`AfterFunc` は runbook 実行後に実行される。`AfterFuncIf` も存在するが、本設計の基本は `AfterFunc` で統一する。 ([Go Packages][1])

### 7.2 実行順序

事前処理の実行順序は以下とする。

1. 設定ファイルの共通 before
2. コマンドライン引数の `--before-sql`

事後処理の実行順序は以下とする。

1. コマンドライン引数の `--after-sql`
2. 設定ファイルの共通 after

この順序にすることで、共通初期化のあとにケース固有初期化を追加し、cleanup は逆順で片付けられる。

### 7.3 フック単位

PL/SQL は原則として「1 ファイル = 1 実行単位」とする。
匿名ブロック中の `;` を安全に扱うため、文単位の自動分割は行わない。

### 7.4 エラー時方針

* 共通 before 失敗: runbook 実行を開始しない
* 追加 before 失敗: runbook 実行を開始しない
* runbook 失敗: 失敗として記録する
* 追加 after 失敗: cleanup 失敗として記録する
* 共通 after 失敗: cleanup 失敗として記録する

runbook 実行結果は `RunResult` に格納され、`ID`、`Desc`、`Path`、`Err`、`StepResults` などを参照できる。 ([Go Packages][1])

## 8. CLI 設計

### 8.1 コマンド構成

```text
runnora
 ├─ run
 ├─ list        (alias: ls)
 ├─ coverage
 ├─ loadt       (alias: loadtest)
 ├─ new         (alias: append)
 ├─ rprof       (alias: prof, rrprof, rrrprof)
 └─ version
```

### 8.2 Cobra ベース構成

`cobra-cli init` でベースを作成し、その後 `run` コマンドを追加する。
Cobra はコマンド、引数、フラグを中心に CLI を構築する設計で、実装時は `RunE` を使うとエラー返却を自然に扱いやすい。 ([GitHub][4])

### 8.3 `run` コマンドの仕様

#### Use

```text
run [options] <runbook...>
```

#### フラグ

* `--config string`
* `--before-sql stringArray`
* `--after-sql stringArray`
* `--report-format string`
* `--report-out string`
* `--trace`
* `--fail-fast`

#### 引数

* runbook パスを 1 個以上
* ファイルパス指定を基本とする
* 将来 glob 対応は拡張項目とする

### 8.4 `version` コマンド

* ツール名
* バージョン
* ビルド日時
* Git commit

### 8.5 `list` コマンドの仕様

#### Use

```text
list [PATH_PATTERN ...] (alias: ls)
```

#### フラグ

* `--long` / `-l`: フル ID (44 文字) とパスを表示する
* `--format string`: 出力形式 (`json`)

#### 動作

`runn.LoadOnly()` でロードのみを行い、実際には runbook を実行しない。
`runn.Scopes("read:parent")` で親ディレクトリへのファイルアクセスを許可する。
`op.SelectedOperators()` で runbook の一覧を取得し、ID・説明・ラベル・if 条件・ステップ数・パスを表示する。

通常表示では ID を先頭 8 文字に短縮する。`--long` で 44 文字のフル ID を表示する。

### 8.6 `coverage` コマンドの仕様

#### Use

```text
coverage [PATH_PATTERN ...]
```

#### フラグ

* `--long` / `-l`: エンドポイントごとのアクセス回数を詳細表示する
* `--format string`: 出力形式 (`json`)

#### 動作

runbook が参照している OpenAPI 3 スペックや gRPC サービス定義のエンドポイントカバレッジを計測する。
`runn.LoadOnly()` と `runn.Scopes("read:parent")` でロードし、`op.CollectCoverage(ctx)` でカバレッジを集計する。
通常表示では `covered: N / M (XX.X%)` を表示する。`--long` で各エンドポイントとアクセス回数を表示する。

### 8.7 `loadt` コマンドの仕様

#### Use

```text
loadt [PATH_PATTERN ...] (alias: loadtest)
```

#### フラグ

* `--load-concurrent int`: 同時実行ゴルーチン数（デフォルト: 1）
* `--duration string`: 負荷テスト時間（デフォルト: `10s`）
* `--warm-up string`: ウォームアップ時間（デフォルト: `5s`）
* `--max-rps int`: 最大 RPS（デフォルト: 1）
* `--threshold string`: 合否判定式（例: `"error_rate < 0.01"`）
* `--format string`: 出力形式 (`json`)

#### 動作

`otchkiss` ライブラリを使って指定の同時実行数・RPS で runbook を繰り返し実行する。
`donegroup.WithCancel` でキャンセル可能なコンテキストを管理し、`ot.Start(ctx)` で負荷テストを開始する。
`runn.NewLoadtResult` で集計し、`--threshold` が指定されている場合は `lr.CheckThreshold(threshold)` で合否を判定する。
`time.ParseDuration` で解析できない `"5sec"` 形式もフォールバックで対応する。

#### 実行フロー

1. `donegroup.WithCancel` でキャンセル可能なコンテキストを生成
2. `runn.Load` で runbook をロード
3. `setting.New` で並行数・RPS・時間設定を構築
4. `otchkiss.FromConfig` で負荷テストエンジンを初期化（リクエスト上限: 100,000,000 で事実上無制限）
5. `ot.Start(ctx)` で負荷テストを実行（duration + warm-up の間）
6. 結果を集計し threshold チェックを行う

### 8.8 `new` コマンドの仕様

#### Use

```text
new [STEP_COMMAND ...] (alias: append)
```

#### フラグ

* `--desc string`: runbook の説明
* `--out string`: 出力先ファイルパス（省略時は標準出力）
* `--and-run`: 作成後すぐに実行する（`--out` が必要）

#### 動作

コマンドライン引数からステップを生成して runbook を新規作成、または既存 runbook に追記する。
出力先ファイルが既存の場合は `runn.ParseRunbook` で読み込んで追記モードになる。
位置引数を `rb.AppendStep(args...)` に渡すことで、HTTP リクエストなどのステップを自動生成する。
`--and-run` 指定時は作成した runbook を `runn.Load` → `op.RunN(ctx)` で即時実行する。
ファイルのパーミッションは `0600`（オーナーのみ読み書き）とする。

#### 使用例

```bash
# 新規作成して stdout に出力
runnora new GET https://example.com/health

# ファイルに保存
runnora new --out ./runbooks/foo.yml GET https://example.com/health

# 既存 runbook に追記
runnora append --out ./runbooks/foo.yml POST https://example.com/users

# 作成後すぐに実行
runnora new --out /tmp/smoke.yml --and-run GET https://example.com/health
```

### 8.9 `rprof` コマンドの仕様

#### Use

```text
rprof [PROFILE_PATH] (alias: prof, rrprof, rrrprof)
```

#### フラグ

* `--depth int`: スパンツリーの展開深度（デフォルト: 4）
* `--unit string`: 経過時間の表示単位 `ns|us|ms|s|m`（デフォルト: `ms`）
* `--sort string`: ソート順 `elapsed|started-at|stopped-at`（デフォルト: `elapsed`）

#### 動作

`runn` が `stopw` ライブラリで記録した実行プロファイル（JSON）を読み込み、テーブル形式で表示する。
`stopw.Span` の木構造を深さ優先で走査し、`--depth` までのスパンをフラットなリストに変換する。
`Span.Repair()` で欠損した時刻情報を補完する。

ソート動作:
* `elapsed`: 経過時間の降順（最も遅いスパンが先頭）
* `started-at`: 開始時刻の昇順（実行順序）
* `stopped-at`: 終了時刻の昇順（終了順序）

表示単位と精度:
* `ns`: ナノ秒（整数）
* `us`: マイクロ秒（小数点 3 桁）
* `ms`: ミリ秒（小数点 3 桁）
* `s`: 秒（小数点 3 桁）
* `m`: 分（小数点 4 桁）

### 8.10 サンプル Cobra 構造

```text
main.go
cmd/
  root.go
  run.go
  list.go
  coverage.go
  loadt.go
  new.go
  rprof.go
  version.go
internal/
  config/
  app/
  hook/
  oracle/
  reporter/
```

## 9. 設定ファイル設計

### 9.1 基本方針

設定ファイルは YAML とする。
設定ファイルでは共通の DB 接続設定、共通 before、共通 after、レポート設定を管理する。
CLI 引数は追加フックとして扱い、設定ファイルの値を上書きせず追加する。

### 9.2 設定ファイル例

```yaml
app:
  name: runnora

db:
  driver: oracle
  dsn: "oracle://user:pass@host:1521/service"
  max_open_conns: 5
  max_idle_conns: 2
  conn_max_lifetime_sec: 300

runn:
  db_runner_name: "db"
  trace: false

hooks:
  common:
    before:
      - "./sql/common/session_init.sql"
      - "./sql/common/master_seed.sql"
    after:
      - "./sql/common/session_cleanup.sql"

report:
  format: "text"
  output: ""
```

### 9.3 マージ規則

最終実行構成は以下で組み立てる。

* 設定ファイルを基準とする
* `--before-sql` は `hooks.common.before` の後ろに追加
* `--after-sql` は `hooks.common.after` の前に追加
* report 設定は CLI が指定された場合のみ上書き
* runbook は CLI 位置引数でのみ指定

## 10. Oracle 接続設計

### 10.1 採用ドライバ

Oracle 接続ドライバは `github.com/sijms/go-ora/v2` を採用する。
`go-ora` の README では v2 の import が案内されており、Oracle 10.2 以上では v2 が推奨されている。`database/sql` を通して `sql.Open("oracle", connStr)` で利用できる。 ([GitHub][2])

### 10.2 接続方針

* 1 プロセスで 1 つの `*sql.DB` を共有
* `Ping` により接続確認
* `SetMaxOpenConns` 等を設定ファイルから反映
* DSN は設定ファイルまたは環境変数で与える

### 10.3 SQL Plus 非依存

DB 実行は Go の `database/sql` と `go-ora/v2` を使って完結させる。
よって SQL Plus を前提にしない。

## 11. runn 連携設計

### 11.1 基本利用形態

`runnora` は `runn` を外部コマンドとして起動せず、Go パッケージとして組み込む。
これにより、`BeforeFunc` / `AfterFunc` と Oracle 実行をひとつのプロセスで扱える。

各コマンドで使用する `runn` API は以下のとおりである。

| コマンド   | 主な runn API                                                       |
|----------|---------------------------------------------------------------------|
| `run`    | `runn.Load`, `runn.BeforeFunc`, `runn.AfterFunc`, `runn.DBRunner`, `runn.FailFast`, `op.RunN` |
| `list`   | `runn.LoadOnly`, `runn.Scopes`, `runn.Load`, `op.SelectedOperators` |
| `coverage` | `runn.LoadOnly`, `runn.Scopes`, `runn.Load`, `op.CollectCoverage`  |
| `loadt`  | `runn.Scopes`, `runn.Load`, `op.SelectedOperators`, `runn.NewLoadtResult`, `lr.CheckThreshold`, `lr.Report`, `lr.ReportJSON` |
| `new`    | `runn.NewRunbook`, `runn.ParseRunbook`, `rb.AppendStep`, `runn.Scopes`, `runn.Load`, `op.RunN` |
| `rprof`  | （runn 直接使用なし。`stopw.Span` の JSON を解析）                       |

`runn.LoadOnly()` は実行を行わずにロードのみを行うオプションである。
`runn.Scopes("read:parent")` は runbook から親ディレクトリのファイルへのアクセスを許可するオプションである。

### 11.2 `DBRunner` の役割

`DBRunner("db", db)` は runbook 内の `db:` step 用とする。
PL/SQL の前処理・後処理は `DBRunner` ではなく `BeforeFunc` / `AfterFunc` で実行する。`DBRunner` は runbook 側で SQL による検証をしたい場面に限定する。`runn` のドキュメントでも `DBRunner(name, client Querier)` が公開されている。 ([Go Packages][1])

### 11.3 runbook 記述方針

runbook 側には以下を記載する。

* HTTP リクエスト
* gRPC リクエスト
* レスポンス検証
* 必要なら DB step による確認

runbook 側には以下を記載しない。

* 共通 PL/SQL
* CLI 指定 PL/SQL
* Oracle 接続設定

## 12. 内部モジュール設計

### 12.1 主要パッケージ

#### `cmd`

Cobra コマンド群を定義する。

* `root.go`
* `run.go`
* `version.go`

#### `internal/config`

* YAML 読込
* 構造体定義
* デフォルト値適用
* CLI マージ

#### `internal/app`

* 実行制御
* `runn.Option` 構築
* 実行結果集約

#### `internal/hook`

* before / after フック解決
* 実行順序制御
* SQL ファイル読込

#### `internal/oracle`

* Oracle DB 接続
* SQL / PL/SQL 実行
* エラー変換

#### `internal/reporter`

* text / json / junit 出力

### 12.2 主要構造体

```go
type Config struct {
    App    AppConfig    `yaml:"app"`
    DB     DBConfig     `yaml:"db"`
    Runn   RunnConfig   `yaml:"runn"`
    Hooks  HooksConfig  `yaml:"hooks"`
    Report ReportConfig `yaml:"report"`
}
```

```go
type DBConfig struct {
    Driver              string `yaml:"driver"`
    DSN                 string `yaml:"dsn"`
    MaxOpenConns        int    `yaml:"max_open_conns"`
    MaxIdleConns        int    `yaml:"max_idle_conns"`
    ConnMaxLifetimeSec  int    `yaml:"conn_max_lifetime_sec"`
}
```

```go
type HooksConfig struct {
    Common CommonHooks `yaml:"common"`
}

type CommonHooks struct {
    Before []string `yaml:"before"`
    After  []string `yaml:"after"`
}
```

```go
type RunOptions struct {
    ConfigPath      string
    BeforeSQLFiles  []string
    AfterSQLFiles   []string
    RunbookPaths    []string
    ReportFormat    string
    ReportOutput    string
    Trace           bool
    FailFast        bool
}
```

## 13. フック実装設計

### 13.1 `BeforeFunc` 実装

`BeforeFunc` では、以下を順次実行する。

1. 共通 before ファイル群
2. CLI 追加 before ファイル群

疑似コード:

```go
runn.BeforeFunc(func(rr *runn.RunResult) error {
    files := buildBeforeFiles(cfg, opts)
    for _, f := range files {
        if err := oracleExec.ExecFile(ctx, f); err != nil {
            return wrapHookErr("before", f, rr.Path, err)
        }
    }
    return nil
})
```

`BeforeFunc` は runbook 実行前の関数登録に使う API として公開されている。 ([Go Packages][1])

### 13.2 `AfterFunc` 実装

`AfterFunc` では、以下を順次実行する。

1. CLI 追加 after ファイル群
2. 共通 after ファイル群

疑似コード:

```go
runn.AfterFunc(func(rr *runn.RunResult) error {
    files := buildAfterFiles(cfg, opts)
    for _, f := range files {
        if err := oracleExec.ExecFile(ctx, f); err != nil {
            return wrapHookErr("after", f, rr.Path, err)
        }
    }
    return nil
})
```

`AfterFunc` は runbook 実行後の関数登録 API として公開されている。 ([Go Packages][1])

### 13.3 Oracle 実行器

```go
type Executor interface {
    ExecFile(ctx context.Context, path string) error
    ExecText(ctx context.Context, sqlText string) error
    Close() error
}
```

```go
type OracleExecutor struct {
    db *sql.DB
}
```

役割:

* SQL ファイル読込
* `ExecContext` 実行
* ORA エラー整形
* ログ出力

## 14. レポート設計

### 14.1 標準出力

* 対象 runbook 数
* 成功数
* 失敗数
* cleanup 失敗数
* 失敗詳細

### 14.2 ファイル出力

* `text`
* `json`
* `junit`

### 14.3 出力内容

* runbook path
* before 実行結果
* step 実行結果
* after 実行結果
* エラー内容
* 終了時刻

## 15. エラー設計

### 15.1 エラー分類

* 設定エラー
* 引数エラー
* DB 接続エラー
* before フックエラー
* runbook 実行エラー
* after フックエラー
* レポート出力エラー

### 15.2 終了コード

* 0: 全成功
* 1: runbook 失敗
* 2: 設定・引数不正
* 3: DB 接続失敗
* 4: before / after フック失敗
* 5: レポート出力失敗

## 16. ログ設計

### 16.1 出力内容

* 開始時刻
* 実行コマンド
* 設定ファイルパス
* runbook パス
* 実行した before/after ファイル
* Oracle エラー
* 結果要約

### 16.2 マスキング

以下はログ出力時にマスクする。

* DB パスワード
* Authorization ヘッダ
* Cookie
* Bearer token
* DSN 中の資格情報

## 17. セキュリティ設計

* 本番 DB 接続設定は別プロファイルに分離
* 実行対象 SQL ディレクトリを制限可能にする
* 設定ファイルに平文パスワードを置かない運用を推奨
* 環境変数展開に対応可能な実装とする
* 実行ログに機密値を残さない

## 18. ディレクトリ構成案

```text
runnora/
  main.go
  cmd/
    root.go
    run.go
    list.go
    coverage.go
    loadt.go
    new.go
    rprof.go
    version.go
  internal/
    app/
      runner.go
    config/
      config.go
      loader.go
    hook/
      resolver.go
      executor.go
    oracle/
      connect.go
      executor.go
    reporter/
      text.go
      json.go
      junit.go
  configs/
    config.yaml
  runbooks/
    hello_world.yml
  sql/
    common/
      session_init.sql
      master_seed.sql
      session_cleanup.sql
    tmp/
      setup_case_001.sql
      cleanup_case_001.sql
  test/
    testapi/
    testgrpc/
      proto/
      main.go
    runbooks/
    sql/
```

## 19. 初期実装方針

### 19.1 Phase 1（実装完了）

* Cobra ベース CLI
* `run` コマンド（`--trace`, `--fail-fast` 含む）
* `list` / `ls` コマンド（テキスト・JSON 出力、`--long` オプション）
* `coverage` コマンド（OpenAPI / gRPC エンドポイントカバレッジ）
* `loadt` / `loadtest` コマンド（負荷テスト、threshold 判定）
* `new` / `append` コマンド（runbook 生成・追記・即時実行）
* `rprof` / `prof` コマンド（実行プロファイル分析）
* `version` コマンド
* YAML 設定読込
* `go-ora/v2` 接続
* `BeforeFunc` / `AfterFunc`
* `DBRunner` 任意利用
* text レポート

### 19.2 Phase 2

* JSON / JUnit レポート出力
* shell completion 生成
* 環境別設定切替（プロファイル機能）

### 19.3 Phase 3

* runbook グループ実行
* SQL 実行ディレクトリ制限
* 条件付き after 実行（`AfterFuncIf` 活用）

## 20. 実装サンプル方針

```go
func runE(cmd *cobra.Command, args []string) error {
    cfg, opts, err := loadRunOptions(cmd, args)
    if err != nil {
        return err
    }

    db, err := oracle.Open(cfg.DB)
    if err != nil {
        return err
    }
    defer db.Close()

    runnOpts := []runn.Option{
        runn.BeforeFunc(func(rr *runn.RunResult) error {
            return hook.RunBefore(cmd.Context(), db, cfg, opts, rr)
        }),
        runn.AfterFunc(func(rr *runn.RunResult) error {
            return hook.RunAfter(cmd.Context(), db, cfg, opts, rr)
        }),
    }

    if cfg.Runn.DBRunnerName != "" {
        runnOpts = append(runnOpts, runn.DBRunner(cfg.Runn.DBRunnerName, db))
    }

    loadOpts, err := runn.Load(args, runnOpts...)
    if err != nil {
        return err
    }

    return loadOpts.RunN(cmd.Context())
}
```

上記の構成は、Cobra の `RunE` と `runn` の `BeforeFunc`、`AfterFunc`、`DBRunner` を組み合わせる設計に対応する。Cobra のガイドでも `RunE` の利用が案内されている。 ([Cobra][5])

## 21. 設計上の結論

`runnora` は、`runn` を Go パッケージとして利用し、Oracle の前後処理を `go-ora/v2` と `BeforeFunc` / `AfterFunc` で制御する構成を標準とする。CLI は `spf13/cobra` を採用し、`cobra-cli` で土台を生成する。`runn` は HTTP, gRPC, DB query を扱え、`BeforeFunc` / `AfterFunc` / `DBRunner` を公開しているため、本要件との整合性が高い。Cobra はコマンド・引数・フラグ中心の CLI 構成に適しており、`runnora run [options] <runbook...>` という形を自然に実装できる。`go-ora/v2` は pure Go の Oracle ドライバであり、SQL Plus 非依存で Oracle 実行を組み込める。 ([Go Packages][1])

次に進めるなら、`cobra-cli` 前提のディレクトリ構成に合わせて `main.go`, `cmd/root.go`, `cmd/run.go`, `internal/config/config.go` の雛形をまとめて提示できます。

[1]: https://pkg.go.dev/github.com/k1LoW/runn "runn package - github.com/k1LoW/runn - Go Packages"
[2]: https://github.com/sijms/go-ora "GitHub - sijms/go-ora: Pure go oracle client · GitHub"
[3]: https://github.com/spf13/cobra "GitHub - spf13/cobra: A Commander for modern Go CLI interactions · GitHub"
[4]: https://github.com/spf13/cobra-cli "GitHub - spf13/cobra-cli: Cobra CLI tool to generate applications and commands · GitHub"
[5]: https://cobra.dev/docs/how-to-guides/working-with-commands/ "Working with Commands | Cobra: A Commander for Modern CLI Apps"
