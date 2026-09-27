package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	want := yamlScalar("/pet/findByStatus?status={{ vars.case.queryParams.status }}") + ":"
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
		HasRequestBody:         true,
		RequestBodyContentType: "multipart/form-data",
		MultipartFields: []MultipartField{
			{Name: "additionalMetadata", Sample: "TODO_string"},
			{Name: "file", IsFile: true, Sample: "TODO: path/to/file"},
		},
	}

	got := buildTemplateContent(op, "req")
	for _, want := range []string{
		"            multipart/form-data:\n",
		"              \"additionalMetadata\": \"{{ vars.case.requestBody.additionalMetadata }}\"\n",
		"              \"file\": \"{{ vars.case.requestBody.file }}\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("template does not contain multipart fragment %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "application/json") {
		t.Fatalf("multipart template should not contain application/json body:\n%s", got)
	}
}

func TestRequestBodyOnlyWhenOperationDefinesOne(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		hasRequestBody bool
		sample         interface{}
		wantBody       bool
		wantCaseBody   interface{}
	}{
		{name: "POST without requestBody", method: "post", hasRequestBody: false, wantBody: false, wantCaseBody: nil},
		{name: "POST with requestBody and sample", method: "post", hasRequestBody: true, sample: map[string]interface{}{"name": "x"}, wantBody: true, wantCaseBody: map[string]interface{}{"name": "x"}},
		{name: "POST with requestBody but no sample", method: "post", hasRequestBody: true, wantBody: true, wantCaseBody: map[string]interface{}{"TODO": "fill in request body"}},
		{name: "PUT without requestBody", method: "put", hasRequestBody: false, wantBody: false, wantCaseBody: nil},
		{name: "GET with requestBody", method: "get", hasRequestBody: true, wantBody: false, wantCaseBody: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &OperationInfo{
				Method:            tt.method,
				Path:              "/loans/{loanId}/return",
				PrimaryTag:        "loans",
				OperationID:       "returnLoan",
				OperationKey:      tt.method + "_returnLoan",
				RunbookPath:       "/loans/{{ vars.case.pathParams.loanId }}/return",
				HasRequestBody:    tt.hasRequestBody,
				RequestBodySample: tt.sample,
			}

			template := buildTemplateContent(op, "req")
			if got := strings.Contains(template, "          body:\n"); got != tt.wantBody {
				t.Errorf("template has body = %v, want %v:\n%s", got, tt.wantBody, template)
			}

			caseBody := buildCaseData(op).RequestBody
			gotJSON, _ := json.Marshal(caseBody)
			wantJSON, _ := json.Marshal(tt.wantCaseBody)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("case requestBody = %s, want %s", gotJSON, wantJSON)
			}
		})
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

func TestEmittersContainUnsafeOpenAPIIdentifiers(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "out")
	op := &OperationInfo{
		Method:       "get",
		Path:         "/safe",
		PrimaryTag:   "../../../escape",
		OperationKey: "get_../../outside",
		RunbookPath:  "/safe",
		ExpectStatus: 200,
	}

	templatePath, err := EmitTemplate(outDir, op, "spec.yaml", "req", false)
	if err != nil {
		t.Fatal(err)
	}
	casePath, err := EmitCase(outDir, op, false)
	if err != nil {
		t.Fatal(err)
	}
	suitePath, err := EmitSuite(outDir, op, []string{casePath}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{templatePath, casePath, suitePath} {
		rel, err := filepath.Rel(outDir, path)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("generated path escaped output directory: %q (rel=%q, err=%v)", path, rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unsafe tag created a directory outside output: %v", err)
	}
	if safeFileSegment("a/b") == safeFileSegment("a_b") {
		t.Fatal("different identifiers collapsed to the same filename")
	}
}

func TestGeneratedYAMLQuotesOpenAPIText(t *testing.T) {
	op := &OperationInfo{
		Method:       "get",
		Path:         "/safe",
		Summary:      "safe\nsteps:\n  injected: {test: false}",
		PrimaryTag:   "tag\nrunners:\n  injected: {}",
		OperationID:  "id\nvars: {injected: true}",
		OperationKey: "get_safe",
		RunbookPath:  "/safe\n  injected: true",
	}
	for _, content := range []string{
		buildTemplateContent(op, "req"),
		buildSuiteContent(op, t.TempDir(), []string{"case.json"}),
	} {
		var parsed struct {
			Desc    string         `yaml:"desc"`
			Runners map[string]any `yaml:"runners"`
			Steps   map[string]any `yaml:"steps"`
		}
		if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
			t.Fatalf("generated YAML is invalid: %v\n%s", err, content)
		}
		if !strings.HasPrefix(parsed.Desc, op.Summary) || len(parsed.Steps) != 1 {
			t.Fatalf("OpenAPI text changed the YAML structure: %+v\n%s", parsed, content)
		}
		if len(parsed.Runners) > 0 {
			if len(parsed.Runners) != 1 {
				t.Fatalf("OpenAPI text added a runner: %+v", parsed.Runners)
			}
		}
	}
}
