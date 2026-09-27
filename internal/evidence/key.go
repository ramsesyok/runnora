// Package evidence は runbook の実行中の送受信を、ステップごとの証跡ファイルとして保存する。
//
// runn の Capturer を実装し、runn が渡す HTTP / gRPC / DB / exec の送受信を、
// 実行中のステップ (runn の Trail) ごとに集めて JSON ファイルに書き出す。
// 詳細は docs/design/evidence-report.md の 4 章。
package evidence

import (
	"strconv"
	"strings"

	"github.com/k1LoW/runn"
)

// StepKey は Trail から、ステップのキーとトップレベルのステップの番号 (1 始まり) を作る。
//
//   - トップレベルのステップ: "create_loan"
//   - include 先のステップ: "inspect_history_book_0.call" (呼び出したステップのキーと . でつなぐ)
//   - loop の回 (0 始まり): "member_loans[2]"
//   - steps が配列で書かれた runbook: 配列の添字をキーにする ("3")
//
// ステップの Trail がなければ ok は false。
func StepKey(trs runn.Trails) (key string, index int, ok bool) {
	var segs []string
	for _, tr := range trs {
		switch tr.Type {
		case runn.TrailTypeStep:
			k := tr.StepKey
			if k == "" && tr.StepIndex != nil {
				k = strconv.Itoa(*tr.StepIndex)
			}
			if len(segs) == 0 && tr.StepIndex != nil {
				index = *tr.StepIndex + 1
			}
			segs = append(segs, k)
		case runn.TrailTypeLoop:
			// ステップの loop と runbook (include 先) の loop のどちらも、直前のステップの回として扱う
			if tr.LoopIndex != nil && len(segs) > 0 {
				segs[len(segs)-1] += "[" + strconv.Itoa(*tr.LoopIndex) + "]"
			}
		}
	}
	if len(segs) == 0 {
		return "", 0, false
	}
	return strings.Join(segs, "."), index, true
}

// fileNameReplacer はファイル名に使えない文字を _ に置き換える。
var fileNameReplacer = strings.NewReplacer(
	"/", "_", `\`, "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
)

// SafeName はキーやシナリオ ID をファイル名・ディレクトリ名に使える形にする。
func SafeName(s string) string {
	return fileNameReplacer.Replace(s)
}
