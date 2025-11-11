package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
	"github.com/velvee-ai/ai-workflow/pkg/services"
)

var (
	// Version information
	version string
	commit  string
	date    string
)

var rootCmd = &cobra.Command{
	Use:   "work",
	Short: "Work - Git workflow and development tool",
	Long:  `Work is a CLI tool for orchestrating git workflows, featuring powerful git worktree management for parallel branch development.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior when no subcommand is provided
		cmd.Help()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display version, commit, and build date information.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("work version %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// SetVersionInfo sets the version information
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

func init() {
	// Initialize configuration
	cobra.OnInitialize(initConfig)

	// Add version command
	rootCmd.AddCommand(versionCmd)

	// Global flags can be added here
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.work/config.yaml)")
}

// initConfig initializes the configuration and services
func initConfig() {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize config: %v\n", err)
		return
	}

	if err := services.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize services: %v\n", err)
	}
}
