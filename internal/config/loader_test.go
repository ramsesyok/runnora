package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ramsesyok/runnora/internal/config"
)

func TestLoadWithDefaults_AppliesDefaultsWhenFieldsMissing(t *testing.T) {
	raw := `app: {}`
	cfg, err := config.LoadWithDefaults([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Oracle.MaxOpenConns != 10 {
		t.Errorf("got MaxOpenConns=%d, want 10", cfg.Oracle.MaxOpenConns)
	}
	if cfg.Oracle.MaxIdleConns != 2 {
		t.Errorf("got MaxIdleConns=%d, want 2", cfg.Oracle.MaxIdleConns)
	}
	if cfg.Oracle.ConnMaxLifetimeSec != 300 {
		t.Errorf("got ConnMaxLifetimeSec=%d, want 300", cfg.Oracle.ConnMaxLifetimeSec)
	}
	if cfg.Report.Format != "text" {
		t.Errorf("got Report.Format=%q, want %q", cfg.Report.Format, "text")
	}
	if cfg.Runn.DBRunnerName != "db" {
		t.Errorf("got Runn.DBRunnerName=%q, want %q", cfg.Runn.DBRunnerName, "db")
	}
}

func TestLoad_ReadsFromFile(t *testing.T) {
	content := `app: {name: testapp}`
	f := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.Name != "testapp" {
		t.Errorf("got %q, want testapp", cfg.App.Name)
	}
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildBeforeFiles(t *testing.T) {
	tests := []struct {
		name        string
		commonFiles []string
		cliFiles    []string
		want        []string
	}{
		{
			name:        "common のみ",
			commonFiles: []string{"a.sql"},
			cliFiles:    nil,
			want:        []string{"a.sql"},
		},
		{
			name:        "CLI は common の後に追加される",
			commonFiles: []string{"a.sql"},
			cliFiles:    []string{"b.sql"},
			want:        []string{"a.sql", "b.sql"},
		},
		{
			name:        "common が空でも CLI ファイルが返る",
			commonFiles: nil,
			cliFiles:    []string{"cli.sql"},
			want:        []string{"cli.sql"},
		},
		{
			name:        "どちらも空のときは空スライスを返す",
			commonFiles: nil,
			cliFiles:    nil,
			want:        []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Hooks: config.HooksConfig{
					Common: config.CommonHooks{Before: tt.commonFiles},
				},
			}
			opts := &config.RunOptions{BeforeSQLFiles: tt.cliFiles}
			got := config.BuildBeforeFiles(cfg, opts)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildAfterFiles(t *testing.T) {
	tests := []struct {
		name        string
		commonFiles []string
		cliFiles    []string
		want        []string
	}{
		{
			name:        "common のみ",
			commonFiles: []string{"common_cleanup.sql"},
			cliFiles:    nil,
			want:        []string{"common_cleanup.sql"},
		},
		{
			name:        "CLI after は common after の前に来る",
			commonFiles: []string{"common_cleanup.sql"},
			cliFiles:    []string{"cli_after.sql"},
			want:        []string{"cli_after.sql", "common_cleanup.sql"},
		},
		{
			name:        "どちらも空のときは空スライスを返す",
			commonFiles: nil,
			cliFiles:    nil,
			want:        []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Hooks: config.HooksConfig{
					Common: config.CommonHooks{After: tt.commonFiles},
				},
			}
			opts := &config.RunOptions{AfterSQLFiles: tt.cliFiles}
			got := config.BuildAfterFiles(cfg, opts)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
