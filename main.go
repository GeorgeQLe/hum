package main

import "os"

func main() {
	rootCmd := newRootCmd()
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
