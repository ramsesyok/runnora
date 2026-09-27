# runnora ツール群 連携設計（方針）

作成日: 2026-09-26
更新日: 2026-09-27
状態: 方針合意。実施順 1・2・4・5 と 7 の一部を実装済み（進み具合は 11 章）。

runnora / oapi2wire / runnora-diff / runnora-docgen / runnora-e2e のベータ版完成を受けて、ツール間の連携を見直した結果をまとめる。
個別ツールの仕様ではなく、ツール群としての役割分担・データの持ち方・実施順を定める。

## 1. 前提

### 想定利用者

1. OpenAPI で WebAPI を設計・実装している
2. Oracle Database を使っている
3. 内部演算に gRPC を使っている

### ワークフロー

| # | 作業 | 使うツール |
|---|---|---|
| 1 | OpenAPI / proto を定義する | ― |
| 2 | テストシナリオの雛形、リクエスト JSON、期待値 JSON の雛形を生成する | runnora |
| 3 | テストケースを編集・追加する | ― |
| 4 | API モックの雛形を作る（gRPC は除く） | oapi2wire |
| 5 | モック情報を追加しながら runbook の動作を確認する | oapi2wire + runnora |
| 6 | シナリオを手順書にする | runnora-docgen |
| 7 | レビューする | ― |
| 8 | テストを実施する（複雑な数値比較は runnora-diff） | runnora + runnora-diff |
| 9 | テスト結果を集計する | スコープ外 |

### スコープ外（今回の見直しでは扱わない）

- gRPC のモック（将来は目指す。当面は API 側のスタブで対応する。受け口だけ用意する。→ 7 章）
- 手順書の良否欄への自動記入（品質保証チームと調整中。ID をそろえることだけ行う。→ 8 章）
- テスト結果の集計（ワークフロー 9）

## 2. 現状の課題（runnora-e2e で分かったこと）

| # | 課題 | 表れている箇所 |
|---|---|---|
| A | runnora は `genmock` として oapi2wire を取り込み済みだが、README に載っておらず、e2e は単体の oapi2wire を使っている。入口が 2 つある | `cmd/genmock.go` |
| B | 同じ operation について「ケース」のモデルが 2 系統ある（runnora の case JSON と oapi2wire の `mock-cases.yaml`）。1 ケースの追加に 4 か所を編集し、重複する事実が 3 つある。食い違いの検出に専用ツール（`tools/contract-check`）が必要になっている | e2e api-test |
| C | 生成した runbook がそのまま使えない。template を正規表現で書き換えている。suite の `loop` は途中ケースの失敗を見逃す。`bodyMode` が効かない | `add-response-evidence.ps1`、`internal/generate/emitter.go` |
| D | シナリオ固有の前後処理 SQL と期待 exit code が runbook の外（スクリプト）にあり、run と docgen に別々に渡している | `scripts/scenarios.psd1` |
| E | `generate` が OpenAPI 専用で、proto からは生成しない | `internal/generate` |
| F | runnora-diff を `exec` で呼んでいる。`--scopes run:exec`、stdin 末尾の空白、CWD 基準のパス、`shell: pwsh` への依存がある | grpc-test |
| G | 証跡のために、全 runbook に `dump` ステップと `RUNNORA_EVIDENCE_DIR` を手で書いている | e2e 全体 |
| H | endpoint と DSN が config と runbook に直書きで、環境を切り替えられない | e2e 全体 |
| I | リクエスト・レスポンスの雛形を runnora と oapi2wire が別々の規則で作り、置き場所も違う。仮の値が `TODO` や `0` で、proto からは作らない | `internal/generate/sample.go`、oapi2wire `internal/openapi/sample_generator.go`（詳細は [sample-generation](design/sample-generation.md)） |

## 3. ツール構成

**runnora を中核の CLI とし、他はライブラリまたはファイル契約で連携する。**

| ツール | 位置づけ |
|---|---|
| runnora | 中核の CLI。generate / mock / run / doc 連携の入口。いまの `genmock` サブコマンドは `mock` に改名する（`runnora mock init` / `build` / `validate`） |
| oapi2wire | Go ライブラリ（`pkg/oapi2wire`）として runnora が取り込む。**単体 CLI も存続**する（フロントチームは runnora も Oracle も使わず、oapi2wire と WireMock だけで使う） |
| runnora-diff | Go ライブラリ（`jsondiff`）として runnora が取り込み、runn の組み込み関数にする。単体 CLI も存続する |
| runnora-docgen | 別バイナリのまま。Quarto / ddq の様式という関心事とリリースサイクルが違うため。runbook と `runnora.yaml` をファイル契約として読む |
| runnora-e2e | 新形式の見本であり、旧形式からの移行の実例。移行ツールのゴールデンデータにもする |

検討した他の案:

- 別 CLI のまま規約だけ整える案は、課題 B・D が残り、利用者ごとに接着剤を作ることになるため採らない。
- 5 つを 1 リポジトリ・1 バイナリにする案は、Quarto 依存や WireMock（Java）周りが中核に入り、リリースが重くなるため採らない。

### runnora が扱う範囲

runnora が扱うのは、テストの実行・判定・証跡・モック・文書の原稿までとする（2026-09-27 決定）。**テスト対象の環境の起動**（DB、API・gRPC サーバ、WireMock の起動と停止、応答するまでの待ち合わせ）は扱わず、docker compose・CI・短いスクリプトに任せる。

- プロセス管理、ログ、後始末、OS ごとの違いを runnora が抱えると、docker compose や CI の機能と重なるため。
- 新形式に書き直した runnora-e2e（実施順 4）を、「残るスクリプトは環境の起動だけ」という最小の見本にする。README に、スクリプトが残る範囲とその理由を書く（2026-09-27 に書き換え済み。runnora-e2e の README「runnora.yaml とスクリプトの分担」）。

## 4. テストレベル

| レベル | 対象（実物） | 差し替えるもの | 主な検証 |
|---|---|---|---|
| API 単体 | API + Oracle | 裏の gRPC → 当面は API 側のスタブ | 本文の全体一致、DB の状態、前後処理 |
| gRPC 単体 | gRPC 演算サービス | なし | 数値の比較（runnora-diff） |
| 結合 | API + gRPC + Oracle | なし | API 単体のシナリオを抜粋または全部流用する |

**モックを使った runbook の作成確認はテストレベルではない。** API 単体テストの runbook を作り込む過程の作業であり（リクエストの組み立てとケース選択の確認）、品質保証向けの実施結果・手順書・件数には含めない。

- この作業で整えた `mock-cases.yaml` と応答 JSON が、副産物としてフロントに渡すデータになる。
- runn の `openapi3` 検証で、モックの応答が OpenAPI に合っていることも確かめる。これは渡す前の品質確認を兼ねる。
- 対象は HTTP だけ。gRPC 単体の runbook は本物の gRPC サービスに向けて作る。

## 5. データモデル

### 5.1 `runnora.yaml`（プロジェクトファイル）

横断する情報を持つ。`config.yaml` を置き換える。

```yaml
version: 2
environments:
  unit:
    endpoints: { api: http://127.0.0.1:18081 }
    oracle: { dsn: ${UNIT_DSN} }
    hooks:
      before: [sql/common/00_reset.sql, sql/common/10_seed_master.sql]
      after:  [sql/common/90_verify_integrity.sql]
    backends: { calc: { mode: stub } }     # 裏の gRPC の扱い（宣言と記録のみ。→ 7 章）
  integration:
    endpoints: { api: http://int-host:8080 }
    oracle: { dsn: ${INT_DSN} }
    backends: { calc: { mode: real } }
suites:
  api-unit:
    env: unit
    select: { labels: [scenario] }
  integration:
    env: integration
    select: { ids: [LIB-001, LIB-004, LIB-007] }   # 抜粋。all: true で全件
    overrides:
      tolerance: rules/integration.yaml
```

- runbook は 1 本だけにする。単体と結合の違いは、環境とスイートの選択で表す（runbook を複製しない）。
- 結合で差が出るもの（許容誤差、DB フック、一部の期待値）は上書きで表す。
- endpoint と DSN は環境変数で展開する。runbook には直書きしない。

### 5.2 runbook の `runnora:` ブロック

シナリオ固有の情報は runbook 自身に書く。

```yaml
desc: LIB-004 貸出上限3冊に達した会員は借りられず、1冊返すと借りられる
runnora:
  id: LIB-004
  before: [sql/cases/lib004_fill_loans_before.sql]
  after: []
  expectExit: 0
labels: [scenario, loans]
runners:
  req: { endpoint: '${API_URL}' }
  db:  '${ORACLE_DSN}'
```

- 1 ファイルを見ればシナリオの前提が分かる。レビューも移行もファイル単位で済む。
- `runnora run runbooks/scenarios/*.yml` だけで、固有の SQL も含めて正しく実行される。
- docgen も同じブロックを読むので、`--before-sql` を二重に指定しなくてよい。runnora と docgen の `--before-sql` / `--after-sql` は廃止し、前後処理の指定はブロックと `runnora.yaml` に一本化する（[format-v2](design/format-v2.md) の 5.3）。
  - runnora は廃止済み。docgen は `runnora.yaml` とブロックを読むようにしたが、旧形式の `--config` と追加の `--before-sql` / `--after-sql` は、先行チームの移行が済むまで残している（実施順 7 の残り）。
- runn（v1.9.2）はトップレベルの未知のキーを無視するため、このブロックがあっても runbook はそのまま runn に渡せる（確認済み。詳細は [format-v2](design/format-v2.md) の 2 章）。

### 5.3 モックと契約テストのケース

役割が違うため、一方を他方から自動で導出しない。

| 観点 | フロント向けモック | 契約テスト |
|---|---|---|
| リクエストの照合 | 緩い（`matches`、fallback） | 厳密（具体的な値） |
| ケース数 | 多い | 代表ケース |

- `mock-cases.yaml` と応答 JSON が**モックの正本**。持ち主はテストチームで、形式は oapi2wire のまま変えない。
- 契約テストのケースは**モックケースを参照するだけ**にする。

  ```json
  { "name": "B0001",
    "request": { "pathParams": { "bookId": "B0001" } },
    "expect": { "mock": "getBook_B0001", "ignorePaths": [] } }
  ```

  - status と期待本文は、参照先のモックケースから runnora が読む。
  - 実 API とモックで結果が違う場合だけ、`expect.status` と `expect.body` で上書きする。
- runnora が読み込み時に、`request` が参照先モックの matcher（`equalTo` / `matches` / `equalToJson` / `matchesJsonPath`）に一致するかを評価する。一致しなければエラーにする（`contract-check` を内蔵する）。
- どのテストからも参照されないモックケースは「フロント専用」として一覧表示する（エラーにはしない）。
- suite はケースディレクトリから導出する（手で書かない。`loop` は使わず、ケースごとに include ステップを並べる）。
- シナリオ試験（状態と DB を扱う）はモックを参照しない。モックへの依存は契約テストの層に閉じ込める。

### 5.4 テストプロジェクトのファイル構成

1 つの API（と、その裏の gRPC 演算サービス）のテスト一式を、1 つのプロジェクト（`runnora.yaml` が 1 つ）にまとめる。API 単体・gRPC 単体・結合は、同じプロジェクトの中で環境とスイートを切り替えて実行する。

```text
<project>/                             プロジェクトルート（runnora.yaml の場所。Git リポジトリのルートを想定）
├─ runnora.yaml                        プロジェクトファイル（環境・スイート・変数・共通フック）
├─ openapi/
│  └─ openapi.yaml                     API 定義（正本）                                  ★フロント
├─ proto/
│  └─ *.proto                          gRPC 定義（正本）
├─ mock/                               モック（テストチームの持ち物）
│  ├─ mock-cases.yaml                  oapi2wire のモックケース                          ★フロント
│  └─ responses/
│     └─ <operationId>/<caseId>.json   応答本文。契約テストの期待本文も兼ねる            ★フロント
├─ runbooks/
│  ├─ generated/                       runnora generate の出力（再生成する。手で編集しない）
│  │  ├─ api/<tag>/<method>_<operationId>.template.yml / .suite.yml
│  │  └─ grpc/<service>/<method>.template.yml / .suite.yml          （実施順 8 で追加）
│  ├─ contract/                        API の契約テスト（operation ごとの template と suite。suite は実施順 6 でケースから導出する）
│  ├─ scenarios/                       API 単体シナリオ（runnora: ブロックあり。結合テストで流用）
│  └─ grpc/                            gRPC 単体シナリオ（runnora: ブロックあり）
├─ cases/
│  ├─ generated/                       generate の出力（再生成する）
│  ├─ contract/<operationId>/<NN_name>.json   契約ケース（request と expect.mock）
│  ├─ scenarios/<scenarioId>/*.json    API シナリオで使う入力・期待値
│  └─ grpc/<scenarioId>/request.json, expected.json   gRPC シナリオの入力・期待値
├─ rules/                              runnora-diff の許容誤差（default.yaml、integration.yaml など）
├─ sql/
│  ├─ common/                          環境の共通フック（リセット・シード・不変条件の検証）
│  └─ cases/                           シナリオ固有の前後処理
├─ docs/                               手順書（Quarto book）。_quarto.yml・章立て・本文は手書き
│  └─ generated/                       runnora-docgen の出力（再生成する）
├─ out/                                ビルド成果物（Git 管理外）
│  ├─ wiremock/                        mappings/ と __files/（runnora mock build）
│  └─ frontend-mock.zip                フロント向けの一式（oapi2wire pack。→ 6 章）
└─ reports/                            実行結果（Git 管理外）
   └─ <日時>-<suite>/                  レポート（text / json / junit / html）と evidence/
```

★フロント：フロントチームに渡すもの（6 章）。

| ディレクトリ | 作り方 | 手で編集 | Git 管理 | 持ち主 | フロントに渡す |
|---|---|---|---|---|---|
| `runnora.yaml` | `runnora init` の雛形を編集 | する | する | テストチーム | 渡さない |
| `openapi/`、`proto/` | API・gRPC の設計者が作成 | ―（正本を置くだけ） | する | 設計者 | `openapi.yaml` だけ渡す |
| `mock/mock-cases.yaml` | `runnora mock init` の雛形を編集 | する | する | テストチーム | 渡す |
| `mock/responses/` | 同上（応答の雛形を編集） | する | する | テストチーム | 渡す |
| `runbooks/generated/`、`cases/generated/` | `runnora generate` | しない | する（再生成の差分をレビューで確認するため） | ― | 渡さない |
| `runbooks/contract/`、`cases/contract/` | 生成物をもとに作成 | する | する | テストチーム | 渡さない |
| `runbooks/scenarios/`、`runbooks/grpc/`、`cases/scenarios/`、`cases/grpc/` | 手書き | する | する | テストチーム | 渡さない |
| `rules/`、`sql/` | 手書き | する | する | テストチーム | 渡さない |
| `docs/`（`generated/` 以外） | 手書き | する | する | テストチーム | 渡さない |
| `docs/generated/` | `runnora-docgen` | しない | する | ― | 渡さない |
| `out/` | `runnora mock build`、`oapi2wire pack` | しない | しない | ― | `frontend-mock.zip` を渡す |
| `reports/` | `runnora run` | しない | しない | ― | 渡さない |

- runbook からの参照は runn の仕様どおり runbook の位置が基準になる（例：`runbooks/contract/` の template から `json://../../cases/contract/...`）。`runnora:` ブロックと `runnora.yaml` の中のパスはプロジェクトルート基準（[format-v2](design/format-v2.md) の 3 章）。
- runnora-e2e の現状との対応：`fixtures/responses/` → `mock/responses/`、`mock/wiremock-out/` → `out/wiremock/`、`config.yaml` と `config.mock.yaml` → `runnora.yaml` の環境、`scripts/scenarios.psd1` → 各 runbook の `runnora:` ブロック。`api-test/` と `grpc-test/` は、見本としては分けたままでもよいが、実案件では 1 つのプロジェクトにまとめる形を推奨する。

## 6. フロントチームへのモック受け渡し

- 受け渡しは一方通行で、同期はしない。フロントは受け取ったあと自由に改変する。
- 渡すのは `openapi.yaml`、`mock-cases.yaml`、応答 JSON、ビルド済みの WireMock 資産、oapi2wire のバイナリの一式（ビルド済みの資産だけでは改変しにくい）。一式を固めるコマンド（`oapi2wire pack`）を用意する。中身は次の「渡すファイル」のとおり。
- データの性格はテスト向け（具体値の照合、異常系が多い）であることを、受け渡しのときに伝える。
- フロントが早い時期にモックを必要とする場合は、第 1 弾として `oapi2wire init` の雛形を渡し、テストで作り込んだ版を後から渡す。
- 任意（要望が出たら）：`--cases` を複数指定して重ねられるようにし、フロントが自分たちの改変を別ファイルに分けておけるようにする。

### 渡すファイル

`oapi2wire pack` で次の一式を 1 つの zip（`out/frontend-mock.zip`）にまとめる。

```text
frontend-mock/
├─ README.md                           起動と再ビルドの手順、データの性格（テスト向け）、作成元（OpenAPI の version、Git のコミット、作成日）
├─ openapi.yaml                        ← openapi/openapi.yaml
├─ mock-cases.yaml                     ← mock/mock-cases.yaml
├─ responses/<operationId>/<caseId>.json   ← mock/responses/
├─ wiremock/                           ← out/wiremock/（ビルド済み。受け取ってすぐ起動できる）
│  ├─ mappings/
│  └─ __files/
└─ bin/oapi2wire(.exe)                 改変後の再ビルド用（任意。OS ごとに用意）
```

- WireMock 本体（Java の jar）は同梱しない。フロント側で用意する（同梱する場合は Apache License 2.0 の表記を README に入れる）。
- **渡さないもの**：`runnora.yaml`、runbook、ケース、`sql/`、`rules/`、`docs/`、`reports/`。テストの内部情報（DB の接続先、シード用のデータ、テストの判定条件）を含むため。
- 応答 JSON に社外に出せないデータ（実在の人名、社内のホスト名など）を入れないことは、モックケースを作るときの約束として手順書に書いておく。

フロント側での使い方：

| やりたいこと | 手順 |
|---|---|
| そのまま使う | `java -jar wiremock-standalone.jar --root-dir wiremock` |
| モックを改変する | `mock-cases.yaml` と `responses/` を編集し、`oapi2wire build --openapi openapi.yaml --cases mock-cases.yaml --responses-root responses --out wiremock --clean` |
| API の変更を取り込む | 新しい `openapi.yaml` を受け取り、`oapi2wire validate` で改変済みのモックケースとの不整合を確認する |

### API 変更への追従

変更の起点は常に OpenAPI。自社とフロントがそれぞれ OpenAPI に対して検証し直す。そのための静的検査を oapi2wire に追加する（runnora からも使う）。

| 検査 | 検出できること |
|---|---|
| 応答 JSON（bodyFile）を OpenAPI の応答スキーマで検証 | 項目の追加・削除・型変更に取り残された応答ファイル |
| status が OpenAPI の responses に定義されているか | 未定義ステータスのモック |
| matcher が参照するパラメータが OpenAPI に存在するか | 名前の変わったパラメータ |
| （runnora）`generate` の差分レポート | 追加・削除・変更された operation と、影響を受けるケース・シナリオ |

## 7. gRPC のモック（将来）

当面は API 側のスタブで対応し、ツールでは扱わない。将来の対応に備えて次だけ行う。

- `runnora.yaml` の環境に `backends`（stub / mock / real）を宣言できるようにする。
- 実行環境の `backends` の設定を、レポートと手順書に記録する。スタブを使っている間は、API 単体テストの期待値がスタブの固定値に依存するので、その前提をレビューで追えるようにする。
- ケースの `expect.mock` は HTTP（WireMock）のモックケースに限定すると明記する。裏の gRPC のモック応答は、将来別のキーで追加する。

## 8. 実施結果とレポート

詳細設計は [evidence-report](design/evidence-report.md)。証跡の中身（既定は応答だけ、`evidence.mode: full` でリクエストも）、秘密情報を隠す範囲、実行ごとのフォルダ（`reports/<日時>-<スイート名>/`）などは同書の 2 章で決めた。

- 証跡は `run --evidence-dir` で自動保存する（runn の capture 機構を使う）。runbook から `dump` ステップをなくす。
- runnora-diff を組み込み関数にする（例：`test: diffEps(vars.expected, steps.x.res.body, "rules.yaml")`）。
- JSON レポートを**ステップ単位**に拡張する。各ステップに `scenarioId`、`stepKey`、合否、失敗メッセージ、証跡ファイルのパスを持たせる。キーは docgen の `manifest.json` と共通にする。
- `run --report-format html` でサマリー HTML を出力する。担当者はこれを見て、手順書に手書きで良否を記入する（当面の運用）。
- 手順書への良否の自動記入は、品質部門の了承後に別途検討する。そのときは docgen が JSON レポートを読み、manifest のキーで引くだけで済むようにしておく。
- レポートに `suite` と `env` を載せ、同じシナリオの単体と結合の結果を後から集計できるようにする。

## 9. 互換性と移行

- 先行チーム（独自スクリプトで実行と SQL を管理し、手書きシナリオを作成中）がいるため、移行手段つきで形式を変える。
- runnora 本体は新形式だけを読む。新形式のファイルは `version: 2` を持ち、旧形式を読み込んだら「runnora-migrate を使ってください」と表示して終了する。
- 移行ツールは runnora リポジトリ内の別バイナリ `cmd/runnora-migrate/` とする。新形式の型と検証を共有でき、役目を終えたらディレクトリごと削除できる。

| 対象 | 方法 |
|---|---|
| `dump` ステップと `RUNNORA_EVIDENCE_DIR` の削除 | 自動（実施順 3 の実装後に追加する） |
| `exec: runnora-diff`（jsondiff-eps）を `diffEps()` に置き換え | 自動（実施順 3 の実装後に追加する）。変則的な書き方は TODO として報告 |
| 直書きの endpoint と DSN を変数に置き換え | 自動で抽出し、`runnora.yaml` の環境の候補を生成 |
| `config.yaml` から `runnora.yaml` へ | 自動 |
| スクリプト内のシナリオと SQL の対応を `runnora:` ブロックへ | 半自動。人が書き写した**シナリオ対応表**から入れる。対応表にない runbook には空のブロックを挿入し、TODO 一覧を出す |
| スクリプトが環境変数で渡していた値、runbook をまとめて実行していた単位 | シナリオ対応表から環境の `vars` と `suites` に入れる |
| `runbooks/generated/` | 移行しない（再生成する） |
| `mock-cases.yaml` と応答 JSON | 変更なし |

使い方と対応表の書き方は [runnora-migrate](migrate.md) を参照。

runnora-e2e を移行する過程で、「同じスイートを複数の環境で流し、一部の環境でだけ前提データを作る」書き方が必要だと分かり、スイートの前後処理（`suites.<名前>.hooks`）を新形式に追加した（2026-09-27。[format-v2](design/format-v2.md) の 5.3）。

動作の原則:

- 既定は dry-run。`--write` を付けたときだけ書き込む。
- Git の作業ツリーがきれいでなければ書き込みを拒否する。
- 何度実行しても同じ結果になる。
- 旧 runnora と新 runnora で同じ環境に対して流し、ケース ID ごとの合否が一致することで等価性を確認する。
- 先行チームは、移行が済むまで旧 runnora のバージョンを固定して使う。

## 10. VSCode 拡張

- 先に、runbook の拡張部分、ケース JSON、`runnora.yaml`、`mock-cases.yaml` の JSON Schema を公開する。yaml-language-server で補完と検証が効くので、拡張がなくても入力ミスの大半を防げる。
- 拡張本体（operation × ケースのツリー、ケース単位の実行、モックの起動と停止、差分表示、手順書のプレビュー）は、CLI の JSON 出力が揃ってから作る。ロジックは CLI 側に置き、拡張は CLI を呼ぶだけにする。
- 閉域環境では vsix の手配布が前提になる。

## 11. 実施順

| # | 内容 | 状態 |
|---|---|---|
| 1 | すぐ直す：grpc-test の runnora-diff ビルド、oapi2wire の mapping id を安定化、e2e README の古い記述 | 対応済み（各リポジトリの main にマージ済み） |
| 2 | 新形式を固める：`runnora.yaml`、`runnora:` ブロック、変数展開、`version: 2` | 実装済み（[format-v2](design/format-v2.md)。実装で決めた細部は同書の 14 章）。旧形式の最終版はタグ `v0.3.0` |
| 3 | runtime：証跡の自動保存、`diffEps()` の内蔵、ステップ単位の JSON レポート、サマリー HTML | 詳細設計済み（[evidence-report](design/evidence-report.md)）。同書 14 章の 1（証跡の自動保存と実行ごとのフォルダ）、2（ステップ単位の report.json）、3（diffEps()）、4（summary.html）と、5 のうち runnora-migrate の拡張を実装済み |
| 4 | e2e を新形式に書き直す（見本と移行の実例） | 書き換え済み（runnora-migrate で移行し、TODO を手で対応）。旧形式はタグ `format-v1`。**Windows での実行確認（旧形式と合否が同じこと）が残っている** |
| 5 | `runnora-migrate` を作り、先行チームへ適用する | 作成済み（[runnora-migrate](migrate.md)）。e2e の `format-v1` を入力にしたゴールデンテストあり。先行チームへの適用はこれから |
| 6 | 契約ケースとモック参照の統一、suite の導出、OpenAPI の静的検査、サンプル生成の共通化と改善（リクエスト・レスポンスを別ファイルに、制約に沿った値に） | サンプル生成は詳細設計済み（[sample-generation](design/sample-generation.md)） |
| 7 | docgen の入力を新形式に切り替える | 必要最小限を実施済み（e2e の書き換えに必要だったため前倒し）。`--project` / `--env` / `--suite` で `runnora.yaml` の環境・スイートの `hooks` と runbook の `runnora:` ブロックを読み、runnora と同じ順で前後処理を載せる。残り：旧形式の `--config` と `--before-sql` / `--after-sql` の廃止、HTTP 呼び出し表の URL に環境の変数を展開するか（いまは `${API_URL}` のまま表示）の決定 |
| 8 | `generate --proto`（proto からリクエスト・期待値の雛形を生成）、JSON Schema、VSCode 拡張 | proto のサンプル生成は詳細設計済み（[sample-generation](design/sample-generation.md)） |

優先順位は、先行チーム（手書きシナリオ中心）に効く 2・3 を先にし、契約テスト周りの 6 を後にしている。

実施順 4（e2e の書き換え）で見つかり、その場で直したこと（2026-09-27）:

| 見つかったこと | 対応 |
|---|---|
| 同じスイートを複数の環境で流し、一部の環境でだけ前提データを作る書き方ができない | スイートの前後処理（`suites.<名前>.hooks`）を追加（9 章、[format-v2](design/format-v2.md) の 5.3） |
| `runnora generate` の suite に `runnora:` ブロックがなく、スイートで選べない（スイートの前後処理を付けられない） | `generate` が suite に `id: GEN-<operationId>` のブロックを付ける（template には付けない） |
| `runnora coverage` が `runnora.yaml` の変数を展開せず、接続先を `${API_URL}` と書いた runbook を黙って読み飛ばす | `coverage` を `run` と同じ規則（`--project` / `--env` / `--suite` / `--var`）で変数を展開するように修正 |
| docgen が旧形式の `config.yaml` と `--before-sql` を前提にしていて、手順書を作れない | 実施順 7 を必要最小限だけ前倒し（上表） |
| シナリオをスイートでまとめて流すと、証跡（`dump`）のファイル名が重なる | e2e ではファイル名の先頭にシナリオを付けて回避。実施順 3 の証跡の自動保存では、保存先をシナリオ ID ごとに分ける |
| loop で template を include する suite は、runn のカバレッジ集計の対象にならない | runn の仕様。e2e では template を指定して集計する（README に注記） |
