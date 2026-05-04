package oracle_test

import (
	"errors"
	"testing"

	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/oracle"
)

func TestOpen_EmptyDSN_ReturnsError(t *testing.T) {
	cfg := &config.OracleConfig{Driver: "oracle", DSN: ""}
	_, err := oracle.Open(cfg)
	if err == nil {
		t.Fatal("expected error for empty DSN, got nil")
	}
	if !errors.Is(err, oracle.ErrEmptyDSN) {
		t.Errorf("expected ErrEmptyDSN, got: %v", err)
	}
}
