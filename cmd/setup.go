package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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

	// Run checks concurrently where possible
	type checkResult struct {
		name        string
		status      string
		details     []string
		critical    bool
		failed      bool
		order       int
	}

	results := make(chan checkResult, 5)
	var wg sync.WaitGroup

	// Independent checks that can run in parallel
	// 1. Check git
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := checkResult{name: "git", order: 1, critical: true}
		if err := exec.Command("git", "--version").Run(); err != nil {
			result.status = "❌ NOT FOUND"
			result.details = []string{"Install git: https://git-scm.com/downloads"}
			result.failed = true
		} else {
			output, _ := exec.Command("git", "--version").Output()
			result.status = fmt.Sprintf("✓ %s", strings.TrimSpace(string(output)))
		}
		results <- result
	}()

	// 3. Check default git folder
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := checkResult{name: "default_git_folder", order: 3, critical: true}
		gitFolder := config.GetString("default_git_folder")
		if gitFolder == "" {
			result.status = "❌ NOT CONFIGURED"
			result.details = []string{"Run: work setup"}
			result.failed = true
		} else {
			// Expand home directory if needed
			if strings.HasPrefix(gitFolder, "~/") {
				homeDir, _ := os.UserHomeDir()
				gitFolder = filepath.Join(homeDir, gitFolder[2:])
			}

			if info, err := os.Stat(gitFolder); os.IsNotExist(err) {
				result.status = fmt.Sprintf("❌ DOES NOT EXIST (%s)", gitFolder)
				result.details = []string{"Run: work setup"}
				result.failed = true
			} else if !info.IsDir() {
				result.status = fmt.Sprintf("❌ NOT A DIRECTORY (%s)", gitFolder)
				result.failed = true
			} else {
				// Test write permissions
				testFile := filepath.Join(gitFolder, ".work-test")
				if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
					result.status = fmt.Sprintf("❌ NOT WRITABLE (%s)", gitFolder)
					result.failed = true
				} else {
					os.Remove(testFile)
					result.status = fmt.Sprintf("✓ %s", gitFolder)
				}
			}
		}
		results <- result
	}()

	// 5. Check preferred IDE
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := checkResult{name: "preferred_ide", order: 5, critical: false}
		ide := config.GetString("preferred_ide")
		if ide == "" || ide == "none" {
			result.status = "✓ none (auto-open disabled)"
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
				result.status = fmt.Sprintf("⚠ %s command not found (set to '%s')", command, ide)
				result.details = []string{"IDE won't auto-open but checkout will still work"}
			} else {
				result.status = fmt.Sprintf("✓ %s", ide)
			}
		}
		results <- result
	}()

	// GitHub CLI checks (must be sequential within this goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()

		// 2. Check GitHub CLI
		ghResult := checkResult{name: "gh (GitHub CLI)", order: 2, critical: true}
		if err := exec.Command("gh", "--version").Run(); err != nil {
			ghResult.status = "❌ NOT FOUND"
			ghResult.details = []string{"Install gh: https://cli.github.com/"}
			ghResult.failed = true
			results <- ghResult
			return
		}

		output, _ := exec.Command("gh", "--version").Output()
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			ghResult.status = fmt.Sprintf("✓ %s", strings.TrimSpace(lines[0]))
		}
		results <- ghResult

		// Check gh authentication (depends on gh being installed)
		authResult := checkResult{name: "gh authentication", order: 2, critical: true}
		authCmd := exec.Command("gh", "auth", "status")
		authOutput, err := authCmd.CombinedOutput()
		outputStr := string(authOutput)

		hasValidAuth := strings.Contains(outputStr, "✓ Logged in to")
		hasFailedAuth := strings.Contains(outputStr, "X Failed to log in")

		if err != nil && !hasValidAuth {
			authResult.status = "❌ NOT AUTHENTICATED"
			authResult.details = []string{"Run: gh auth login"}
			authResult.failed = true
		} else if hasValidAuth && hasFailedAuth {
			authResult.status = "⚠️  PARTIAL AUTHENTICATION"
			authResult.details = []string{
				"",
				"Details from 'gh auth status':",
			}
			for _, line := range strings.Split(strings.TrimSpace(outputStr), "\n") {
				authResult.details = append(authResult.details, line)
			}
			authResult.details = append(authResult.details,
				"",
				"You have at least one valid account, but some accounts have invalid tokens.",
				"To fix invalid accounts, run: gh auth login -h github.com",
			)
		} else {
			authResult.status = "✓"
		}
		results <- authResult

		// 4. Check preferred orgs (depends on gh)
		orgsResult := checkResult{name: "preferred_orgs", order: 4, critical: false}
		orgs := config.GetStringSlice("preferred_orgs")
		if len(orgs) == 0 {
			orgsResult.status = "⚠ NOT CONFIGURED"
			orgsResult.details = []string{"Run: work setup"}
			results <- orgsResult
			return
		}

		orgsResult.status = fmt.Sprintf("✓ %v", orgs)
		results <- orgsResult

		// Try to verify access to at least one org
		orgAccessResult := checkResult{name: "org access", order: 4, critical: false}
		hasAccess := false
		for _, org := range orgs {
			if org == "" || org == "myorg" {
				continue
			}
			orgCmd := exec.Command("gh", "api", fmt.Sprintf("orgs/%s", org))
			if err := orgCmd.Run(); err == nil {
				hasAccess = true
				break
			}
		}
		if hasAccess {
			orgAccessResult.status = "✓"
		} else {
			orgAccessResult.status = "⚠ Cannot access configured orgs (may need valid org names)"
		}
		results <- orgAccessResult
	}()

	// Close results channel when all checks complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	var allResults []checkResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// Sort results by order to maintain consistent output
	for i := 0; i < len(allResults); i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[i].order > allResults[j].order {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	// Display results in order
	allGood := true
	for _, result := range allResults {
		fmt.Printf("Checking %s... %s\n", result.name, result.status)
		for _, detail := range result.details {
			if detail == "" {
				fmt.Println()
			} else {
				fmt.Printf("   %s\n", detail)
			}
		}
		if result.critical && result.failed {
			allGood = false
		}
		fmt.Println() // Blank line separator
	}

	// Summary
	fmt.Println("========================")
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
