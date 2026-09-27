# runnora 動作確認テスト環境

runnora を使った WebAPI・gRPC・Oracle DB (PL/SQL) の統合テスト環境です。

## ディレクトリ構成

```
test/
├── docker-compose.yaml          # Oracle / testapi / testgrpc の起動定義
├── runnora.yaml                 # runnora プロジェクトファイル (環境・共通フック・スイート)
│
├── oracle-init/
│   └── 01_init.sql              # DB 初期化 (testuser 作成 + USERS テーブル DDL)
│
├── testapi/                     # HTTP テスト対象サービス
│   ├── main.go                  # Go 1.22 パターンルーティング + go-ora CRUD
│   ├── openapi.yaml             # OpenAPI 3.0 定義
│   ├── go.mod
│   └── Dockerfile
│
├── testgrpc/                    # gRPC テスト対象サービス
│   ├── main.go                  # gRPC サーバー (サーバーリフレクション有効)
│   ├── proto/userv1/
│   │   ├── user.proto           # Protocol Buffers 定義
│   │   ├── user.pb.go           # protoc 生成コード
│   │   └── user_grpc.pb.go     # protoc 生成コード
│   ├── go.mod
│   └── Dockerfile
│
├── sql/
│   ├── common/
│   │   ├── before.sql           # 共通 before フック: TRUNCATE (PL/SQL)
│   │   └── after.sql            # 共通 after フック: 残存レコード確認 (PL/SQL)
│   └── plsql/
│       ├── seed_with_cursor.sql     # PL/SQL カーソル + コレクションでシードデータ INSERT
│       └── cleanup_with_exception.sql  # RAISE_APPLICATION_ERROR / EXCEPTION ハンドリングのサンプル
│
└── runbooks/
    ├── health_check.yaml        # GET /health
    ├── user_create.yaml         # POST /users + DB 検証
    ├── user_list.yaml           # GET /users (DB シード経由)
    ├── user_get.yaml            # GET /users/{id} + 404 確認
    ├── user_update.yaml         # PUT /users/{id} + DB 検証
    ├── user_delete.yaml         # DELETE /users/{id} + DB 検証
    ├── grpc_create_user.yaml    # gRPC CreateUser + DB 検証
    ├── grpc_list_users.yaml     # gRPC ListUsers
    ├── grpc_get_user.yaml       # gRPC GetUser + NotFound 確認
    ├── grpc_update_user.yaml    # gRPC UpdateUser + DB 検証
    ├── grpc_delete_user.yaml    # gRPC DeleteUser + DB 検証
    ├── plsql_hook_verify.yaml   # PL/SQL シードフック動作確認 (runnora: ブロックで前後処理を指定)
    └── plsql_after_hook_failure.yaml  # 事後処理の PL/SQL 例外の検知 (expect: hookFail)
```

---

## テスト対象サービス

### testapi (HTTP / REST)

| 項目 | 値 |
|---|---|
| ポート | `8081` |
| OpenAPI | `testapi/openapi.yaml` |
| エンドポイント | `GET /health`, `GET /users`, `POST /users`, `GET /users/{id}`, `PUT /users/{id}`, `DELETE /users/{id}` |
| DB | Oracle FREEPDB1 の `testuser.users` テーブル |

### testgrpc (gRPC)

| 項目 | 値 |
|---|---|
| ポート | `50051` |
| proto | `testgrpc/proto/userv1/user.proto` |
| サービス | `user.v1.UserService` |
| メソッド | `CreateUser`, `GetUser`, `ListUsers`, `UpdateUser`, `DeleteUser` |
| サーバーリフレクション | 有効 (runbook に proto ファイル指定不要) |

---

## 前提条件

- Docker / Docker Compose
- runnora バイナリ (`go build -o runnora .` でルートディレクトリからビルド)

---

## セットアップ

### 1. runnora をビルドする

```bash
# リポジトリルートで実行
cd /path/to/runnora
go build -o runnora .
```

### 2. Oracle DB を起動する

```bash
cd test
docker compose up -d oracle
```

Oracle Free は初回起動に **3〜5 分**かかります。ヘルスチェックが `healthy` になるまで待機してください。

```bash
# 状態確認
docker compose ps

# ログで初期化完了を確認
docker compose logs -f oracle
# "DATABASE IS READY TO USE!" が出力されたら完了
```

### 3. DB の初期化を確認する

Oracle コンテナの起動時に `oracle-init/01_init.sql` が `/opt/oracle/scripts/startup` から実行され、未作成なら `testuser` と `USERS` テーブルが作成されます。コンテナが healthy になった後、初期化結果を確認します。

```bash
docker exec -i oracle-free sqlplus -s "/ as sysdba" <<'SQL'
WHENEVER SQLERROR EXIT FAILURE
ALTER SESSION SET CONTAINER = FREEPDB1;
SELECT COUNT(*) FROM testuser.users;
EXIT
SQL
```

初期化スクリプトは再起動時にも実行されますが、既存のユーザーとテーブルは維持します。

### 4. テスト対象サービスを起動する

```bash
# testapi と testgrpc を起動
docker compose up -d testapi testgrpc
```

またはローカルビルドで直接起動する場合:

```bash
# testapi
cd testapi
go build -o testapi . && DB_DSN="oracle://testuser:TestPass1!@localhost:1521/FREEPDB1" ./testapi &

# testgrpc
cd testgrpc
go build -o testgrpc . && DB_DSN="oracle://testuser:TestPass1!@localhost:1521/FREEPDB1" ./testgrpc &
```

---

## テストの実行

以降のコマンドはすべて `test/` ディレクトリから実行します。`test/runnora.yaml` は自動で見つかるので、`--project` の指定は不要です。

```bash
cd test
```

### HTTP runbook を全て実行する

```bash
../runnora run \
  ./runbooks/health_check.yaml \
  ./runbooks/user_create.yaml \
  ./runbooks/user_list.yaml \
  ./runbooks/user_get.yaml \
  ./runbooks/user_update.yaml \
  ./runbooks/user_delete.yaml
```

### gRPC runbook を全て実行する

```bash
../runnora run \
  ./runbooks/grpc_create_user.yaml \
  ./runbooks/grpc_list_users.yaml \
  ./runbooks/grpc_get_user.yaml \
  ./runbooks/grpc_update_user.yaml \
  ./runbooks/grpc_delete_user.yaml
```

### HTTP + gRPC を一括実行する

```bash
../runnora run \
  ./runbooks/health_check.yaml \
  ./runbooks/user_create.yaml \
  ./runbooks/user_list.yaml \
  ./runbooks/user_get.yaml \
  ./runbooks/user_update.yaml \
  ./runbooks/user_delete.yaml \
  ./runbooks/grpc_create_user.yaml \
  ./runbooks/grpc_list_users.yaml \
  ./runbooks/grpc_get_user.yaml \
  ./runbooks/grpc_update_user.yaml \
  ./runbooks/grpc_delete_user.yaml
```

### PL/SQL フック動作確認

PL/SQL の前後処理は、runbook の `runnora:` ブロックに書きます。`plsql` スイートで 2 つの runbook をまとめて実行します。

```bash
../runnora run --suite plsql
```

`plsql_hook_verify.yaml` の `runnora:` ブロック:

```yaml
runnora:
  id: PLSQL-HOOK-VERIFY
  before: [sql/plsql/seed_with_cursor.sql]       # PL/SQL カーソルで 3 件 INSERT
  after: [sql/plsql/cleanup_with_exception.sql]  # 投入した 3 件を DELETE
```

**フック実行順序:**

```
[before] runnora.yaml の hooks.before (before.sql: TRUNCATE)
         → runnora: ブロックの before (seed_with_cursor.sql: PL/SQL カーソルで 3件 INSERT)
         → runbook 実行 (3件存在することを API / DB の両面で確認)
[after]  runnora: ブロックの after (cleanup_with_exception.sql: 3件 DELETE)
         → runnora.yaml の hooks.after (after.sql: 残存レコード数を DBMS_OUTPUT に出力)
```

`plsql_after_hook_failure.yaml` は、データが 0 件の状態で `cleanup_with_exception.sql` を事後処理として実行し、PL/SQL 例外 (`ORA-20001`) がフック失敗として検知されることを確かめます。`runnora:` ブロックに `expect: hookFail` と書いているため、期待どおりにフックが失敗すれば合格として数えます。

GitHub Actions の `Oracle PL/SQL hooks` ジョブでは Oracle Free と testapi を起動し、`runnora validate` で検査した後に `plsql` スイートを JSON レポートで実行します。2 つの runbook が期待どおりであること、フック失敗の内容に `ORA-20001` が含まれること、事後処理によって Oracle のデータが 0 件になっていることを確認します。

### レポートをファイルに出力する

```bash
../runnora run \
  --report-format text \
  --report-out /tmp/result.txt \
  ./runbooks/*.yaml
cat /tmp/result.txt
```

### 失敗時に即停止する (fail-fast)

```bash
../runnora run --fail-fast \
  ./runbooks/health_check.yaml \
  ./runbooks/user_create.yaml
```

---

## PL/SQL フックの詳細

### 環境の共通フック (runnora.yaml の hooks。全 runbook に自動適用)

| ファイル | タイミング | 内容 |
|---|---|---|
| `sql/common/before.sql` | runbook 実行前 | `EXECUTE IMMEDIATE 'TRUNCATE TABLE testuser.users'` でテストデータをリセット |
| `sql/common/after.sql` | runbook 実行後 | `SELECT COUNT(*) INTO v_cnt` で残存レコードを確認し `DBMS_OUTPUT` に警告を出力 |

`before.sql` は PL/SQL の匿名ブロックとして実行されます。`TRUNCATE` は DDL のため `EXECUTE IMMEDIATE` を使って発行しています。

### 専用 PL/SQL サンプル (`sql/plsql/`)

| ファイル | 確認できる PL/SQL 機能 |
|---|---|
| `seed_with_cursor.sql` | `TYPE ... IS RECORD`, `TABLE OF ... INDEX BY PLS_INTEGER`, `WHILE LOOP`, `COMMIT` |
| `cleanup_with_exception.sql` | `RAISE_APPLICATION_ERROR`, `EXCEPTION WHEN OTHERS`, `ROLLBACK / RAISE` |

`cleanup_with_exception.sql` はデータが 0 件のとき意図的にエラーを発生させます。**異常系フックのテスト**（終了コード 4 の確認）に使用できます。

```bash
# 異常系: before.sql で TRUNCATE 後に cleanup_with_exception.sql を実行 → フック失敗
# plsql_after_hook_failure.yaml は expect: hookFail なので、期待どおりの失敗として合格 (exit 0) になる
../runnora run ./runbooks/plsql_after_hook_failure.yaml
echo "exit code: $?"   # 0 が返る (expect を書かない runbook でフックが失敗した場合は 4)
```

---

## runbook の構造

各 runbook は次のパターンで構成されています。

```yaml
desc: 説明文
runners:
  req:                              # HTTP ランナー
    endpoint: http://localhost:8081
  greq:                             # gRPC ランナー (サーバーリフレクションで proto 自動取得)
    addr: localhost:50051
    tls: false
  db:                               # DB ランナー (Oracle)
    dsn: "oracle://testuser:TestPass1%21@localhost:1521/FREEPDB1"

steps:
  step_name:
    req:                            # HTTP リクエスト
      /path:
        method:
          body: ...
    test: steps.step_name.res.status == 200   # アサーション (expr-lang)

  bind_step:
    bind:                           # 後続ステップへの値の受け渡し
      var_name: steps.prev.res.body.id

  db_step:
    db:
      query: SELECT ... WHERE id = {{ var_name }}   # テンプレートで変数を埋め込む
    test: steps.db_step.rows[0].COL == "value"
```

### 注意事項

| 項目 | 詳細 |
|---|---|
| Oracle NUMBER 型 | go-ora は NUMBER を Go の `string` で返す。DB アサーションは `== "1"` のように文字列で比較する |
| gRPC ステータスコード | `0`=OK, `5`=NotFound, `6`=AlreadyExists, `3`=InvalidArgument |
| テンプレート変数 | 数値フィールドに埋め込む場合は `"{{ var }}"` とクォートが必要 (YAML パーサの誤認識を防ぐ) |
| `size()` vs `len()` | runn は `expr-lang/expr` を使用。`size()` は CEL 専用のため `len()` を使う |

---

## 終了コード

| コード | 意味 |
|---|---|
| `0` | 全 runbook 成功 |
| `1` | runbook 失敗 (アサーション失敗など) |
| `2` | 設定・引数不正 |
| `3` | DB 接続失敗 |
| `4` | before / after フック失敗 |
| `5` | レポート出力失敗 |
