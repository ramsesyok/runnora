# サンプル生成 詳細設計：リクエスト JSON・レスポンス JSON の雛形

作成日: 2026-09-27
状態: 詳細設計（レビュー待ち）。実装は未着手。
上位文書: [ツール群 連携設計（方針）](../integration-design.md) の実施順 6（OpenAPI）と実施順 8（gRPC）

## 1. 目的

ワークフロー 2「テストシナリオの雛形と、リクエスト JSON・レスポンス JSON（期待値）の雛形を生成する」を、次の 2 点で改善する。

1. **ファイルの形**：リクエストとレスポンスを、それぞれ独立した JSON ファイルとして出力する。
2. **値の中身**：OpenAPI・proto の定義と制約（enum、format、最小値・最大値など）から決められる値はそれを入れる。決められない文字列だけを `"TODO"` とし、編集が必要な箇所を一目で分かるようにする。gRPC（proto）からも同じように生成する。

## 2. 現状と課題

2026-09-27 時点のコードを調べた結果。

| | runnora `generate` | oapi2wire `init`（runnora `genmock init`） |
|---|---|---|
| リクエスト JSON | ケース JSON の `requestBody` に埋め込み | case YAML の `equalToJson` に埋め込み |
| レスポンス JSON | ケース JSON の `expect.body` に埋め込み | 別ファイル（`mock-responses/<operationId>/<operationId>_default.json`） |
| 値の決め方 | example → examples → schema.example → default → enum の先頭 → 型 | schema.example → 型（allOf / anyOf / oneOf は最初の 1 つ） |
| 型から作る値 | 文字列 `"TODO_string"`（date・date-time・uuid は書式どおり）、数値 `0`、真偽 `false` | 文字列 `"TODO"`、数値 `0`、真偽 `true` |
| 入れ子の上限 | 3 階層。allOf / oneOf は未対応 | 10 階層 |
| gRPC | 生成しない | 対象外 |

見つかった問題:

| # | 問題 | 場所 |
|---|---|---|
| 1 | 同じ OpenAPI から、2 つのツールが違う値の雛形を作る | `runnora/internal/generate/sample.go`、`oapi2wire/internal/openapi/sample_generator.go` |
| 2 | レスポンスの置き場所が 2 系統ある。新しい設計では `mock/responses/` を期待値として共有するので、ケース JSON への埋め込みと合わない | 同上 |
| 3 | `generate --emit-response-example` が効いていない（フラグは読み込まれるが、どこでも使われていない）。**2026-09-27 対応：非推奨にし、指定すると警告を出す。動作は変えない** | `cmd/generate.go`、`internal/generate/service.go` |
| 4 | requestBody のない POST にも `{"TODO": "fill in request body"}` を入れる（e2e のフィードバック #5）。**2026-09-27 修正済み：OpenAPI に requestBody がある operation だけにボディを付ける** | `internal/generate/emitter.go` の `buildCaseData` |
| 5 | 仮の値（`TODO`）と、OpenAPI の example から取った本物の値の区別がファイル上でつかない | ― |
| 6 | proto からは何も生成しない | ― |

## 3. 全体の構成

```text
OpenAPI ─┐
         ├─ サンプル生成（共通ライブラリ：oapi2wire/pkg/sample）
         │     ├─ runnora generate    → リクエスト JSON（cases/）
         │     └─ runnora mock init   → レスポンス JSON（mock/responses/）＋ mock-cases.yaml
proto ───┴─ サンプル生成（runnora/internal/generate/grpcsample）
               └─ runnora generate --proto → リクエスト JSON・期待値 JSON（cases/grpc/）
```

- **OpenAPI のサンプル生成は 1 か所に集約する**。oapi2wire に公開パッケージ `pkg/sample` を作り、runnora はそれを使う（runnora はすでに oapi2wire を依存に持っている）。`runnora/internal/generate/sample.go` は削除する。
  - oapi2wire に置く理由：フロントチームが単体の oapi2wire でモックを作り直したときも、テストの雛形と同じ規則で値が作られるようにするため。
- **proto のサンプル生成は runnora に置く**。oapi2wire は HTTP 専用なので関係しない。proto の解析には、runn がすでに使っている `bufbuild/protocompile` を直接使う（新しいモジュールは増えない）。

## 4. 出力するファイル

ファイル構成は [連携設計の 5.4](../integration-design.md#54-テストプロジェクトのファイル構成) に従う。

### 4.1 OpenAPI（HTTP）

```text
cases/generated/<tag>/<method>_<operationId>/
├─ default.json                    ケース（pathParams・query・headers・expect.mock）
└─ default.request.json            リクエストボディ（requestBody がある operation だけ）

mock/
├─ mock-cases.yaml                 <operationId>_default のモックケースを追記
└─ responses/<operationId>/
   ├─ <operationId>_default.json   代表レスポンス（最初の 2xx）の本文
   └─ <operationId>_<status>.json  その他のステータス（--all-statuses を指定したとき）
```

- **リクエストボディは常に別ファイル**（`<ケース名>.request.json`）にする（2026-09-27 決定）。ケース JSON には、パス・クエリ・ヘッダと期待値の参照だけを持たせる。
- **レスポンスの本文はケース JSON に埋め込まず、`mock/responses/` に出す**。ケースは `expect.mock: <operationId>_default` でモックケースを参照し、期待本文はそこから読む（連携設計 5.3）。
- 生成した suite は、include の `vars` でケースとリクエストボディの両方を渡す。

  ```yaml
  steps:
    default:
      include:
        path: ./post_createBook.template.yml
        vars:
          case: json://../../../cases/generated/books/post_createBook/default.json
          request: json://../../../cases/generated/books/post_createBook/default.request.json
  ```

  template は `body: { application/json: "{{ vars.request }}" }` で送る。runn は JSON ファイルの中に書いた `json://` を展開しないので、ケース JSON から参照させるのではなく、suite の `vars` に並べる。
- requestBody のない operation には、`.request.json` を作らず、template にも `body` を書かない（問題 #4 の修正）。
- `--all-statuses`：OpenAPI に書かれたすべてのレスポンスステータス（400・404・409 など）について、レスポンス JSON と、**コメントアウトした**モックケース（`request` の条件は `TODO`）を `mock-cases.yaml` に追記する。oapi2wire の case YAML には無効化の項目がないため、コメントにしておき、使うときにコメントを外して条件を書く。異常系の雛形作りを楽にするための選択機能で、既定では代表レスポンスだけを作る（2026-09-27 決定）。
- `--emit-response-example` は廃止する（レスポンスは常に `mock/responses/` に出すため。問題 #3）。

### 4.2 gRPC（proto）

```text
cases/grpc/<package>.<Service>/<Method>/
├─ default.request.json            送信メッセージ
└─ default.expected.json           期待値（ステータスと受信メッセージ）
```

- 期待値ファイルの形は、いまの e2e の grpc-test と同じにする。

  ```json
  { "status": 0, "message": { "book_id": "TODO", "title": "TODO" } }
  ```

  Server streaming では `"messages": [ ... ]`（1 要素）とする。
- JSON の形は、runn が gRPC の応答を JSON にするときの設定に合わせる。runn v1.9.2 は `protojson.MarshalOptions{UseProtoNames: true, UseEnumNumbers: true, EmitUnpopulated: true}` を使っている（`grpc.go`）。そのため次のようにする。
  - フィールド名は **proto に書いた名前**（`book_id`）。lowerCamelCase（`bookId`）には変換しない（e2e grpc-test の README 5 と同じ）。
  - enum は**数値**で書く。
  - 64 bit の整数は protojson の仕様どおり**文字列**で書く。
  - 値を入れていないフィールドも応答には現れるので、期待値ファイルにはすべてのフィールドを書く（6 章の規則ではすべてのフィールドに値を入れる）。
- リクエストのファイルも、期待値と同じ書き方（proto の名前、enum は数値、64 bit は文字列）にそろえる。runn はリクエストを `protojson.Unmarshal` で読むので、この書き方で送れる。
- 対象は Unary と Server streaming（docgen の対応範囲と同じ）。Client streaming と双方向 streaming は、ファイルは作るが template は作らず、警告を出す。

### 4.3 上書きの規則

| 出力先 | 既存のファイルがあるとき |
|---|---|
| `runbooks/generated/`、`cases/generated/` | 上書きする（再生成する場所なので、手で編集しない） |
| `mock/mock-cases.yaml` | 既存のケースは変えない。ないモックケース（`<operationId>_default`）だけを末尾に追記する |
| `mock/responses/` | 上書きしない（テストチームが編集する場所）。`--force` のときだけ上書きする |
| `cases/grpc/` | 上書きしない。`--force` のときだけ上書きする |

`runnora generate` と `runnora mock init` は、どちらも同じ `mock/responses/<operationId>/<operationId>_default.json` を作る。先に実行した方がファイルを作り、後から実行した方は既存のファイルを使う。どちらから始めてもよい。

## 5. OpenAPI の値の決め方

### 5.1 優先順位

上から順に探し、最初に見つかったものを使う。

| 順 | 取り出し元 | 備考 |
|---|---|---|
| 1 | media type の `example` | リクエスト・レスポンス全体の例 |
| 2 | media type の `examples` の最初の値 | |
| 3 | schema の `example`（3.1 では `examples` の最初） | プロパティごとに再帰して見る |
| 4 | schema の `default` | |
| 5 | schema の `const` | 3.1 |
| 6 | schema の `enum` の最初の値 | |
| 7 | `format` から作る値 | 5.2 |
| 8 | 制約と型から作る値 | 5.3 |

パラメータ（パス・クエリ・ヘッダ）は、parameter の `example` → `examples` → schema の順に同じ規則で作る。

### 5.2 `format` から作る値

乱数は使わず、毎回同じ値を出す（生成物を Git で差分管理するため）。

| format | 値 |
|---|---|
| `date` | `"2026-01-01"` |
| `date-time` | `"2026-01-01T00:00:00Z"` |
| `time` | `"00:00:00"` |
| `email` | `"user@example.com"` |
| `uri` / `url` | `"https://example.com/"` |
| `hostname` | `"example.com"` |
| `ipv4` / `ipv6` | `"192.0.2.1"` / `"2001:db8::1"`（文書用に予約されたアドレス） |
| `uuid` | `"00000000-0000-4000-8000-000000000001"` |
| `byte` | `"c2FtcGxl"`（"sample" の base64） |
| `binary` | `"TODO: path/to/file"`（multipart のファイル項目。いまと同じ） |
| `int32` / `int64` / `float` / `double` | 5.3 の数値の規則に従う |

### 5.3 制約と型から作る値

| 型 | 値 |
|---|---|
| string | **`"TODO"`**（2026-09-27 決定）。編集していない箇所が一目で分かり、`TODO` で検索できるようにするため。`minLength` / `maxLength` に合わなくても `"TODO"` のままにする（編集を前提とした目印なので）。`pattern` がある場合は 5.4 |
| integer | **`0`**（2026-09-27 決定）。`minimum` / `maximum` / `exclusiveMinimum` / `exclusiveMaximum` が 0 を許さない場合だけ、0 に最も近い許される値にする（例：`minimum: 1` なら `1`、`exclusiveMinimum: 0` なら `1`、`maximum: -1` なら `-1`）。`multipleOf` は 0 が常に満たすので、範囲で 0 以外になったときだけ、条件を満たす 0 に最も近い倍数にする |
| number | **`0.0`**（2026-09-27 決定）。範囲の扱いは integer と同じ（`exclusiveMinimum: 0` のように 0 に最も近い値が決まらない場合は、境界 + 1 の `1.0` とする）。JSON に `0` ではなく `0.0` と書き出す（5.6） |
| boolean | `true` |
| array | `minItems`（なければ 1）個の要素を作る。`uniqueItems` で要素が文字列なら `"TODO-1"`、`"TODO-2"` のように番号を付けて重複を避ける |
| object | `properties` を順に作る。`additionalProperties` だけの object は `{"TODO": <値>}` |
| null を許す型（`nullable: true`、`type: [string, "null"]`） | null ではない方の型で作る |

構成の扱い:

- `allOf`：すべての schema の `properties` と `required` を**合成**してから作る（いまの oapi2wire は最初の 1 つしか見ていない）。
- `oneOf` / `anyOf`：最初の選択肢を使う。`discriminator` があれば、最初の `mapping` の値を判別プロパティに入れる。
- `readOnly` のプロパティはリクエストに含めない。`writeOnly` のプロパティはレスポンスに含めない。
- `$ref` の循環は、同じ schema が経路上に 2 回目に現れた時点で打ち切る（`null` にする）。深さの上限は 10 とする。
- `required` にないプロパティも含める（既定）。`--sample-fields required` を指定すると、`required` のプロパティだけを作る。

### 5.4 `pattern`

- 文字クラス、量指定子、選択（`|`）、グループからなる**よくある正規表現**は、決まった規則で一致する文字列を作る（例：`^[0-9]{3}-[0-9]{4}$` → `"000-0000"`）。
- 後方参照や先読みなど作れない表現のときは、`"TODO: pattern ^...$"` とし、5.5 の一覧に載せる。
- 作った値が `pattern` に一致するかを Go の `regexp` で確かめ、一致しなければ同じく `TODO` 扱いにする。

### 5.5 仮の値の一覧

OpenAPI の example 由来の値と、規則で作った仮の値を区別できるようにする（問題 #5）。

- 生成のたびに、**規則で作った値（5.1 の 7・8）の場所**を operation ごとに一覧にして表示する。例：

  ```text
  createBook  request  .title, .isbn        (example なし: 規則で生成)
  createBook  response .bookId              (example なし: 規則で生成)
  createBook  response .note                (TODO: pattern を満たす値を作れない)
  ```

- `--emit-manifest` を指定したときは、同じ一覧を manifest に入れる。
- `runnora validate` は、`cases/`・`mock/responses/` に残っている `"TODO` で始まる値を警告する。

### 5.6 浮動小数の書き出し

JSON には整数と浮動小数の区別がなく、Go の `encoding/json` は浮動小数の 0 も `0` と書き出す。雛形では型が分かるように、浮動小数（OpenAPI の `number`、proto の `float` / `double`）の値は**小数点を付けて書き出す**（`0.0`、`1.0`）。

- 規則で作った値だけでなく、example から取った値も、`number` 型なら小数点を付ける（例：example が `3` なら `3.0`）。
- 読み込む側（runn の `compare`、runnora-diff）は `0` と `0.0` を同じ数値として扱うので、比較の結果には影響しない。
- 実装では、生成した値を書き出すときに浮動小数を `json.Number` に変換してから出力する。

## 6. proto の値の決め方

proto には example を書く標準の方法がないため、型から作る。

| proto の型 | 値 |
|---|---|
| `string` | `"TODO"`（5.3 と同じ） |
| `bytes` | `"c2FtcGxl"`（base64） |
| `int32` / `uint32` / `sint32` / `fixed32` / `sfixed32` | `0` |
| `int64` / `uint64` / `sint64` / `fixed64` / `sfixed64` | `"0"`（protojson の仕様で文字列） |
| `float` / `double` | `0.0`（5.6） |
| `bool` | `true` |
| `enum` | 値が 0 ではない最初の値の**番号**（0 は `*_UNSPECIFIED` とする慣例のため）。0 しかなければ 0。5.5 の一覧には値の名前も載せる |
| message | フィールドを順に再帰して作る |
| `repeated` | 1 要素 |
| `map` | 1 エントリ（キーは型に応じた値） |
| `oneof` | 最初のフィールドだけを作る |
| `optional` | 値を入れる |
| `google.protobuf.Timestamp` | `"2026-01-01T00:00:00Z"` |
| `google.protobuf.Duration` | `"1s"` |
| ラッパー型（`StringValue` など） | 中身の型の値 |
| `google.protobuf.Struct` / `Value` / `Any` | `{}`（`TODO` として一覧に載せる） |

- 再帰する message は、同じ message が経路上に 2 回目に現れた時点で打ち切る。
- 期待値ファイルの `status` は `0`（OK）とする。
- 5.5 と同じく、すべての値が規則で作った仮の値なので、一覧にはファイル単位で「proto から生成（要編集）」と表示する。

## 7. CLI

| コマンド | 変更 |
|---|---|
| `runnora generate` | `--proto <file>`（複数可）と `--import-path` を追加。`--all-statuses`、`--sample-fields` を追加。`--emit-response-example` を廃止 |
| `runnora mock init` | 共通のサンプル生成を使う。`--all-statuses`、`--sample-fields` を追加 |
| `oapi2wire init` | 同上（単体 CLI でも同じ規則で作る） |
| `runnora validate` | 残っている `TODO` の値を警告（5.5） |

`runnora.yaml` の `generate` セクションに `proto`、`import_paths`、`all_statuses`、`sample_fields` を追加し、CLI の指定を優先する。

## 8. 実装の配置

| リポジトリ | パッケージ | 内容 |
|---|---|---|
| oapi2wire | `pkg/sample`（新規・公開） | 5 章の規則。入力は libopenapi の schema、出力は Go の値と「仮の値の場所」の一覧 |
| oapi2wire | `internal/openapi/sample_generator.go` | 削除し、`pkg/sample` を使う |
| runnora | `internal/generate/sample.go` | 削除し、`oapi2wire/pkg/sample` を使う |
| runnora | `internal/generate/grpcsample`（新規） | 6 章の規則。proto の解析は `bufbuild/protocompile` |
| runnora | `internal/generate/emitter.go` | 4.1 のファイル構成で出力。requestBody のない operation の扱い（問題 #4） |

リリースの順序：oapi2wire に `pkg/sample` を追加してタグを打つ → runnora の `go.mod` を更新して切り替える。

## 9. テスト方針

- 5 章・6 章の規則を、それぞれ表のとおりテーブル駆動のテストで確かめる。
- 生成した値が、元の schema で**検証に通る**ことを確かめる（libopenapi の schema 検証を使う）。規則のバグで制約違反の値を作っていないことの確認になる。
- `testdata/` の OpenAPI と proto から、生成物一式をゴールデンファイルで比較する（同じ入力なら同じ出力になること）。
- runnora と oapi2wire が、同じ operation について同じレスポンス JSON を作ることを確かめる。

## 10. 実施順への組み込み

| 実施順 | 追加する内容 |
|---|---|
| 6（契約ケースとモック参照の統一） | OpenAPI のサンプル生成の共通化と改善（3〜5 章、7 章）、ファイル構成の変更（4.1）、問題 #3・#4 の修正 |
| 8（`generate --proto`） | proto のサンプル生成（4.2、6 章） |

問題 #3（効かないフラグ）と #4（ボディのない POST）は小さな修正なので、実施順 6 を待たずに先に直してもよい。

## 11. 決定事項

1. **文字列の仮の値**：**決定（2026-09-27）：`"TODO"`**（5.3 のとおり）。編集していない箇所が一目で分かり、検索もできる。そのまま送ると API の入力チェックで弾かれやすいが、雛形は編集して使う前提なので問題としない。数値・真偽値・format のある文字列など、規則で作った `TODO` 以外の値は 5.5 の一覧で区別する。
2. **リクエストボディを常に別ファイルにするか**：**決定（2026-09-27）：常に別ファイル**（4.1 のとおり）。置き場所を 1 通りにして、ツール・docgen・利用者の扱いを単純にする。小さいボディでもファイルが 2 つになることは許容する。
3. **`--all-statuses` の既定**：**決定（2026-09-27）：既定では代表レスポンスだけを作り、異常系は `--all-statuses` を指定したときだけ作る**（4.1 のとおり）。使わない雛形を残さないため。
