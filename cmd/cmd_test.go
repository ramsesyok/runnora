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

func TestRunCmd_ProjectFlags_Exist(t *testing.T) {
	root := cmd.NewRootCmd()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil || runCmd == nil {
		t.Fatalf("find run command: %v", err)
	}
	for _, name := range []string{"project", "env", "suite", "var"} {
		if runCmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s flag not found on run command", name)
		}
	}
	// 廃止したフラグは案内を出すために非表示で残している
	for _, name := range []string{"config", "before-sql", "after-sql"} {
		f := runCmd.Flags().Lookup(name)
		if f == nil || !f.Hidden {
			t.Errorf("--%s should remain as a hidden flag", name)
		}
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
