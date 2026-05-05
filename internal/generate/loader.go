// Package generate は OpenAPI 定義ファイルからテスト資産を生成する機能を提供する。
//
// 生成物は以下の 3 層で構成される。
//  1. template runbook  — 1 つの case を受け取って 1 回 API を呼ぶ薄い runbook
//  2. case JSON         — request / expect の具体値を保持するファイル
//  3. suite runbook     — 複数 case を loop で回し template を include する runbook
package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// OperationInfo は OpenAPI 定義の 1 つの operation を表す。
// LoadOperations によって抽出され、各 Emitter に渡される。
type OperationInfo struct {
	// OpenAPI から読み取った情報
	Path        string   // e.g. "/users/{id}"
	Method      string   // lowercase: "get", "post", "put", "delete", …
	OperationID string   // operationId (なければ自動生成)
	Tags        []string // operation の tags
	Summary     string   // operation の summary
	Deprecated  bool     // deprecated フラグ

	// 生成物の識別子
	OperationKey string // ファイル名等に使う: e.g. "post_createUser"
	PrimaryTag   string // 最初の tag (なければ "default")

	// サンプルデータ
	RequestBodySample      interface{} // nil = request body なし
	RequestBodyContentType string
	MultipartFields        []MultipartField
	PathParams             []ParameterInfo
	QueryParams            []ParameterInfo
	ExpectStatus           int         // 代表成功ステータス (0 → 200 にフォールバック)
	ExpectBodySample       interface{} // nil = response body サンプルなし

	// template runbook 用パス
	// path params を "{{ vars.case.pathParams.X }}" 形式に変換済み
	RunbookPath string
}

type ParameterInfo struct {
	Name    string
	Style   string
	Explode bool
	Sample  interface{}
}

type MultipartField struct {
	Name   string
	IsFile bool
	Sample interface{}
}

// LoadOperations は OpenAPI ファイル (YAML/JSON) を解析して operation 一覧を返す。
// OpenAPI 3.0.x / 3.1.x に対応する。
func LoadOperations(path string) ([]*OperationInfo, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("generate: read openapi %s: %w", path, err)
	}

	doc, docErr := libopenapi.NewDocument(data)
	if docErr != nil {
		return nil, fmt.Errorf("generate: parse openapi: %w", docErr)
	}

	v3doc, buildErr := doc.BuildV3Model()
	if buildErr != nil {
		return nil, fmt.Errorf("generate: build v3 model: %w", buildErr)
	}

	if v3doc.Model.Paths == nil || v3doc.Model.Paths.PathItems == nil {
		return nil, nil
	}

	var ops []*OperationInfo
	for pair := v3doc.Model.Paths.PathItems.Oldest(); pair != nil; pair = pair.Next() {
		apiPath := pair.Key
		item := pair.Value

		for _, m := range pathItemMethods(item) {
			if m.op == nil {
				continue
			}
			info, buildErr := buildOperationInfo(apiPath, m.method, item.Parameters, m.op)
			if buildErr != nil {
				return nil, buildErr
			}
			ops = append(ops, info)
		}
	}
	return ops, nil
}

// methodOp は HTTP メソッド名と対応する Operation をペアで保持する。
type methodOp struct {
	method string
	op     *v3.Operation
}

// pathItemMethods は PathItem から全 HTTP メソッドをスライスで返す。
// nil の Operation は含むが、呼び出し元で nil チェックする。
func pathItemMethods(item *v3.PathItem) []methodOp {
	return []methodOp{
		{"get", item.Get},
		{"post", item.Post},
		{"put", item.Put},
		{"delete", item.Delete},
		{"patch", item.Patch},
		{"head", item.Head},
		{"options", item.Options},
		{"trace", item.Trace},
	}
}

// buildOperationInfo は 1 つの operation メタデータと生成用情報を組み立てる。
func buildOperationInfo(apiPath, method string, pathParams []*v3.Parameter, op *v3.Operation) (*OperationInfo, error) {
	info := &OperationInfo{
		Path:    apiPath,
		Method:  method,
		Summary: op.Summary,
		Tags:    op.Tags,
	}

	if op.Deprecated != nil {
		info.Deprecated = *op.Deprecated
	}

	info.OperationID = op.OperationId

	// PrimaryTag: 最初のタグ、なければ "default"
	if len(info.Tags) > 0 {
		info.PrimaryTag = info.Tags[0]
	} else {
		info.PrimaryTag = "default"
	}

	// OperationKey: ファイル名などに使う識別子
	// operationId があれば "<method>_<operationId>", なければパスを正規化して使う
	if info.OperationID != "" {
		info.OperationKey = method + "_" + info.OperationID
	} else {
		normalized := strings.ReplaceAll(strings.Trim(apiPath, "/"), "/", "_")
		normalized = strings.ReplaceAll(normalized, "{", "")
		normalized = strings.ReplaceAll(normalized, "}", "")
		info.OperationKey = method + "_" + normalized
	}

	// RunbookPath: path params を runn 式に変換
	// e.g. "/users/{id}" → "/users/{{ vars.case.pathParams.id }}"
	params := mergeParameters(pathParams, op.Parameters)
	info.PathParams = extractParameters(params, "path")
	info.QueryParams = extractParameters(params, "query")
	info.RunbookPath = appendQueryParams(convertPathParams(apiPath), info.QueryParams)

	// Request body サンプルを抽出する
	// Content は *orderedmap.Map[string, *v3.MediaType] なので Oldest() で順に走査する
	if op.RequestBody != nil && op.RequestBody.Content != nil {
		for pair := op.RequestBody.Content.Oldest(); pair != nil; pair = pair.Next() {
			if strings.Contains(strings.ToLower(pair.Key), "multipart/form-data") {
				info.RequestBodyContentType = "multipart/form-data"
				info.MultipartFields = extractMultipartFields(pair.Value)
				info.RequestBodySample = multipartSample(info.MultipartFields)
				break
			}
			if strings.Contains(strings.ToLower(pair.Key), "application/json") {
				info.RequestBodyContentType = "application/json"
				info.RequestBodySample = extractSampleFromMediaType(pair.Value)
				break
			}
		}
	}

	// レスポンスのサンプルを抽出する
	if op.Responses != nil {
		info.ExpectStatus, info.ExpectBodySample = extractResponseSample(op.Responses)
	}
	if info.ExpectStatus == 0 {
		info.ExpectStatus = 200
	}

	return info, nil
}

func mergeParameters(pathParams, operationParams []*v3.Parameter) []*v3.Parameter {
	merged := make([]*v3.Parameter, 0, len(pathParams)+len(operationParams))
	overrides := map[string]struct{}{}
	for _, p := range operationParams {
		if p == nil {
			continue
		}
		overrides[p.In+"\x00"+p.Name] = struct{}{}
	}
	for _, p := range pathParams {
		if p == nil {
			continue
		}
		if _, ok := overrides[p.In+"\x00"+p.Name]; ok {
			continue
		}
		merged = append(merged, p)
	}
	merged = append(merged, operationParams...)
	return merged
}

func extractParameters(params []*v3.Parameter, in string) []ParameterInfo {
	var result []ParameterInfo
	for _, p := range params {
		if p == nil || p.In != in || p.Name == "" {
			continue
		}
		explode := true
		if p.Explode != nil {
			explode = *p.Explode
		}
		result = append(result, ParameterInfo{
			Name:    p.Name,
			Style:   p.Style,
			Explode: explode,
			Sample:  parameterSample(p),
		})
	}
	return result
}

func appendQueryParams(path string, params []ParameterInfo) string {
	if len(params) == 0 {
		return path
	}

	var sb strings.Builder
	sb.WriteString(path)
	if strings.Contains(path, "?") {
		sb.WriteByte('&')
	} else {
		sb.WriteByte('?')
	}
	for i, p := range params {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(p.Name)
		sb.WriteString("={{ vars.case.queryParams.")
		sb.WriteString(p.Name)
		sb.WriteString(" }}")
	}
	return sb.String()
}

func parameterSample(p *v3.Parameter) interface{} {
	if p == nil {
		return nil
	}
	if p.Example != nil {
		if v := yamlNodeToInterface(p.Example); v != nil {
			return v
		}
	}
	if p.Examples != nil {
		for pair := p.Examples.Oldest(); pair != nil; pair = pair.Next() {
			if pair.Value != nil && pair.Value.Value != nil {
				if v := yamlNodeToInterface(pair.Value.Value); v != nil {
					return v
				}
			}
			break
		}
	}
	if p.Schema == nil {
		return nil
	}
	schema, err := p.Schema.BuildSchema()
	if err != nil || schema == nil {
		return nil
	}
	if schemaType(schema) == "array" {
		if v := arrayItemSample(schema); v != nil {
			return v
		}
	}
	return schemaToSample(schema, 0)
}

func arrayItemSample(schema *base.Schema) interface{} {
	if schema == nil || schema.Items == nil || !schema.Items.IsA() || schema.Items.A == nil {
		return nil
	}
	itemSchema, err := schema.Items.A.BuildSchema()
	if err != nil || itemSchema == nil {
		return nil
	}
	return schemaToSample(itemSchema, 0)
}

func extractMultipartFields(mt *v3.MediaType) []MultipartField {
	if mt == nil || mt.Schema == nil {
		return nil
	}
	schema, err := mt.Schema.BuildSchema()
	if err != nil || schema == nil || schema.Properties == nil {
		return nil
	}

	var fields []MultipartField
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		propSchema, propErr := pair.Value.BuildSchema()
		if propErr != nil || propSchema == nil {
			fields = append(fields, MultipartField{Name: pair.Key, Sample: nil})
			continue
		}
		isFile := schemaType(propSchema) == "string" && propSchema.Format == "binary"
		sample := schemaToSample(propSchema, 0)
		if isFile {
			sample = "TODO: path/to/file"
		}
		fields = append(fields, MultipartField{
			Name:   pair.Key,
			IsFile: isFile,
			Sample: sample,
		})
	}
	return fields
}

func multipartSample(fields []MultipartField) map[string]interface{} {
	result := map[string]interface{}{}
	for _, field := range fields {
		result[field.Name] = field.Sample
	}
	return result
}

// convertPathParams は OpenAPI パスの {param} を runn 式に変換する。
// e.g. "/users/{id}/posts/{postId}" → "/users/{{ vars.case.pathParams.id }}/posts/{{ vars.case.pathParams.postId }}"
func convertPathParams(path string) string {
	var sb strings.Builder
	i := 0
	for i < len(path) {
		if path[i] == '{' {
			j := strings.IndexByte(path[i:], '}')
			if j < 0 {
				sb.WriteByte(path[i])
				i++
				continue
			}
			paramName := path[i+1 : i+j]
			sb.WriteString("{{ vars.case.pathParams.")
			sb.WriteString(paramName)
			sb.WriteString(" }}")
			i += j + 1
		} else {
			sb.WriteByte(path[i])
			i++
		}
	}
	return sb.String()
}
