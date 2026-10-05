# JSON と CSV を multipart で送る

`runnora run` では `multipart(parts)` 関数でパートごとの Content-Type を指定できる。
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

関数の引数は、パート名をキー、以下の descriptor を値とするマップ。

| 項目 | 内容 |
|---|---|
| `value` | 値。`file` とどちらか一方が必要 |
| `file` | ローカルファイルのパス。`value` とどちらか一方が必要 |
| `contentType` | パートの MIME 型。JSON は `application/json`、CSV は `text/csv` など |
| `filename` | ファイル送信時の名前。省略時はパスの basename |

`value` と `application/json`（または `+json` の型）の組み合わせでは JSON にシリアライズする。
JSON オブジェクトを送る場合はマップを渡す。JSON の文字列を渡すと JSON 文字列にシリアライズされる。
既存の JSON ファイルは `file` と `contentType: application/json` で中身をそのまま送れる。
型を省略した値は `text/plain; charset=utf-8`、ファイルは中身による自動判定になる。
CSV や JSON の正確な MIME 型を要求する API には必ず明示する。

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

既存の `image: file://...` など簡易書式の動作は変わらない。
拡張 descriptor を通常の `body: multipart/form-data:` に直接書く方式は未対応。
今回の関数は `run` の実行経路に登録しており、`loadt` への登録は対象外。

**生成する本文全体の上限は 4 MiB（4,194,304 bytes）**。ファイル・JSON・パートのヘッダー・boundary をすべて含む。
ファイル自体が 4 MiB でも、ヘッダーなどが加わるため上限を超える。複数パートの合計にも同じ上限が適用される。
超過すると `multipart: encoded body exceeds maximum size ...` で bind ステップが失敗し、送信ステップは実行されない。
ファイルはサイズ確認と上限付き読み込みを行い、通常ファイル以外は受け付けない。

本文はメモリに読み、runn の YAML 展開でもバイナリを保持できる数値配列で渡す。
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

Spring Boot を起動して curl と比較する検証は、
[runnora-e2e の multipart テスト](https://github.com/ramsesyok/runnora-e2e/tree/main/multipart-test)を参照。
CI では従来方式の 415、型を指定した CSV・JSON ファイル・PNG の 200、誤った型の 415 と
受信したファイルの SHA-256・サイズ、テキスト本文の boundary と証跡を確認する。
日本語ファイル名の受信は OpenAPI リクエスト検証を有効にして確認し、Go のテストで
本文サイズの境界、複数パート合計、超過時に HTTP 送信が起きないことも検証する。

## CI の保守

`.github/workflows/test.yml` の `multipart-spring` は push / pull request ごとにそのコミットの
runnora をビルドし、runnora-e2e の共通ワークフローで Spring Boot E2E を実行する。
既存の Linux の `go test ./...` に加えて、Windows でも multipart builder と
include / loop を通る結合テストを実行する。ログと証跡は `multipart-spring-evidence` artifact で確認できる。

共通ワークフローの `uses: ...@<SHA>` と `e2e-ref` は同じ E2E コミットに固定している。
E2E を更新する場合は、E2E 側のコミットを先に push してから両方の SHA を更新する。
初回も E2E の追加コミットを公開してからこのブランチの CI を実行する。
