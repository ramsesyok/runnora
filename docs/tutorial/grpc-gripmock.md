# GripMock で gRPC の4つの通信方式をテストする

このチュートリアルでは、gRPCの4つの通信方式を表すprotoをGripMockでモックし、runnoraのrunbookからテストします。

- Unary RPC
- Client Streaming RPC
- Server Streaming RPC
- Bidirectional Streaming RPC

完成済みのproto、GripMock stub、runbookを`docs/tutorial/grpc/`に用意しています。最初に一式を実行し、その後で通信方式ごとの違いを確認します。

## 前提条件

リポジトリルートからコマンドを実行します。次のツールが必要です。

| ツール | 用途 |
|---|---|
| runnora | gRPC runbookの実行 |
| GripMock v3.17.1 | protoとstubからgRPCモックを起動 |
| curl | GripMockの状態確認 |

runnoraをソースから用意する場合は、リポジトリルートでビルドします。

```bash
go build -o runnora .
```

Windowsでは出力名を`runnora.exe`にしても構いません。以降は`runnora`がPATHから実行できるものとして記述します。

GripMockは[公式リリース](https://github.com/bavix/gripmock/releases/tag/v3.17.1)から実行ファイルを取得してPATHへ追加します。Goを使ってインストールする場合は、次のコマンドでも同じバージョンの実行ファイルを用意できます。

```bash
go install github.com/bavix/gripmock/v3@v3.17.1
```

## 用意されているファイル

```text
docs/tutorial/grpc/
├─ config.example.yaml
├─ proto/
│  ├─ unary.proto
│  ├─ client_streaming.proto
│  ├─ server_streaming.proto
│  └─ bidirectional_streaming.proto
├─ stubs/
│  ├─ unary.yaml
│  ├─ client_streaming.yaml
│  ├─ server_streaming.yaml
│  └─ bidirectional_streaming.yaml
└─ runbooks/
   ├─ unary.yml
   ├─ client_streaming.yml
   ├─ server_streaming.yml
   └─ bidirectional_streaming.yml
```

各ディレクトリの役割は次のとおりです。

| パス | 役割 |
|---|---|
| `proto/` | gRPCサービスとメッセージの定義 |
| `stubs/` | GripMockが照合する入力と返す出力 |
| `runbooks/` | runnoraが送るリクエストとレスポンスの検証 |
| `config.example.yaml` | Oracleフックを使わずにrunbookを実行する設定 |

通信方式ごとに、proto、stub、runbookが1対1で対応しています。

| 通信方式 | protoのRPC定義 | runnoraの送信 | runnoraの受信 | GripMock stub |
|---|---|---|---|---|
| Unary | `Request -> Response` | `message` | `res.message` | `input` + `output.data` |
| Client Streaming | `stream Request -> Response` | `messages` | `res.message` | `inputs` + `output.data` |
| Server Streaming | `Request -> stream Response` | `message` | `res.messages` | `input` + `output.stream` |
| Bidirectional Streaming | `stream Request -> stream Response` | `messages` | `res.messages` | `inputs` + `output.stream` |

GripMockのストリーミングstub形式については、[GripMock公式のStreamingガイド](https://gripmock.org/guide/stubs/streaming.html)も参照してください。

## 1. GripMockを起動する

1つ目のターミナルでGripMock v3.17.1の実行ファイルを起動し、4つのprotoと4つのstubをまとめて読み込みます。

```bash
gripmock \
  --stub=docs/tutorial/grpc/stubs \
  docs/tutorial/grpc/proto/unary.proto \
  docs/tutorial/grpc/proto/client_streaming.proto \
  docs/tutorial/grpc/proto/server_streaming.proto \
  docs/tutorial/grpc/proto/bidirectional_streaming.proto
```

使用するポートは次の2つです。

| ポート | 用途 |
|---|---|
| `4770` | runnoraが接続するgRPCポート |
| `4771` | GripMockの管理APIとWeb UI |

readinessを確認します。

```bash
curl -fsS http://127.0.0.1:4771/api/health/readiness
```

正常時は`message`が`ok`のJSONが返ります。

```json
{"message":"ok","time":"..."}
```

読み込まれたstubは次のAPIで確認できます。

```bash
curl -fsS http://127.0.0.1:4771/api/stubs
```

レスポンスに`Greeter`、`ScoreRecorder`、`UpdateService`、`ChatService`の4サービスが含まれていることを確認します。

## 2. Unary RPCをテストする

[`unary.proto`](grpc/proto/unary.proto)は、1つのリクエストに1つのレスポンスを返します。

```proto
service Greeter {
  rpc SayHello(HelloRequest) returns (HelloResponse);
}
```

[`unary.yaml`](grpc/stubs/unary.yaml)では、`name: Alice`に一致したときに`Hello, Alice!`を返します。

```yaml
input:
  equals:
    name: Alice
output:
  data:
    message: Hello, Alice!
```

[`unary.yml`](grpc/runbooks/unary.yml)は`message`で1件を送り、単一レスポンスの`res.message`を検証します。

```yaml
greq:
  tutorial.grpc.unary.v1.Greeter/SayHello:
    message:
      name: Alice
test: |
  current.res.status == 0 &&
  current.res.message.message == "Hello, Alice!"
```

実行します。

```bash
runnora run \
  --config docs/tutorial/grpc/config.example.yaml \
  docs/tutorial/grpc/runbooks/unary.yml
```

## 3. Client Streaming RPCをテストする

[`client_streaming.proto`](grpc/proto/client_streaming.proto)は、クライアントから複数のスコアを送り、サーバーから集計結果を1件受け取ります。

```proto
service ScoreRecorder {
  rpc Summarize(stream ScoreRequest) returns (ScoreSummary);
}
```

[`client_streaming.yaml`](grpc/stubs/client_streaming.yaml)は、複数入力を`inputs`で順番に定義し、単一レスポンスを`output.data`で返します。

```yaml
inputs:
  - equals:
      player: Alice
      points: 10
  - equals:
      player: Bob
      points: 20
  - equals:
      player: Carol
      points: 30
output:
  data:
    count: 3
    total_points: 60
```

[`client_streaming.yml`](grpc/runbooks/client_streaming.yml)は`messages`の各要素をクライアントストリームへ送信します。サーバーから返る値は1件なので`res.message`を検証します。

```yaml
messages:
  - player: Alice
    points: 10
  - player: Bob
    points: 20
  - player: Carol
    points: 30
test: |
  current.res.status == 0 &&
  current.res.message.count == 3 &&
  current.res.message.total_points == 60
```

実行します。

```bash
runnora run \
  --config docs/tutorial/grpc/config.example.yaml \
  docs/tutorial/grpc/runbooks/client_streaming.yml
```

## 4. Server Streaming RPCをテストする

[`server_streaming.proto`](grpc/proto/server_streaming.proto)は、1つのtopicを受け取り、更新情報を複数件返します。

```proto
service UpdateService {
  rpc ListUpdates(UpdateRequest) returns (stream UpdateResponse);
}
```

[`server_streaming.yaml`](grpc/stubs/server_streaming.yaml)は、複数レスポンスを`output.stream`へ並べます。

```yaml
output:
  stream:
    - sequence: 1
      message: build started
    - sequence: 2
      message: tests passed
    - sequence: 3
      message: release ready
```

[`server_streaming.yml`](grpc/runbooks/server_streaming.yml)は`message`で1件を送り、受信した配列`res.messages`の件数、順序、内容を検証します。

```yaml
test: |
  current.res.status == 0 &&
  len(current.res.messages) == 3 &&
  current.res.messages[0].sequence == 1 &&
  current.res.messages[1].message == "tests passed" &&
  current.res.messages[2].sequence == 3
```

実行します。

```bash
runnora run \
  --config docs/tutorial/grpc/config.example.yaml \
  docs/tutorial/grpc/runbooks/server_streaming.yml
```

## 5. Bidirectional Streaming RPCをテストする

[`bidirectional_streaming.proto`](grpc/proto/bidirectional_streaming.proto)は、クライアントとサーバーの両方向で複数メッセージを送受信します。

```proto
service ChatService {
  rpc Chat(stream ChatRequest) returns (stream ChatResponse);
}
```

[`bidirectional_streaming.yaml`](grpc/stubs/bidirectional_streaming.yaml)では、`inputs`と`output.stream`の同じ位置に対応するリクエストとレスポンスを書きます。

[`bidirectional_streaming.yml`](grpc/runbooks/bidirectional_streaming.yml)では、通常のメッセージに2つの制御値を混ぜて送受信順を表します。

| 制御値 | 意味 |
|---|---|
| `receive` | サーバーから次の1メッセージを受信する |
| `close` | クライアント側の送信ストリームを閉じる |

```yaml
messages:
  - user: Alice
    message: Hello
  - receive
  - user: Alice
    message: Bye
  - receive
  - close
test: |
  current.res.status == 0 &&
  len(current.res.messages) == 2 &&
  current.res.messages[0].message == "Hello, Alice!" &&
  current.res.messages[1].message == "See you!"
```

実行します。

```bash
runnora run \
  --config docs/tutorial/grpc/config.example.yaml \
  docs/tutorial/grpc/runbooks/bidirectional_streaming.yml
```

## 6. 4方式をまとめて検証する

4つのrunbookは1回の`runnora run`へまとめて渡せます。

```bash
runnora run \
  --config docs/tutorial/grpc/config.example.yaml \
  docs/tutorial/grpc/runbooks/unary.yml \
  docs/tutorial/grpc/runbooks/client_streaming.yml \
  docs/tutorial/grpc/runbooks/server_streaming.yml \
  docs/tutorial/grpc/runbooks/bidirectional_streaming.yml
```

全テストが成功すると、次の集計が表示されます。

```text
Runbooks: 4, Passed: 4, Failed: 0
```

GripMock側でも、4つのstubが使われ、未使用stubが残っていないことを確認できます。

```bash
curl -fsS http://127.0.0.1:4771/api/stubs/used
curl -fsS http://127.0.0.1:4771/api/stubs/unused
```

1つ目のレスポンスには4つのstubが含まれ、2つ目は次のようになります。

```json
[]
```

## 7. GripMockを停止する

検証後は、GripMockを起動したターミナルで`Ctrl+C`を押して終了します。

## トラブルシューティング

### `connection refused`になる

GripMockが起動しているか、readiness APIとGripMockを起動したターミナルのログを確認します。

```bash
curl -fsS http://127.0.0.1:4771/api/health/readiness
```

### `no matching stub`になる

runbookの送信値とstubの`input`または`inputs`は、大文字・小文字や数値型を含めて一致させます。`/api/stubs`で実際に読み込まれた値も確認できます。

### protoのメソッドが見つからない

runbookでは`package.Service/Method`の完全修飾名を使います。一方、今回のGripMock stubの`service`にはprotoのservice名だけを書いています。

```text
runnora:  tutorial.grpc.unary.v1.Greeter/SayHello
GripMock: Greeter / SayHello
```
