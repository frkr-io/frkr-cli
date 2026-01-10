package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "frkr",
	Short: "frkr CLI - Stream and forward mirrored API traffic",
	Long:  `frkr CLI connects to the Streaming Gateway and forwards mirrored requests to local services.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
