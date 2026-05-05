package generate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCaseDataMarshalKeepsTopLevelFieldOrder(t *testing.T) {
	op := &OperationInfo{
		Method:       "post",
		Path:         "/pets",
		Summary:      "Add a pet",
		ExpectStatus: 201,
	}

	b, err := json.MarshalIndent(buildCaseData(op), "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}

	got := string(b)
	keys := []string{
		`"name"`,
		`"description"`,
		`"pathParams"`,
		`"queryParams"`,
		`"headers"`,
		`"requestBody"`,
		`"expect"`,
	}

	last := -1
	for _, key := range keys {
		idx := strings.Index(got, key)
		if idx == -1 {
			t.Fatalf("marshaled case JSON does not contain %s:\n%s", key, got)
		}
		if idx <= last {
			t.Fatalf("marshaled case JSON key %s is out of order:\n%s", key, got)
		}
		last = idx
	}
}

func TestBuildTemplateContentHasEndpoint(t *testing.T) {
	op := &OperationInfo{
		Method:       "get",
		Path:         "/pets",
		Summary:      "List pets",
		PrimaryTag:   "pet",
		OperationID:  "listPets",
		OperationKey: "get_listPets",
		RunbookPath:  "/pets",
	}

	got := buildTemplateContent(op, "req")

	if !strings.Contains(got, "endpoint: ${RUNNORA_BASE_URL}") {
		t.Fatalf("template does not contain RUNNORA_BASE_URL endpoint:\n%s", got)
	}
	if strings.Contains(got, "openapi3:") {
		t.Fatalf("template should not contain openapi3 field:\n%s", got)
	}
}

func TestBuildTemplateContentAddsQueryString(t *testing.T) {
	op := &OperationInfo{
		Method:       "get",
		Path:         "/pet/findByStatus",
		Summary:      "Find pets by status",
		PrimaryTag:   "pet",
		OperationID:  "findPetsByStatus",
		OperationKey: "get_findPetsByStatus",
		RunbookPath: appendQueryParams("/pet/findByStatus", []ParameterInfo{
			{Name: "status", Sample: "available"},
		}),
	}

	got := buildTemplateContent(op, "req")
	want := "/pet/findByStatus?status={{ vars.case.queryParams.status }}:"
	if !strings.Contains(got, want) {
		t.Fatalf("template path does not contain query parameter %q:\n%s", want, got)
	}
}

func TestAppendQueryParamsKeepsOpenAPIOrder(t *testing.T) {
	got := appendQueryParams("/pets", []ParameterInfo{
		{Name: "status", Sample: "available"},
		{Name: "limit", Sample: 10},
		{Name: "cursor", Sample: "next"},
	})
	want := "/pets?status={{ vars.case.queryParams.status }}&limit={{ vars.case.queryParams.limit }}&cursor={{ vars.case.queryParams.cursor }}"
	if got != want {
		t.Fatalf("appendQueryParams() = %q, want %q", got, want)
	}
}

func TestBuildTemplateContentUsesMultipartBody(t *testing.T) {
	op := &OperationInfo{
		Method:                 "post",
		Path:                   "/pet/{petId}/uploadImage",
		Summary:                "Upload image",
		PrimaryTag:             "pet",
		OperationID:            "uploadFile",
		OperationKey:           "post_uploadFile",
		RunbookPath:            "/pet/{{ vars.case.pathParams.petId }}/uploadImage",
		RequestBodyContentType: "multipart/form-data",
		MultipartFields: []MultipartField{
			{Name: "additionalMetadata", Sample: "TODO_string"},
			{Name: "file", IsFile: true, Sample: "TODO: path/to/file"},
		},
	}

	got := buildTemplateContent(op, "req")
	for _, want := range []string{
		"            multipart/form-data:\n",
		"              additionalMetadata: \"{{ vars.case.requestBody.additionalMetadata }}\"\n",
		"              file: \"{{ vars.case.requestBody.file }}\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("template does not contain multipart fragment %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "application/json") {
		t.Fatalf("multipart template should not contain application/json body:\n%s", got)
	}
}

func TestBuildCaseDataIncludesMultipartFields(t *testing.T) {
	op := &OperationInfo{
		Method:       "post",
		Path:         "/pet/{petId}/uploadImage",
		Summary:      "Upload image",
		ExpectStatus: 200,
		PathParams: []ParameterInfo{
			{Name: "petId", Sample: 0},
		},
		RequestBodySample: map[string]interface{}{
			"additionalMetadata": "TODO_string",
			"file":               "TODO: path/to/file",
		},
	}

	got := buildCaseData(op)
	body, ok := got.RequestBody.(map[string]interface{})
	if !ok {
		t.Fatalf("requestBody = %#v, want map", got.RequestBody)
	}
	if got.PathParams["petId"] != 0 {
		t.Fatalf("pathParams.petId = %#v, want 0", got.PathParams["petId"])
	}
	if body["additionalMetadata"] != "TODO_string" {
		t.Fatalf("additionalMetadata = %#v, want TODO_string", body["additionalMetadata"])
	}
	if body["file"] != "TODO: path/to/file" {
		t.Fatalf("file = %#v, want TODO path", body["file"])
	}
}

func TestNormalizeYAMLPath(t *testing.T) {
	got := normalizeYAMLPath(`docs\tutorial/openapi.yaml`)
	want := "docs/tutorial/openapi.yaml"
	if got != want {
		t.Fatalf("normalizeYAMLPath() = %q, want %q", got, want)
	}
}
