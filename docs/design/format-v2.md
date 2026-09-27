# 新形式（format v2）詳細設計：`runnora.yaml` と `runnora:` ブロック

作成日: 2026-09-26
状態: 詳細設計（レビュー待ち）。実装は未着手。
上位文書: [ツール群 連携設計（方針）](../integration-design.md) の実施順 2

## 1. 目的と範囲

この文書は、実施順 2「新形式を固める」の詳細を定める。

| 対象 | 内容 |
|---|---|
| `runnora.yaml` | `config.yaml` を置き換えるプロジェクトファイル。環境、共通フック、スイート、変数を持つ |
| runbook の `runnora:` ブロック | シナリオ固有の ID、前後処理 SQL、期待する結果 |
| 変数展開 | `runnora.yaml` の環境ごとの変数を、runbook と `runnora.yaml` 自身に展開する |
| 旧形式の検出 | `config.yaml` を読み込んだら、`runnora-migrate` を案内して終了する |
| CLI の変更 | `run`、`list`、`generate` の引数の変更と、`validate` の追加 |

次は範囲外で、後の実施順で扱う。

- 証跡の自動保存、`diffEps()`、ステップ単位のレポート、サマリー HTML（実施順 3）
- 契約ケースとモック参照（実施順 6）
- docgen の入力の切り替え（実施順 7）

範囲外の機能が後から乗るように、関連するキー（`backends` など）の置き場所だけはこの文書で決めておく。

## 2. 事前検証の結果

runn v1.9.2（runnora が使っている版）のソースと、実際の動作で次を確認した。

| # | 確認したこと | 結果 | 設計への影響 |
|---|---|---|---|
| 1 | runbook のトップレベルにある未知のキー（`runnora:`）を runn が受け付けるか | **受け付ける**。goccy/go-yaml を strict モードなしで使っており、未知のキーは無視される。include される側の runbook でも同じ | runbook から `runnora:` ブロックを取り除く前処理は不要。runbook はそのまま runn に渡せる |
| 2 | runn は runbook 内の `${VAR}` をどう展開するか | YAML を解析する**前に**、ファイル全体をプロセスの環境変数（`os.LookupEnv`）で展開する。`${VAR:-default}` の形式も使える | runnora は環境の変数を**プロセスの環境変数に設定してから** runn に読み込ませればよい。runbook 側の記法は runn 標準の `${VAR}` のまま |
| 3 | `runn.BeforeFunc` / `AfterFunc` は include された runbook でも呼ばれるか | **呼ばれない**（トップレベルの runbook だけ）。関数には runbook の絶対パスが渡される | シナリオ固有のフックは、トップレベルの runbook の `runnora:` ブロックだけを見ればよい。パスで引いて実行できる |
| 4 | runnora-docgen は未知のキーで壊れないか | 壊れない。`yaml.Node` で読んでおり、知らないキーは無視される | docgen の対応（実施順 7）までの間も、新形式の runbook を手順書にできる |

確認用のテストは一時的なもので、リポジトリには含めていない。

## 3. ファイル構成とパスの基準

プロジェクトのファイル構成は [連携設計の 5.4](../integration-design.md#54-テストプロジェクトのファイル構成) に定める。この文書で扱うのは、そのうち `runnora.yaml`、`runbooks/`、`sql/` に関わる部分である。

### パスの基準

| 書く場所 | 相対パスの基準 | 理由 |
|---|---|---|
| `runnora.yaml` の中のパス | プロジェクトルート | どこから実行しても同じ意味になる |
| runbook の `runnora:` ブロックの中のパス（SQL） | **プロジェクトルート** | SQL は `sql/` に集まる。runbook の位置を基準にすると `../../sql/...` だらけになり、runbook を移動すると壊れる |
| runbook の runn 標準の記法（`json://`、`include`、`openapi3` など） | runbook ファイルの位置（runn の仕様のまま） | runn の挙動は変えない |

現状は、フックのパスを**実行時のカレントディレクトリ**で解決している（`hook.Resolver` が `os.Stat` をそのまま呼ぶ）。これをプロジェクトルート基準に改める。

### `runnora.yaml` の見つけ方

1. `--project <path>` の指定があればそれを使う。
2. なければ、カレントディレクトリから親へ向かって `runnora.yaml` を探す（Git が `.git` を探すのと同じ）。
3. 見つからなければ、プロジェクトなしとして動く。環境も共通フックもない状態で、runbook だけを実行する（チュートリアルの最小構成を壊さないため）。この場合、`runnora:` ブロックの SQL は実行時のカレントディレクトリを基準に解決する。

## 4. `runnora.yaml` の仕様

### 4.1 全体の例（runnora-e2e api-test を書き直した場合）

```yaml
version: 2

project:
  name: library-api-test

defaults:
  env: unit                        # --env を省略したときの環境

environments:
  mock:                            # モックを使った runbook の作成確認（テストレベルではない）
    description: WireMock 相手の runbook 作成確認
    vars:
      RUNNORA_BASE_URL: http://127.0.0.1:18080
      API_URL: http://127.0.0.1:18080
    # oracle を書かない → DB を使わない環境

  unit:                            # API 単体テスト
    description: API 単体（ローカル Oracle、演算サービスは API 内蔵スタブ）
    vars:
      RUNNORA_BASE_URL: http://127.0.0.1:18081
      API_URL: http://127.0.0.1:18081
      ORACLE_DSN: oracle://libapp:${LIBAPP_PASSWORD:-libapp_pw}@127.0.0.1:1522/FREEPDB1
    oracle:
      dsn: ${ORACLE_DSN}
      max_open_conns: 5
    hooks:
      before: [sql/common/00_reset.sql, sql/common/10_seed_master.sql]
      after:  [sql/common/90_verify_integrity.sql]
    backends:
      calc: { mode: stub, note: API 内蔵スタブ }

  integration:                     # 結合テスト
    vars:
      RUNNORA_BASE_URL: ${INT_API_URL}
      API_URL: ${INT_API_URL}
      ORACLE_DSN: ${INT_ORACLE_DSN}
    oracle:
      dsn: ${ORACLE_DSN}
    hooks:
      before: [sql/common/00_reset.sql, sql/common/10_seed_master.sql]
      after:  [sql/common/90_verify_integrity.sql]
    backends:
      calc: { mode: real }

suites:
  contract:
    select:
      paths: [runbooks/contract/*.suite.yml]
  scenarios:
    select:
      paths: [runbooks/scenarios/*.yml]
      labels: [scenario]
  integration:
    env: integration
    select:
      paths: [runbooks/scenarios/*.yml]
      ids: [LIB-001, LIB-004, LIB-007]
    vars:
      TOLERANCE_RULES: rules/integration.yaml

runn:
  scopes: [run:exec]               # runnora-diff を exec で呼ぶ間だけ必要（実施順 3 で不要になる）
  trace: false

report:
  format: text
  output: ""

generate:                          # 旧 config.yaml の generate セクションをそのまま移す
  openapi: openapi/library-api.yaml
  out_dir: .
  emit_manifest: true
```

### 4.2 キーの一覧

| キー | 型 | 必須 | 説明 |
|---|---|---|---|
| `version` | int | ○ | `2` 固定。これがないファイルは旧形式として扱う（→ 9 章） |
| `project.name` | string | | レポートに載せる名前。旧 `app.name` |
| `defaults.env` | string | | `--env` を省略したときの環境名。これも省略すると、環境が 1 つだけならそれを使い、複数あればエラー |
| `environments.<名前>.description` | string | | レポートと手順書に載せる説明 |
| `environments.<名前>.vars` | map[string]string | | 環境の変数（→ 6 章） |
| `environments.<名前>.oracle` | object | | フック用の Oracle 接続。キーは旧 `oracle` と同じ（`dsn`、`max_open_conns`、`max_idle_conns`、`conn_max_lifetime_sec`）。`driver` は `oracle` 固定なので削除する |
| `environments.<名前>.hooks.before` / `after` | []string | | その環境の全シナリオに共通する前後処理の SQL。旧 `hooks.common` |
| `environments.<名前>.backends` | map[string]object | | 裏のサービスの扱い（`mode`: `stub` / `mock` / `real`、`note`）。**宣言と記録だけ**で、動作は変えない。レポートと手順書に載せる |
| `suites.<名前>.env` | string | | このスイートで使う環境。省略時は `--env`、それもなければ `defaults.env` |
| `suites.<名前>.select.paths` | []string | ○ | 対象の runbook の glob（`**` 可） |
| `suites.<名前>.select.labels` | []string | | どれか 1 つを持つ runbook に絞る |
| `suites.<名前>.select.ids` | []string | | `runnora.id` で絞る。指定した順に実行する |
| `suites.<名前>.vars` | map[string]string | | 環境の変数を上書きする |
| `runn.scopes` | []string | | runn に追加で許可するスコープ。`read:parent` は常に付ける |
| `runn.trace` | bool | | runn のトレース |
| `report.format` / `output` | string | | 旧 `report` と同じ |
| `generate.*` | | | 旧 `generate` と同じ |

旧 `runn.db_runner_name` は、使われていないので削除する。

現状、`runn.trace`（と `run --trace`）は読み込まれるだけで runn に渡されていない（`internal/app/runner.go` の `buildRunnOptions` で未使用）。新形式では `runn.Trace` として実際に渡す。

### 4.3 スイートの選び方

- `select.paths` の glob に一致する runbook のうち、**`runnora:` ブロックを持つものだけ**を対象にする。
  - ブロックを持たない runbook は、include される部品（template）とみなして直接は実行しない。
  - これで、glob に template が混ざっても誤って実行されない。
- `labels` を書いたら、どれか 1 つでも持つものに絞る。
- `ids` を書いたら、その ID のものに絞り、**`ids` の順**に実行する。一覧にあるのに見つからない ID はエラーにする（抜粋の打ち間違いを防ぐ）。
- 同じ ID の runbook が 2 つあればエラーにする。

## 5. runbook の `runnora:` ブロック

### 5.1 例（LIB-004 を書き直した場合）

```yaml
desc: LIB-004 貸出上限3冊に達した会員は借りられず、1冊返すと借りられる
runnora:
  id: LIB-004
  before: [sql/cases/lib004_fill_loans_before.sql]
  after: []
  expect: pass
labels: [scenario, loans, plsql-setup]
runners:
  req:
    endpoint: ${API_URL}
    openapi3: ../../openapi/library-api.yaml
    skipValidateRequest: true
  db: ${ORACLE_DSN}
steps:
  ...
```

### 5.2 キーの一覧

| キー | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | ○ | シナリオ ID。プロジェクト内で一意。レポート、手順書、スイートの選択、将来の集計で使う。使える文字は英数字、`-`、`_`、`.` |
| `before` | []string | | このシナリオだけの前処理 SQL（プロジェクトルート基準） |
| `after` | []string | | このシナリオだけの後処理 SQL |
| `expect` | string | | 期待する結果。`pass`（既定）、`fail`、`hookFail` |
| `envs` | []string | | 実行してよい環境を限る。書かなければすべての環境で実行できる。例：モックでは動かないシナリオに `[unit, integration]` |

- ブロックを読むのは**トップレベルの runbook だけ**。include された runbook のブロックは無視する（runn もフックを呼ばないため。→ 2 章 #3）。
- 旧 `scenarios.psd1` の `Expect = 4`（事後検証が不整合を検知する確認）は `expect: hookFail` にあたる。

### 5.3 前後処理の実行順

```text
[before] 環境の hooks.before → runbook の runnora.before
         runbook 実行
[after]  runbook の runnora.after → 環境の hooks.after
```

いまの「共通 → 固有」の順を保つ。CLI の `--before-sql` / `--after-sql` は**廃止する**（2026-09-26 決定）。前後処理は必ず `runnora.yaml` か runbook の `runnora:` ブロックに書くので、実行した内容と docgen の手順書が常に一致する。その場限りの SQL を足したいときは、ブロックを一時的に書き換えるか、使い捨ての環境を `runnora.yaml` に追加する。

- 各 runbook の前後処理は、`BeforeFunc` / `AfterFunc` に渡されたパスでその runbook のブロックを引いて組み立てる。
- DB への接続は、**実行対象のどれかに前後処理が 1 つでもあるときだけ**、最初に必要になった時点で開く（いまと同じ考え方）。
- すべての SQL ファイルの存在は、実行前にまとめて確認する（いまの `Resolver` と同じ。基準をプロジェクトルートに変える）。

### 5.4 期待する結果と終了コード

各 runbook の実際の結果を `pass` / `fail` / `hookFail` に分類し、`expect` と比べる。

| expect | 実際 | 判定 |
|---|---|---|
| pass | pass | 合格 |
| pass | fail | 不合格（runbook 失敗） |
| pass | hookFail | 不合格（フック失敗） |
| fail / hookFail | 期待どおり | **合格**（レポートには「期待どおりの失敗」と明記する） |
| fail / hookFail | 期待と違う | 不合格 |

プロセスの終了コードは次のとおり。

- すべて合格なら `0`。
- 不合格のうち、実際の結果が `hookFail` のものが 1 つでもあれば `4`。
- それ以外の不合格があれば `1`。
- 2（設定・引数の不正）、3（DB 接続失敗）、5（レポート出力失敗）は今までと同じ。

これで、e2e のスクリプトで「demo は exit 4 が正解」と判定していた処理が不要になる。

## 6. 変数展開

### 6.1 変数の出どころと優先順位

上にあるものほど優先する（2026-09-26 決定）。

1. CLI の `--var NAME=VALUE`（複数指定可）
2. プロセスの環境変数（OS）
3. スイートの `vars`
4. 環境の `vars`

- OS の環境変数は、`runnora.yaml` に書いた同名の値より優先する。CI やスクリプトから、環境変数を設定するだけで接続先などを上書きできるようにするため。
- `runnora.yaml` の `vars` は「環境変数が設定されていないときの既定値」という位置づけになる。
- パスワードなどの秘密情報は `runnora.yaml` に直書きせず、`${LIBAPP_PASSWORD}` のように OS の環境変数を参照する。

#### 意図しない上書きへの対策

開発者の PC に残っていた古い環境変数で接続先が変わる、といった事故を気づけるようにする。

- `runnora.yaml` の `vars` に書いた変数を OS の環境変数が上書きした場合は、実行開始時に**上書きされた変数の名前**を標準エラーに表示する（例：`env override: API_URL, ORACLE_DSN`）。値は秘密情報を含みうるので表示しない。
- 同じ名前の一覧をレポートの `env.overrides` にも記録する（8 章）。
- `runnora validate` でも同じ一覧を表示する。

### 6.2 展開の手順

1. **段階 1**：環境とスイートの `vars` の値の中にある `${...}` を、OS の環境変数と `--var` で展開する（`vars` 同士の参照はしない）。OS の環境変数か `--var` で同名の値が与えられている変数は、`vars` の値を使わないので展開もしない。
2. **段階 2**：優先順位に従って変数表を作る。その表で、`runnora.yaml` の `oracle`、`hooks`、`report` の値の中の `${...}` を展開する。
3. **段階 3**：変数表を**プロセスの環境変数に設定してから** runn に runbook を読み込ませる。runbook の中の `${API_URL}` などは runn が自分で展開する（→ 2 章 #2）。`exec` ステップで起動する外部コマンドにも同じ変数が渡る。

- 記法は runn と同じく `${NAME}` と `${NAME:-既定値}` を使う。
- runnora は 1 回の起動で 1 つの環境しか扱わないので、プロセスの環境変数を書き換えても衝突しない。

### 6.3 未定義の変数の検出

runn は未定義の `${NAME}` を空文字に展開するので、打ち間違いに気づきにくい。そこで runnora が実行前に検査する。

- `runnora.yaml` の中の未定義の変数は、既定値（`:-`）がなければエラーにする（終了コード 2）。
- 実行対象の runbook（トップレベルだけ）の本文から `${NAME}` を拾い、変数表にも OS の環境変数にもなく、既定値もないものはエラーにする（終了コード 2）。include される runbook は runn が実行時に読むので対象外だが、`runnora validate` では include 先もたどって警告する。

## 7. CLI の変更

### 7.1 `run`

```text
runnora run [--project runnora.yaml] [--env <名前>] [--suite <名前>] [--var K=V ...] [runbook ...]
```

| フラグ | 変更 | 説明 |
|---|---|---|
| `--project` | 新規 | `runnora.yaml` のパス。省略時は親ディレクトリへ向かって探す |
| `--env` | 新規 | 使う環境 |
| `--suite` | 新規 | 使うスイート。runbook の引数と同時には指定できない |
| `--var` | 新規 | 変数の上書き |
| `--config` | **削除** | 指定されたら「runnora.yaml に移行してください（runnora-migrate）」と表示して終了コード 2 |
| `--before-sql` / `--after-sql` | **削除** | 指定されたら「前後処理は runbook の runnora: ブロックに書いてください（runnora-migrate で移行できます）」と表示して終了コード 2（→ 5.3） |
| `--scopes`、`--report-*`、`--trace`、`--fail-fast` | 維持 | `runnora.yaml` の値より CLI の指定を優先する |

- runbook の引数を指定したときは、`runnora:` ブロックの有無にかかわらず指定されたものを実行する。ブロックがない runbook では ID をファイルパスから作る（例：`runbooks/demo/x.yml` → `runbooks/demo/x`）。
- 環境の `envs` 制限（→ 5.2）に合わない runbook は実行せず、「対象外」としてレポートに載せる。

### 7.2 `validate`（新規）

実行せずに次を検査する。実施順 5 の移行ツールの確認と、将来の VSCode 拡張からも使う。

- `runnora.yaml` の構文、`version`、参照している環境やスイートが存在するか
- 変数の未定義（→ 6.3、include 先も警告）
- SQL ファイルが存在するか
- `runnora.id` の重複、スイートの `ids` に見つからない ID があるか
- 結果は人向けの text と `--format json` で出す。エラーがあれば終了コード 2

### 7.3 その他のコマンド

- `list`：`runnora.id` と、選ばれるスイートを表示する列を追加する。
- `generate`：設定を `runnora.yaml` の `generate` から読む（`--config` を `--project` に置き換える）。生成する template の endpoint は、いまと同じく `${RUNNORA_BASE_URL}` とする（環境の `vars` で与える）。
- `init`：`config.yaml` の代わりに `runnora.yaml` の雛形を作る。

## 8. レポートへの追加項目

この段階では runbook 単位のまま、次を追加する（ステップ単位への拡張は実施順 3）。

| 範囲 | 項目 |
|---|---|
| レポート全体 | `project`、`env`（名前、`description`、OS の環境変数や `--var` で上書きされた変数名の一覧 `overrides`）、`suite`、`backends` |
| runbook ごと | `id`、`expect`、`actual`（`pass` / `fail` / `hookFail` / `skipped`）、`passed`（期待と一致したか） |

JUnit では `id` を `testcase` の `name` に使い、`env` と `suite` を `properties` に入れる。

## 9. 旧形式の検出

runnora 本体は旧形式を読まない（連携設計の 9 章）。

| 状況 | 動作 |
|---|---|
| `--before-sql` / `--after-sql` が指定された | 7.1 のとおり案内を表示して終了コード 2 |
| `--config` が指定された | 「`--config` は廃止されました。runnora-migrate で runnora.yaml に移行してください」と表示し、終了コード 2 |
| `runnora.yaml` に `version` がない、または 2 でない | 同様の案内を表示して終了コード 2 |
| `runnora.yaml` がなく、カレントディレクトリに `config.yaml` がある | 警告だけ表示し、プロジェクトなしとして実行を続ける（runbook だけのチュートリアルを壊さないため） |

## 10. Go の実装構成

| パッケージ | 変更 | 内容 |
|---|---|---|
| `internal/project`（新規） | 追加 | `runnora.yaml` の型（`Project`、`Environment`、`Suite` など）、探索、読み込み、検証、変数表の組み立て（6 章） |
| `internal/scenario`（新規） | 追加 | runbook の `runnora:` ブロック（`Meta`）の読み込み、ID の導出、スイートの選択（4.3） |
| `internal/config` | 縮小 | `RunOptions` と `GenerateOptions` だけを残し、ファイルの型は `internal/project` に移す |
| `internal/hook` | 変更 | パスの基準をプロジェクトルートにする。runbook ごとにフックの一覧を組み立てる関数を追加 |
| `internal/app` | 変更 | `trace` を runn に渡す（現状は未配線）、変数をプロセスの環境変数に設定する処理、`BeforeFunc` / `AfterFunc` での runbook 別のフック、期待する結果の判定、終了コードの規則（5.4） |
| `internal/reporter` | 変更 | 8 章の項目を追加 |
| `cmd` | 変更 | `run` と `generate` のフラグ、`validate` の追加（`cobra-cli add validate`）、`init` の雛形 |

`runnora:` ブロックは runnora 自身が YAML として読む（runn の内部型には依存しない）。

## 11. テスト方針

テーブル駆動のテストで、最低限次を確かめる。

1. `runnora.yaml` の読み込みと検証（`version` なし、未知の環境、スイートの参照の誤り）
2. 変数の優先順位と展開（段階 1〜3）、未定義の検出
3. プロジェクトの探索（親ディレクトリ、`--project`、なし）
4. スイートの選択（ブロックなしの除外、ラベル、ID の順序、見つからない ID、重複 ID）
5. 前後処理の実行順（環境と runbook の組み合わせ）。いまと同じく、スタブの Executor で Oracle なしに確認する
6. 期待する結果と終了コード（5.4 の表のすべての行）
7. `include` された runbook のブロックが無視されること
8. 旧形式の検出（9 章）

既存の Oracle 実機の CI（`Oracle PL/SQL hooks`）の `test/config.yaml` は `test/runnora.yaml` に書き換える。

## 12. 移行への影響（実施順 5 の前提）

この設計で、旧形式からの対応は次のように決まる。

| 旧 | 新 |
|---|---|
| `config.yaml` の `app.name` | `project.name` |
| `oracle.*` | `environments.<名前>.oracle`（`driver` は削除） |
| `hooks.common.before` / `after` | `environments.<名前>.hooks.before` / `after` |
| `runn.trace` | `runn.trace` |
| `report.*`、`generate.*` | そのまま |
| `config.mock.yaml` のような設定ファイルの使い分け | `environments` の 1 つ |
| スクリプトの `--before-sql` / `--after-sql` / 期待 exit code | 各 runbook の `runnora:` ブロック |
| runbook に直書きの endpoint と DSN | `${API_URL}` / `${ORACLE_DSN}` と環境の `vars` |
| `RUNNORA_BASE_URL` を環境変数で渡すスクリプト | 環境の `vars.RUNNORA_BASE_URL` |

## 13. 決定事項

設計上の選択のうち、利用者の運用に関わる次の 3 点は、利用者に確認して決めた。

1. **`runnora:` ブロックの SQL パスの基準**：**決定（2026-09-26）：プロジェクトルート基準**（3 章のとおり）。runbook の位置を基準にする案は、`../../sql/...` になり runbook を移すと壊れるため採らない。
2. **`runnora.yaml` の値と OS の環境変数の優先順位**：**決定（2026-09-26）：OS の環境変数を優先**（6.1 のとおり）。CI やスクリプトから環境変数で上書きできる運用に合わせる。意図しない上書きは、上書きされた変数名の表示とレポートへの記録で気づけるようにする。
3. **`--before-sql` / `--after-sql` を残すか**：**決定（2026-09-26）：廃止する**（5.3 のとおり）。前後処理の指定を `runnora.yaml` と `runnora:` ブロックに一本化し、実行した内容と手順書がずれる余地をなくす。runnora-docgen の同名オプションも、入力を新形式に切り替えるとき（実施順 7）に廃止する。
