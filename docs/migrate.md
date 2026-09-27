# runnora-migrate：旧形式から新形式への移行

`runnora-migrate` は、旧形式（runnora `v0.3.0` まで。`config.yaml`・`--config`・`--before-sql` / `--after-sql`）で作ったテストプロジェクトを、新形式（`runnora.yaml` と runbook の `runnora:` ブロック）に移行する使い捨てのツールです。移行が済んだら使いません（runnora 本体のサブコマンドにはしていません）。

新形式（`runnora.yaml` がある）のプロジェクトに使うと、証跡の自動保存と `diffEps()` に合わせた runbook の書き換え（後述）だけを行います。

新形式の仕様は [新形式 (v2) 詳細設計](design/format-v2.md) を参照してください。

## インストール

```bash
go install github.com/ramsesyok/runnora/cmd/runnora-migrate@latest
```

## 使い方

```bash
runnora-migrate [options] [dir]
```

| フラグ | 説明 |
|---|---|
| `--env 名前=ファイル` | 旧形式の設定ファイルと環境名の対応（複数指定可）。省略時は `config.yaml` → `default`、`config.<名前>.yaml` → `<名前>` |
| `--scenarios ファイル` | シナリオ対応表（後述。`dir` 基準） |
| `--write` | 書き込む。省略時は dry-run（変更の予定と TODO を表示するだけ） |
| `--allow-dirty` | Git の未コミットの変更があっても書き込む |

手順:

1. 移行するプロジェクトをコミットし、Git の作業ツリーをきれいにする（`--write` はきれいでないと書き込みません。結果を `git diff` で確認し、必要なら `git checkout` で戻すため）。
2. スクリプトを見ながら**シナリオ対応表**を書く。
3. dry-run で変更の予定と TODO を確認する。

   ```bash
   runnora-migrate --env unit=config.yaml --env mock=config.mock.yaml --scenarios migrate-scenarios.yaml .
   ```

4. `--write` を付けて書き込み、`git diff` で確認する。
5. TODO に対応する（スクリプトの `--config` などを外す、環境ごとの値を直す、`runbooks/generated/` を再生成する）。
6. `runnora validate` で検査し、旧形式と同じ環境で実行して合否が変わらないことを確かめる。

もう一度実行しても何も変わりません（移行済みのファイルは変更しません）。

## 移行する内容

| 旧形式 | 新形式 | 方法 |
|---|---|---|
| `config.yaml`・`config.<名前>.yaml` | `runnora.yaml` の `environments.<名前>`（`oracle.dsn` は `vars.ORACLE_DSN`、`hooks.common` は `hooks`）。`app.name`・`runn.trace`・`report`・`generate` は最初の設定ファイルから | 自動（旧ファイルは削除） |
| runbook の `runners` に直書きした HTTP の `endpoint`・DB の DSN・gRPC の `addr` | `${API_URL}`・`${ORACLE_DSN}`・`${GRPC_ADDR}` などに置き換え、値を環境の `vars` に入れる（同じ値は同じ変数） | 自動（TODO：環境ごとに値を確認） |
| 実行の入口になる runbook（他の runbook から include されていないもの） | `runnora:` ブロックを追加。`id` は対応表 → `desc` の先頭の ID（`LIB-004 ...` など）→ ファイル名の順で決める | 自動 |
| スクリプトで runbook ごとに指定していた `--before-sql` / `--after-sql` と期待する終了コード | `runnora:` ブロックの `before` / `after` / `expect` | 対応表から |
| スクリプトが環境変数で渡していた値（`RUNNORA_BASE_URL` など） | 環境の `vars` | 対応表から |
| スクリプトで runbook をまとめて実行していた単位 | `runnora.yaml` の `suites` | 対応表から |
| スクリプトの `--config` / `--before-sql` / `--after-sql` / `--scopes`、runbook のコメントにある旧形式の実行方法 | ― | TODO として報告 |
| `RUNNORA_EVIDENCE_DIR` の下に書く `dump` ステップ | 削除（応答は runnora が証跡として自動で保存する） | 自動 |
| `exec` で runnora-diff を呼んで比較するステップ | `test: diffEps(...)`（期待ファイルは `vars` に `json://` で追加） | 自動（形が合わないものは TODO） |
| スクリプトで `RUNNORA_EVIDENCE_DIR` を設定する処理、`diffEps()` に置き換えた後の runnora-diff のビルド | ― | TODO として報告（ファイルごとに 1 件） |
| `runbooks/generated/` | ― | 移行しない（新しい runnora で再生成する） |
| `mock-cases.yaml` と応答 JSON | ― | 変更しない |

runbook は文字列として書き換えるので、コメントや書式、改行コード（CRLF を含む）はそのまま残ります。

### 証跡の dump ステップと runnora-diff の書き換え

runnora は HTTP・gRPC・DB・exec の各ステップの応答を証跡として自動で保存し、runnora-diff と同じ比較を `diffEps()` で行えます（[証跡とレポートの詳細設計](design/evidence-report.md)）。そこで、次の書き換えを行います（旧形式のプロジェクトでは、上の移行に続けて行います）。

**`dump` ステップの削除:** `dump` の `out` が `RUNNORA_EVIDENCE_DIR` を含み、ステップに `dump`・`desc`・`if` 以外のキーがなく、ほかのステップから参照されていないものを削除します。次のものは残して TODO として報告します。

- 出力先が `RUNNORA_EVIDENCE_DIR` の下ではない `dump`（証跡以外の目的の可能性があるため）
- `dump` と一緒に `test` などがあるステップ
- リスト形式の `steps`（削除すると `steps[n]` の番号がずれるため）

**runnora-diff の `diffEps()` への置き換え:** 次の形の `exec` ステップを、`test: diffEps(...)` 1 行にします。

```yaml
# 変更前
compare_with_epsilon:
  exec:
    command: ./bin/runnora-diff.exe --config cases/series-analysis/tolerances.yaml cases/series-analysis/expected.json -
    shell: pwsh -NoProfile -Command {0}
    stdin: '{{ toJSON(steps.analyze_series.res.message) }} '
  test: current.stdout == "" && current.stderr == "" && current.exit_code == 0

# 変更後 (vars に expected: json://../cases/series-analysis/expected.json を追加)
compare_with_epsilon:
  test: 'diffEps(vars.expected, steps.analyze_series.res.message, "cases/series-analysis/tolerances.yaml")'
```

- `command` の実行ファイル名が `runnora-diff` か `jsondiff-eps`（パスや `.exe` は問わない）で、引数が `[--format 形式] [--config 設定] 期待ファイル -` の形のもの。
- `stdin` が `{{ toJSON(<式>) }}` の形のもの。`<式>` を `diffEps()` の実際の値にします。
- `test` が終了コード（`current.exit_code == 0` / `== 1` / `!= 0`）、`summary.differences`、`equal`、空の `stdout` / `stderr` だけを `&&` でつないだもの。差分がないことを期待していれば `diffEps(...)`、差分があることを期待していれば `!diffEps(...)` にします。
- ファイルのパスは、`exec` がプロジェクトルートで実行されていた前提で読みます。期待ファイルは runbook からの相対パスで `vars` に追加し（同じファイルの変数があれば使い回す。名前は `expected`、使われていれば `expected2` …）、`--config` はプロジェクトルート基準のまま `diffEps()` に渡します。
- `desc` と `if` は残します。ほかのキー（`loop` など）、ほかのオプション（`--ignore` など）、変数や式を使ったパスがあるもの、`vars` がフロー形式（`vars: {}`）のものは、変更せずに TODO として報告します。

## シナリオ対応表

スクリプトに書かれていた内容は、ツールでは読み取れません。人が YAML に書き写します。

```yaml
scenarios:
  # runbook ごとの前後処理と期待する結果 (runbook は dir 基準。glob 可。最初に一致した行を使う)
  - runbook: runbooks/scenarios/lib-004-loan-limit.yml
    before: [sql/cases/lib004_fill_loans_before.sql]
  - runbook: runbooks/demo/hook-failure-detection.yml
    id: DEMO-HOOK-FAILURE                 # 省略時は desc の先頭の ID かファイル名
    after: [sql/cases/demo_break_invariant_after.sql]
    expectExit: 4                         # 旧形式の終了コード (0 → pass、1 → fail、4 → hookFail)。expect: hookFail とも書ける
  - runbook: runbooks/contract/*.suite.yml

environments:                             # スクリプトが環境変数で渡していた値
  unit:
    vars:
      RUNNORA_BASE_URL: http://127.0.0.1:18081
  mock:
    vars:
      RUNNORA_BASE_URL: http://127.0.0.1:18080

suites:                                   # runnora.yaml の suites にそのまま写す
  contract-unit:
    env: unit
    select:
      paths: [runbooks/contract/*.suite.yml]
    hooks:                                # この環境でスイートを流すときだけの前後処理
      before: [sql/cases/contract_setup.sql]
  scenarios:
    env: unit
    select:
      paths: [runbooks/scenarios/*.yml]
```

対応表にない入口の runbook には、`runnora:` ブロックに `# TODO(runnora-migrate)` のコメントを付け、TODO として報告します。

## 実例

runnora-e2e のタグ `format-v1`（旧形式のサンプル）と、証跡の自動保存より前の新形式を移行した入力・対応表・結果を、ゴールデンテストのデータとして `internal/migrate/testdata/` に置いています。

| ディレクトリ | 内容 |
|---|---|
| `testdata/e2e-v1/` | 入力（`config*.yaml`・`runbooks/`・`scripts/` と、人が書いた `migrate-scenarios.yaml`） |
| `testdata/e2e-v1-golden/` | 移行後に作成・変更されたファイルと、実行結果のレポート（`MIGRATION_REPORT.txt`） |
| `testdata/e2e-v2/` | 新形式の入力（`runnora.yaml`・`runbooks/`・`scripts/`。`dump` ステップと `exec: runnora-diff` を含む） |
| `testdata/e2e-v2-golden/` | 書き換えた runbook と、実行結果のレポート |
