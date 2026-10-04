# gRPC の64ビット整数と数値文字列の比較

ProtoJSON は64ビット整数を文字列にする。一方、別プログラムが出す計算結果の期待値は JSON 数値になるため、型の不一致だけを理由に全体比較が失敗する。期待値の出力元の変更や ID 名の列挙に依存せず、epsilon を適用できるようにする。

## 責務

- runnora-diff は汎用 JSON 比較を維持する。`Options.NumericStrings` / 設定の `numericStrings` / CLI の `--numeric-string` で選択されたノードだけ、数値文字列を既存の任意精度数値比較に渡す。proto の依存を追加しない。
- runnora は RPC のレスポンス descriptor を参照し、64ビット整数の対象パスを生成して `NumericStrings` に追加する。呼び出しごとに設定をコピーし、キャッシュされた設定を変更しない。
- `loadJSON()` は期待値を `json.Number` で読み込み、比較前の float64 の丸めを避ける。

## 型情報の取得と適用範囲

runn の Capturer を包み、`CaptureGRPCStart` で `protoregistry.GlobalFiles` からサービスのメソッドと出力 descriptor を取得する。runn はローカル proto のコンパイルまたはサーバーリフレクションによる解決後、RPC 開始前に descriptor を登録する。

`CaptureGRPCResponseMessage` ではレスポンスとネストしたコンテナの同一性に descriptor を対応付ける。元の値は変更せず保持し、アドレスが再利用されないよう、runbook の実行中は参照を保持する。レスポンスの形やフィールド名だけで通常の JSON に型を推測しない。証跡の有効・無効とは独立して記録する。

`diffEps()` の生の引数から対応するコンテナを探し、実データと descriptor を再帰的に辿る。64ビット整数のフィールドと Int64Value / UInt64Value は数値比較、通常の文字列は従来の比較とする。repeated には任意の要素数を扱うパスを生成し、未設定の子メッセージが先頭にある場合にも後続の要素を比較できるよう `[]?` を使う。再帰的なメッセージ型は実データの深さで探索を終了する。

map は自動判定の対象外。単独のスカラー、JSON 化後のコピー、Any 内の動的型は型の対応付けを保持しないので、汎用の `numericStrings` を明示する。欠落・追加・null・配列長・順序は数値文字列への対応によって変更しない。

型情報は runn が登録したグローバル descriptor に従う。異なるスキーマで同じサービス名・ファイル名を使う runbook を同一プロセスで実行するケースは、runn のグローバル登録の衝突処理に依存する。スキーマごとに別プロセスで実行する。

## 精度と許容誤差

数値文字列は float64 に変換せず、元のリテラルで比較する。両側が文字列の場合にも数値比較を適用する。epsilon は従来の `default` / `tolerances` で指定する。ID を完全一致にする場合は対応する許容誤差を0にする。

`loadJSON("cases/expected.json")` のパス基準はプロジェクトルート。`json://` や HTTP の body など、runn が既に float64 にした値は復元できない。期待値の64ビット整数は `loadJSON()` から直接 `diffEps()` に渡す。

## 検証

runnora-diff の既定の型比較、数値文字列の明示指定、絶対・相対誤差、64ビット整数の最大・最小値と隣接値、通常の文字列、null・欠落・unordered を検証する。runnora は全64ビット整数型、ネスト・repeated・未設定の子メッセージ・map の対象外を検証する。実際の gRPC サーバーとの通信でも proto 指定とサーバーリフレクションの両方を使い、同一ステップと後続ステップ、ストリーミング、エビデンスなしでの自動比較を検証する。

2026-10-05 のローカル検証結果（Windows / Go 1.26）:

- runnora-diff: `go test -race ./...`、`go vet ./...`、`go mod tidy -diff` が通過。
- runnora: `go test -race ./internal/diffeps ./internal/app`、`go vet ./...` が通過。正式版 runnora-diff v0.2.2 を使用し、`GOWORK=off` でも両チェックが通過。descriptor が取得できない場合のエラーと、設定キャッシュに自動生成したパスが残らないことも検証。
- runnora の `go test ./...` は cmd のパス表記、migrate の Git 作業ツリー / golden、evidence の既存テストで失敗。変更前の HEAD を別ディレクトリに展開して実行し、同じ失敗が再現することを確認。
- runnora の `go mod tidy -diff` に残る yaml/v4 の direct / indirect 分類と go.sum の差分も変更前の HEAD で再現。今回の変更では整理しない。
- staticcheck はインストール済みバイナリが Go 1.24.3 でビルドされており、Go 1.26 の export data を読めないため未検証。

## ローカル開発

go.mod は正式版 runnora-diff v0.2.2 に依存しており、runnora 単独でビルドと検証ができる。両リポジトリを同時に修正する場合は Go workspace で連携できる（go.work は既存の .gitignore の対象）。

```sh
go work init . ../json-diff-with-epsilon
go test ./...
```

runnora-diff の API を追加変更する場合も、runnora のリリース前には runnora-diff を先にリリースし、go.mod の依存バージョンをそのリリースに更新する。依存を更新した際は `go run scripts/licenses.go` で THIRD_PARTY_NOTICES.md を再生成する。公開していないバージョンを仮に指定したり、配布用の go.mod にローカル replace を残したりしない。
