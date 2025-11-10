package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all configuration settings
type Config struct {
	DefaultGitFolder  string   `mapstructure:"default_git_folder"`
	PreferredOrgs     []string `mapstructure:"preferred_orgs"`
	PreferredIDE      string   `mapstructure:"preferred_ide"`
}

var (
	configFileName = "config"
	configFileType = "yaml"
)

// Init initializes the configuration system
func Init() error {
	// Set config file name and type
	viper.SetConfigName(configFileName)
	viper.SetConfigType(configFileType)

	// Add config path
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	viper.AddConfigPath(configDir)

	// Set default values
	setDefaults()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create it with defaults
			if err := ensureConfigDir(); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	return nil
}

// setDefaults sets default configuration values
func setDefaults() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "~"
	}
	defaultGitFolder := filepath.Join(homeDir, "git")
	viper.SetDefault("default_git_folder", defaultGitFolder)
	viper.SetDefault("preferred_orgs", []string{"myorg"})
	viper.SetDefault("preferred_ide", "none") // Options: "vscode", "cursor", "none"
	viper.SetDefault("default_remote", "origin") // Default git remote name
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".work"), nil
}

// ensureConfigDir ensures the config directory exists
func ensureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(configDir, 0755)
}

// Get returns the current configuration
func Get() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &cfg, nil
}

// Set sets a configuration value and saves it
func Set(key string, value interface{}) error {
	viper.Set(key, value)
	return viper.WriteConfig()
}

// GetString returns a string configuration value
func GetString(key string) string {
	return viper.GetString(key)
}

// GetStringSlice returns a string slice configuration value
func GetStringSlice(key string) []string {
	return viper.GetStringSlice(key)
}

// GetConfigFilePath returns the full path to the config file
func GetConfigFilePath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, fmt.Sprintf("%s.%s", configFileName, configFileType)), nil
}
