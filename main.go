package main

import "os"

// Set via -ldflags at build time by GoReleaser.
var version = "dev"

func main() {
	rootCmd := newRootCmd()
	rootCmd.Version = version
	rootCmd.AddCommand(
		newPingCmd(),
		newStatusCmd(),
		newAddCmd(),
		newStatsCmd(),
		newScanCmd(),
		newDevCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
