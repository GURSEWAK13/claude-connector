package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "claude-connector",
	Short: "LAN-based Claude.ai session sharing proxy",
	Long: `claude-connector is a local Anthropic-compatible API proxy that transparently
routes requests through teammates' Claude.ai sessions when yours is rate-limited,
falling back to Ollama/LM Studio when no one has capacity.`,
	// Don't print usage on runtime errors (only on bad flags/args)
	SilenceUsage: true,
	// Don't double-print errors — we handle it below
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/claude-connector/config.toml)")
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
}
