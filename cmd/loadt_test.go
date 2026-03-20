package cmd_test

import (
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestLoadtCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, err := root.Find([]string{"loadt"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if loadtCmd == nil {
		t.Fatal("loadt command not found")
	}
}

func TestLoadtCmd_HasAlias_loadtest(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, _ := root.Find([]string{"loadt"})
	found := false
	for _, a := range loadtCmd.Aliases {
		if a == "loadtest" {
			found = true
		}
	}
	if !found {
		t.Error("loadt command should have 'loadtest' alias")
	}
}

func TestLoadtCmd_HasConcurrentFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, _ := root.Find([]string{"loadt"})
	if loadtCmd.Flags().Lookup("load-concurrent") == nil {
		t.Fatal("--load-concurrent flag not found")
	}
}

func TestLoadtCmd_HasDurationFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, _ := root.Find([]string{"loadt"})
	if loadtCmd.Flags().Lookup("duration") == nil {
		t.Fatal("--duration flag not found")
	}
}

func TestLoadtCmd_HasMaxRPSFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, _ := root.Find([]string{"loadt"})
	if loadtCmd.Flags().Lookup("max-rps") == nil {
		t.Fatal("--max-rps flag not found")
	}
}

func TestLoadtCmd_HasWarmUpFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	loadtCmd, _, _ := root.Find([]string{"loadt"})
	if loadtCmd.Flags().Lookup("warm-up") == nil {
		t.Fatal("--warm-up flag not found")
	}
}
