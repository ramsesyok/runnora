# runnora

WebAPI / gRPC シナリオテストツール。[runn](https://github.com/k1LoW/runn) を Go パッケージとして組み込み、Oracle DB への前後処理 (PL/SQL フック) を統合したテスト実行基盤です。

## 特徴

- HTTP / gRPC シナリオを runbook (YAML) で記述して実行
- プロジェクトファイル `runnora.yaml` で、接続先などの変数・共通の PL/SQL・実行する runbook の組み合わせ (スイート) を環境ごとに管理
- シナリオ固有の PL/SQL と期待する結果 (成功 / 失敗 / フック失敗) を、runbook の `runnora:` ブロックに記述
- `validate` で、実行せずにプロジェクトファイルと runbook を検査
- `go-ora/v2` を使った Pure Go 実装のため、Oracle Client 不要
- テキスト / JSON / JUnit 形式のレポート出力と CI 対応の終了コード
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

### 1. プロジェクトファイルを作成する

```bash
runnora init
```

`runnora.yaml` が作成されます。パスはすべてこのファイルのあるディレクトリ (プロジェクトルート) が基準です。
Oracle DB の SQL フックを使う場合は、環境の `oracle` と `hooks` を設定します (`runnora init --dsn ...` で雛形に入れることもできます)。HTTP / gRPC の runbook だけを実行する場合は不要です。

```yaml
# runnora.yaml
version: 2
project:
  name: my-api-test
defaults:
  env: local
environments:
  local:
    vars:                          # runbook から ${API_URL} で参照する
      API_URL: http://localhost:8080
      ORACLE_DSN: oracle://user:${ORACLE_PASSWORD}@localhost:1521/FREEPDB1
    oracle:
      dsn: ${ORACLE_DSN}
    hooks:                         # この環境の全 runbook の前後で実行する
      before: [sql/common/reset.sql]
      after: [sql/common/verify.sql]
suites:
  scenarios:
    select:
      paths: [runbooks/scenarios/*.yml]
```

### 2. runbook を作成する

```yaml
# runbooks/scenarios/hello_world.yml
desc: Hello World API テスト
runnora:                           # runnora が読むシナリオ情報 (runn は無視する)
  id: HELLO-001
  before: [sql/cases/hello_seed.sql]   # このシナリオだけの前処理 (プロジェクトルート基準)
runners:
  req:
    endpoint: ${API_URL}
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
runnora run --suite scenarios                    # スイートで選んだ runbook を実行
runnora run runbooks/scenarios/hello_world.yml   # runbook を直接指定して実行
```

## コマンドリファレンス

```
runnora [command]

コマンド一覧:
  init        runnora.yaml (プロジェクトファイル) の雛形を作成する
  run         runbook を実行する
  validate    runnora.yaml と runbook を実行せずに検査する
  list        runbook を一覧表示する
  coverage    OpenAPI / gRPC のカバレッジを表示する
  generate    OpenAPI 定義からテスト資産を生成する
  genmock     OpenAPI とモックケースから WireMock のモックを生成する
  loadt       runbook を使って負荷テストを実行する
  new         新しい runbook を作成またはステップを追加する
  rprof       runbook 実行プロファイルを読み込んで表示する
  version     バージョン情報を表示する
```

---

### `init` — プロジェクトファイルを作成する

```bash
runnora init [options]
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--out` | `runnora.yaml` | 出力先ファイルパス |
| `--dsn` | — | Oracle DSN (SQL フックを使う場合に指定。省略時は `oracle` をコメントで出力) |
| `--force` | `false` | 既存ファイルを上書きする |

**使用例:**

```bash
runnora init
runnora init --dsn "oracle://user:pass@host:1521/service"
runnora init --out ./project/runnora.yaml --force
```

---

### `run` — runbook を実行する

```bash
runnora run [options] [runbook...]
runnora run [options] --suite <name>
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--project` | カレントディレクトリから親へ探す | `runnora.yaml` のパス |
| `--env` | `defaults.env` | 使う環境 (`environments` の名前) |
| `--suite` | — | 実行するスイート (`suites` の名前)。runbook の引数とは同時に指定できない |
| `--var` | — | 変数の上書き `NAME=VALUE` (複数指定可) |
| `--report-format` | `text` | レポート形式 (`text` \| `json` \| `junit`) |
| `--report-out` | — | レポート出力先ファイル（省略時は標準出力） |
| `--trace` | — | トレースモードを有効にする |
| `--fail-fast` | — | 最初の失敗で停止する |
| `--scopes` | — | runn に追加で許可するスコープ（例: `run:exec`。複数指定可） |

`--config`、`--before-sql`、`--after-sql` は廃止しました。指定すると移行を案内して終了コード 2 で終了します。旧形式のプロジェクトは [runnora-migrate](docs/migrate.md) で移行できます。前後処理の SQL は `runnora.yaml` の `environments.<名前>.hooks` か、runbook の `runnora:` ブロックに書きます。

**変数:** runbook の `${NAME}` は、次の優先順で決まる値に展開されます (runn の記法のまま。`${NAME:-既定値}` も使えます)。

1. `--var NAME=VALUE`
2. OS の環境変数
3. スイートの `vars`
4. 環境の `vars`

`runnora.yaml` の `vars` を OS の環境変数か `--var` が上書きした場合は、実行開始時に `env override: NAME, ...` と変数名を表示します (値は表示しません)。既定値のない未定義の変数を参照している runbook は、実行前にエラーになります。

**PL/SQL フックの実行順序:**

```
[before] 環境の hooks.before → スイートの hooks.before → runbook の runnora.before
         runbook 実行
[after]  runbook の runnora.after → スイートの hooks.after → 環境の hooks.after
```

スイートの `hooks` は `--suite` で実行したときだけ使います。同じ runbook を複数の環境で流し、一部の環境でだけ前提データを作る場合に使います。

**runnora: ブロック:**

| キー | 説明 |
|---|---|
| `id` | シナリオ ID (必須。英数字、`-`、`_`、`.`)。レポートとスイートの選択に使う |
| `before` / `after` | このシナリオだけの前後処理の SQL (プロジェクトルート基準) |
| `expect` | 期待する結果。`pass` (既定) / `fail` / `hookFail` |
| `envs` | 実行してよい環境。対象外の環境では実行せず、レポートに SKIP として載せる |

ブロックを読むのは実行する runbook だけで、include された runbook のブロックは使いません。スイートは、条件に合う runbook のうち `runnora:` ブロックを持つものだけを選びます (include 用の部品は選ばれません)。

**使用例:**

```bash
# スイートを実行 (環境は defaults.env)
runnora run --suite scenarios

# 環境を指定して実行
runnora run --env integration --suite integration

# runbook を直接指定し、接続先を上書き
runnora run --var API_URL=http://localhost:18081 runbooks/scenarios/*.yml

# CI 用の JUnit XML をファイルへ保存する
runnora run --suite scenarios --report-format junit --report-out ./junit.xml
```

レポートには、プロジェクト名・環境・スイート・環境の `backends` の宣言と、runbook ごとの `id` / `expect` / `actual` (`pass` / `fail` / `hookFail` / `skipped`) / `passed` (期待どおりか) を出力します。`expect: fail` や `expect: hookFail` の runbook が期待どおりに失敗した場合は合格として数えます。形式と出力先は `runnora.yaml` の `report.format` / `report.output` でも指定でき、CLI フラグを指定した場合はそちらを優先します。

**終了コード:**

| コード | 意味 |
|---|---|
| `0` | すべて期待どおり |
| `1` | 期待どおりでない runbook がある |
| `2` | 設定・引数不正 (未定義の変数、旧形式の設定を含む) |
| `3` | DB 接続失敗 |
| `4` | 期待どおりでない runbook のうち、実際の結果がフック失敗のものがある |
| `5` | レポート出力失敗 |

---

### `validate` — プロジェクトファイルと runbook を検査する

```bash
runnora validate [options] [runbook...]
```

runbook を実行せずに、`runnora.yaml` と、スイートが選ぶ runbook (引数を指定した場合はその runbook) を検査します。

- `runnora.yaml` の構文、`version`、環境・スイートの参照
- 変数の未定義 (`--env` の環境で展開して確かめる。include 先は警告)
- 前後処理の SQL ファイルの存在
- `runnora:` ブロックの内容、シナリオ ID の重複、スイートの `ids` に見つからない ID

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--project` | カレントディレクトリから親へ探す | `runnora.yaml` のパス |
| `--env` | `defaults.env` | 変数を展開して確かめる環境 |
| `--format` | `text` | 出力形式 (`text` \| `json`) |

エラーがあれば終了コード 2 を返します。

---

### `list` (alias: `ls`) — runbook を一覧表示する

```bash
runnora list [options] <path-pattern...>
```

| フラグ | 説明 |
|---|---|
| `-l`, `--long` | フル ID とパスを表示する |
| `--format json` | JSON 形式で出力する |
| `--project` | `runnora.yaml` のパス (省略時はカレントディレクトリから親へ探す) |

`runnora:` ブロックのシナリオ ID と、その runbook を選ぶスイートも表示します。

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

OpenAPI 3 スペックや Protocol Buffers のメソッドに対して、runbook がどの程度のエンドポイントをカバーしているかを計測します（runbook は実行しません）。
runners の接続先などの `${VAR}` は、`run` と同じく `runnora.yaml` の環境の `vars`・OS の環境変数・`--var` で展開します。
loop で template を include する suite は、runn の集計の対象になりません。include される template を指定してください。

```bash
runnora coverage [options] <runbook...>      # glob 可
runnora coverage [options] --suite <名前>
```

| フラグ | 説明 |
|---|---|
| `-l`, `--long` | エンドポイントごとの詳細を表示する |
| `--format json` | JSON 形式で出力する |
| `--project` / `--env` / `--suite` / `--var` | `run` と同じ |

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
| `--project` | カレントディレクトリから親へ探す | `runnora.yaml` のパス。`generate` セクションを既定値として使う |
| `--openapi` | `generate.openapi` | OpenAPI ファイルパス (YAML/JSON) |
| `--out` | `generate.out_dir`、なければプロジェクトルート | 生成物の出力基底ディレクトリ |
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
| `--emit-response-example` | — | 非推奨（効果なし）。レスポンス example は指定しなくても常に case に含まれる。今後のリリースで削除する |

生成物は再生成前提です。手編集が必要な runbook は `runbooks/evidence/` にコピーして育てる運用を推奨します。
タグや operationId にファイル名として使えない文字がある場合、生成先の名前は安全な文字に置き換え、識別用の短いハッシュを付けます。`--emit-manifest` を指定した場合、元の値は manifest のメタデータに残ります。

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

## プロジェクトファイル (runnora.yaml) リファレンス

```yaml
version: 2                             # 必須。2 固定 (旧形式の config.yaml は読み込まない)

project:
  name: library-api-test               # レポートに載せる名前

defaults:
  env: unit                            # --env を省略したときの環境

environments:
  unit:
    description: API 単体 (ローカル Oracle)   # レポートに載せる説明
    vars:                              # runbook の ${NAME} に渡す変数 (OS の環境変数と --var が優先)
      API_URL: http://127.0.0.1:18081
      ORACLE_DSN: oracle://libapp:${LIBAPP_PASSWORD:-libapp_pw}@127.0.0.1:1522/FREEPDB1
    oracle:                            # SQL/PLSQL フック用の接続 (フックを使わない環境では省略)
      dsn: ${ORACLE_DSN}
      max_open_conns: 10               # 最大オープン接続数 (default: 10)
      max_idle_conns: 2                # 最大アイドル接続数 (default: 2)
      conn_max_lifetime_sec: 300       # 接続最大ライフタイム秒 (default: 300)
    hooks:                             # この環境の全 runbook の前後で実行する SQL
      before: [sql/common/00_reset.sql, sql/common/10_seed_master.sql]
      after: [sql/common/90_verify_integrity.sql]
    backends:                          # 裏のサービスの扱い (stub / mock / real)。記録用で動作は変えない
      calc: { mode: stub, note: API 内蔵スタブ }

suites:
  scenarios:
    select:
      paths: [runbooks/scenarios/*.yml]   # glob (** 可)。runnora: ブロックを持つ runbook だけが対象
      labels: [scenario]                  # どれか 1 つを持つ runbook に絞る
  integration:
    env: integration                      # このスイートで使う環境
    select:
      paths: [runbooks/scenarios/*.yml]
      ids: [LIB-001, LIB-004]              # ID で絞り、この順に実行する
    vars:                                 # 環境の vars を上書きする
      TOLERANCE_RULES: rules/integration.yaml
    hooks:                                # このスイートを実行するときだけの前後処理
      before: [sql/cases/integration_setup.sql]
  generated:
    select:
      paths: [runbooks/generated/**/*.suite.yml]   # generate が作る suite は runnora: ブロック (id: GEN-<operationId>) を持つ

runn:
  scopes: [run:exec]                   # runn に追加で許可するスコープ
  trace: false                         # トレース出力

report:
  format: text                         # 出力形式 (text | json | junit)
  output: ""                           # ファイル出力先 (省略時は標準出力)

generate:                              # generate コマンドの既定値 (パスはプロジェクトルート基準)
  openapi: openapi/library-api.yaml
  out_dir: .
```

`oracle` は runnora の SQL/PLSQL フック用接続設定です。runbook 内に書く通常の runn DB runner 設定とは独立しています (runbook からは `${ORACLE_DSN}` のように変数で参照できます)。

設計の詳細は [新形式 (v2) 詳細設計](docs/design/format-v2.md) を参照してください。

## セキュリティ

- DSN に含まれるパスワード・トークン類はログに出力されません
- 本番 DB の設定は別プロファイルに分離することを推奨します
- 設定ファイルへの平文パスワード記載を避け、環境変数での注入を推奨します

## ライセンス

MIT License

## ドキュメント

- [チュートリアル一覧](docs/index.md)
- [基本設計書](docs/basic-design-runnora.md)（`config.yaml` と `--before-sql` / `--after-sql` の記述は旧形式。新形式は下の詳細設計を参照）
- [新形式 (v2) 詳細設計](docs/design/format-v2.md)
- [runnora-migrate：旧形式から新形式への移行](docs/migrate.md)
- [ツール群 連携設計（方針）](docs/integration-design.md)
