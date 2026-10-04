# ライセンスと配布条件

## 自作部分と第三者部分

runnora の自作部分は MIT License、著作権名義は `Copyright (c) 2026 ramsesyok` です。商用利用、改変、有償での配布を許可します。コピーや重要部分の配布には LICENSE の著作権表示と許諾表示を保持してください。

依存ライブラリや転用コードには元のライセンスが適用されます。runnora の MIT 表示によって第三者部分の条件を置き換えることはありません。必要な本文・著作権・NOTICE は [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) に収録します。MIT / BSD / ISC の表示条件、Apache-2.0 のライセンス・NOTICE・変更表示など、対象に応じた義務を維持します。

## MPL-2.0 は runn 経由の依存

現在の通常ビルドで確認した最短経路は次のとおりです。

| MPL-2.0 のモジュール | バージョン | 依存経路 |
|---|---|---|
| github.com/go-sql-driver/mysql | v1.10.0 | runnora/cmd → runn → mysql |
| github.com/hashicorp/go-envparse | v0.1.0 | runnora/cmd → runn → go-envparse |
| github.com/hashicorp/golang-lru/v2 | v2.0.7 | runnora/cmd → runn → go-sql-spanner → parser → golang-lru/v2 |

`go mod why -m` とソースの import を確認しました。runnora の自作ソースから3件への直接 import はありませんが、runnora のバイナリには組み込まれます。間接依存でも配布時の条件は必要です。

MPL-2.0 は商用利用を許可し、別ファイルの自作コードを MIT で公開することも認めます。静的リンク自体も runnora 全体を MPL に変更する理由にはなりません。MPL 対象コードをコピーした新しいファイルや、対象ファイルの改変には MPL が適用されます。

バイナリ配布時は、変更していない場合も、対象ソースを MPL の条件で取得する方法を受領者に案内します。THIRD_PARTY_NOTICES.md の固定バージョンのソースアーカイブがその取得先です。現在は upstream の対象ファイルを変更せずに使用します。将来変更する場合は、変更後の対象ソースも公開して案内を更新してください。

根拠: [MPL-2.0 3.1〜3.4](https://www.mozilla.org/en-US/MPL/2.0/)、[Mozilla FAQ Q5 / Q8 / Q11](https://www.mozilla.org/en-US/MPL/2.0/FAQ/)。

## 表示の更新とリリース

```sh
go run scripts/licenses.go
go run scripts/licenses.go -check
```

生成ツールは CGO 無効の Linux / macOS / Windows、amd64 / arm64 の通常ビルドの依存を集約し、モジュールとパッケージの祖先ディレクトリにあるライセンス・NOTICE を保持します。runn の転用コード用 `_EXTRA_CREDITS`、modernc の第三者表示も収録します。Go のランタイムと標準ライブラリの LICENSE も収録します。

CI は表示の鮮度を検査し、GoReleaser はリリース時に表示を再生成して、LICENSE と THIRD_PARTY_NOTICES.md を各アーカイブに同梱します。ソース取得先はバージョンを固定します。Go proxy の取得ができなくなった場合も受領者が取得できるよう、リリース担当者は対象ソースを保管してください。

これは互換性を自動判定する仕組みではありません。依存を更新・追加する際は、新しい本文、ソース内だけのライセンス、生成・転用コード、変更した MPL 部分をレビューしてください。ライセンスが空・未確定の依存は、正式な許諾のあるバージョンに更新してから配布します。`replace` を使う構成は取得先と改変内容の個別確認が必要なため、生成を止めます。

## 外部ツールとテスト環境

WireMock、Java、Oracle Database、Quarto、ドキュメント生成ツール、Docker などは、それぞれの利用・配布条件に従います。runnora の MIT はこれらの製品を再許諾するものではありません。特に Oracle Database Free は Oracle Free Use Terms and Conditions に基づく外部製品です。テスト用のコンテナ参照を、そのイメージを MIT で再配布できるという意味に解釈しないでください。

`modernc.org/libc` の GPL-2.0 テストデータは通常ビルドのパッケージとして含まれません。依存ソースのテストデータをまとめて再配布する場合は、バイナリの表示とは別に当該ファイルの条件を確認してください。

過去のタグ・配布アーカイブは、この変更だけでは書き換わりません。過去版を配布する場合も、該当バージョンに対応する表示とソース取得案内を提供してください。
