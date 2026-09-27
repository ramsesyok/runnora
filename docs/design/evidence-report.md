# 実施順 3 詳細設計：証跡の自動保存、`diffEps()`、ステップ単位のレポート、サマリー HTML

作成日: 2026-09-27
状態: 詳細設計（決定事項は 2 章。実装はこれから）

[連携設計](../integration-design.md) の 8 章「実施結果とレポート」と、実施順 3 を具体化する。
前提とする新形式（`runnora.yaml` と runbook の `runnora:` ブロック）は [format-v2](format-v2.md) を参照。

## 1. 目的と範囲

runnora-e2e を新形式に書き換えた時点で、スクリプトと runbook に次が残っている。

| 残っているもの | 困ること |
|---|---|
| スクリプトが `reports/<日時>/evidence/` を作り、`RUNNORA_EVIDENCE_DIR` で runbook に渡す | 「スクリプトは環境の起動だけ」にならない |
| runbook の `dump` ステップ（応答 1 件ごとに 1 ステップ） | runbook が長くなる。書き忘れると証跡が残らない。スイートでまとめて流すとファイル名が重なる |
| `exec: runnora-diff`（別プログラムを呼んで数値を許容誤差付きで比較） | OS ごとに `shell` を書き分ける。バイナリのビルドと配置が要る。差分の中身がレポートに残らない |
| レポートは runbook 単位 | どのステップで失敗したか、どの証跡を見ればよいかが分からない。手順書と対応が取れない |

これを次の 4 つで解消する。

1. **証跡の自動保存**：runnora が runn の Capturer でステップごとの送受信を受け取り、ファイルに書く。
2. **`diffEps()`**：runnora-diff の比較処理（Go パッケージ `jsondiff`）を runn の組み込み関数として登録する。
3. **ステップ単位の JSON レポート**（`report.json`）。
4. **サマリー HTML**（`summary.html`）。

範囲外:

- 手順書への良否の自動記入（品質部門の了承後。`report.json` と docgen の `manifest.json` を同じキーで引けるようにしておくところまでを本設計で行う）
- 実行結果の集計（連携設計のスコープ外）
- 証跡の保管期間や削除

## 2. 決定事項

| # | 論点 | 決定 |
|---|---|---|
| 1 | 自動保存する証跡の中身 | 既定は**応答だけ**（今の `dump` と同じ）。`runnora.yaml` の `evidence.mode: full` で**リクエストと応答の両方**を保存する |
| 2 | 秘密情報 | `Authorization`・`Proxy-Authorization`・`Cookie`・`Set-Cookie`（gRPC のメタデータの同名も）は必ず `***` に置き換える。ほかは `runnora.yaml` の `evidence.mask` で追加する |
| 3 | 保存するか、どこに | runnora が**毎回**保存し、実行ごとのフォルダ `reports/<日時>-<スイート名>/` を runnora が作る。`evidence.dir` / `--evidence-dir` で変更、`--no-evidence` で保存しない |
| 4 | レポート・証跡・手順書の対応キー | runbook に書いた**ステップのキーの並び**（include は `.` でつなぎ、loop は `[回数]`）。docgen の `manifest.json` にも同じキーを書く |
| 5 | `diffEps()` | 真偽値を返す。差分の中身は runnora が `report.json` の失敗メッセージ（要約）と証跡の `<キー>.diff.json`（全件）に書く |
| 6 | 実行ごとのフォルダに出すもの | 毎回 `report.json`・`summary.html`・`evidence/` を出す。画面にはこれまでどおりテキストの要約。JUnit は `--report-format junit` のときだけ追加 |
| 7 | 既存の `dump` と `exec: runnora-diff` | 当面は動かす（`RUNNORA_EVIDENCE_DIR` にシナリオごとの証跡フォルダを設定し、警告を出す）。runnora-migrate を v2 のプロジェクトにも使えるようにし、`dump` の削除と `diffEps()` への書き換えを行う |

## 3. 実行ごとのフォルダ

### 3.1 構成

```text
reports/                                  report.dir（既定 reports。プロジェクトルート基準）
└─ 20260927-153012-scenarios/             <日時>-<スイート名>。スイートなしは <日時>-run
   ├─ report.json                         ステップ単位の JSON レポート（6 章）
   ├─ summary.html                        サマリー HTML（7 章）
   ├─ report.xml                          --report-format junit のときだけ
   └─ evidence/                           証跡（4 章）
      ├─ LIB-001/                         シナリオ ID（runnora: ブロックの id、なければパスから作る ID の / を _ に）
      │  ├─ 01-member_before.json
      │  ├─ 07-create_loan.json
      │  ├─ 15-member_loans[0].json       loop の回ごと
      │  ├─ 15-member_loans[1].json
      │  └─ …
      ├─ LIB-007/
      │  ├─ 08-inspect_history_book_0.call.json   include 先のステップ（キーを . でつなぐ）
      │  └─ …
      └─ GRPC-series-analysis/
         ├─ 01-analyze_series.json
         └─ 03-compare_with_epsilon.diff.json     diffEps の差分（5 章）
```

- 日時はローカル時刻の `YYYYMMDD-HHMMSS`。同じ名前のフォルダがあれば `-2`、`-3` … を付ける（上書きしない）。
- 先頭の番号はトップレベルのステップの実行順（1 始まり、2 桁。100 以上は 3 桁）。include 先と loop の回は、呼び出したステップの番号を使う。
- ファイル名に使えない文字（`/ \ : * ? " < > |`）は `_` に置き換える。キーそのものは置き換えずに `report.json` に書く。
- 実行後、画面の最後に「レポート: reports/20260927-153012-scenarios/summary.html」と表示する。

### 3.2 設定とフラグ

| 設定（`runnora.yaml`） | フラグ | 既定 | 意味 |
|---|---|---|---|
| `report.dir` | ― | `reports` | 実行ごとのフォルダを作る場所 |
| `evidence.dir` | `--evidence-dir` | （実行ごとのフォルダの `evidence/`） | 証跡の保存先を別にしたいとき。指定したフォルダの下に `<シナリオID>/` を作る |
| ― | `--no-evidence` | ― | 証跡を保存しない（`report.json` と `summary.html` は出す） |
| `evidence.mode` | ― | `response` | `response`：応答だけ。`full`：リクエストと応答（4.2） |
| `evidence.mask.headers` | ― | ― | 追加で隠すヘッダ名（大文字小文字を区別しない） |
| `evidence.mask.paths` | ― | ― | 本文の JSON で隠す場所（jq 形式。runnora-diff の `ignore` と同じ書き方） |
| `report.format` / `--report-format` | ― | `text` | **画面（または `--report-out`）に出す形式**。`junit` のときは実行ごとのフォルダにも `report.xml` を出す |
| `report.output` / `--report-out` | ― | 画面 | 画面の代わりにファイルへ出す（これまでどおり） |

`report.json` と `summary.html` は `--report-format` によらず毎回出す。スクリプトは、フォルダを作る処理と `RUNNORA_EVIDENCE_DIR` の設定をやめる。
`.gitignore` には `reports/` を入れることを推奨する（`runnora init` の雛形に入れる）。

## 4. 証跡

### 4.1 取り方

runn の `Capturer`（`runn.Capture(...)` で登録する）を runnora が実装する。runn はステップの実行中に、HTTP のリクエストと応答、gRPC の送受信、DB の SQL と結果、exec のコマンドと出力を Capturer に渡し、`SetCurrentTrails` で「どの runbook のどのステップ（include・loop を含む）か」を知らせる。
runnora は、Trail からキーを作り（4.4）、ステップの終わりに 1 ファイルへ書き出す。

runbook ごとに 1 つの Capturer を作る（runnora は runbook を 1 本ずつ実行するので、並行は考えない）。

### 4.2 ファイルの中身

1 ステップにつき 1 つの JSON ファイル。共通の項目と、ランナーの種類ごとの項目を持つ。

```json
{
  "scenarioId": "LIB-001",
  "key": "create_loan",
  "index": 7,
  "runner": "http",
  "runnerKey": "req",
  "startedAt": "2026-09-27T15:30:14.120+09:00",
  "request": {
    "method": "POST",
    "url": "http://127.0.0.1:18081/loans",
    "headers": { "Content-Type": ["application/json"], "Authorization": ["***"] },
    "body": { "memberId": "M0001", "bookId": "B0001" }
  },
  "response": {
    "status": 201,
    "headers": { "Content-Type": ["application/json"] },
    "body": { "loanId": 1, "dueDate": "2026-10-11" }
  }
}
```

| ランナー | `evidence.mode: response`（既定） | `full` で加わるもの |
|---|---|---|
| HTTP | `response`：ステータス、ヘッダ、本文 | `request`：メソッド、URL、ヘッダ、本文 |
| gRPC | `response`：ステータス（コード・メッセージ）、ヘッダ、トレーラ、`messages`（ストリームは受信した全件） | `request`：サービス、メソッド、ヘッダ、`messages`（送信した全件） |
| DB | `response`：結果の行（`rows`）、影響行数 | `request`：SQL |
| exec | `response`：標準出力、標準エラー（終了コードは runn が Capturer に渡さないので載せない。`current.exit_code` で判定する） | `request`：コマンド、シェル、標準入力 |
| 上記以外（`test` だけのステップ、`bind` など） | 保存しない | 保存しない |

- 本文が JSON として読めればオブジェクトのまま、読めなければ文字列で書く。1 MiB を超える本文は先頭 1 MiB だけ書き、`"truncated": true` を付ける。
- ステップが失敗しても、それまでに受け取った分は書く（応答を受け取れずに失敗した場合は `request` だけ、または `error` だけになる）。今の `dump` は失敗したステップの後に書けないことがあったが、自動保存ではそれがなくなる。
- ステップの所要時間は証跡に書かない（runn が Capturer に渡さないため）。`report.json` のステップの `elapsedMs` に書く（6 章）。
- 前後処理の SQL（フック）は証跡に書かない。フックの失敗は `report.json` に記録する（6 章）。

### 4.3 秘密情報を隠す

- 次のヘッダは必ず値を `***` に置き換える：`Authorization`、`Proxy-Authorization`、`Cookie`、`Set-Cookie`。gRPC のメタデータ（ヘッダとトレーラ）の同名のキーも同じ。
- `evidence.mask.headers` に書いた名前も同じように隠す。
- `evidence.mask.paths` に書いた jq 形式のパスに当たる本文の値を `"***"` に置き換える（リクエストと応答の両方。gRPC のメッセージにも使う）。
- 隠した場合は、ファイルに `"masked": ["headers.Authorization", ".password"]` のように隠した場所を書く（値は書かない）。
- `evidence.mask.paths` の書き方の誤りは `runnora validate` でエラーにする。

### 4.4 ステップのキー

Trail（runn が渡す実行中の位置）から作る。

| 位置 | キーの例 |
|---|---|
| トップレベルのステップ | `create_loan` |
| include 先のステップ | `inspect_history_book_0.call`（呼び出したステップのキーと、include 先のステップのキーを `.` でつなぐ） |
| loop の回（0 始まり） | `member_loans[2]` |
| include 先の loop | `run_case.call_api[1]` |
| steps が配列で書かれた runbook | 配列の添字（`3`）をキーにする |

このキーは、`report.json`、証跡のファイル名、docgen の `manifest.json`（9 章）で共通に使う。

### 4.5 既存の `dump` ステップとの共存（決定 7）

- runnora は runbook ごとに、そのシナリオの証跡フォルダ（`evidence/<シナリオID>/`）を `RUNNORA_EVIDENCE_DIR` に設定してから実行する。古い `dump` はこのフォルダに書く。スクリプトが設定した値より優先する（スクリプトの値は使わない）。
- `dump` ステップが 1 つ以上ある runbook は、実行時に 1 回だけ警告を出す：「dump ステップは証跡の自動保存と重複しています。runnora-migrate で削除できます」。`runnora validate` も警告にする（エラーにはしない）。
- `--no-evidence` のときは `RUNNORA_EVIDENCE_DIR` を設定しない（古い `dump` はこれまでどおりスクリプトの値を使う）。

## 5. `diffEps()`

### 5.1 書き方

```yaml
steps:
  analyze_series:
    greq:
      sample.library.v1.LibraryService/AnalyzeSeries:
        message: "{{ vars.request }}"
    test: diffEps(vars.expected, current.res.message, "cases/series-analysis/tolerances.yaml")
```

```text
diffEps(expected, actual)                 許容誤差なし（完全一致）
diffEps(expected, actual, rules)          rules は設定ファイルのパス（文字列）か、設定そのもの（map）
```

- `expected` と `actual` は JSON として扱える値（map、配列、数値、文字列、真偽値、null）。runbook の `json://` で読んだ値や、`res.body` / `res.message` をそのまま渡す。
- `rules` が文字列なら、**プロジェクトルート基準**のパスとして runnora-diff の設定ファイル（YAML / JSON）を読む（SQL のパスと同じ規則。format-v2 の 3 章）。読んだ設定は実行中キャッシュする。
- `rules` が map なら、runnora-diff の設定ファイルと同じ形の設定として使う（例：`{default: {abs: 1e-6}, ignore: [".meta.timestamp"]}`）。
- 戻り値は、差分がなければ `true`、あれば `false`。差分を期待するテストは `!diffEps(...)` と書く。
- `rules` のファイルが読めない、形式が誤っている、`expected` / `actual` が JSON にできない場合は、式の評価エラーにする（そのステップは失敗）。

### 5.2 差分の記録（決定 5）

`diffEps()` は、呼ばれたステップ（4.4 のキー）に比較結果を記録する。

- 差分があれば、証跡のフォルダに `<番号>-<キー>.diff.json` を書く。中身は runnora-diff の `--format json` と同じ（`summary` と `differences`）。`--no-evidence` のときは書かない。
- そのステップが失敗したとき、`report.json` の `steps[].diff` に要約を入れる：差分の件数と先頭 5 件（パス、期待値、実際の値、適用した許容誤差）。サマリー HTML とテキストの要約にも同じものを出す。
- 1 つのステップで複数回呼んだ場合は、呼んだ順に `diffs` の配列に入れる（ファイル名は `<番号>-<キー>.diff.<n>.json`）。

### 5.3 実装

- runnora-diff の Go パッケージ `github.com/ramsesyok/runnora-diff/jsondiff`（`Compare`、`LoadConfig`、`ParseYAMLConfig`）を go.mod に加える。runnora-diff の CLI とは同じ比較処理を使うので、結果は `exec` で呼んだ場合と変わらない。
- `runn.Func("diffEps", ...)` で登録する。関数は Capturer と同じ「実行中のステップ」を参照して記録する（runbook を 1 本ずつ実行するので、1 つの状態で足りる）。
- runnora-diff 側は変更しない（公開済みの API で足りる）。

## 6. `report.json`（ステップ単位の JSON レポート）

今の JSON レポート（format-v2 の 8 章）に、実行の情報とステップを加える。今の項目は名前を変えない。

```json
{
  "schemaVersion": 2,
  "runnora": "v0.4.0",
  "startedAt": "2026-09-27T15:30:12+09:00",
  "elapsedMs": 48210,
  "project": "runnora-e2e",
  "env": { "name": "unit", "description": "実 API (Go + Oracle)", "overrides": ["LIBAPP_PASSWORD"] },
  "suite": "scenarios",
  "backends": { "calc": { "mode": "stub", "note": "API 内蔵スタブ" } },
  "evidenceDir": "evidence",
  "total": 8, "passed": 8, "failed": 0, "skipped": 0,
  "results": [
    {
      "id": "LIB-001",
      "desc": "LIB-001 本を借りて返すまでの正常系",
      "path": "runbooks/scenarios/lib-001-loan-lifecycle.yml",
      "expect": "pass", "actual": "pass", "passed": true,
      "elapsedMs": 3120,
      "hooks": [
        { "phase": "before", "file": "sql/common/00_reset.sql", "ok": true },
        { "phase": "after", "file": "sql/cases/lib001_assert_after.sql", "ok": true }
      ],
      "steps": [
        { "key": "member_before", "index": 1, "desc": "会員 M0001 の貸出状況を確認する", "runner": "http",
          "result": "success", "elapsedMs": 12, "evidence": ["LIB-001/01-member_before.json"] },
        { "key": "member_loans", "index": 15, "runner": "http", "result": "success",
          "evidence": ["LIB-001/15-member_loans[0].json", "LIB-001/15-member_loans[1].json", "LIB-001/15-member_loans[2].json"] },
        { "key": "check_book", "index": 16, "runner": "test", "result": "failure",
          "error": "(steps.book_lent.res.body.availableCopies == 2) … actual 3",
          "diff": { "differences": 1, "items": [ { "path": ".availableCopies", "expected": 2, "actual": 3 } ] } }
      ]
    }
  ]
}
```

- `steps` は実行した順に、include 先を**平らに並べる**（キーで入れ子が分かる。`inc.call`）。
- `result` は `success` / `failure` / `skipped`（runn の `if` で飛ばしたもの） / `notRun`（前のステップの失敗で実行しなかったもの）。
- runn はステップの loop の回ごとの結果を持たない（1 つにまとめる）ので、loop のステップは 1 つ（キーに `[n]` を付けない）で、`evidence` に回ごとの証跡を並べる。loop で include した場合も同じく、include 先のステップは 1 つ（`twice.call`）で、証跡は回ごと（`twice[0].call`、`twice[1].call`）。
- `evidence` は `evidenceDir` からの相対パスの配列。証跡がないステップ（`test` だけなど）は省く。
- `index` はトップレベルのステップの番号（1 始まり）。include 先のステップは呼び出したステップの番号。
- `diff` は `diffEps()` の実装（14 章の 3）で加える。
- `hooks` は実行した前後処理を順に書く。失敗したものは `ok: false` と `error`（ORA-xxxxx を含む）を持つ。
- パスはすべて `/` 区切りの相対パス（プロジェクトルート基準、証跡は `evidenceDir` 基準）。別の PC でフォルダごと開いても読める。
- `--report-format json` で画面（`--report-out`）に出す JSON も、この形にそろえる。

## 7. サマリー HTML（`summary.html`）

担当者が実行結果を見て、手順書に手書きで良否を記入するためのもの（連携設計 8 章の当面の運用）。

- **1 ファイルで完結させる**。CSS と、開閉用の少しの JavaScript は埋め込む。外部のファイル・フォント・CDN は読まない（閉域環境でもブラウザで開ける）。JavaScript が無効でも、`<details>` で開閉できる。
- 構成
  1. 実行の情報：プロジェクト、環境（説明、上書きされた変数名）、スイート、`backends`、開始日時、所要時間、runnora のバージョン
  2. 集計：合計・合格・不合格・スキップ（不合格があれば目立つ色）
  3. runbook の一覧：シナリオ ID、説明、期待、結果、合否。不合格を先頭に並べる切り替えを付ける
  4. runbook を開くと：前後処理の一覧（失敗は赤）、ステップの表（キー、説明、合否、失敗メッセージ、`diffEps` の差分の要約、証跡へのリンク）
- 証跡へのリンクは `evidence/…` への相対リンク。ブラウザで JSON をそのまま開ける。
- 文字は UTF-8。印刷しても読めるように、印刷用の CSS で全部開いた状態にする。

## 8. 画面のテキスト

これまでのテキストの要約（runbook ごとの合否と集計）は変えない。次を加える。

- 失敗した runbook の下に、失敗したステップのキーとメッセージを 1 行ずつ（最大 5 件）。
- 最後に、実行ごとのフォルダの場所（`レポート: reports/…/summary.html`）。

## 9. docgen との対応（manifest.json）

runnora-docgen の `manifest.json` に、手順番号とステップのキーの対応を加える（runnora-docgen 側の変更）。

```json
{
  "scenario": "LIB-001 本を借りて返すまでの正常系",
  "scenarioId": "LIB-001",
  "steps": [
    { "number": 1, "key": "member_before" },
    { "number": 15, "key": "member_loans", "loop": true }
  ]
}
```

- `scenarioId` は runbook の `runnora:` ブロックの `id`。
- loop のステップは、手順書では 1 行なので、キーは `[n]` を付けない形で書き、`loop: true` を付ける。`report.json` 側で `member_loans[0]`、`member_loans[1]` … をまとめて引く。
- これで、将来「`report.json` の結果を手順書の良否欄に入れる」ときは、`scenarioId` とキーで引くだけで済む（連携設計 8 章）。自動記入そのものは本設計の範囲外。

## 10. runnora-migrate の拡張（決定 7）

新形式（`version: 2`）のプロジェクトを渡されたら、runbook の次の書き換えだけを行う（旧形式なら、これまでの移行の後に続けて行う）。

| 対象 | 書き換え |
|---|---|
| `dump` ステップで、`out` が `RUNNORA_EVIDENCE_DIR` の下のもの | ステップを削除する |
| `dump` で、上記以外の場所に書くもの | 残して TODO として報告する（証跡以外の目的の可能性があるため） |
| `exec` の `command` が `runnora-diff`（`jsondiff-eps`、`./bin/runnora-diff.exe` なども）で、`stdin` が `{{ toJSON(<式>) }}` の形、`test` が終了コードか `summary.differences` だけを見るもの | `test: diffEps(<期待ファイルを json:// で読んだ vars>, <式>, "<--config のパス>")` に書き換える。期待が差分ありなら `!diffEps(...)`。期待ファイルは `vars` に `json://` で追加する |
| 上記の形に当てはまらない `exec: runnora-diff` | 残して TODO として報告する |
| 削除した `dump` だけを参照していた `test`（`check_xxx` のような判定だけのステップ） | 残す（判定は必要） |

- runbook は文字列として書き換える（これまでと同じく、コメント・書式・改行コードを残す）。
- ゴールデンテストに、e2e の新形式（書き換え前）を入力として加える。

## 11. runnora-e2e の書き換え

実施順 3 の実装後に、runnora-migrate で e2e を書き換える。

- runbook から `dump` を削除し、grpc-test の `exec: runnora-diff` を `diffEps()` にする（`bin/runnora-diff.exe` のビルドと `shell: pwsh` の指定が不要になる）。
- `scripts/add-response-evidence.ps1`（生成した template に `dump` を足すスクリプト）を削除する。
- スクリプトから `New-ReportDir`・`RUNNORA_EVIDENCE_DIR` の設定を削除する。
- 証跡のファイル名に付けた `lib-001-` などの接頭辞は不要になる（シナリオ ID ごとのフォルダに分かれるため）。

## 12. Go の実装構成

| パッケージ | 変更 | 内容 |
|---|---|---|
| `internal/evidence`（新規） | 追加 | Capturer の実装、キーの組み立て、マスク、ファイルの書き出し |
| `internal/diffeps`（新規） | 追加 | `diffEps` 関数、設定のキャッシュ、実行中のステップへの記録 |
| `internal/app` | 変更 | 実行ごとのフォルダの作成、Capturer と `diffEps` の登録、`RUNNORA_EVIDENCE_DIR` の設定、runn の `StepResult` からステップの結果を集める |
| `internal/reporter` | 変更 | `report.json` の新しい形、`summary.html`（`html/template` と `embed` で 1 ファイルに）、テキストの追加行 |
| `internal/project` | 変更 | `report.dir`、`evidence.*` のキー |
| `cmd` | 変更 | `run` の `--evidence-dir` / `--no-evidence`、`validate` の `dump` 警告と `evidence.mask.paths` の検査、`init` の雛形 |
| `internal/migrate` | 変更 | 10 章の書き換え |

依存の追加は `github.com/ramsesyok/runnora-diff`（`jsondiff`。その依存の `gojq` を含む）だけ。

## 13. テスト方針

- **証跡**：httptest のサーバと gRPC のテスト用サーバ（runn のテストと同じ方法）に対して runbook を流し、ファイルの中身をゴールデンファイルと比べる（日時と所要時間は除く）。include・loop・steps の配列・失敗したステップ・`evidence.mode` の 2 通りを含める。
- **マスク**：決まったヘッダ、`mask.headers`、`mask.paths`（配列の中、gRPC のメッセージ）の表駆動テスト。
- **`diffEps`**：一致・不一致・許容誤差・設定ファイルと map・読めない設定・JSON にできない値の表駆動テスト。runnora-diff の CLI と同じ結果になることを、runnora-diff の examples で確かめる。
- **report.json**：スキーマの形をゴールデンファイルで確認する。キーが証跡のファイル名と一致することを確認する。
- **summary.html**：外部を参照していないこと（`http`・`src=` の検査）、不合格の runbook と差分の要約が出ることを確認する。
- **Oracle の CI ジョブ**：`test/runnora.yaml` のスイートを流し、`report.json` の `hooks` に `ORA-20001` が記録されることを jq で確認する（今の確認を置き換える）。
- **migrate**：e2e の新形式を入力にしたゴールデンテスト。

## 14. 実装の順序

1. `internal/evidence` と実行ごとのフォルダ（`report.json` はまだ runbook 単位。証跡のファイルの一覧を runbook ごとに載せる）… 実装済み
2. ステップ単位の `report.json` と、画面のテキストの追加行 … 実装済み
3. `diffEps()`
4. `summary.html`
5. runnora-migrate の拡張と、e2e の書き換え
6. docgen の `manifest.json` の拡張（runnora-docgen）

1〜4 は runnora の PR を分けて出す（レビューしやすくするため）。

## 15. 実装時に確かめること

実装順 1 で確かめた結果（2026-09-27）：include 先のステップ（`inc.call`）、ステップの loop の回（`poll[0]`、`poll[1]`）、`if` で飛ばしたステップ（書かない）、`test` だけのステップ（書かない）は、runn v1.9.2 の Trail から期待どおりに取れる（`internal/evidence` のテスト）。gRPC は e2e の grpc-test で確かめる。

- runn の Capturer に、include 先のステップと loop の回が、期待どおりの Trail で渡されること（runn v1.9.2 のソースでは `SetCurrentTrails` がステップごとに呼ばれている）。
- `runn.Func` で登録した関数から、実行中のステップを正しく特定できること（評価のタイミングが Capturer の `SetCurrentTrails` の後であること）。
- gRPC の Server streaming で、受信したメッセージが全件 Capturer に渡されること。
- runn の `dump` ステップと Capturer が同じ応答を二重に書いても問題ないこと（別ファイルになるので、中身が同じであることだけ確認する）。
