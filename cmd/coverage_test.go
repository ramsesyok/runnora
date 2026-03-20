package cmd_test

import (
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestCoverageCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, err := root.Find([]string{"coverage"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if covCmd == nil {
		t.Fatal("coverage command not found")
	}
}

func TestCoverageCmd_HasLongFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, _ := root.Find([]string{"coverage"})
	flag := covCmd.Flags().Lookup("long")
	if flag == nil {
		t.Fatal("--long flag not found on coverage command")
	}
}

func TestCoverageCmd_HasFormatFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, _ := root.Find([]string{"coverage"})
	flag := covCmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("--format flag not found on coverage command")
	}
}

func TestCoverageCmd_NoArgs_ReturnsError(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"coverage"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no path specified, got nil")
	}
}
