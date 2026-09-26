# runnora ツール群 連携設計（方針）

作成日: 2026-09-26
状態: 方針合意。各項目の詳細設計・実装はこれから。

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

## 3. ツール構成

**runnora を中核の CLI とし、他はライブラリまたはファイル契約で連携する。**

| ツール | 位置づけ |
|---|---|
| runnora | 中核の CLI。generate / mock / run / doc 連携の入口 |
| oapi2wire | Go ライブラリ（`pkg/oapi2wire`）として runnora が取り込む。**単体 CLI も存続**する（フロントチームは runnora も Oracle も使わず、oapi2wire と WireMock だけで使う） |
| runnora-diff | Go ライブラリ（`jsondiff`）として runnora が取り込み、runn の組み込み関数にする。単体 CLI も存続する |
| runnora-docgen | 別バイナリのまま。Quarto / ddq の様式という関心事とリリースサイクルが違うため。runbook と `runnora.yaml` をファイル契約として読む |
| runnora-e2e | 新形式の見本であり、旧形式からの移行の実例。移行ツールのゴールデンデータにもする |

検討した他の案:

- 別 CLI のまま規約だけ整える案は、課題 B・D が残り、利用者ごとに接着剤を作ることになるため採らない。
- 5 つを 1 リポジトリ・1 バイナリにする案は、Quarto 依存や WireMock（Java）周りが中核に入り、リリースが重くなるため採らない。

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
- docgen も同じブロックを読むので、`--before-sql` を二重に指定しなくてよい。
- 要検証：runn がトップレベルの未知のキーを許容するか。許容しない場合は、runnora が読み込み時にこのブロックを取り除いてから runn に渡す。

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

## 6. フロントチームへのモック受け渡し

- 受け渡しは一方通行で、同期はしない。フロントは受け取ったあと自由に改変する。
- 渡すのは `openapi.yaml`、`mock-cases.yaml`、応答 JSON、oapi2wire のバイナリの一式（ビルド済みの `wiremock-out/` だけでは改変しにくい）。一式を固めるコマンド（例：`oapi2wire pack`）を用意する。
- データの性格はテスト向け（具体値の照合、異常系が多い）であることを、受け渡しのときに伝える。
- フロントが早い時期にモックを必要とする場合は、第 1 弾として `oapi2wire init` の雛形を渡し、テストで作り込んだ版を後から渡す。
- 任意（要望が出たら）：`--cases` を複数指定して重ねられるようにし、フロントが自分たちの改変を別ファイルに分けておけるようにする。

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
| `dump` ステップと `RUNNORA_EVIDENCE_DIR` の削除 | 自動 |
| `exec: runnora-diff`（jsondiff-eps）を `diffEps()` に置き換え | 自動。変則的な書き方は TODO として報告 |
| 直書きの endpoint と DSN を変数に置き換え | 自動で抽出し、`runnora.yaml` の環境の候補を生成 |
| `config.yaml` から `runnora.yaml` へ | 自動 |
| スクリプト内のシナリオと SQL の対応を `runnora:` ブロックへ | 半自動。空のブロックを挿入し、TODO 一覧を出す |
| `runbooks/generated/` | 移行しない（再生成する） |
| `mock-cases.yaml` と応答 JSON | 変更なし |

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
| 1 | すぐ直す：grpc-test の runnora-diff ビルド、oapi2wire の mapping id を安定化、e2e README の古い記述 | 対応済み（各リポジトリの作業ブランチ） |
| 2 | 新形式を固める：`runnora.yaml`、`runnora:` ブロック、変数展開、`version: 2` | |
| 3 | runtime：証跡の自動保存、`diffEps()` の内蔵、ステップ単位の JSON レポート、サマリー HTML | |
| 4 | e2e を新形式に書き直す（見本と移行の実例） | |
| 5 | `runnora-migrate` を作り、先行チームへ適用する | |
| 6 | 契約ケースとモック参照の統一、suite の導出、OpenAPI の静的検査 | |
| 7 | docgen の入力を新形式に切り替える | |
| 8 | `generate --proto`、JSON Schema、VSCode 拡張 | |

優先順位は、先行チーム（手書きシナリオ中心）に効く 2・3 を先にし、契約テスト周りの 6 を後にしている。
