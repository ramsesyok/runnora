package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ManifestEntry は 1 つの operation に対応する生成物の情報を保持する。
type ManifestEntry struct {
	OperationID  string   `json:"operation_id"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	Tag          string   `json:"tag"`
	TemplatePath string   `json:"template_path"`
	SuitePath    string   `json:"suite_path"`
	CasePaths    []string `json:"case_paths"`
}

// Manifest は generate コマンドで生成されたすべての成果物を記録する。
// <outDir>/runbooks/generated/manifest.json として書き出される。
type Manifest struct {
	GeneratedAt string          `json:"generated_at"`
	OpenAPI     string          `json:"openapi"`
	Entries     []ManifestEntry `json:"entries"`
}

// EmitManifest は manifest.json を <outDir>/runbooks/generated/ に書き出す。
func EmitManifest(outDir, openAPIPath string, entries []ManifestEntry) error {
	dir := filepath.Join(outDir, "runbooks", "generated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("generate: mkdir for manifest %s: %w", dir, err)
	}

	m := Manifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		OpenAPI:     openAPIPath,
		Entries:     entries,
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("generate: marshal manifest: %w", err)
	}
	b = append(b, '\n')

	dest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(dest, b, 0o600); err != nil {
		return fmt.Errorf("generate: write manifest %s: %w", dest, err)
	}
	return nil
}
