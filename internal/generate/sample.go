package generate

import (
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"go.yaml.in/yaml/v4"
)

// extractSampleFromMediaType は MediaType から JSON サンプル値を抽出する。
//
// 優先順位 (設計書 §13.1):
//  1. content.application/json.example
//  2. content.application/json.examples (最初の entry の Value)
//  3. schema.example
//  4. schema.default
//  5. 型ベース自動サンプル (schema のプロパティを再帰的に辿る, 深さ 3 まで)
//  6. nil (呼び出し元が適切なプレースホルダーを補完する)
func extractSampleFromMediaType(mt *v3.MediaType) interface{} {
	if mt == nil {
		return nil
	}

	// 1. media type レベルの example
	if mt.Example != nil {
		if v := yamlNodeToInterface(mt.Example); v != nil {
			return v
		}
	}

	// 2. examples マップの最初の entry
	if mt.Examples != nil {
		for pair := mt.Examples.Oldest(); pair != nil; pair = pair.Next() {
			if pair.Value != nil && pair.Value.Value != nil {
				if v := yamlNodeToInterface(pair.Value.Value); v != nil {
					return v
				}
			}
			break // 最初の 1 件だけ使う
		}
	}

	// 3-5. スキーマから抽出
	if mt.Schema != nil {
		schema, schemaErr := mt.Schema.BuildSchema()
		if schemaErr == nil && schema != nil {
			return schemaToSample(schema, 0)
		}
	}

	return nil
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
	for pair := resp.Content.Oldest(); pair != nil; pair = pair.Next() {
		if strings.Contains(pair.Key, "application/json") {
			return extractSampleFromMediaType(pair.Value)
		}
	}
	return nil
}

// schemaToSample はスキーマを再帰的に辿って型ベースのサンプル値を生成する。
//
// 優先順位: schema.example → schema.default → 型ベース自動生成
// depth は再帰の深さ上限 (3 まで)。
func schemaToSample(schema *base.Schema, depth int) interface{} {
	if schema == nil || depth > 3 {
		return nil
	}

	// schema.example
	if schema.Example != nil {
		if v := yamlNodeToInterface(schema.Example); v != nil {
			return v
		}
	}

	// schema.default
	if schema.Default != nil {
		if v := yamlNodeToInterface(schema.Default); v != nil {
			return v
		}
	}
	if len(schema.Enum) > 0 {
		if v := yamlNodeToInterface(schema.Enum[0]); v != nil {
			return v
		}
	}

	// 型ベース生成
	t := schemaType(schema)
	switch t {
	case "string":
		if schema.Format == "date" {
			return "2006-01-02"
		}
		if schema.Format == "date-time" {
			return "2006-01-02T15:04:05Z"
		}
		if schema.Format == "uuid" {
			return "00000000-0000-0000-0000-000000000000"
		}
		return "TODO_string"
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		if v := arrayItemSample(schema); v != nil {
			return []interface{}{v}
		}
		return []interface{}{}
	case "object":
		return objectSample(schema, depth)
	}

	// type 不明の場合: properties があればオブジェクト扱い
	if schema.Properties != nil {
		return objectSample(schema, depth)
	}

	return nil
}

// objectSample は object 型のスキーマからプロパティを辿ってサンプルを生成する。
func objectSample(schema *base.Schema, depth int) map[string]interface{} {
	result := map[string]interface{}{}
	if schema.Properties == nil {
		return result
	}
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		propSchema, propErr := pair.Value.BuildSchema()
		if propErr != nil || propSchema == nil {
			result[pair.Key] = nil
			continue
		}
		result[pair.Key] = schemaToSample(propSchema, depth+1)
	}
	return result
}

// schemaType は Schema.Type の最初の要素を返す。
// OpenAPI 3.0 では単一文字列、3.1 では配列なので最初の要素を使う。
func schemaType(schema *base.Schema) string {
	if len(schema.Type) > 0 {
		return schema.Type[0]
	}
	return ""
}

// yamlNodeToInterface は *yaml.Node を Go の interface{} に変換する。
// 変換に失敗した場合は nil を返す。
func yamlNodeToInterface(node *yaml.Node) interface{} {
	if node == nil {
		return nil
	}
	var v interface{}
	if err := node.Decode(&v); err != nil {
		return nil
	}
	return v
}
