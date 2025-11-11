package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration settings",
	Long:  `View and modify configuration settings for the work CLI tool.`,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration settings",
	Long:  `Display all current configuration settings and their values.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Get()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Current configuration:")
		fmt.Printf("  default_git_folder: %s\n", cfg.DefaultGitFolder)
		if len(cfg.PreferredOrgs) > 0 {
			fmt.Printf("  preferred_orgs: %v\n", cfg.PreferredOrgs)
		} else {
			fmt.Printf("  preferred_orgs: []\n")
		}
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long:  `Get the value of a specific configuration setting.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := config.GetString(key)

		if value == "" {
			fmt.Fprintf(os.Stderr, "Configuration key '%s' not found or is empty\n", key)
			os.Exit(1)
		}

		fmt.Printf("%s: %s\n", key, value)
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set the value of a specific configuration setting.

For array values, use JSON format:
  work config set preferred_orgs '["org1","org2"]'`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]

		// Check if value is a JSON array
		var configValue interface{} = value
		if strings.HasPrefix(strings.TrimSpace(value), "[") {
			var arr []string
			if err := json.Unmarshal([]byte(value), &arr); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing JSON array: %v\n", err)
				os.Exit(1)
			}
			configValue = arr
		}

		if err := config.Set(key, configValue); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully set %s = %v\n", key, configValue)
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	Long:  `Display the path to the configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		path, err := config.GetConfigFilePath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config path: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Configuration file: %s\n", path)
	},
}

func init() {
	// Add subcommands to config command
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configPathCmd)

	// Register config command with root
	rootCmd.AddCommand(configCmd)
}
