# JSON と CSV を multipart で送る

runnora v0.5.0 以降の `runnora run` では `multipart(parts)` 関数でパートごとの Content-Type を指定できる。
組み込む runn のバージョンやコードを変更する必要はない。

`runnora.yaml` のあるディレクトリに `fixtures/data.csv` を置き、runbook に次のように書く。
`API_URL` はプロジェクトの環境変数で接続先（例：`http://localhost:8080`）を設定する。
パート名 `metadata` / `file` と URL `/api/upload` は、対象 API の仕様に合わせる。

```yaml
runnora:
  id: CSV-UPLOAD
runners:
  req: ${API_URL}
vars:
  metadata:
    name: 取込データ
    enabled: true
steps:
  prepare:
    bind:
      upload: 'multipart({"metadata": {"contentType": "application/json", "value": vars.metadata}, "file": {"contentType": "text/csv", "file": "fixtures/data.csv"}})'
  send:
    req:
      /api/upload:
        post:
          headers:
            Accept: application/json
            Content-Type: "{{ upload.contentType }}"
          body:
            application/octet-stream: "{{ upload.body }}"
    test: current.res.status == 200
```

`runnora run runbooks/upload.yml`、またはこの runbook を選択するスイートで実行する。

`prepare` が型付きの各パートと boundary を生成する。`send` の `application/octet-stream` は
runn の生本文送信を選ぶためのキーで、実際の HTTP Content-Type は `upload.contentType`
（`multipart/form-data; boundary=...`）になる。本文と Content-Type は同じ呼び出しの戻り値を使う。
`Accept` は受け取りたい応答の形式を指定するので、JSON と CSV の混在にかかわらず `application/json`。

## パートの指定

関数の引数は、パート名をキー、各パートの設定（descriptor）を値とするマップ。

### 固定のキーと任意の名前

| 記載する場所 | 例 | 固定 / 任意 |
|---|---|---|
| 関数名 | `multipart` | 固定 |
| 引数の外側のキー（パート名） | `metadata`、`metadata1`、`metadata2`、`file`、`csv` | 任意。API の受信パート名に合わせる |
| 各パートの設定キー | `value`、`file`、`contentType`、`filename` | 固定。大文字・小文字もこの表記に合わせる |
| bind 変数名 | `upload` | 任意。送信ステップの参照も同じ名前にする |
| 関数の戻り値のキー | `body`、`contentType` | 固定。`upload.body`、`upload.contentType` と参照する |
| 送信ステップの `body` 内のキー | `application/octet-stream` | この送信例ではそのまま使用し、runn の生本文送信を選ぶ |

例えば、次の `csv` は任意のパート名で、内側の `file` と `contentType` は固定の設定キー。

```text
multipart({"csv": {"file": "fixtures/data.csv", "contentType": "text/csv"}})
```

パート名に `file` を使った `{"file": {"file": "fixtures/data.csv"}}` では、
**外側の `file` は任意のパート名、内側の `file` は固定の読み込み方法の指定**。
`file` という名前のパートが必須という意味ではない。
パート名を `csv` にした場合、Spring Boot 側も `@RequestPart("csv")` として受け取る。

### 各パートの設定

| 固定キー | 指定条件と内容 |
|---|---|
| `value` | 本文を作るための値。`file` とどちらか一方だけが必要 |
| `file` | 読み込むローカルファイルのパス。`value` とどちらか一方だけが必要 |
| `contentType` | パートの MIME 型。省略可能だが、JSON は `application/json`、CSV は `text/csv` など API が要求する型を明示する |
| `filename` | `file` 指定時の送信ファイル名。省略時はパスの basename。`value` だけのパートに送信ファイル名は指定できない |

`value` / `file` は、**この関数が本文の作り方を選ぶための設定キー**。
HTTP multipart の標準仕様（[RFC 7578](https://www.rfc-editor.org/rfc/rfc7578.html#section-4.2)）に
この設定キーがあるわけではない。
同じパートに両方を指定した場合、どちらもない場合、未知の設定キー（例：`content_type`）を
指定した場合は bind ステップで失敗し、送信しない。

`value` と `application/json`（または `+json` の型）の組み合わせでは JSON にシリアライズする。
JSON オブジェクトを送る場合はマップを渡す。JSON の文字列を渡すと JSON 文字列にシリアライズされる。
既存の JSON ファイルは `file` と `contentType: application/json` で中身をそのまま送れる。
型を省略した値は `text/plain; charset=utf-8`、ファイルは中身による自動判定になる。
CSV や JSON の正確な MIME 型を要求する API には必ず明示する。

### JSON オブジェクトは事前に文字列化しない

**JSON オブジェクトを送るときは、元の値を `value` に渡し、JSON 化は `multipart()` に任せる。**
例えば Spring の `@RequestPart("metadata") Metadata` のように DTO として受け取る API では、
次の指定を使う。`vars.metadata` は最初の例の YAML マップ。

```yaml
prepare:
  bind:
    upload: 'multipart({"metadata": {"contentType": "application/json", "value": vars.metadata}, "file": {"contentType": "text/csv", "file": "fixtures/data.csv"}})'
```

次の事前変換は、JSON オブジェクトを受け取る API への送信では避ける。

```yaml
# DTO を受け取る API では使わない：JSON を二重にシリアライズする
prepare:
  bind:
    upload: 'multipart({"metadata": {"contentType": "application/json", "value": toJSON(vars.metadata)}, "file": {"contentType": "text/csv", "file": "fixtures/data.csv"}})'
```

最初の例の metadata なら、パート本文の違いは次のとおり（空白・改行を省略。JSON のキー順は問わない）。

| `value` に渡す式 | パート本文 | JSON としての型 |
|---|---|---|
| `vars.metadata` | `{"enabled":true,"name":"取込データ"}` | オブジェクト。DTO へ変換できる形式 |
| `toJSON(vars.metadata)` | `"{\"enabled\":true,\"name\":\"取込データ\"}"` | 文字列。今回の Spring DTO では 400 |

`toJSON()` は JSON テキストが必要な用途で使う関数。
ここで避けるのは、`multipart()` が JSON 化する値を先に JSON テキストへ変換すること。
API が JSON 文字列を要求する場合は、`value` に文字列を渡す指定が正しい。
JSON オブジェクト・配列・文字列など、API が要求する JSON の型に合わせる。
既存の JSON ファイルなら `file` と `contentType: application/json` を使い、中身をそのまま送れる。

送信時も `application/octet-stream: "{{ upload.body }}"` の参照を使う。
`upload.body` は生成済み本文のバイト列を保持する戻り値なので、追加の `toJSON()` は不要。

### HTTP リクエストに現れる内容

設定用の `value` / `file` キーは、そのまま HTTP リクエストには出力しない。
その指定から作った本文とヘッダーを送る。API は次の情報を受け取る。

| 関数に指定した情報 | HTTP パートに現れる内容 |
|---|---|
| パート名 `metadata1`、`csv` など | `Content-Disposition: form-data; name="metadata1"` などの `name` |
| `contentType` | パートごとの `Content-Type` ヘッダー |
| `value` | 値から生成した本文。JSON 型なら JSON にシリアライズした内容 |
| `file` | 読み込んだファイルの内容と、`Content-Disposition` の `filename` |
| `filename` | 上記 `filename` を明示した送信名にする |

前の `csv` の例は、次のようなパートを作る（外側に boundary が付く）。

```http
Content-Disposition: form-data; name="csv"; filename="data.csv"
Content-Type: text/csv

<CSV ファイルの内容>
```

パート名自体を `file` にしていれば `name="file"` として現れる。
JSON データ自身に `value` / `file` というプロパティがあれば、そのプロパティも JSON の本文に現れる。
設定キーと、パート名・JSON データのプロパティ名は区別する。

### 複雑な JSON と複数の JSON パート

JSON のオブジェクト・配列をネストした値を渡せる。数値・boolean・null も JSON として送る。
`metadata1` / `metadata2` のように別名のパートを追加すれば、複数の JSON と CSV を同じ本文に含められる。
最初の runbook の `vars` と `prepare` を次のように置き換える。
送信ステップでは引き続き同じ `upload.body` と `upload.contentType` を使う。

```yaml
vars:
  metadata1:
    customer:
      name: 山田太郎
      address:
        city: 東京
    items:
      - code: A001
        quantity: 2
  metadata2:
    options:
      enabled: true
      tags: [a, b]
    optional: null
steps:
  prepare:
    bind:
      upload: 'multipart({"metadata1": {"contentType": "application/json", "value": vars.metadata1}, "metadata2": {"contentType": "application/json", "value": vars.metadata2}, "csv": {"contentType": "text/csv", "file": "fixtures/data.csv"}})'
```

`value` に `vars.metadata1` のような値を直接渡す。
事前の文字列化を避ける理由は [JSON オブジェクトは事前に文字列化しない](#json-オブジェクトは事前に文字列化しない) を参照。
Java 側も `@RequestPart("metadata1")`、`@RequestPart("metadata2")`、`@RequestPart("csv")` と
対応する型を定義する必要がある。JSON のフィールドや型が DTO と合わなければ、受信時にエラーになり得る。

引数はマップなので、同じパート名を繰り返して複数パートを送る指定は未対応。
パートは名前でソートして送るため、引数の記載順を送信順として使わない。

### JSON ファイルと送信ファイル名を指定する

既に JSON ファイルがある場合は、`value` の代わりに `file` を使う。
CSV の送信名を指定する場合は `filename` を加える。送信ステップは上の例と同じ。
日本語ファイル名は UTF-8 の `filename="取込.csv"` として送る。
引用符とバックスラッシュはエスケープし、multipart/form-data で禁止されている `filename*` は使わない。

```yaml
prepare:
  bind:
    upload: 'multipart({"metadata": {"contentType": "application/json", "file": "fixtures/metadata.json"}, "file": {"contentType": "text/csv", "file": "fixtures/data.csv", "filename": "import.csv"}})'
```

画像を送る場合も同じ形式で、ファイルパートを
`{"contentType": "image/png", "file": "fixtures/cover.png"}` に置き換える。
応答が JSON の API なら `Accept: application/json` のままでよい。

### ファイルパスと対応範囲

**関数内のファイルパスは `runnora.yaml` のあるプロジェクトルート基準**。
プロジェクトファイルがない場合は CLI のカレントディレクトリ基準。
通常の `body: multipart/form-data:` の runbook 相対パスとは異なる。
`file://` 接頭辞付きのローカルパスも使用できるが、vars での file:// 展開と混同しないため
この関数では通常のパスを推奨する。include・loop 内も同じプロジェクトルートを基準にする。

v0.5.0 は旧形式の `config.yaml` を読み込まない。旧構成の runbook を単独で実行する場合も、
`runnora.yaml` がなければ CLI のカレントディレクトリを基準にする。
旧 `config.yaml` の設定を引き継ぐには `runnora.yaml` に移行する。
ファイル名の変更だけでは移行できないため、[移行手順](../migrate.md)を参照。

既存の `image: file://...` など簡易書式の動作は変わらない。
拡張 descriptor を通常の `body: multipart/form-data:` に直接書く方式は未対応。
今回の関数は `run` の実行経路に登録しており、`loadt` への登録は対象外。
パートは少なくとも一つ必要で、パート名は空にできず、制御文字は使えない。

**生成する本文全体の上限は 4 MiB（4,194,304 bytes）**。ファイル・JSON・パートのヘッダー・boundary をすべて含む。
ファイル自体が 4 MiB でも、ヘッダーなどが加わるため上限を超える。複数パートの合計にも同じ上限が適用される。
超過すると `multipart: encoded body exceeds maximum size ...` で bind ステップが失敗し、送信ステップは実行されない。
ファイルはサイズ確認と上限付き読み込みを行い、通常ファイル以外は受け付けない。

本文はメモリに読み、runn の YAML 展開でもバイナリを保持できる数値配列で渡す。
`application/octet-stream: "{{ upload.body }}"` は、runn の展開処理で数値配列に復元され、
HTTP 送信時にバイト列に変換される。数値配列の YAML テキストを本文に送る指定ではない。
`upload.body` を `toJSON()` などで文字列化せず、戻り値をそのまま参照する。
この変換は元データより多くのメモリを使うため上限を設けており、設定で解除する機能はない。
ストリーミング送信ではない。既にメモリ上にある JSON 値はシリアライズ後にサイズを検査する。
通常のキャプチャ・OpenAPI 検証・HTTP ステップの結果確認は利用できる。
`evidence.mode: full` の証跡には HTTP ヘッダーとテキスト形式の multipart 本文も保存される。
PNG などのバイナリを含む本文は、既存の証跡機能に従ってサイズの要約になる。

## Spring Boot での受信

```java
@PostMapping(value = "/api/upload",
    consumes = MediaType.MULTIPART_FORM_DATA_VALUE,
    produces = MediaType.APPLICATION_JSON_VALUE)
public Result upload(@RequestPart("metadata") Metadata metadata,
                     @RequestPart("file") MultipartFile file) {
    // CSV を解析して処理する
}
```

文字列パートの Content-Type が省略されると、Spring の JSON → DTO 変換で 415 になる場合がある。
この関数は metadata に `application/json` を付け、DTO として受け取れる本文を送る。

## curl からの置き換えと 415 の確認

次の curl と同じ型指定は、このページの JSON ファイルの例で表せる。

```sh
curl http://localhost:8080/api/upload \
  -H 'Accept: application/json' \
  -F 'metadata=@fixtures/metadata.json;type=application/json' \
  -F 'file=@fixtures/data.csv;type=text/csv;filename=import.csv'
```

| 指定する場所 | 値 | 意味 |
|---|---|---|
| リクエストの `Accept` | `application/json` | API の応答形式 |
| リクエストの `Content-Type` | `{{ upload.contentType }}` | multipart の形式と本文に対応する boundary |
| metadata の `contentType` | `application/json` | Spring が JSON を DTO に変換するための型 |
| file の `contentType` | `text/csv` / `image/png` | API が受け付けるファイルパートの型 |

415 が続く場合は、全体の Content-Type に boundary が付いているか、本文も同じ `upload`
から取っているか、各パートの型・名前が API の仕様と一致するかを確認する。
`Content-Type: multipart/form-data` の固定値は使わず、生成した `upload.contentType` を渡す。
`Accept` を `multipart/form-data` に変更しても、アップロードのパート型は変わらない。

### 送信前の失敗とサーバの応答を区別する

以下は runnora v0.5.0 / Java 17 / Spring Boot 2.7.15 / OpenAPI Generator 7.26.0 で
確認した例。JSON を DTO として受け取る生成 Spring API に対して、失敗する段階が異なる。

| 指定・問題 | 失敗する段階 | 確認した結果 |
|---|---|---|
| 通常の `body: multipart/form-data:` に `"{{ toJSON(vars.metadata) }}"` を指定 | クライアントの本文生成中。HTTP 送信前 | `http request failed ... invalid body: map[...]` |
| `multipart()` の JSON パートの `value` に `toJSON(vars.metadata)` を渡す | サーバの JSON → DTO 変換中 | HTTP 400 |
| JSON パートの Content-Type を省略、または `text/plain` にする | サーバがパートの型を判定するとき | HTTP 415 |

一つ目の再現は、次の通常 multipart の書き方で起きた。

```yaml
# この JSON のテンプレート展開では送信前に失敗する
body:
  multipart/form-data:
    metadata: "{{ toJSON(vars.metadata) }}"
```

展開後の値がマップになり、通常 multipart のエンコーダーが受け付けない。
このページの `multipart()` と生本文送信を組み合わせた例に置き換える。
`http request failed` という表示だけで原因は断定できないため、後続の詳細を確認する。
今回確認した送信前の失敗には `invalid body: map[...]` が続き、HTTP 応答はない。
400 / 415 はサーバから受け取った HTTP ステータスであり、上記の送信前エラーとは区別する。

Spring Boot を起動して curl と比較する検証は、
[runnora-e2e の multipart テスト](https://github.com/ramsesyok/runnora-e2e/tree/main/multipart-test)を参照。
CI では従来方式の 415、型を指定した CSV・JSON ファイル・PNG の 200、誤った型の 415 と
受信したファイルの SHA-256・サイズ、テキスト本文の boundary と証跡を確認する。
日本語ファイル名の受信は OpenAPI リクエスト検証を有効にして確認し、Go のテストで
本文サイズの境界、複数パート合計、超過時に HTTP 送信が起きないことも検証する。

OpenAPI 定義から Spring の API / DTO を生成する検証は、
[OpenAPI 生成 Spring の multipart テスト](https://github.com/ramsesyok/runnora-e2e/tree/main/multipart-openapi-test)を参照。
Java 17 / Spring Boot 2.7.15 / OpenAPI Generator 7.26.0 で、JSON 2 パートと text/csv の
受信内容を curl と比較する。二重シリアライズによる 400、パート型による 415、
通常 multipart のテンプレート展開による送信前エラーも確認する。

## CI の保守

`.github/workflows/test.yml` の `multipart-spring` は push / pull request ごとにそのコミットの
runnora をビルドし、runnora-e2e の共通ワークフローで Spring Boot E2E を実行する。
既存の Linux の `go test ./...` に加えて、Windows でも multipart builder と
include / loop を通る結合テストを実行する。ログと証跡は `multipart-spring-evidence` artifact で確認できる。

共通ワークフローの `uses: ...@<SHA>` と `e2e-ref` は同じ E2E コミットに固定している。
E2E を更新する場合は、E2E 側のコミットを先に push してから両方の SHA を更新する。
初回も E2E の追加コミットを公開してからこのブランチの CI を実行する。
