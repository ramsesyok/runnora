# runnora

WebAPI / gRPC シナリオテストツール。[runn](https://github.com/k1LoW/runn) を Go パッケージとして組み込み、Oracle DB への前後処理 (PL/SQL フック) を統合したテスト実行基盤です。

## 特徴

- HTTP / gRPC シナリオを runbook (YAML) で記述して実行
- Oracle DB に対する共通 PL/SQL を設定ファイルで管理し、runbook の前後で自動実行
- 実行時追加の SQL ファイルを `--before-sql` / `--after-sql` で動的に指定可能
- `go-ora/v2` を使った Pure Go 実装のため、Oracle Client 不要
- テキスト / JSON 形式のレポート出力と CI 対応の終了コード
- `generate` による OpenAPI からのテスト資産生成、`coverage` による OpenAPI / gRPC カバレッジ計測
- `loadt` による負荷テスト

## インストール

```bash
go install github.com/ramsesyok/runnora@latest
```

ソースからビルドする場合:

```bash
git clone https://github.com/ramsesyok/runnora.git
cd runnora
go build -o runnora .
```

## クイックスタート

### 1. 設定ファイルを作成する

```bash
runnora init
```

Oracle DB の SQL フックを使う場合は `oracle.dsn` を設定します。HTTP / gRPC の runbook だけを実行する場合、`oracle.dsn` は空のままで実行できます。

```yaml
# config.yaml
app:
  name: runnora

oracle:
  driver: oracle
  dsn: ""
  max_open_conns: 5
  max_idle_conns: 2
  conn_max_lifetime_sec: 300

hooks:
  common:
    before: []
    after: []

report:
  format: "text"
```

### 2. runbook を作成する

```yaml
# runbooks/hello_world.yml
desc: Hello World API テスト
runners:
  req:
    endpoint: http://localhost:8080
steps:
  get_hello:
    req:
      /hello:
        get:
          body: null
    test: steps.get_hello.res.status == 200
```

### 3. 実行する

```bash
runnora run --config ./config.yaml runbooks/hello_world.yml
```

## コマンドリファレンス

```
runnora [command]

コマンド一覧:
  init       デフォルトの config.yaml を作成する
  run         runbook を実行する
  list        runbook を一覧表示する
  coverage    OpenAPI / gRPC のカバレッジを表示する
  generate    OpenAPI 定義からテスト資産を生成する
  loadt       runbook を使って負荷テストを実行する
  new         新しい runbook を作成またはステップを追加する
  rprof       runbook 実行プロファイルを読み込んで表示する
  version     バージョン情報を表示する
```

---

### `init` — 設定ファイルを作成する

```bash
runnora init [options]
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--out` | `config.yaml` | 出力先 config ファイルパス |
| `--dsn` | — | Oracle DSN (SQL フックを使う場合に指定) |
| `--force` | `false` | 既存ファイルを上書きする |

**使用例:**

```bash
runnora init
runnora init --dsn "oracle://user:pass@host:1521/service"
runnora init --out ./config/config.yaml --force
```

---

### `run` — runbook を実行する

```bash
runnora run [options] <runbook...>
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--config` | `./config.yaml` | 設定ファイルパス |
| `--before-sql` | — | 実行前 SQL/PL/SQL ファイル（複数指定可） |
| `--after-sql` | — | 実行後 SQL/PL/SQL ファイル（複数指定可） |
| `--report-format` | `text` | レポート形式 (`text` \| `json` \| `junit`) |
| `--report-out` | — | レポート出力先ファイル（省略時は標準出力） |
| `--trace` | — | トレースモードを有効にする |
| `--fail-fast` | — | 最初の失敗で停止する |

**PL/SQL フックの実行順序:**

```
[before] 設定ファイルの common.before → --before-sql で指定したファイル
         runbook 実行
[after]  --after-sql で指定したファイル → 設定ファイルの common.after
```

**使用例:**

```bash
# 基本実行
runnora run --config ./config.yaml runbooks/hello_world.yml

# 複数 runbook を一度に実行
runnora run --config ./config.yaml runbooks/hello_world.yml runbooks/user_create.yml

# ケース固有の前後処理を追加
runnora run \
  --config ./config.yaml \
  --before-sql ./sql/tmp/setup_case_001.sql \
  --after-sql  ./sql/tmp/cleanup_case_001.sql \
  runbooks/case_001.yml

# fail-fast モードで最初の失敗で停止
runnora run --config ./config.yaml --fail-fast runbooks/*.yml
```

**終了コード:**

| コード | 意味 |
|---|---|
| `0` | 全成功 |
| `1` | runbook 失敗 |
| `2` | 設定・引数不正 |
| `3` | DB 接続失敗 |
| `4` | before / after フック失敗 |
| `5` | レポート出力失敗 |

---

### `list` (alias: `ls`) — runbook を一覧表示する

```bash
runnora list [options] <path-pattern...>
```

| フラグ | 説明 |
|---|---|
| `-l`, `--long` | フル ID とパスを表示する |
| `--format json` | JSON 形式で出力する |

**使用例:**

```bash
# テキスト形式で一覧表示
runnora list ./runbooks/*.yml

# JSON 形式で一覧表示
runnora list --format json ./runbooks/*.yml

# フル ID を表示
runnora ls --long ./runbooks/**/*.yml
```

---

### `coverage` — OpenAPI / gRPC カバレッジを表示する

OpenAPI 3 スペックや Protocol Buffers のメソッドに対して、runbook がどの程度のエンドポイントをカバーしているかを計測します。

```bash
runnora coverage [options] <path-pattern...>
```

| フラグ | 説明 |
|---|---|
| `-l`, `--long` | エンドポイントごとの詳細を表示する |
| `--format json` | JSON 形式で出力する |

**使用例:**

```bash
# カバレッジのサマリーを表示
runnora coverage ./runbooks/*.yml

# エンドポイントごとの詳細を表示
runnora coverage --long ./runbooks/*.yml

# JSON で出力して jq でフィルタリング
runnora coverage --format json ./runbooks/*.yml | jq '.specs[].key'
```

---

### `generate` — OpenAPI 定義からテスト資産を生成する

OpenAPI 3.0.x / 3.1.x の定義ファイルから、生成用 runbook と case JSON を作成します。

生成されるファイル:

| 種類 | 出力先 |
|---|---|
| template runbook | `runbooks/generated/<tag>/<method>_<operationId>.template.yml` |
| case JSON | `cases/generated/<tag>/<method>_<operationId>/default.json` |
| suite runbook | `runbooks/generated/<tag>/<method>_<operationId>.suite.yml` |

```bash
runnora generate [options]
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--config` | `./config.yaml` | 設定ファイルパス |
| `--openapi` | — | OpenAPI ファイルパス (YAML/JSON) |
| `--out` | `.` | 生成物の出力基底ディレクトリ |
| `--tags` | — | 生成対象タグ (カンマ区切り) |
| `--operation-ids` | — | 生成対象 operationId (カンマ区切り) |
| `--mode` | `shallow` | 生成モード |
| `--case-format` | `json` | case ファイル形式 |
| `--case-style` | `bundled` | case スタイル |
| `--clean` | — | 生成前に `generated/` ディレクトリを掃除する |
| `--force` | — | 既存ファイルを強制上書きする |
| `--skip-deprecated` | — | deprecated な operation をスキップする |
| `--server` | — | template runbook の endpoint として使う server URL |
| `--runner-name` | `req` | template runbook のランナー名 |
| `--emit-manifest` | — | manifest.json を生成する |
| `--emit-response-example` | — | レスポンス example を case に含める |

生成物は再生成前提です。手編集が必要な runbook は `runbooks/evidence/` にコピーして育てる運用を推奨します。

**使用例:**

```bash
# OpenAPI 定義から一式を生成
runnora generate --openapi ./openapi/openapi.yaml --out .

# users タグだけを生成
runnora generate \
  --openapi ./openapi/openapi.yaml \
  --out . \
  --tags users

# 既存の generated/ を掃除して再生成
runnora generate \
  --openapi ./openapi/openapi.yaml \
  --out . \
  --clean \
  --force
```

---

### `loadt` (alias: `loadtest`) — 負荷テストを実行する

runbook を繰り返し実行して負荷テストを行います。

```bash
runnora loadt [options] <path-pattern...>
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--load-concurrent` | `1` | 同時実行数 |
| `--duration` | `10s` | 負荷テスト時間 |
| `--warm-up` | `5s` | ウォームアップ時間 |
| `--max-rps` | `1` | 最大 RPS |
| `--threshold` | — | 合否判定式 (例: `error_rate < 0.01`) |
| `--format json` | — | JSON 形式で結果を出力する |

**使用例:**

```bash
# 10 秒間、最大 10 RPS で負荷テスト
runnora loadt \
  --duration 10s \
  --warm-up 3s \
  --max-rps 10 \
  ./runbooks/hello_world.yml

# エラー率 1% 未満を合否条件として設定
runnora loadtest \
  --duration 30s \
  --load-concurrent 5 \
  --max-rps 50 \
  --threshold "error_rate < 0.01" \
  ./runbooks/*.yml
```

---

### `new` (alias: `append`) — runbook を作成またはステップを追加する

コマンドライン引数からステップを追加して runbook を生成します。

```bash
runnora new [options] [STEP_COMMAND ...]
```

| フラグ | 説明 |
|---|---|
| `--desc` | runbook の説明 |
| `--out` | 出力先ファイルパス（省略時は標準出力） |
| `--and-run` | 作成後すぐに実行する（`--out` が必要） |

**使用例:**

```bash
# 標準出力に runbook を出力
runnora new GET https://example.com/hello

# 説明付きで runbook をファイルに保存
runnora new --desc "Hello API テスト" --out ./runbooks/hello.yml \
  GET https://api.example.com/hello

# 既存の runbook にステップを追加
runnora append --out ./runbooks/hello.yml \
  POST https://api.example.com/users '{"name":"alice"}'

# 作成して即実行
runnora new --desc "smoke test" --out /tmp/smoke.yml --and-run \
  GET https://api.example.com/health
```

---

### `rprof` (alias: `prof`, `rrprof`, `rrrprof`) — 実行プロファイルを読み込む

`run --profile-out` で生成されたプロファイルファイルを読み込み、実行時間のブレークダウンを表示します。

```bash
runnora rprof [options] <profile-path>
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--depth` | `4` | ブレークダウンの最大深度 |
| `--unit` | `ms` | 時間単位 (`ns` \| `us` \| `ms` \| `s` \| `m`) |
| `--sort` | `elapsed` | ソート順 (`elapsed` \| `started-at` \| `stopped-at`) |

**使用例:**

```bash
# プロファイルを表示（ミリ秒単位）
runnora rprof ./profile.json

# 秒単位、開始時刻順でソート
runnora prof --unit s --sort started-at ./profile.json

# 深度 2 までのサマリーを表示
runnora rprof --depth 2 ./profile.json
```

---

### `version` — バージョン情報を表示する

```bash
runnora version
```

## 設定ファイルリファレンス

```yaml
app:
  name: runnora                        # アプリケーション名

oracle:
  driver: oracle                       # ドライバ (oracle)
  dsn: "oracle://user:pass@host:1521/service"  # 接続文字列
  max_open_conns: 10                   # 最大オープン接続数 (default: 10)
  max_idle_conns: 2                    # 最大アイドル接続数 (default: 2)
  conn_max_lifetime_sec: 300           # 接続最大ライフタイム秒 (default: 300)

runn:
  trace: false                         # トレース出力

hooks:
  common:
    before:                            # runbook 実行前に必ず実行する SQL ファイル
      - "./sql/common/session_init.sql"
      - "./sql/common/master_seed.sql"
    after:                             # runbook 実行後に必ず実行する SQL ファイル
      - "./sql/common/session_cleanup.sql"

report:
  format: "text"                       # 出力形式 (text | json | junit)
  output: ""                           # ファイル出力先 (省略時は標準出力)
```

`oracle` セクションは runnora の SQL/PLSQL フック用接続設定です。runbook 内に書く通常の runn DB runner 設定とは独立しています。SQL フックを使わない場合、`oracle.dsn` は空のままで構いません。

## セキュリティ

- DSN に含まれるパスワード・トークン類はログに出力されません
- 本番 DB の設定は別プロファイルに分離することを推奨します
- 設定ファイルへの平文パスワード記載を避け、環境変数での注入を推奨します

## ライセンス

MIT License

## ドキュメント

- [基本設計書](docs/basic-design-runnora.md)
