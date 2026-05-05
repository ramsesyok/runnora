# OpenAPI 定義から runbook を生成して API テストを始める

このチュートリアルでは、`runnora generate` を使って OpenAPI 定義から API テストの雛形を生成し、最初の runbook を実行できる状態まで育てます。

題材には `docs/tutorial/openapi.yaml` に保存されている Petstore の OpenAPI 定義を使います。OpenAPI の path、operationId、schema は読めるけれど、runbook や API テストを作るのは初めて、という人を対象にしています。

## このチュートリアルで作るもの

`runnora generate` は OpenAPI 定義から、次の 3 種類のファイルを生成します。

| 種類 | 役割 | 出力先 |
|---|---|---|
| template runbook | 1 件の case を受け取って API を 1 回呼ぶ薄い runbook | `practice/runbooks/generated/<tag>/<method>_<operationId>.template.yml` |
| case JSON | リクエスト値と期待値を書くファイル | `practice/cases/generated/<tag>/<method>_<operationId>/default.json` |
| suite runbook | case JSON を読み込み、template runbook を実行する runbook | `practice/runbooks/generated/<tag>/<method>_<operationId>.suite.yml` |

この 3 つは別々のテストではなく、1 つの API operation をテストするために組み合わさって動きます。

```text
suite runbook
  |
  | cases に並んだ case JSON を 1 件ずつ読む
  v
case JSON
  |
  | request / expect の具体値を template runbook に渡す
  v
template runbook
  |
  | 受け取った値で 1 つの API を呼び、結果を検証する
  v
API
```

template runbook は、1 つの API に対する「呼び出しと検証の流れ」を表すテンプレートです。たとえば `GET /pet/findByStatus` なら、「どの runner を使うか」「どの path を呼ぶか」「レスポンス status をどう検証するか」を持ちます。

suite runbook は、template runbook を実行する入口です。suite runbook は `vars.cases` に並んだ case JSON を順番に読み、case の数だけ template runbook を呼び出します。

case JSON は、template runbook に渡す具体値です。同じ API でも、query parameterを`?status=available`、`?status=pending`、`?status=sold` のように入力を変えたり、期待値を変えたい場合は、case JSON を増やします。

最初に編集するのは、基本的に case JSON です。API の呼び出し方そのものを変えるときは template runbook、実行する case の一覧や順番を変えるときは suite runbook を編集します。

## generated と evidence の考え方

`runnora generate` の生成物は、再生成されることを前提にしたファイルです。

そのため、`runbooks/generated/` と `cases/generated/` は、OpenAPI 定義に追従するための作業場所として扱います。このチュートリアルではリポジトリ直下を散らかさないよう、生成物を `practice/` 配下にまとめます。生成された runbook を手で大きく直して業務テストとして育てたい場合は、`runbooks/evidence/` など別の場所へコピーしてから編集します。

```text
runbooks/
  generated/   # runnora generate で再生成する
  evidence/    # 手で育てる証跡向け runbook
cases/
  generated/   # runnora generate で再生成する
```

この分け方にしておくと、OpenAPI 定義が更新されたときに `generated/` を安心して作り直せます。

## runbook の最小知識

生成された runbook を読むために、まず次の要素だけ押さえます。

| 要素 | 意味 |
|---|---|
| `desc` | runbook の説明 |
| `labels` | runbook を分類するラベル |
| `runners` | HTTP API や DB など、実行先の定義 |
| `vars` | runbook 内で使う変数 |
| `steps` | 実行する処理の並び |
| `test` | レスポンスなどに対する検証式 |
| `include` | 別の runbook を呼び出す |
| `loop` | 同じ step を繰り返す |

`generate` が作る suite runbook は、case JSON を読み込み、template runbook を呼び出すときに `case` という名前の変数として渡します。

template runbook の中では、渡された `case` 変数を `vars.case` という名前で参照します。つまり、`case` は変数名、`vars.case` は runbook 内でその変数を参照するときの書き方です。

| 場所 | 書き方 | 意味 |
|---|---|---|
| suite runbook | `case: "{{ vars.cases[i] }}"` | i 番目の case JSON を `case` という変数名で template runbook に渡す |
| template runbook | `vars.case` | suite runbook から渡された `case` 変数を参照する |
| template runbook | `vars.case.headers` | case JSON の `headers` を参照する |
| template runbook | `vars.case.expect.status` | case JSON の `expect.status` を参照する |

```text
case JSON
   |
   v
suite runbook
  vars.cases[i] を case として渡す
   |
   v
template runbook
  vars.case として参照する
   |
   v
API 呼び出し
```

## どのファイルを編集するか

生成後にテストを育てるときは、変更したい内容によって編集するファイルを分けます。

| やりたいこと | 編集するファイル | 理由 |
|---|---|---|
| リクエスト値を変える | case JSON | path parameter、query parameter、header、request body は case ごとの具体値だから |
| 期待 status を変える | case JSON | 期待値は case ごとに変わるから |
| レスポンス body の期待値を追加する | case JSON | 同じ API でも case ごとに期待する body が変わるから |
| 正常系、異常系、境界値を増やす | case JSON と suite runbook | case JSON を増やし、suite runbook の `vars.cases` に追加する |
| query parameter の値を変える | case JSON | 生成された template runbook が `vars.case.queryParams.<name>` を request path に反映するため |
| 認証 header を全 case 共通で付ける | template runbook または case JSON | 全 case 共通なら template、case ごとに違うなら case JSON に書く |
| API 呼び出しの前後に step を追加する | template runbook | 1 回の API テストの流れを変えるため |
| 実行する case の順番を変える | suite runbook | suite runbook が case の一覧と実行順を持つため |
| 別の template runbook を呼ぶ | suite runbook | suite runbook が `include` で template runbook を呼び出すため |

迷ったときは、次の基準で考えます。

```text
値を変えたい        -> case JSON
API の呼び方を変えたい -> template runbook
case の実行順を変えたい -> suite runbook
```

## Petstore OpenAPI を確認する

入力ファイルは次の場所にあります。

```bash
docs/tutorial/openapi.yaml
```

この OpenAPI 定義には、主に次の tag が含まれています。

| tag | 内容 |
|---|---|
| `pet` | ペットの登録、検索、更新、削除 |
| `store` | 注文や在庫 |
| `user` | ユーザー作成、ログイン、取得、削除 |

このチュートリアルでは、最初の題材として `pet` tag を使います。

代表的な operation は次のとおりです。

| operationId | 見るポイント |
|---|---|
| `findPetsByStatus` | query parameter を使う検索 API |
| `getPetById` | path parameter を使う取得 API |
| `addPet` | request body を持つ登録 API |
| `findPetsByTags` | deprecated の例 |

## テスト資産を生成する

まず、Petstore の OpenAPI 定義から `pet` tag のテスト資産を生成します。

```bash
mkdir -p practice

runnora generate \
  --openapi docs/tutorial/openapi.yaml \
  --out practice \
  --tags pet \
  --skip-deprecated
```

`--skip-deprecated` を指定しているため、`deprecated: true` の `findPetsByTags` は生成対象から外れます。

既存の `generated/` を作り直したい場合は、`--clean` と `--force` を付けます。

```bash
runnora generate \
  --openapi docs/tutorial/openapi.yaml \
  --out practice \
  --tags pet \
  --skip-deprecated \
  --clean \
  --force
```

> **注意**: `--clean` を指定すると、`--out practice` の場合は `practice/runbooks/generated/` と `practice/cases/generated/` が削除されてから再生成されます。case JSON を手で編集した後に `--clean --force` を実行すると、その編集内容は失われます。case を育てた後は `--force` のみ（`--clean` なし）を使うか、編集済みの case JSON をあらかじめ別の場所にコピーしてから実行してください。

生成後、次のようなファイルが作られます。

```text
practice/runbooks/generated/pet/get_findPetsByStatus.template.yml
practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
practice/cases/generated/pet/get_findPetsByStatus/default.json

practice/runbooks/generated/pet/get_getPetById.template.yml
practice/runbooks/generated/pet/get_getPetById.suite.yml
practice/cases/generated/pet/get_getPetById/default.json

practice/runbooks/generated/pet/post_addPet.template.yml
practice/runbooks/generated/pet/post_addPet.suite.yml
practice/cases/generated/pet/post_addPet/default.json

practice/runbooks/generated/pet/put_updatePet.template.yml
practice/runbooks/generated/pet/put_updatePet.suite.yml
practice/cases/generated/pet/put_updatePet/default.json

practice/runbooks/generated/pet/post_updatePetWithForm.template.yml
...
```

`pet` tag に含まれる全 operation（`findPetsByTags` のみ `--skip-deprecated` で除外）が生成されます。このチュートリアルでは主に `findPetsByStatus` と `getPetById` を題材にします。

ファイル名は `<method>_<operationId>` です。OpenAPI の `operationId` が変わると、生成されるファイル名も変わります。

## 生成された case JSON を読む

`findPetsByStatus` の case JSON を開きます。

```bash
practice/cases/generated/pet/get_findPetsByStatus/default.json
```

生成直後の case JSON は、API テストの出発点です。次のような構造になっています。

```json
{
  "name": "default",
  "description": "Finds Pets by status",
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
        "id": 0,
        "name": "doggie",
        "status": "available"
      }
    ],
    "ignorePaths": []
  }
}
```

`findPetsByStatus` は query parameter を持つ API なので、生成直後から `queryParams.status` に OpenAPI の default/enum 由来の初期値が入ります。レスポンスが配列なので、`body` の初期値も配列になります。OpenAPI schema の example や enum/default から値を作れる場合は、`expect.body` の雛形にも初期値が入ります。

各項目の意味は次のとおりです。

| 項目 | 意味 |
|---|---|
| `name` | case の名前 |
| `description` | case の説明 |
| `pathParams` | `/pet/{petId}` のような path parameter に入れる値 |
| `queryParams` | `?status=available` のような query parameter に入れる値 |
| `headers` | API 呼び出し時に付ける HTTP header |
| `requestBody` | POST/PUT/PATCH で送る body |
| `expect.status` | 期待する HTTP status |
| `expect.bodyMode` | body 検証を追加するときの比較モード |
| `expect.body` | 期待するレスポンス body の雛形 |
| `expect.ignorePaths` | body 検証を追加するときに除外する JSON path |

生成直後の template runbook が自動で検証するのは `expect.status` です。`expect.body`、`expect.bodyMode`、`expect.ignorePaths` は期待レスポンスを整理するための雛形で、レスポンス body まで検証したい場合は template runbook の `test` にフィールドごとの検証式を追加します。

## 最初の API テストを作る

`findPetsByStatus` は、`status` query parameter でペットを検索する API です。

OpenAPI 定義では `status` に次の値が定義されています。

```yaml
enum:
  - available
  - pending
  - sold
```

まず `default.json` を、`available` の検索 case として編集します。

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
    "body": [],
    "ignorePaths": []
  }
}
```

この時点では「`status=available` で呼んだら 200 が返る」ことを確認するテストです。API テストを初めて作る場合は、この粒度から始めると失敗原因を追いやすくなります。

レスポンスの形まで確認したくなったら、`expect.body` に期待する一部の値を書き、その値に合わせて template runbook の `test` に検証式を追加します。

```json
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
```

`bodyMode: subset` は、レスポンス全体が完全一致しなくても、指定した一部が含まれていればよい、という期待値の考え方です。ID や日時のように毎回変わる値がある API では、最初から完全一致を狙わない方が安定します。

## path parameter を使うテストを作る

次に `getPetById` を見ます。

OpenAPI の path は次の形です。

```yaml
/pet/{petId}
```

`generate` された template runbook では、`{petId}` が `vars.case.pathParams.petId` を参照する形に変換されます。

case JSON では、`pathParams` に `petId` を入れます。

```json
{
  "name": "get pet by id",
  "description": "指定した petId の pet を取得できること",
  "pathParams": {
    "petId": 1
  },
  "queryParams": {},
  "headers": {},
  "requestBody": null,
  "expect": {
    "status": 200,
    "bodyMode": "subset",
    "body": {
      "id": 1
    },
    "ignorePaths": []
  }
}
```

ここでは `petId: 1` のデータが API サーバ側に存在している必要があります。存在しない場合は、事前に `addPet` で作るか、実際に存在する ID に変更します。

## request body を持つテストを作る

`addPet` は request body を持つ API です。

case JSON の `requestBody` に、送信する JSON を書きます。

```json
{
  "name": "add available pet",
  "description": "available の pet を登録できること",
  "pathParams": {},
  "queryParams": {},
  "headers": {
    "Content-Type": "application/json"
  },
  "requestBody": {
    "id": 1001,
    "name": "doggie",
    "category": {
      "id": 1,
      "name": "dogs"
    },
    "photoUrls": [
      "https://example.com/doggie.png"
    ],
    "tags": [
      {
        "id": 1,
        "name": "friendly"
      }
    ],
    "status": "available"
  },
  "expect": {
    "status": 200,
    "bodyMode": "subset",
    "body": {
      "id": 1001,
      "name": "doggie",
      "status": "available"
    },
    "ignorePaths": []
  }
}
```

OpenAPI の schema から作られる値は、あくまで雛形です。実際の API サーバが必須にしている値、認証、事前データ、採番ルールに合わせて case JSON を調整します。

`multipart/form-data` の request body を持つ API では、case JSON の `requestBody` に form field ごとの値が入ります。`type: string`, `format: binary` の property は file part として扱われるため、生成直後は `TODO: path/to/file` を実際にアップロードするファイルパスへ変更します。

```json
{
  "pathParams": {
    "petId": 1
  },
  "queryParams": {},
  "headers": {},
  "requestBody": {
    "additionalMetadata": "sample image",
    "file": "./testdata/doggie.png"
  }
}
```

## suite runbook を実行する

生成された suite runbook は、実行の入口になるファイルです。

suite runbook を実行すると、次の順に処理されます。

1. suite runbook が `vars.cases` に書かれた case JSON を読み込む
2. `loop` で case JSON を 1 件ずつ取り出す
3. 取り出した case を `case` という変数名で template runbook に渡す
4. template runbook が `vars.case` を使って API を呼ぶ
5. template runbook が `vars.case.expect.status` などを使って結果を検証する

つまり、suite runbook を 1 回実行すると、suite runbook に登録されている case の数だけ template runbook が実行されます。

API の接続先は `RUNNORA_BASE_URL` で指定します。

`runnora run` はデフォルトで `./config.yaml` を読み込みます。このチュートリアルではリポジトリ直下ではなく `practice/config.yaml` を使います。SQL フックを使わないため、`oracle.dsn` は空のままで構いません。

```bash
runnora init --out practice/config.yaml --force
```

PowerShell の例:

```powershell
$env:RUNNORA_BASE_URL = "https://petstore3.swagger.io/api/v3"
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
```

Bash の例:

```bash
export RUNNORA_BASE_URL="https://petstore3.swagger.io/api/v3"
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
```

実行に失敗した場合は、まず次の順に確認します。

| 確認項目 | 見る内容 |
|---|---|
| 接続先 | `RUNNORA_BASE_URL` が正しいか |
| path parameter | `pathParams` の名前と値が OpenAPI と合っているか |
| query parameter | `queryParams` の名前と値が OpenAPI と合っているか |
| request body | 必須項目が入っているか |
| status | API が実際に返す status と `expect.status` が合っているか |
| 認証 | API key や token が必要ではないか |

生成直後のテストがそのまま全て成功するとは限りません。OpenAPI 定義、実行環境、テストデータを合わせていく作業が API テスト作成の中心です。

## case を増やす

1 つの operation に対して、case は複数作れます。

たとえば `findPetsByStatus` なら、次のように case を増やします。

```text
practice/cases/generated/pet/get_findPetsByStatus/
  available.json
  pending.json
  sold.json
```

それぞれの JSON では、`queryParams.status` と `name` を変えます。

`available.json`:

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
    "body": {},
    "ignorePaths": []
  }
}
```

query parameter を使う API では、生成された template runbook の request path に query string が組み込まれます。case JSON に値を書くと、その値が `vars.case.queryParams.<name>` として参照されます。

`practice/runbooks/generated/pet/get_findPetsByStatus.template.yml` の該当箇所を確認します。

```yaml
runners:
  req:
    endpoint: ${RUNNORA_BASE_URL}

steps:
  call_api:
    req:
      /pet/findByStatus?status={{ vars.case.queryParams.status }}:
        get:
          headers: "{{ vars.case.headers }}"
```

`endpoint` の値は `${RUNNORA_BASE_URL}` という shell 変数展開形式で書きます。`RUNNORA_BASE_URL` 環境変数が読み込まれます。

`available.json`、`pending.json`、`sold.json` のように case JSON だけを増やして、同じ template runbook で複数の検索条件を実行できます。

`sold.json`:

```json
{
  "name": "sold pets",
  "description": "sold の pet を検索できること",
  "pathParams": {},
  "queryParams": {
    "status": "sold"
  },
  "headers": {},
  "requestBody": null,
  "expect": {
    "status": 200,
    "bodyMode": "subset",
    "body": {},
    "ignorePaths": []
  }
}
```

case を増やしたら、suite runbook の `vars.cases` に追加します。

```yaml
vars:
  cases:
    - json://../../../cases/generated/pet/get_findPetsByStatus/available.json
    - json://../../../cases/generated/pet/get_findPetsByStatus/pending.json
    - json://../../../cases/generated/pet/get_findPetsByStatus/sold.json
```

最初に作る case は、次の順番がおすすめです。

| 種類 | 例 |
|---|---|
| 正常系 | `status=available` で 200 |
| 別パターン | `status=sold` で 200 |
| 異常系 | 不正な status で 400 |
| 境界値 | ID の最小値、最大値、存在しない ID |

## OpenAPI 定義が更新されたときの手順

OpenAPI 定義は、API の変更に合わせて更新されます。`docs/tutorial/openapi.yaml` が更新されたら、生成物と手で育てたテストを分けて扱います。

### 1. 変更の影響を確認する

まず、OpenAPI のどこが変わったかを確認します。

| 変更箇所 | API テストへの影響 |
|---|---|
| `path` | template runbook の呼び出し先が変わる |
| HTTP method | ファイル名と呼び出し方が変わる |
| `operationId` | 生成されるファイル名が変わる |
| `tags` | 出力ディレクトリが変わる |
| parameters | `pathParams` や `queryParams` の修正が必要 |
| requestBody | `requestBody` の修正が必要 |
| responses | `expect.status` や `expect.body` の修正が必要 |
| `deprecated` | `--skip-deprecated` 指定時に生成対象から外れる |

特に `operationId` は重要です。`getPetById` が `findPetById` に変わると、生成ファイル名も `get_getPetById.*` から `get_findPetById.*` に変わります。

### 2. generated を再生成する

`generated/` は再生成前提なので、OpenAPI 更新後は作り直します。

```bash
runnora generate \
  --openapi docs/tutorial/openapi.yaml \
  --out practice \
  --tags pet \
  --skip-deprecated \
  --clean \
  --force
```

`--clean` は `practice/runbooks/generated/` と `practice/cases/generated/` を掃除してから生成します。`practice/runbooks/evidence/` は対象外です。

### 3. 手で育てたテストと比較する

手で育てた runbook や case がある場合は、再生成されたファイルと比較します。

見るポイントは次のとおりです。

| 見るもの | 確認内容 |
|---|---|
| case JSON | 追加された必須パラメータがないか |
| template runbook | path や method が変わっていないか |
| suite runbook | case の読み込み先が変わっていないか |
| OpenAPI response | `expect.status` や `expect.body` を更新する必要がないか |

たとえば request body に必須項目が追加された場合、既存 case の `requestBody` にもその項目を追加します。

### 4. 削除された API のテストを整理する

OpenAPI から operation が削除された場合、その operation の generated ファイルは再生成されません。

手で育てた evidence 側のテストが残っている場合は、次のどちらかを選びます。

| 選択肢 | 使う場面 |
|---|---|
| 削除する | API が本当に廃止された |
| 保留する | API は残っているが OpenAPI 定義から一時的に消えている |

保留する場合は、テスト名やディレクトリ名に状態が分かるような印を付け、CI からは一時的に外します。

### 5. suite を実行して確認する

再生成と case 修正が終わったら、suite runbook を実行します。

```bash
runnora run --config practice/config.yaml practice/runbooks/generated/pet/get_findPetsByStatus.suite.yml
```

OpenAPI 更新後に失敗したテストは、API の変更を検知できたという意味では役に立っています。失敗内容を見て、case、期待値、または API 側のどれを直すべきか判断します。

### 6. チュートリアルも更新する

OpenAPI の変更により、operationId、生成ファイル名、コマンド例、期待レスポンスが変わった場合は、このチュートリアルも一緒に更新します。

Pull Request では、次のファイルをまとめてレビューすると変更の意図が伝わりやすくなります。

```text
docs/tutorial/openapi.yaml
docs/tutorial/openapi-generate-runbook.md
practice/runbooks/generated/
practice/cases/generated/
```

## 生成物を業務テストへ育てる

`generated/` の runbook は、OpenAPI から広く浅く API を確認するための入口です。

業務シナリオとして長く使うテストは、次のように育てます。

1. `generated/` で最初の雛形を作る
2. case JSON を編集して API が通る状態にする
3. 必要に応じて case を増やす
4. 安定した runbook を `runbooks/evidence/` へコピーする
5. evidence 側で業務の前提データ、複数 API の流れ、DB 検証などを追加する

この流れにすると、OpenAPI の変更に追従する生成物と、手で作り込む証跡テストを混ぜずに管理できます。

## トラブルシュート

| 症状 | 確認すること |
|---|---|
| `unknown command "generate"` と表示される | `root.AddCommand(newGenerateCmd())` が登録されたバイナリを使っているか |
| `--openapi` のファイルが読めない | `docs/tutorial/openapi.yaml` へのパスが実行ディレクトリから見て正しいか |
| 生成物が更新されない | `--force` を付けているか |
| 古い generated ファイルが残る | `--clean` を付けて再生成する |
| deprecated な API も生成される | `--skip-deprecated` を付けているか |
| 実行時に接続できない | `RUNNORA_BASE_URL` が正しいか |
| 期待 status が合わない | API が実際に返す status と `expect.status` を比較する |
| レスポンス body の検証が不安定 | `bodyMode: subset` にして、毎回変わる値を `ignorePaths` に入れる |

## まとめ

`runnora generate` は、OpenAPI 定義から API テストの出発点を作るためのコマンドです。

生成された template runbook、case JSON、suite runbook の役割を分けて理解すると、最初にどこを編集すればよいかが分かりやすくなります。

OpenAPI が更新されたときは `generated/` を再生成し、手で育てた evidence 側のテストへ必要な変更だけを取り込みます。この運用にしておくと、API 仕様の変更に追従しながら、安定した API テストを少しずつ増やせます。

生成したテストを実 API ではなく WireMock モックに向けて確認したい場合は、次に [OpenAPI 定義から runnora のテストと WireMock モックをそろえる](openapi-generate-mock.md) を参照してください。
