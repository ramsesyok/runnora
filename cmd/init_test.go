package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/ramsesyok/runnora/internal/config"
)

func TestInitCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	initCmd, _, err := root.Find([]string{"init"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if initCmd == nil {
		t.Fatal("init command not found")
	}
}

func TestInitCmd_WritesDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "config.yaml")

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"init", "--out", outFile})
	var out bytes.Buffer
	root.SetOut(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("unmarshal generated config: %v", err)
	}
	if cfg.Oracle.DSN != "" {
		t.Errorf("default oracle.dsn should be empty, got %q", cfg.Oracle.DSN)
	}
	if cfg.Oracle.Driver != "oracle" {
		t.Errorf("oracle.driver: got %q, want oracle", cfg.Oracle.Driver)
	}
	if cfg.Report.Format != "text" {
		t.Errorf("report.format: got %q, want text", cfg.Report.Format)
	}
	if !strings.Contains(out.String(), "created") {
		t.Errorf("output should mention created file: %q", out.String())
	}
}

func TestInitCmd_DoesNotOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(outFile, []byte("existing"), 0o600); err != nil {
		t.Fatalf("setup file: %v", err)
	}

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"init", "--out", outFile})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for existing file")
	}

	content, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(content) != "existing" {
		t.Errorf("existing file should not be overwritten: %q", string(content))
	}
}

func TestInitCmd_ForceOverwritesWithDSN(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(outFile, []byte("existing"), 0o600); err != nil {
		t.Fatalf("setup file: %v", err)
	}

	dsn := "oracle://user:pass@localhost:1521/FREEPDB1"
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"init", "--out", outFile, "--force", "--dsn", dsn})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("unmarshal generated config: %v", err)
	}
	if cfg.Oracle.DSN != dsn {
		t.Errorf("oracle.dsn: got %q, want %q", cfg.Oracle.DSN, dsn)
	}
}
