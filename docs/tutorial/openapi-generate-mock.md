# OpenAPI 定義から runnora のテストと WireMock モックをそろえる

このチュートリアルでは、`runnora generate` で作る API テスト資産と、`runnora genmock` で作る WireMock モック資産を、同じ OpenAPI 定義から育てる流れを説明します。

題材には、前のチュートリアルと同じ `docs/tutorial/openapi.yaml` の Petstore OpenAPI 定義を使います。`runnora generate` で case JSON と runbook を作り、`runnora genmock` で WireMock の `mappings/` と `__files/` を作ります。

```text
docs/tutorial/openapi.yaml
  |
  +-- runnora generate
  |     |
  |     +-- practice/runbooks/generated/
  |     +-- practice/cases/generated/
  |
  +-- runnora genmock
        |
        +-- practice/mock-cases.yaml
        +-- practice/mock-responses/
        +-- practice/wiremock-out/
              |
              +-- mappings/
              +-- __files/
```

`practice/cases/generated/` の case JSON は、runnora が送るリクエストと期待値を表します。`practice/mock-cases.yaml` と `practice/mock-responses/` は、WireMock がどのリクエストにどのレスポンスを返すかを表します。

この 2 つを別々に考えすぎると、runnora の期待レスポンス JSON と WireMock のレスポンス JSON を二重管理しがちです。このチュートリアルでは、レスポンス body の実体は `mock-responses/` に寄せ、runnora 側の case JSON には検証したい一部だけを書く運用にします。

## 前提条件

リポジトリルートから実行する前提で説明します。

必要なものは次のとおりです。

| 必要なもの | 用途 |
|---|---|
| `runnora` | `generate`、`genmock`、`run` を実行する |
| Java | WireMock standalone を起動する |
| WireMock standalone jar | `wiremock-out/` を読み込んでモックサーバを起動する |
| `docs/tutorial/openapi.yaml` | テストとモックの元になる OpenAPI 定義 |

WireMock standalone jar は Git に登録しない前提です。ここでは `docs/tools/wiremock-standalone-3.13.2.jar` に置いたものとして説明します。

jar は Maven Central から取得します。

```bash
mkdir -p docs/tools
curl -L -o docs/tools/wiremock-standalone-3.13.2.jar \
  https://repo1.maven.org/maven2/org/wiremock/wiremock-standalone/3.13.2/wiremock-standalone-3.13.2.jar
```

## 作成するもの

このチュートリアルでは、次のファイルとディレクトリを作ります。

```text
practice/
├─ runbooks/generated/pet/
├─ cases/generated/pet/
├─ config.yaml
├─ mock-cases.yaml
├─ mock-responses/
└─ wiremock-out/
   ├─ mappings/
   └─ __files/
```

役割は次のとおりです。

| パス | 役割 |
|---|---|
| `practice/runbooks/generated/pet/` | runnora が API を呼ぶ template / suite runbook |
| `practice/cases/generated/pet/` | runnora が送るリクエスト値と期待値 |
| `practice/config.yaml` | runnora run 用の設定ファイル |
| `practice/mock-cases.yaml` | WireMock の返し分け条件 |
| `practice/mock-responses/` | WireMock が返すレスポンス body |
| `practice/wiremock-out/` | WireMock が読み込む生成済みファイル |

## 1. runnora のテスト資産を生成する

まず `runnora generate` で、Petstore の `pet` tag からテスト資産を生成します。

```bash
mkdir -p practice

runnora generate \
  --openapi docs/tutorial/openapi.yaml \
  --out practice \
  --tags pet \
  --skip-deprecated \
  --clean \
  --force
```

生成後、代表的なファイルは次のようになります。

```text
practice/runbooks/generated/pet/get_findPetsByStatus.template.yml
practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
practice/cases/generated/pet/get_findPetsByStatus/default.json

practice/runbooks/generated/pet/get_getPetById.template.yml
practice/runbooks/generated/pet/get_getPetById.suite.yml
practice/cases/generated/pet/get_getPetById/default.json
```

`runnora generate` の生成物は再生成前提です。業務テストとして手で育てる場合は `runbooks/evidence/` などへコピーしますが、このチュートリアルでは `generated/` のまま WireMock に接続して流れを確認します。

## 2. WireMock 用の雛形を生成する

次に、同じ OpenAPI 定義から `runnora genmock init` で mock case YAML と response stub を生成します。

```bash
runnora genmock init \
  --openapi docs/tutorial/openapi.yaml \
  --out-cases practice/mock-cases.yaml \
  --responses-root practice/mock-responses \
  --tags pet \
  --force
```

成功すると、次のような出力になります。

```text
wrote case YAML -> practice/mock-cases.yaml
generated <n> cases, <n> response stubs
```

`mock-cases.yaml` には OpenAPI の `operationId` ごとに case の雛形が作られます。`mock-responses/` には、それぞれの case が参照する JSON stub が作られます。

`--tags pet` を指定すると、`pet` tag の operation だけが対象になります。複数 tag を対象にしたい場合は `--tags pet,store` のようにカンマ区切りで指定します。

`runnora generate --skip-deprecated` と違い、`genmock init` は deprecated operation も生成対象に含めます。この Petstore では `findPetsByTags` も mock case と response stub に含まれるため、不要であれば `mock-cases.yaml` から削除します。

生成された `mock-cases.yaml` には、リクエストマッチャーのプレースホルダーとして `equalTo: "TODO"` が入っています。例えば `getPetById` の case は次のようになります。

```yaml
- id: getPetById_default
  operationId: getPetById
  request:
    pathParams:
      petId:
        equalTo: "TODO"
  response:
    status: 200
    bodyFile: getPetById/getPetById_default.json
```

`equalTo: "TODO"` のままでは `runnora genmock build` を実行しても WireMock が正しくリクエストを照合できません。次のステップで、実際にテストしたいリクエスト値に書き換えます。

## 3. runnora の case JSON を決める

最初は `GET /pet/{petId}` のテストを、WireMock に対して通します。

`practice/cases/generated/pet/get_getPetById/default.json` を、次のように編集します。

```json
{
  "name": "get pet by id",
  "description": "petId=100 の pet を取得できること",
  "pathParams": {
    "petId": 100
  },
  "queryParams": {},
  "headers": {},
  "requestBody": null,
  "expect": {
    "status": 200,
    "bodyMode": "subset",
    "body": {
      "id": 100,
      "name": "doggie",
      "status": "available"
    },
    "ignorePaths": []
  }
}
```

ここで決めていることは、runnora 側の入力と期待値です。

| 項目 | 意味 |
|---|---|
| `pathParams.petId` | runnora が `/pet/100` を呼ぶ |
| `expect.status` | HTTP 200 を期待する |
| `expect.body` | レスポンス body に含まれてほしい値の雛形 |

生成直後の template runbook が自動で検証するのは `expect.status` です。`expect.body` は WireMock レスポンスと期待値をそろえるための材料として使います。レスポンス body まで runnora で検証したい場合は、template runbook の `test` にフィールドごとの検証式を追加します。

## 4. WireMock の返し分け条件を書く

次に、runnora が送る `/pet/100` に WireMock が一致できるよう、`practice/mock-cases.yaml` の `getPetById` case を編集します。

```yaml
- id: getPetById_100
  operationId: getPetById
  priority: 10
  request:
    pathParams:
      petId:
        equalTo: "100"
  response:
    status: 200
    bodyFile: getPetById/getPetById_100.json
```

`operationId` は OpenAPI の operation と紐づけるキーです。`request.pathParams.petId.equalTo` は WireMock 側の matcher です。

`bodyFile` は `--responses-root` からの相対パスです。Windows でもここは `/` 区切りの相対パスとして書きます。

```text
practice/mock-responses/getPetById/getPetById_100.json
```

## 5. WireMock のレスポンス JSON を書く

`practice/mock-responses/getPetById/getPetById_100.json` を、runnora の `expect.body` と矛盾しない内容にします。

```json
{
  "id": 100,
  "name": "doggie",
  "status": "available",
  "category": {
    "id": 1,
    "name": "dogs"
  },
  "photoUrls": [
    "https://example.com/doggie.png"
  ],
  "tags": [
    {
      "id": 10,
      "name": "friendly"
    }
  ]
}
```

この JSON がレスポンス body の正本です。runnora の case JSON には、期待値として残したい一部だけを `expect.body` に書きます。

```text
mock-responses/getPetById/getPetById_100.json
  |
  | WireMock が返す実レスポンス
  v
runnora expect.body
  |
  | subset として確認したい値だけを書く
  v
API テスト
```

完全一致の期待値として残したい場合は、runnora の `expect.body` に同じ JSON を書く運用もできます。ただしレスポンスが大きくなるほど二重管理になりやすいため、最初は重要な値だけを書く方が扱いやすいです。

## 6. query parameter のケースをそろえる

`findPetsByStatus` では、runnora の case JSON と WireMock の matcher の両方に `status=available` を書きます。

`practice/cases/generated/pet/get_findPetsByStatus/default.json`:

```json
{
  "name": "available pets",
  "description": "available の pet を検索できること",
  "pathParams": {},
  "queryParams": {
    "status": "available"
  },
  "headers": {},
  "requestBody": null,
  "expect": {
    "status": 200,
    "bodyMode": "subset",
    "body": [
      {
        "status": "available"
      }
    ],
    "ignorePaths": []
  }
}
```

`practice/mock-cases.yaml`:

```yaml
- id: findPetsByStatus_available
  operationId: findPetsByStatus
  priority: 10
  request:
    query:
      status:
        equalTo: "available"
  response:
    status: 200
    bodyFile: findPetsByStatus/findPetsByStatus_available.json
```

`practice/mock-responses/findPetsByStatus/findPetsByStatus_available.json`:

```json
[
  {
    "id": 100,
    "name": "doggie",
    "status": "available"
  }
]
```

query parameter を WireMock に送るため、`runnora generate` は OpenAPI の query parameter を template runbook の request path に反映します。

`practice/runbooks/generated/pet/get_findPetsByStatus.template.yml` の request path が次のようになっているか確認します。

```yaml
steps:
  call_api:
    req:
      /pet/findByStatus?status={{ vars.case.queryParams.status }}:
        get:
          headers: "{{ vars.case.headers }}"
```

case JSON の `queryParams.status` を変えると、同じ template runbook のまま WireMock に送る query string も変わります。古い生成物に query string が含まれていない場合は、再生成するか、この形へ調整します。

## 7. OpenAPI と mock case の整合性を検証する

WireMock 用ファイルを生成する前に、`runnora genmock validate` で OpenAPI と `mock-cases.yaml` の整合性を確認します。

```bash
runnora genmock validate \
  --openapi docs/tutorial/openapi.yaml \
  --cases practice/mock-cases.yaml \
  --responses-root practice/mock-responses \
  --tags pet \
  --fail-on-missing-operation \
  --fail-on-missing-body-file
```

問題がなければ次のように表示されます。

```text
OK: no issues found
```

`--tags` を指定せずに `genmock init` を実行し、リクエストマッチャーなしの case（`getInventory_default`、`logoutUser_default` など、パラメータを持たない GET 系エンドポイント）が生成された場合は、次のような警告が出ます。

```text
warning: cases[N]: case "getInventory_default" has no request matchers and is not a fallback
```

これはエラーではありません。exit code 0 のままです。マッチャーなし case を fallback として扱いたい場合は、`mock-cases.yaml` に `fallback: true` を追加します。

よく検出されるのは次のような問題です。

| 問題 | 確認する場所 |
|---|---|
| `operationId` が存在しない | OpenAPI と `mock-cases.yaml` |
| `bodyFile` が見つからない | `mock-responses/` |
| path parameter 名が違う | OpenAPI の path parameters |
| query parameter 名が違う | OpenAPI の query parameters |

## 8. WireMock 用ファイルを生成する

`runnora genmock build` で WireMock が読み込む `mappings/` と `__files/` を生成します。

```bash
runnora genmock build \
  --openapi docs/tutorial/openapi.yaml \
  --cases practice/mock-cases.yaml \
  --responses-root practice/mock-responses \
  --out practice/wiremock-out \
  --tags pet \
  --clean \
  --fail-on-missing-operation \
  --fail-on-missing-body-file
```

成功すると、次のような出力になります。

```text
build complete -> practice/wiremock-out
generated <n> mappings, <n> fallbacks
```

生成後の構成は次のようになります。

```text
practice/wiremock-out/
├─ mappings/
│  ├─ getPetById__getPetById_100.json
│  ├─ findPetsByStatus__findPetsByStatus_available.json
│  └─ _generated__fallback__*.json
└─ __files/
   ├─ getPetById/
   ├─ findPetsByStatus/
   └─ _generated/
```

明示的な fallback ケースを定義していない operationId には、自動 fallback が生成されます。自動 fallback を作りたくない場合は `--no-auto-fallback` を指定します。

```bash
runnora genmock build \
  --openapi docs/tutorial/openapi.yaml \
  --cases practice/mock-cases.yaml \
  --responses-root practice/mock-responses \
  --out practice/wiremock-out \
  --tags pet \
  --clean \
  --no-auto-fallback
```

## 9. WireMock を起動する

WireMock standalone を `practice/wiremock-out` を root directory として起動します。

```bash
java -jar docs/tools/wiremock-standalone-3.13.2.jar \
  --root-dir practice/wiremock-out \
  --port 8080
```

起動後、WireMock は `mappings/` と `__files/` を読み込んで、OpenAPI 由来の path に対するリクエストを待ち受けます。

## 10. runnora を WireMock に向けて実行する

runnora の接続先を WireMock に向けます。

`runnora run` はデフォルトで `./config.yaml` を読み込みます。このチュートリアルではリポジトリ直下ではなく `practice/config.yaml` を使います。SQL フックを使わないため、`oracle.dsn` は空のままで構いません。

```bash
runnora init --out practice/config.yaml --force
```

PowerShell:

```powershell
$env:RUNNORA_BASE_URL = "http://localhost:8080"
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_getPetById.suite.yml
```

Bash:

```bash
export RUNNORA_BASE_URL="http://localhost:8080"
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_getPetById.suite.yml
```

`findPetsByStatus` も確認します。

```bash
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
```

成功したら、次の対応が取れています。

| runnora 側 | WireMock 側 |
|---|---|
| `pathParams.petId: 100` | `request.pathParams.petId.equalTo: "100"` |
| `queryParams.status: "available"` | `request.query.status.equalTo: "available"` |
| `expect.status: 200` | `response.status: 200` |
| `expect.body` | `mock-responses/` の JSON とそろえる期待値の雛形 |

## 運用の目安

OpenAPI が更新されたときは、テスト資産とモック資産を同じ順序で更新します。

```text
1. OpenAPI を更新する
2. runnora generate --clean --force で generated/ を作り直す
3. runnora genmock init --tags pet --force で practice/mock-cases.yaml と practice/mock-responses/ の雛形を更新する
4. 手で育てた case JSON、mock-cases.yaml、mock-responses/ を差分確認して戻す
5. runnora genmock validate --tags pet で整合性を確認する
6. runnora genmock build --tags pet --clean で WireMock 用ファイルを作り直す
7. WireMock に対して runnora run を実行する
```

`wiremock-out/` は `build` で再生成できます。通常は Git 管理せず、`mock-cases.yaml` と `mock-responses/` を共有する方が扱いやすいです。

一方、`mock-responses/` はモックが返すレスポンスの正本なので、チームで同じモックを使いたい場合は Git 管理する価値があります。

## トラブルシュート

| 症状 | 確認すること |
|---|---|
| `unknown command "genmock"` と表示される | `genmock` が登録された runnora バイナリを使っているか |
| `operationId` が見つからない | `mock-cases.yaml` の `operationId` が OpenAPI と一致しているか |
| `bodyFile` が見つからない | `--responses-root` 配下に `bodyFile` の JSON が存在するか |
| WireMock が fallback を返す | runnora の request と `mock-cases.yaml` の matcher が一致しているか |
| query parameter が一致しない | case JSON の `queryParams` と `mock-cases.yaml` の `request.query` が同じ値か、template runbook の request path が `vars.case.queryParams.<name>` を参照しているか |
| body 期待値とモックレスポンスがずれる | `mock-responses/` の JSON と `expect.body` が矛盾していないか |

## まとめ

`runnora generate` は、OpenAPI から API テストの入口を作ります。`runnora genmock` は、同じ OpenAPI から WireMock のモック資産を作ります。

両方を組み合わせると、まだ実 API が安定していない段階でも、OpenAPI に沿ったモックサーバに対して runnora のテストを育てられます。

レスポンス JSON の正本を `mock-responses/` に寄せ、runnora の `expect.body` には確認したい一部だけを書くと、WireMock レスポンスと API テスト期待値の二重管理を小さくできます。生成直後の runbook では body は自動検証されないため、body まで検証したい場合は template runbook の `test` に必要な検証式を追加します。
