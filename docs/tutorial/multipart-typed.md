# JSON と CSV を multipart で送る

`runnora run` では `multipart(parts)` 関数でパートごとの Content-Type を指定できる。
組み込む runn のバージョンやコードを変更する必要はない。

```yaml
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
```

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

**関数内のファイルパスは `runnora.yaml` のあるプロジェクトルート基準**。
プロジェクトファイルがない場合は CLI のカレントディレクトリ基準。
通常の `body: multipart/form-data:` の runbook 相対パスとは異なる。
`file://` 接頭辞付きのローカルパスも使用できるが、vars での file:// 展開と混同しないため
この関数では通常のパスを推奨する。include・loop 内も同じプロジェクトルートを基準にする。

既存の `image: file://...` など簡易書式の動作は変わらない。
拡張 descriptor を通常の `body: multipart/form-data:` に直接書く方式は未対応。
今回の関数は `run` の実行経路に登録しており、`loadt` への登録は対象外。

本文はファイル全体をメモリに読み、runn の YAML 展開でもバイナリを保持できる数値配列で渡す。
ストリーミング送信ではない。通常のキャプチャ・OpenAPI 応答検証・HTTP ステップの結果確認は利用できる。
`evidence.mode: full` の証跡には multipart 本文も保存される。

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
