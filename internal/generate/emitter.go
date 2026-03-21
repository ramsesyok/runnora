package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EmitResult は 1 つの operation に対して生成されたファイルパスを保持する。
type EmitResult struct {
	TemplatePath string // template runbook のパス
	SuitePath    string // suite runbook のパス
	CasePaths    []string // case ファイルのパス (default.json のみ)
}

// EmitTemplate は template runbook を生成して書き出す。
//
// 出力先: <outDir>/runbooks/generated/<tag>/<method>_<operationId>.template.yml
//
// template runbook は 1 件の case を vars.case で受け取り、
// 1 回だけ API を呼んでステータスコードを検証する薄い runbook。
// suite runbook からのみ直接実行されることを想定している。
func EmitTemplate(outDir string, op *OperationInfo, openAPIPath, runnerName string, force bool) (string, error) {
	dir := filepath.Join(outDir, "runbooks", "generated", op.PrimaryTag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("generate: mkdir %s: %w", dir, err)
	}

	filename := op.OperationKey + ".template.yml"
	path := filepath.Join(dir, filename)

	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil // 既存ファイルはスキップ
		}
	}

	content := buildTemplateContent(op, openAPIPath, runnerName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("generate: write template %s: %w", path, err)
	}
	return path, nil
}

// buildTemplateContent は template runbook の YAML 文字列を組み立てる。
func buildTemplateContent(op *OperationInfo, openAPIPath, runnerName string) string {
	var sb strings.Builder

	// desc
	sb.WriteString("desc: ")
	if op.Summary != "" {
		sb.WriteString(op.Summary)
	} else {
		sb.WriteString(strings.ToUpper(op.Method) + " " + op.Path + " template")
	}
	sb.WriteString(" template\n")

	// labels
	sb.WriteString("labels:\n")
	sb.WriteString("  - generated\n")
	sb.WriteString("  - openapi\n")
	if op.PrimaryTag != "default" {
		sb.WriteString("  - " + op.PrimaryTag + "\n")
	}
	sb.WriteString("  - method:" + op.Method + "\n")
	if op.OperationID != "" {
		sb.WriteString("  - operation:" + op.OperationID + "\n")
	}
	sb.WriteString("  - mode:template\n")

	// runners
	sb.WriteString("\nrunners:\n")
	sb.WriteString("  " + runnerName + ":\n")
	sb.WriteString("    endpoint: \"{{ env `RUNNORA_BASE_URL` }}\"\n")
	sb.WriteString("    openapi3: \"" + openAPIPath + "\"\n")

	// vars
	sb.WriteString("\nvars:\n")
	sb.WriteString("  case: {}\n")

	// steps
	sb.WriteString("\nsteps:\n")
	sb.WriteString("  call_api:\n")
	sb.WriteString("    " + runnerName + ":\n")
	sb.WriteString("      " + op.RunbookPath + ":\n")
	sb.WriteString("        " + op.Method + ":\n")
	sb.WriteString("          headers: \"{{ vars.case.headers }}\"\n")

	// request body は POST/PUT/PATCH のみ
	if hasRequestBody(op.Method) {
		sb.WriteString("          body:\n")
		sb.WriteString("            application/json: \"{{ vars.case.requestBody }}\"\n")
	}

	sb.WriteString("    test: |\n")
	sb.WriteString("      current.res.status == vars.case.expect.status\n")

	return sb.String()
}

// EmitCase は default case JSON ファイルを生成して書き出す。
//
// 出力先: <outDir>/cases/generated/<tag>/<operationKey>/default.json
func EmitCase(outDir string, op *OperationInfo, force bool) (string, error) {
	dir := filepath.Join(outDir, "cases", "generated", op.PrimaryTag, op.OperationKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("generate: mkdir %s: %w", dir, err)
	}

	path := filepath.Join(dir, "default.json")

	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil // 既存ファイルはスキップ
		}
	}

	caseData := buildCaseData(op)
	b, err := json.MarshalIndent(caseData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("generate: marshal case json: %w", err)
	}
	b = append(b, '\n') // 末尾改行

	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", fmt.Errorf("generate: write case %s: %w", path, err)
	}
	return path, nil
}

// buildCaseData は case JSON の内容を構築する (設計書 §12.2)。
func buildCaseData(op *OperationInfo) map[string]interface{} {
	reqBody := op.RequestBodySample
	if reqBody == nil && hasRequestBody(op.Method) {
		reqBody = map[string]interface{}{"TODO": "fill in request body"}
	}

	expectBody := op.ExpectBodySample
	if expectBody == nil {
		expectBody = map[string]interface{}{}
	}

	desc := op.Summary
	if desc == "" {
		desc = strings.ToUpper(op.Method) + " " + op.Path + " default case"
	}

	return map[string]interface{}{
		"name":        "default",
		"description": desc,
		"pathParams":  map[string]interface{}{},
		"queryParams": map[string]interface{}{},
		"headers":     map[string]interface{}{},
		"requestBody": reqBody,
		"expect": map[string]interface{}{
			"status":      op.ExpectStatus,
			"bodyMode":    "subset",
			"body":        expectBody,
			"ignorePaths": []interface{}{},
		},
	}
}

// EmitSuite は suite runbook を生成して書き出す。
//
// 出力先: <outDir>/runbooks/generated/<tag>/<operationKey>.suite.yml
//
// suite runbook は cases/ 配下の case JSON を loop で読み込み、
// template runbook を include する。
func EmitSuite(outDir string, op *OperationInfo, casePaths []string, force bool) (string, error) {
	dir := filepath.Join(outDir, "runbooks", "generated", op.PrimaryTag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("generate: mkdir %s: %w", dir, err)
	}

	filename := op.OperationKey + ".suite.yml"
	path := filepath.Join(dir, filename)

	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil // 既存ファイルはスキップ
		}
	}

	content := buildSuiteContent(op, outDir, casePaths)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("generate: write suite %s: %w", path, err)
	}
	return path, nil
}

// buildSuiteContent は suite runbook の YAML 文字列を組み立てる。
func buildSuiteContent(op *OperationInfo, outDir string, casePaths []string) string {
	var sb strings.Builder

	// desc
	sb.WriteString("desc: ")
	if op.Summary != "" {
		sb.WriteString(op.Summary)
	} else {
		sb.WriteString(strings.ToUpper(op.Method) + " " + op.Path)
	}
	sb.WriteString(" suite\n")

	// labels
	sb.WriteString("labels:\n")
	sb.WriteString("  - generated\n")
	sb.WriteString("  - openapi\n")
	if op.PrimaryTag != "default" {
		sb.WriteString("  - " + op.PrimaryTag + "\n")
	}
	sb.WriteString("  - method:" + op.Method + "\n")
	if op.OperationID != "" {
		sb.WriteString("  - operation:" + op.OperationID + "\n")
	}
	sb.WriteString("  - mode:suite\n")

	// vars.cases: suite ファイルから case ファイルへの相対パス
	suiteDir := filepath.Join(outDir, "runbooks", "generated", op.PrimaryTag)
	sb.WriteString("\nvars:\n")
	sb.WriteString("  cases:\n")
	for _, cp := range casePaths {
		rel, err := filepath.Rel(suiteDir, cp)
		if err != nil {
			rel = cp
		}
		// filepath.Rel は OS のパス区切り文字を使うので / に統一する
		rel = filepath.ToSlash(rel)
		sb.WriteString("    - json://" + rel + "\n")
	}

	// steps: loop + include
	sb.WriteString("\nsteps:\n")
	sb.WriteString("  run_case:\n")
	sb.WriteString("    loop:\n")
	sb.WriteString("      count: len(vars.cases)\n")
	sb.WriteString("    include:\n")
	sb.WriteString("      path: ./" + op.OperationKey + ".template.yml\n")
	sb.WriteString("      vars:\n")
	sb.WriteString("        case: \"{{ vars.cases[i] }}\"\n")

	return sb.String()
}

// CleanGenerated は <outDir>/runbooks/generated/ と <outDir>/cases/generated/ を削除する。
// 設計書 §9.3: generated/ は再生成前提なのでこの操作は安全。
// evidence/ は対象外。
func CleanGenerated(outDir string) error {
	dirs := []string{
		filepath.Join(outDir, "runbooks", "generated"),
		filepath.Join(outDir, "cases", "generated"),
	}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			return fmt.Errorf("generate: clean %s: %w", d, err)
		}
	}
	return nil
}

// hasRequestBody は HTTP メソッドがリクエストボディを持つかを返す。
func hasRequestBody(method string) bool {
	switch strings.ToLower(method) {
	case "post", "put", "patch":
		return true
	}
	return false
}
