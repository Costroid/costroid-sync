package cmd

import "github.com/spf13/cobra"

// Version is overridden at link time:
//
//	go build -ldflags "-X github.com/costroid/costroid-sync/cmd.Version=v1.2.3"
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "costroid-sync",
	Short:   "Track and forecast AI/LLM spending from your terminal",
	Long:    "costroid-sync is the open-source Costroid CLI for tracking, forecasting, and alerting on AI/LLM spend across providers using metadata only.",
	Version: Version,
}

func Execute() error {
	return rootCmd.Execute()
}
