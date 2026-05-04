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

func TestNormalizeYAMLPath(t *testing.T) {
	got := normalizeYAMLPath(`docs\tutorial/openapi.yaml`)
	want := "docs/tutorial/openapi.yaml"
	if got != want {
		t.Fatalf("normalizeYAMLPath() = %q, want %q", got, want)
	}
}
