package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestRprofCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	rprofCmd, _, err := root.Find([]string{"rprof"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if rprofCmd == nil {
		t.Fatal("rprof command not found")
	}
}

func TestRprofCmd_HasAliases(t *testing.T) {
	root := cmd.NewRootCmd()
	rprofCmd, _, _ := root.Find([]string{"rprof"})
	wantAliases := []string{"prof"}
	for _, want := range wantAliases {
		found := false
		for _, a := range rprofCmd.Aliases {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("rprof command should have alias %q", want)
		}
	}
}

func TestRprofCmd_HasDepthFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	rprofCmd, _, _ := root.Find([]string{"rprof"})
	if rprofCmd.Flags().Lookup("depth") == nil {
		t.Fatal("--depth flag not found")
	}
}

func TestRprofCmd_HasUnitFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	rprofCmd, _, _ := root.Find([]string{"rprof"})
	if rprofCmd.Flags().Lookup("unit") == nil {
		t.Fatal("--unit flag not found")
	}
}

func TestRprofCmd_MissingFile_ReturnsError(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"rprof", "/nonexistent/profile.json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestRprofCmd_InvalidJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.json")
	os.WriteFile(f, []byte("not valid json {"), 0o600)

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"rprof", f})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
