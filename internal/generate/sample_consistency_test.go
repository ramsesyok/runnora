package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ramsesyok/oapi2wire/pkg/oapi2wire"
	oasample "github.com/ramsesyok/oapi2wire/pkg/sample"
	"gopkg.in/yaml.v3"
)

// 同じ OpenAPI から作った runnora generate のテストケースと oapi2wire init のモックが、
// パラメータ・リクエスト本文・応答本文で同じ値になること (docs/design/sample-generation.md の 9 章)。
const consistencySpec = `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /books/{bookId}:
    get:
      operationId: getBook
      parameters:
        - {name: bookId, in: path, required: true, schema: {type: string}, example: B0001}
        - {name: mode, in: query, required: true, schema: {type: string, enum: [detail, simple]}}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Book"}
  /books:
    post:
      operationId: createBook
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: "#/components/schemas/Book"}
      responses:
        "201":
          description: created
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Book"}
components:
  schemas:
    Book:
      type: object
      properties:
        bookId: {type: string, readOnly: true}
        title: {type: string}
        isbn: {type: string, pattern: "^[0-9]{13}$"}
        copies: {type: integer, minimum: 1}
        price: {type: number}
        available: {type: boolean}
        publishedOn: {type: string, format: date}
        tags: {type: array, items: {type: string}}
        genre: {type: string, enum: [NOVEL, TECH]}
`

func TestSamplesMatchOapi2wireInit(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(spec, []byte(consistencySpec), 0o644); err != nil {
		t.Fatal(err)
	}
	ops, err := LoadOperations(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oapi2wire.Init(oapi2wire.InitOptions{
		OpenAPIPath: spec, OutCasesPath: filepath.Join(dir, "mock-cases.yaml"), ResponsesRoot: filepath.Join(dir, "responses"),
	}); err != nil {
		t.Fatal(err)
	}
	mocks := readMockCases(t, filepath.Join(dir, "mock-cases.yaml"))

	for _, op := range ops {
		t.Run(op.OperationID, func(t *testing.T) {
			mock, ok := mocks[op.OperationID+"_default"]
			if !ok {
				t.Fatalf("mock case %s_default not found", op.OperationID)
			}
			// 応答本文: テストの期待本文 = モックの応答 JSON
			var stub any
			b, err := os.ReadFile(filepath.Join(dir, "responses", op.OperationID, op.OperationID+"_default.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &stub); err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, op.ExpectBodySample, stub) {
				t.Errorf("response: runnora %s, oapi2wire %s", toJSON(t, op.ExpectBodySample), b)
			}
			// パラメータ: テストが送る値 = モックの一致条件
			for _, p := range op.PathParams {
				if got := mock.Request.PathParams[p.Name].EqualTo; got != oasample.String(p.Sample) {
					t.Errorf("path %s: runnora %v, oapi2wire %q", p.Name, p.Sample, got)
				}
			}
			for _, p := range op.QueryParams {
				if m, ok := mock.Request.Query[p.Name]; ok && m.EqualTo != oasample.String(p.Sample) {
					t.Errorf("query %s: runnora %v, oapi2wire %q", p.Name, p.Sample, m.EqualTo)
				}
			}
			// リクエスト本文: テストが送る本文 = モックの equalToJson
			if op.RequestBodySample != nil && !sameJSON(t, op.RequestBodySample, mock.Request.Body.EqualToJSON) {
				t.Errorf("request body: runnora %s, oapi2wire %s", toJSON(t, op.RequestBodySample), toJSON(t, mock.Request.Body.EqualToJSON))
			}
		})
	}
}

type mockCase struct {
	ID      string `yaml:"id"`
	Request struct {
		PathParams map[string]struct {
			EqualTo string `yaml:"equalTo"`
		} `yaml:"pathParams"`
		Query map[string]struct {
			EqualTo string `yaml:"equalTo"`
		} `yaml:"query"`
		Body struct {
			EqualToJSON any `yaml:"equalToJson"`
		} `yaml:"body"`
	} `yaml:"request"`
}

func readMockCases(t *testing.T, path string) map[string]mockCase {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []mockCase `yaml:"cases"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	out := map[string]mockCase{}
	for _, c := range f.Cases {
		out[c.ID] = c
	}
	return out
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sameJSON は 2 つの値を JSON にして比べる (数値の型の違いを無視する)。
func sameJSON(t *testing.T, a, b any) bool {
	t.Helper()
	var x, y any
	_ = json.Unmarshal([]byte(toJSON(t, a)), &x)
	_ = json.Unmarshal([]byte(toJSON(t, b)), &y)
	return reflect.DeepEqual(x, y)
}
