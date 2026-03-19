// cmd/cbi-client/root.go
package main

import (
	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
)

var rootCmd = &cobra.Command{
	Use:     "cbi-client",
	Short:   "CBI Simulator gRPC Client",
	Long:    `CBI Simulator gRPC Client for testing CTC systems.`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}