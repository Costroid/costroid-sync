package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandUsesCostroidName(t *testing.T) {
	if rootCmd.Use != "costroid" {
		t.Fatalf("root command Use = %q, want costroid", rootCmd.Use)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Help(); err != nil {
		t.Fatalf("root help: %v", err)
	}
	help := out.String()
	oldName := "costroid" + "-sync"
	if strings.Contains(help, oldName) {
		t.Fatalf("root help still advertises old command name: %q", help)
	}
	if !strings.Contains(help, "Use \"costroid [command] --help\"") {
		t.Fatalf("root help missing costroid command usage hint: %q", help)
	}
}
