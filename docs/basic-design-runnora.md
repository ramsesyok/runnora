# runnora 基本設計書

## 1. 文書目的

本書は、WebAPI および gRPC のシナリオテストツール `runnora` の基本設計を定義するものである。

`runnora` は `runn` を Go パッケージとして組み込み、HTTP リクエスト、gRPC リクエスト、DB クエリを runbook で実行しつつ、Oracle 向けの共通 PL/SQL と実行時追加 PL/SQL を runbook の前後で実行することを目的とする。

加えて、OpenAPI 定義ファイルから shallow なテスト資産を生成する `generate` サブコマンドを提供する。生成対象は以下の 3 点とする。

1. template runbook
2. case ファイル雛形
3. suite runbook

これにより、OpenAPI からの広く浅い網羅確認と、手作業で育てる証跡向け runbook の両立を図る。

## 2. 設計方針

### 2.1 基本方針

`runnora` の基本方針は以下とする。

* テスト実行エンジンは `runn`
* Oracle 接続は `go-ora/v2`
* CLI 基盤は `spf13/cobra`
* 雛形生成は `cobra-cli`
* 前処理・後処理の PL/SQL 実行は `runn.BeforeFunc` / `runn.AfterFunc`
* runbook 内で DB 検証が必要な場合は runbook 側の通常の runn DB runner 設定を利用
* OpenAPI からのテスト資産生成は `runnora generate` で行う
* 生成物は `generated` と `evidence` を分離して管理する

### 2.2 ツール名

コマンド名は `runnora` とする。
CLI の基本構文は以下とする。

```text
runnora [command] [options]
```

主コマンドは `run` および `generate` とする。

```text
runnora run [options] <runbook...>
runnora generate [options]
```

### 2.3 Cobra 採用方針

CLI 構成には `spf13/cobra` を採用する。
`cobra-cli` で基礎構成を生成し、その上に各コマンドを追加する。

本設計では、既存の実行・参照系コマンド群に加え、OpenAPI からテスト資産を生成する `generate` コマンドを追加する。

## 3. 適用範囲

### 3.1 対象

* HTTP API テスト
* gRPC テスト
* runbook 実行前の共通 PL/SQL 実行
* runbook 実行前の追加 PL/SQL 実行
* runbook 実行後の追加 PL/SQL 実行
* runbook 実行後の共通 PL/SQL 実行
* Oracle DB への接続
* OpenAPI 定義からのテスト資産生成
* JSON 形式 case サンプル生成
* CLI 実行
* CI 実行

### 3.2 対象外

* SQL Plus 依存の実行方式
* GUI
* DB マイグレーション管理
* Oracle の高度な型変換や複雑な OUT パラメータ処理の網羅
* runbook 文法そのものの改変
* OpenAPI から完全自動で業務証跡 runbook を完成させる機能

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
9. OpenAPI 定義から template runbook を生成できること
10. OpenAPI 定義から case JSON を生成できること
11. OpenAPI 定義から suite runbook を生成できること
12. tags や operationId を条件に生成対象を絞り込めること
13. CI で非対話実行できること

### 4.2 非機能要件

1. SQL Plus 未導入環境で動作できること
2. Go バイナリとして配布しやすいこと
3. Linux / Windows で実行しやすいこと
4. ログに機密情報を出力しないこと
5. エラー箇所を特定しやすいこと
6. 再生成時に手編集資産を壊さないこと
7. 将来、機能拡張しやすいこと

## 5. 全体構成

### 5.1 システム構成

```text
+----------------------------------------------------+
| runnora CLI                                        |
|  - root command                                    |
|  - run command                                     |
|  - generate command                                |
|  - utility commands                                |
+----------------------+-----------------------------+
                       |
         +-------------+-------------+
         |                           |
         v                           v
+----------------------+   +--------------------------+
| Run Application      |   | Generate Application     |
| - config loader      |   | - openapi loader         |
| - option builder     |   | - operation filter       |
| - execution control  |   | - template emitter       |
| - reporter           |   | - case emitter           |
+----------+-----------+   | - suite emitter          |
           |               | - manifest/report        |
           v               +------------+-------------+
+----------------------+                |
| runn Engine           |                v
| - HTTP                |        +---------------------+
| - gRPC                |        | generated artifacts |
| - db runner           |        +---------------------+
+----------+-----------+
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

#### C. Run Application

* Oracle 接続生成
* `runn.Option` 構築
* `BeforeFunc` / `AfterFunc` 登録
* SQL フックが設定されている場合のみ Oracle executor を生成
* runbook 実行
* 結果集約

#### D. Oracle Hook Executor

* 共通 before SQL / PL/SQL 実行
* CLI 追加 before SQL / PL/SQL 実行
* CLI 追加 after SQL / PL/SQL 実行
* 共通 after SQL / PL/SQL 実行
* ログ出力
* エラー返却

#### E. Generate Application

* OpenAPI 読込
* operation 抽出
* tags / operationId フィルタ
* template runbook 生成
* case JSON 生成
* suite runbook 生成
* manifest / report 出力

#### F. Reporter

* 実行結果の要約表示
* 失敗箇所表示
* JSON / JUnit XML 出力
* generate 結果の要約表示

## 6. 実行方式

### 6.1 実行コマンド

標準的な run 実行は以下とする。

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
8. SQL フックが設定されている場合のみ Oracle executor を生成する
9. `AfterFunc` を登録する
10. `runn.Load(...).RunN(ctx)` または `runn.New(...).Run(ctx)` を実行する
11. 結果を集約して終了コードを返す

## 7. PL/SQL フック設計

### 7.1 フックの基本思想

PL/SQL は runbook に直接書かず、`runnora` 側で管理する。
前処理・後処理は Go から `BeforeFunc` / `AfterFunc` として登録し、runbook 実行の外側で制御する。

### 7.2 実行順序

事前処理の実行順序は以下とする。

1. 設定ファイルの共通 before
2. コマンドライン引数の `--before-sql`

事後処理の実行順序は以下とする。

1. コマンドライン引数の `--after-sql`
2. 設定ファイルの共通 after

### 7.3 フック単位

PL/SQL は原則として「1 ファイル = 1 実行単位」とする。
匿名ブロック中の `;` を安全に扱うため、文単位の自動分割は行わない。

### 7.4 エラー時方針

* 共通 before 失敗: runbook 実行を開始しない
* 追加 before 失敗: runbook 実行を開始しない
* runbook 失敗: 失敗として記録する
* 追加 after 失敗: cleanup 失敗として記録する
* 共通 after 失敗: cleanup 失敗として記録する

## 8. CLI 設計

### 8.1 コマンド構成

```text
runnora
 ├─ run
 ├─ generate
 ├─ list        (alias: ls)
 ├─ coverage
 ├─ loadt       (alias: loadtest)
 ├─ new         (alias: append)
 ├─ rprof       (alias: prof, rrprof, rrrprof)
 └─ version
```

### 8.2 Cobra ベース構成

`cobra-cli init` でベースを作成し、その後に各コマンドを追加する。
各コマンドの実処理は `RunE` に集約する。

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

### 8.4 `generate` コマンドの仕様

#### Use

```text
generate [options]
```

#### フラグ

* `--openapi string`
* `--config string`
* `--out string`
* `--tags string`
* `--operation-ids string`
* `--mode string`
* `--case-format string`
* `--case-style string`
* `--clean`
* `--force`
* `--skip-deprecated`
* `--server string`
* `--runner-name string`
* `--emit-manifest`
* `--emit-response-example`

#### 入力

* OpenAPI 3.0.x / 3.1.x の YAML または JSON
* generator 設定
* CLI フィルタ条件

#### 出力

* template runbook
* case ファイル雛形
* suite runbook
* manifest
* report

#### 想定コマンド例

```bash
runnora generate \
  --openapi ./openapi/openapi.yaml \
  --out . \
  --tags users,orders \
  --case-format json \
  --mode shallow
```

### 8.5 `list` コマンドの仕様

#### Use

```text
list [PATH_PATTERN ...] (alias: ls)
```

#### フラグ

* `--long` / `-l`: フル ID とパスを表示する
* `--format string`: 出力形式 (`json`)

#### 動作

* `runn.LoadOnly()` でロードのみを行い、runbook は実行しない
* `runn.Scopes("read:parent")` で親ディレクトリへのアクセスを許可する
* `op.SelectedOperators()` で runbook 一覧を取得して表示する

### 8.6 `coverage` コマンドの仕様

#### Use

```text
coverage [PATH_PATTERN ...]
```

#### フラグ

* `--long` / `-l`
* `--format string`

#### 動作

* OpenAPI 3 スペックや gRPC サービス定義のエンドポイントカバレッジを計測する
* `op.CollectCoverage(ctx)` で集計する

### 8.7 `loadt` コマンドの仕様

#### Use

```text
loadt [PATH_PATTERN ...] (alias: loadtest)
```

#### フラグ

* `--load-concurrent int`
* `--duration string`
* `--warm-up string`
* `--max-rps int`
* `--threshold string`
* `--format string`

#### 動作

* 指定の並列数・RPS で runbook を繰り返し実行する
* threshold が指定された場合は合否判定を行う

### 8.8 `new` コマンドの仕様

#### Use

```text
new [STEP_COMMAND ...] (alias: append)
```

#### フラグ

* `--desc string`
* `--out string`
* `--and-run`

#### 動作

* 引数からステップを生成して runbook を新規作成、または既存 runbook に追記する
* `--and-run` で即時実行する

### 8.9 `rprof` コマンドの仕様

#### Use

```text
rprof [PROFILE_PATH] (alias: prof, rrprof, rrrprof)
```

#### フラグ

* `--depth int`
* `--unit string`
* `--sort string`

#### 動作

* 実行プロファイル JSON を読み込み、スパン情報を整形表示する

### 8.10 `version` コマンド

* ツール名
* バージョン
* ビルド日時
* Git commit

### 8.11 サンプル Cobra 構造

```text
main.go
cmd/
  root.go
  run.go
  generate.go
  list.go
  coverage.go
  loadt.go
  new.go
  rprof.go
  version.go
internal/
  config/
  app/
  generate/
  hook/
  oracle/
  reporter/
```

## 9. generate 設計

### 9.1 基本思想

`generate` は OpenAPI から「すぐに説明可能な完成済み証跡 runbook」を作るものではなく、広く浅い確認用のテスト資産を作るものとする。

生成物は以下の 3 層で構成する。

1. template runbook
2. case JSON
3. suite runbook

これにより、1 エンドポイントに対して複数パターンの request / expect を扱いやすくする。

### 9.2 出力ディレクトリ

```text
runbooks/
  generated/
    users/
      get_getUser.template.yml
      get_getUser.suite.yml
      post_createUser.template.yml
      post_createUser.suite.yml
  evidence/
    users/
      create_user_happy.yml

cases/
  generated/
    users/
      get_getUser/
        default.json
      post_createUser/
        default.json
        validation_missing_name.json
```

### 9.3 運用ルール

* `generated/` は再生成前提で手編集禁止
* 手編集が必要な runbook は `evidence/` にコピーして育てる
* `generate --clean` は `generated/` 配下のみを対象とする

### 9.4 生成単位

基本単位は 1 operation = 1 template + 1 suite + 1 以上の case とする。

### 9.5 命名規則

```text
runbooks/generated/<primary-tag>/<method>_<operationId>.template.yml
runbooks/generated/<primary-tag>/<method>_<operationId>.suite.yml
cases/generated/<primary-tag>/<method>_<operationId>/<case-name>.json
```

`operationId` がない場合は `<method>_<normalized-path>` を代替識別子とする。

### 9.6 mode

MVP の正式対応 mode は `shallow` とする。
将来拡張として以下を想定する。

* `evidence-skeleton`
* `negative-skeleton`

## 10. template runbook 仕様

### 10.1 目的

template runbook は 1 件の case を受け取って 1 回 API を呼ぶ薄い runbook とする。
requestBody や期待値は `vars.case` から参照する。

### 10.2 例

```yaml
desc: Create user template
labels:
  - generated
  - openapi
  - users
  - method:post
  - operation:createUser
  - mode:template

runners:
  req:
    endpoint: "{{ env `RUNNORA_BASE_URL` }}"
    openapi3: "./openapi/openapi.yaml"

vars:
  case: {}

steps:
  call_api:
    req:
      /users:
        post:
          headers: "{{ vars.case.headers }}"
          body:
            application/json: "{{ vars.case.requestBody }}"
    test: |
      current.res.status == vars.case.expect.status
```

### 10.3 labels

生成 runbook には最低限以下を付与する。

* `generated`
* `openapi`
* `<tag>`
* `method:<method>`
* `operation:<operationId>`
* `mode:template`

## 11. suite runbook 仕様

### 11.1 目的

suite runbook は複数 case を loop で回し、template runbook を include する。

### 11.2 例

```yaml
desc: Create user suite
labels:
  - generated
  - openapi
  - users
  - method:post
  - operation:createUser
  - mode:suite

vars:
  cases:
    - json://../../../cases/generated/users/post_createUser/default.json
    - json://../../../cases/generated/users/post_createUser/validation_missing_name.json

steps:
  run_case:
    loop:
      count: len(vars.cases)
    include:
      path: ./post_createUser.template.yml
      vars:
        case: "{{ vars.cases[i] }}"
```

## 12. case ファイル仕様

### 12.1 基本方針

case ファイルは JSON を標準とする。
OpenAPI 定義ファイルが YAML であっても、request / response の example を抽出して JSON サンプルとして出力する。

### 12.2 推奨フォーマット

```json
{
  "name": "default",
  "description": "Create user happy path",
  "pathParams": {},
  "queryParams": {},
  "headers": {
    "Authorization": "Bearer ${RUNNORA_TOKEN}"
  },
  "requestBody": {
    "username": "alice",
    "password": "passw0rd"
  },
  "expect": {
    "status": 201,
    "bodyMode": "subset",
    "body": {
      "username": "alice"
    },
    "ignorePaths": [
      "$.id",
      "$.createdAt"
    ]
  }
}
```

### 12.3 bodyMode

* `none`: ステータスのみ検証
* `subset`: レスポンスの一部のみ検証
* `exact`: 完全一致で検証

既定は `subset` とする。

### 12.4 case-style

MVP では `bundled` を標準とし、1 case = 1 JSON とする。
将来は `split` を追加し、`*.request.json` と `*.expect.json` に分離可能とする。

## 13. JSON サンプル生成ルール

### 13.1 requestBody 生成優先順

1. `content.application/json.example`
2. `content.application/json.examples`
3. schema の `example`
4. schema の `default`
5. 型ベース自動サンプル
6. `TODO_*`

### 13.2 expect.body 生成優先順

1. 代表成功レスポンスの `content.application/json.example`
2. `content.application/json.examples`
3. schema の `example`
4. 空オブジェクト

### 13.3 代表成功ステータス選択順

1. 200
2. 201
3. 202
4. 204
5. 最初の 2xx
6. default

### 13.4 pretty-print ルール

* UTF-8
* 2 スペース indent
* 末尾改行あり

## 14. OpenAPI の tags / operationId / 拡張の扱い

### 14.1 tags

OpenAPI の `tags` は生成 runbook の `labels` に写す。
`generate --tags` はこの tags を用いて生成対象を絞り込む。

### 14.2 operationId

`operationId` はファイル名、manifest キー、識別子に利用する。

### 14.3 vendor extension

将来拡張として `x-runnora` を許可する。
想定用途は以下とする。

* 追加 labels
* 代表 example の選択
* 将来の beforeSql / afterSql 候補指定
* evidence 優先度指定

## 15. 設定ファイル設計

### 15.1 基本方針

設定ファイルは YAML とする。
設定ファイルでは以下を管理する。

* 共通 DB 接続設定
* 共通 before / after
* `generate` の既定値
* レポート設定

CLI 引数は run 系では追加フック、generate 系では上書き優先で扱う。

### 15.2 設定ファイル例

```yaml
app:
  name: runnora

oracle:
  driver: oracle
  dsn: "oracle://user:pass@host:1521/service"
  max_open_conns: 5
  max_idle_conns: 2
  conn_max_lifetime_sec: 300

runn:
  trace: false

hooks:
  common:
    before:
      - "./sql/common/session_init.sql"
      - "./sql/common/master_seed.sql"
    after:
      - "./sql/common/session_cleanup.sql"

generate:
  openapi: "./openapi/openapi.yaml"
  out_dir: "."
  case_format: "json"
  case_style: "bundled"
  mode: "shallow"
  clean_generated: false
  emit_manifest: true
  runner_name: "req"

report:
  format: "text"
  output: ""
```

### 15.3 マージ規則

#### run

* 設定ファイルを基準とする
* `--before-sql` は `hooks.common.before` の後ろに追加
* `--after-sql` は `hooks.common.after` の前に追加
* report 設定は CLI が指定された場合のみ上書き
* runbook は CLI 位置引数でのみ指定

#### generate

* config の `generate` を基準とする
* CLI で指定された `--openapi` `--out` `--tags` `--operation-ids` `--mode` `--case-format` は config より優先する
* `--clean` は `generated` 配下の掃除を有効にする

## 16. Oracle 接続設計

### 16.1 採用ドライバ

Oracle 接続ドライバは `github.com/sijms/go-ora/v2` を採用する。

### 16.2 接続方針

* 1 プロセスで 1 つの `*sql.DB` を共有
* `Ping` により接続確認
* `SetMaxOpenConns` 等を設定ファイルから反映
* DSN は設定ファイルまたは環境変数で与える

### 16.3 SQL Plus 非依存

DB 実行は Go の `database/sql` と `go-ora/v2` を使って完結させる。
よって SQL Plus を前提にしない。

## 17. runn 連携設計

### 17.1 基本利用形態

`runnora` は `runn` を外部コマンドとして起動せず、Go パッケージとして組み込む。
これにより、`BeforeFunc` / `AfterFunc` と Oracle 実行をひとつのプロセスで扱える。

### 17.2 各コマンドで使用する主な API

| コマンド       | 主な runn API                                                                                   |
| ---------- | --------------------------------------------------------------------------------------------- |
| `run`      | `runn.Load`, `runn.BeforeFunc`, `runn.AfterFunc`, `runn.FailFast`, `op.RunN` |
| `generate` | runn 実行は行わない。生成物は `openapi3`, `labels`, `include`, `loop` を利用する前提で出力する                        |
| `list`     | `runn.LoadOnly`, `runn.Scopes`, `runn.Load`, `op.SelectedOperators`                           |
| `coverage` | `runn.LoadOnly`, `runn.Scopes`, `runn.Load`, `op.CollectCoverage`                             |
| `loadt`    | `runn.Scopes`, `runn.Load`, `op.SelectedOperators`, `runn.NewLoadtResult`                     |
| `new`      | `runn.NewRunbook`, `runn.ParseRunbook`, `rb.AppendStep`, `runn.Load`, `op.RunN`               |
| `rprof`    | runn 直接使用なし                                                                                   |

### 17.3 DB runner との関係

runnora の `oracle` 設定は、PL/SQL の前処理・後処理を `BeforeFunc` / `AfterFunc` で実行するために使う。
runbook 内の通常の `db:` step は runn の DB runner 設定として扱い、runnora の `oracle` 設定から自動登録しない。

### 17.4 runbook 記述方針

runbook 側には以下を記載する。

* HTTP リクエスト
* gRPC リクエスト
* レスポンス検証
* 必要なら DB step による確認

runbook 側には以下を記載しない。

* 共通 PL/SQL
* CLI 指定 PL/SQL
* Oracle 接続設定

## 18. 内部モジュール設計

### 18.1 主要パッケージ

#### `cmd`

* `root.go`
* `run.go`
* `generate.go`
* `list.go`
* `coverage.go`
* `loadt.go`
* `new.go`
* `rprof.go`
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

#### `internal/generate`

* OpenAPI 読込
* operation 抽出
* フィルタ
* template 生成
* case 生成
* suite 生成
* manifest / report 生成

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
* generate 要約出力

### 18.2 主要構造体

```go
type Config struct {
    App      AppConfig      `yaml:"app"`
    Oracle   OracleConfig   `yaml:"oracle"`
    Runn     RunnConfig     `yaml:"runn"`
    Hooks    HooksConfig    `yaml:"hooks"`
    Generate GenerateConfig `yaml:"generate"`
    Report   ReportConfig   `yaml:"report"`
}
```

```go
type OracleConfig struct {
    Driver             string `yaml:"driver"`
    DSN                string `yaml:"dsn"`
    MaxOpenConns       int    `yaml:"max_open_conns"`
    MaxIdleConns       int    `yaml:"max_idle_conns"`
    ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"`
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
type GenerateConfig struct {
    OpenAPI        string `yaml:"openapi"`
    OutDir         string `yaml:"out_dir"`
    CaseFormat     string `yaml:"case_format"`
    CaseStyle      string `yaml:"case_style"`
    Mode           string `yaml:"mode"`
    CleanGenerated bool   `yaml:"clean_generated"`
    EmitManifest   bool   `yaml:"emit_manifest"`
    RunnerName     string `yaml:"runner_name"`
}
```

```go
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
```

```go
type GenerateOptions struct {
    ConfigPath         string
    OpenAPIPath        string
    OutDir             string
    Tags               []string
    OperationIDs       []string
    Mode               string
    CaseFormat         string
    CaseStyle          string
    Clean              bool
    Force              bool
    SkipDeprecated     bool
    Server             string
    RunnerName         string
    EmitManifest       bool
    EmitResponseExample bool
}
```

## 19. フック実装設計

### 19.1 `BeforeFunc` 実装

`BeforeFunc` では以下を順次実行する。

1. 共通 before ファイル群
2. CLI 追加 before ファイル群

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

### 19.2 `AfterFunc` 実装

`AfterFunc` では以下を順次実行する。

1. CLI 追加 after ファイル群
2. 共通 after ファイル群

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

### 19.3 Oracle 実行器

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

## 20. レポート設計

### 20.1 標準出力

#### run

* 対象 runbook 数
* 成功数
* 失敗数
* cleanup 失敗数
* 失敗詳細

#### generate

* 対象 operation 数
* 生成数
* スキップ数
* 警告数
* 出力先

### 20.2 ファイル出力

* `text`
* `json`
* `junit`
  ただし generate では `json` を優先する

### 20.3 出力内容

#### run

* runbook path
* before 実行結果
* step 実行結果
* after 実行結果
* エラー内容
* 終了時刻

#### generate

* operationId
* method
* path
* tags
* 生成された template / suite / case のパス
* 生成時刻

## 21. エラー設計

### 21.1 エラー分類

* 設定エラー
* 引数エラー
* OpenAPI 読込エラー
* 生成エラー
* DB 接続エラー
* before フックエラー
* runbook 実行エラー
* after フックエラー
* レポート出力エラー

### 21.2 終了コード

* 0: 成功
* 1: runbook 失敗
* 2: 設定・引数不正
* 3: DB 接続失敗
* 4: before / after フック失敗
* 5: レポート出力失敗
* 6: generate 失敗

## 22. ログ設計

### 22.1 出力内容

* 開始時刻
* 実行コマンド
* 設定ファイルパス
* runbook パス
* OpenAPI パス
* 実行した before / after ファイル
* 生成したファイル一覧
* Oracle エラー
* 結果要約

### 22.2 マスキング

* DB パスワード
* Authorization ヘッダ
* Cookie
* Bearer token
* DSN 中の資格情報

## 23. セキュリティ設計

* 本番 DB 接続設定は別プロファイルに分離
* 実行対象 SQL ディレクトリを制限可能にする
* 設定ファイルに平文パスワードを置かない運用を推奨
* 環境変数展開に対応可能な実装とする
* 実行ログに機密値を残さない
* `generate --clean` は `generated` 配下のみを対象とする
* `evidence` 配下は自動上書きしない

## 24. ディレクトリ構成案

```text
runnora/
  main.go
  cmd/
    root.go
    run.go
    generate.go
    list.go
    coverage.go
    loadt.go
    new.go
    rprof.go
    version.go
  internal/
    app/
      runner.go
      generate_service.go
    config/
      config.go
      loader.go
    generate/
      openapi_loader.go
      operation_filter.go
      template_emitter.go
      suite_emitter.go
      case_emitter.go
      manifest.go
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
  openapi/
    openapi.yaml
  runbooks/
    generated/
    evidence/
  cases/
    generated/
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

## 25. 実装方針

### 25.1 Phase 1

* Cobra ベース CLI
* `run` コマンド
* `list` / `coverage` / `loadt` / `new` / `rprof` / `version`
* YAML 設定読込
* `go-ora/v2` 接続
* `BeforeFunc` / `AfterFunc`
* runbook 側 DB runner 設定との共存
* text レポート

### 25.2 Phase 2

* `generate` コマンド
* OpenAPI 読込
* template runbook 生成
* case JSON 生成
* suite runbook 生成
* manifest / report 出力
* JSON / JUnit レポート出力
* shell completion 生成
* 環境別設定切替

### 25.3 Phase 3

* `x-runnora` 対応
* `evidence-skeleton` / `negative-skeleton`
* split case 対応
* runbook グループ実行
* SQL 実行ディレクトリ制限
* 条件付き after 実行

## 26. 実装サンプル方針

### 26.1 run 側

```go
func runE(cmd *cobra.Command, args []string) error {
    cfg, opts, err := loadRunOptions(cmd, args)
    if err != nil {
        return err
    }

    runnOpts := []runn.Option{runn.Scopes("read:parent")}

    if len(opts.BeforeSQLFiles) > 0 || len(opts.AfterSQLFiles) > 0 ||
        len(cfg.Hooks.Common.Before) > 0 || len(cfg.Hooks.Common.After) > 0 {
        db, err := oracle.Open(cfg.Oracle)
        if err != nil {
            return err
        }
        defer db.Close()

        runnOpts = append(runnOpts,
            runn.BeforeFunc(func(rr *runn.RunResult) error {
                return hook.RunBefore(cmd.Context(), db, cfg, opts, rr)
            }),
            runn.AfterFunc(func(rr *runn.RunResult) error {
                return hook.RunAfter(cmd.Context(), db, cfg, opts, rr)
            }),
        )
    }

    loadOpts, err := runn.Load(args, runnOpts...)
    if err != nil {
        return err
    }

    return loadOpts.RunN(cmd.Context())
}
```

### 26.2 generate 側

```go
func generateE(cmd *cobra.Command, args []string) error {
    cfg, opts, err := loadGenerateOptions(cmd)
    if err != nil {
        return err
    }

    spec, err := generate.LoadOpenAPI(opts.OpenAPIPath)
    if err != nil {
        return err
    }

    ops := generate.FilterOperations(spec, opts)

    if opts.Clean {
        if err := generate.CleanGenerated(cfg, opts); err != nil {
            return err
        }
    }

    for _, op := range ops {
        if err := generate.EmitTemplate(cfg, op, opts); err != nil {
            return err
        }
        if err := generate.EmitDefaultCaseJSON(cfg, op, opts); err != nil {
            return err
        }
        if err := generate.EmitSuite(cfg, op, opts); err != nil {
            return err
        }
    }

    return generate.EmitManifestAndReport(cfg, ops, opts)
}
```

## 27. 設計上の結論

`runnora` は、`runn` を Go パッケージとして利用し、Oracle の前後処理を `go-ora/v2` と `BeforeFunc` / `AfterFunc` で制御する構成を標準とする。
CLI は `spf13/cobra` を採用し、`cobra-cli` で土台を生成する。
さらに、OpenAPI から shallow なテスト資産を生成する `generate` を同一 CLI に組み込むことで、以下を両立する。

* 契約整合の広い確認
* request / expect を分離した運用
* JSON ベースの case 管理
* 手編集 runbook との役割分担

必要なら次に、この更新版をそのまま `basic-design-runnora.md` 形式の Markdown ファイルとして整形して出力します。
