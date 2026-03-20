package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestRunCmd_NoRunbooks_ReturnsError(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"run"})
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no runbooks specified, got nil")
	}
}

func TestRunCmd_DefaultConfigFlag_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run command: %v", err)
	}
	if runCmd == nil {
		t.Fatal("run command not found")
	}
	flag := runCmd.Flags().Lookup("config")
	if flag == nil {
		t.Fatal("--config flag not found on run command")
	}
	if flag.DefValue != "./config.yaml" {
		t.Errorf("default --config value: got %q, want %q", flag.DefValue, "./config.yaml")
	}
}

func TestRunCmd_BeforeSQLFlag_AcceptsMultiple(t *testing.T) {
	root := cmd.NewRootCmd()
	runCmd, _, _ := root.Find([]string{"run"})
	flag := runCmd.Flags().Lookup("before-sql")
	if flag == nil {
		t.Fatal("--before-sql flag not found on run command")
	}
}

func TestRunCmd_AfterSQLFlag_AcceptsMultiple(t *testing.T) {
	root := cmd.NewRootCmd()
	runCmd, _, _ := root.Find([]string{"run"})
	flag := runCmd.Flags().Lookup("after-sql")
	if flag == nil {
		t.Fatal("--after-sql flag not found on run command")
	}
}

func TestVersionCmd_PrintsRunnora(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"version"})
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "runnora") {
		t.Errorf("version output should contain 'runnora': %q", out)
	}
}
