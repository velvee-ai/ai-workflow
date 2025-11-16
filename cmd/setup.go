package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard for configuring work CLI",
	Long: `Run an interactive setup wizard to configure the work CLI tool.

This will guide you through setting up:
- Default git folder location
- Preferred GitHub organizations
- Preferred IDE (VSCode, Cursor, or none)

Example:
  work setup`,
	Run: runSetup,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check if all dependencies and configuration are working properly",
	Long: `Run health checks to verify that work CLI is properly configured.

This checks:
- Git CLI is installed
- GitHub CLI (gh) is installed and authenticated
- Configured IDE is available
- Default git folder exists and is writable
- Preferred orgs are accessible

Example:
  work doctor`,
	Run: runDoctor,
}

func runSetup(cmd *cobra.Command, args []string) {
	// Get current values
	currentGitFolder := config.GetString("default_git_folder")
	if currentGitFolder == "" {
		homeDir, _ := os.UserHomeDir()
		currentGitFolder = filepath.Join(homeDir, "git")
	}

	currentOrgs := config.GetStringSlice("preferred_orgs")
	currentOrgsStr := strings.Join(currentOrgs, ", ")

	currentIDE := config.GetString("preferred_ide")
	if currentIDE == "" {
		currentIDE = "none"
	}

	// Form values
	var gitFolder string
	var orgsInput string
	var ide string
	var createDir bool

	// Set the current IDE as default
	ide = currentIDE

	// Create the fancy form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("🔧 Work CLI Setup Wizard").
				Description("Configure your work CLI preferences"),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Default Git Folder").
				Description("Where all repositories will be cloned").
				Placeholder(currentGitFolder).
				Value(&gitFolder).
				Validate(func(s string) error {
					if s == "" {
						gitFolder = currentGitFolder
					}
					return nil
				}),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("GitHub Organizations").
				Description("Organizations to search for repositories (comma-separated)").
				Placeholder(currentOrgsStr).
				Value(&orgsInput),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Preferred IDE").
				Description("Choose your preferred editor to auto-open after checkout").
				Options(
					huh.NewOption("Visual Studio Code", "vscode"),
					huh.NewOption("Cursor Editor", "cursor"),
					huh.NewOption("None (don't auto-open)", "none"),
				).
				Value(&ide),
		),
	)

	// Run the form
	err := form.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Process git folder
	if gitFolder == "" {
		gitFolder = currentGitFolder
	}

	// Expand home directory if needed
	if strings.HasPrefix(gitFolder, "~/") {
		homeDir, _ := os.UserHomeDir()
		gitFolder = filepath.Join(homeDir, gitFolder[2:])
	}

	// Check if directory needs to be created
	if _, err := os.Stat(gitFolder); os.IsNotExist(err) {
		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Directory '%s' doesn't exist", gitFolder)).
					Description("Would you like to create it?").
					Value(&createDir),
			),
		)

		if err := confirmForm.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if createDir {
			if err := os.MkdirAll(gitFolder, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Save git folder
	config.Set("default_git_folder", gitFolder)

	// Process organizations
	var orgs []string
	if orgsInput != "" {
		orgs = strings.Split(orgsInput, ",")
		for i := range orgs {
			orgs[i] = strings.TrimSpace(orgs[i])
		}
		// Filter out empty strings
		filtered := make([]string, 0)
		for _, org := range orgs {
			if org != "" {
				filtered = append(filtered, org)
			}
		}
		orgs = filtered
	} else if len(currentOrgs) > 0 {
		orgs = currentOrgs
	}

	if len(orgs) > 0 {
		config.Set("preferred_orgs", orgs)
	}

	// Save IDE
	config.Set("preferred_ide", ide)

	// Show success message
	fmt.Println("\n✨ Setup Complete!")
	fmt.Println("===================")
	fmt.Printf("📁 Default git folder: %s\n", gitFolder)
	if len(orgs) > 0 {
		fmt.Printf("🏢 Preferred orgs: %v\n", orgs)
	}
	fmt.Printf("⌨️  Preferred IDE: %s\n", ide)
	fmt.Println("\n💡 Tip: Run 'work doctor' to verify everything is working correctly.")
}

func runDoctor(cmd *cobra.Command, args []string) {
	fmt.Println("🩺 Work CLI Health Check")
	fmt.Println("========================")

	allGood := true

	// 1. Check git
	fmt.Print("Checking git... ")
	if err := exec.Command("git", "--version").Run(); err != nil {
		fmt.Println("❌ NOT FOUND")
		fmt.Println("   Install git: https://git-scm.com/downloads")
		allGood = false
	} else {
		output, _ := exec.Command("git", "--version").Output()
		fmt.Printf("✓ %s\n", strings.TrimSpace(string(output)))
	}
	fmt.Println() // Blank line separator

	// 2. Check GitHub CLI
	fmt.Print("Checking gh (GitHub CLI)... ")
	if err := exec.Command("gh", "--version").Run(); err != nil {
		fmt.Println("❌ NOT FOUND")
		fmt.Println("   Install gh: https://cli.github.com/")
		allGood = false
	} else {
		output, _ := exec.Command("gh", "--version").Output()
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			fmt.Printf("✓ %s\n", strings.TrimSpace(lines[0]))
		}

		// Check if authenticated
		fmt.Print("Checking gh authentication... ")
		cmd := exec.Command("gh", "auth", "status")
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Check if at least one account is logged in successfully
		hasValidAuth := strings.Contains(outputStr, "✓ Logged in to")
		hasFailedAuth := strings.Contains(outputStr, "X Failed to log in")

		if err != nil && !hasValidAuth {
			// No valid authentication at all
			fmt.Println("❌ NOT AUTHENTICATED")
			fmt.Println("   Run: gh auth login")
			allGood = false
		} else if hasValidAuth && hasFailedAuth {
			// Has at least one valid account but also some invalid tokens
			fmt.Println("⚠️  PARTIAL AUTHENTICATION")
			fmt.Println("\n   Details from 'gh auth status':")
			for _, line := range strings.Split(strings.TrimSpace(outputStr), "\n") {
				fmt.Printf("   %s\n", line)
			}
			fmt.Println("\n   You have at least one valid account, but some accounts have invalid tokens.")
			fmt.Println("   To fix invalid accounts, run: gh auth login -h github.com")
		} else {
			fmt.Println("✓")
		}
	}
	fmt.Println() // Blank line separator

	// 3. Check default git folder
	fmt.Print("Checking default_git_folder... ")
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		fmt.Println("❌ NOT CONFIGURED")
		fmt.Println("   Run: work setup")
		allGood = false
	} else {
		// Expand home directory if needed
		if strings.HasPrefix(gitFolder, "~/") {
			homeDir, _ := os.UserHomeDir()
			gitFolder = filepath.Join(homeDir, gitFolder[2:])
		}

		if info, err := os.Stat(gitFolder); os.IsNotExist(err) {
			fmt.Printf("❌ DOES NOT EXIST (%s)\n", gitFolder)
			fmt.Println("   Run: work setup")
			allGood = false
		} else if !info.IsDir() {
			fmt.Printf("❌ NOT A DIRECTORY (%s)\n", gitFolder)
			allGood = false
		} else {
			// Test write permissions
			testFile := filepath.Join(gitFolder, ".work-test")
			if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
				fmt.Printf("❌ NOT WRITABLE (%s)\n", gitFolder)
				allGood = false
			} else {
				os.Remove(testFile)
				fmt.Printf("✓ %s\n", gitFolder)
			}
		}
	}
	fmt.Println() // Blank line separator

	// 4. Check preferred orgs
	fmt.Print("Checking preferred_orgs... ")
	orgs := config.GetStringSlice("preferred_orgs")
	if len(orgs) == 0 {
		fmt.Println("⚠ NOT CONFIGURED")
		fmt.Println("   Run: work setup")
	} else {
		fmt.Printf("✓ %v\n", orgs)

		// Try to verify access to at least one org
		fmt.Print("Checking org access... ")
		hasAccess := false
		for _, org := range orgs {
			if org == "" || org == "myorg" {
				continue
			}
			cmd := exec.Command("gh", "api", fmt.Sprintf("orgs/%s", org))
			if err := cmd.Run(); err == nil {
				hasAccess = true
				break
			}
		}
		if hasAccess {
			fmt.Println("✓")
		} else {
			fmt.Println("⚠ Cannot access configured orgs (may need valid org names)")
		}
	}
	fmt.Println() // Blank line separator

	// 5. Check preferred IDE
	fmt.Print("Checking preferred_ide... ")
	ide := config.GetString("preferred_ide")
	if ide == "" || ide == "none" {
		fmt.Println("✓ none (auto-open disabled)")
	} else {
		var command string
		switch ide {
		case "vscode":
			command = "code"
		case "cursor":
			command = "cursor"
		default:
			command = ide
		}

		if err := exec.Command("which", command).Run(); err != nil {
			fmt.Printf("⚠ %s command not found (set to '%s')\n", command, ide)
			fmt.Printf("   IDE won't auto-open but checkout will still work\n")
		} else {
			fmt.Printf("✓ %s\n", ide)
		}
	}

	// Summary
	fmt.Println("\n========================")
	if allGood {
		fmt.Println("✓ All critical checks passed!")
		fmt.Println("You're ready to use: work checkout <repo> <branch>")
	} else {
		fmt.Println("❌ Some issues found")
		fmt.Println("Fix the issues above, then run 'work doctor' again")
	}
	fmt.Println("========================")
}

func init() {
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(doctorCmd)
}
