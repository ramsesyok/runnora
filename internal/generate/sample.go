package generate

import (
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	oasample "github.com/ramsesyok/oapi2wire/pkg/sample"
)

// extractSampleFromMediaType は MediaType から JSON サンプル値を抽出する。
//
// 値の決め方は oapi2wire と共通 (pkg/sample)。同じ OpenAPI から作ったモック (oapi2wire init) と
// テストケース (runnora generate) の本文が同じ値になる:
//  1. content.application/json.example
//  2. content.application/json.examples (最初の entry の Value)
//  3. スキーマの値 (example → default → enum の最初 → allOf/anyOf/oneOf の最初 → 型による値)
//  4. nil (呼び出し元が適切なプレースホルダーを補完する)
func extractSampleFromMediaType(mt *v3.MediaType, mode oasample.Mode) interface{} {
	return oasample.MediaType(mt, mode)
}

// extractResponseSample は Responses から代表成功レスポンスを取得する。
//
// ステータスコード選択優先順 (設計書 §13.3):
//  1. 200, 201, 202, 204
//  2. その他の 2xx
//  3. "default" レスポンス
//
// 戻り値: (representativeStatus, bodySample)
func extractResponseSample(responses *v3.Responses) (int, interface{}) {
	if responses == nil {
		return 0, nil
	}

	if responses.Codes != nil {
		// 1. 優先ステータスを順番にチェック
		for _, code := range []string{"200", "201", "202", "204"} {
			for pair := responses.Codes.Oldest(); pair != nil; pair = pair.Next() {
				if pair.Key == code {
					status, _ := strconv.Atoi(code)
					return status, responseBodySample(pair.Value)
				}
			}
		}

		// 2. その他の 2xx
		for pair := responses.Codes.Oldest(); pair != nil; pair = pair.Next() {
			if strings.HasPrefix(pair.Key, "2") {
				status, _ := strconv.Atoi(pair.Key)
				if status == 0 {
					status = 200
				}
				return status, responseBodySample(pair.Value)
			}
		}
	}

	// 3. "default" レスポンス
	if responses.Default != nil {
		return 200, responseBodySample(responses.Default)
	}

	return 0, nil
}

// responseBodySample は Response の content から application/json のサンプルを抽出する。
func responseBodySample(resp *v3.Response) interface{} {
	if resp == nil || resp.Content == nil {
		return nil
	}
	return extractSampleFromMediaType(oasample.JSONMediaType(resp.Content), oasample.Response)
}

// schemaType は Schema.Type の最初の要素を返す。
// OpenAPI 3.0 では単一文字列、3.1 では配列なので最初の要素を使う。
func schemaType(schema *base.Schema) string {
	if len(schema.Type) > 0 {
		return schema.Type[0]
	}
	return ""
}
