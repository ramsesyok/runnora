package config_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/internal/config"
)

func TestConfig_DBDriver_IsOracle(t *testing.T) {
	raw := `
db:
  driver: oracle
  dsn: "oracle://user:pass@host:1521/svc"
  max_open_conns: 5
`
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DB.Driver != "oracle" {
		t.Errorf("got %q, want %q", cfg.DB.Driver, "oracle")
	}
	if cfg.DB.DSN != "oracle://user:pass@host:1521/svc" {
		t.Errorf("got %q, want oracle DSN", cfg.DB.DSN)
	}
	if cfg.DB.MaxOpenConns != 5 {
		t.Errorf("got %d, want 5", cfg.DB.MaxOpenConns)
	}
}

func TestConfig_HooksConfig_MarshalsCorrectly(t *testing.T) {
	raw := `
hooks:
  common:
    before:
      - "./sql/common/session_init.sql"
      - "./sql/common/master_seed.sql"
    after:
      - "./sql/common/session_cleanup.sql"
`
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Hooks.Common.Before) != 2 {
		t.Errorf("got %d before hooks, want 2", len(cfg.Hooks.Common.Before))
	}
	if len(cfg.Hooks.Common.After) != 1 {
		t.Errorf("got %d after hooks, want 1", len(cfg.Hooks.Common.After))
	}
}
