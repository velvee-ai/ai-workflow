# Work CLI

A powerful CLI tool for orchestrating git workflows, featuring git worktree management for parallel branch development, intelligent autocomplete, and streamlined PR creation.

## Features

- **Git Worktree Management** - Work on multiple branches simultaneously without switching
- **Intelligent Autocomplete** - Repository and branch suggestions with caching for speed
- **Streamlined PR Workflow** - One command to commit, push, and create pull requests
- **Setup Wizard** - Interactive configuration on first run
- **Health Checks** - Verify your environment with `work doctor`
- **IDE Integration** - Automatic workspace opening in VSCode or Cursor
- **GitHub Organization Support** - Filter repos by your preferred GitHub organizations
- **Configuration System** - Flexible settings for customizing your workflow
- **Shell Completion** - Tab completion for Bash, Zsh, Fish, and PowerShell

## Project Structure

```
.
├── main.go              # Entry point
├── cmd/
│   ├── root.go          # Root command and CLI setup with services initialization
│   ├── checkout.go      # Git worktree checkout commands with autocomplete
│   ├── commit.go        # Streamlined commit and PR creation
│   ├── config.go        # Configuration management
│   ├── setup.go         # Setup wizard and health check (doctor)
│   ├── completion.go    # Shell completion generation
│   └── git.go           # Basic git operations
├── pkg/
│   ├── cache/           # Generic TTL cache implementation
│   ├── config/          # Configuration system with path expansion
│   ├── gitexec/         # Context-aware git command runner
│   ├── giturl/          # Git URL parsing utilities
│   └── services/        # Application-wide service singleton
├── go.mod               # Go module definition
├── Makefile             # Build and test targets
├── .goreleaser.yaml     # Release automation
└── README.md
```

## Architecture

### Service Singleton Pattern

The application uses a service singleton pattern to avoid global state and enable testability:

```go
// Initialize during startup (cmd/root.go)
services.MustInit()

// Access from commands
svc := services.Get()
gitRunner := svc.GitRunner
cfg := svc.Config
```

Services are lazily initialized and include:
- **Config**: Application configuration loaded from `~/.work/config.yaml`
- **GitRunner**: Context-aware git command executor with timeouts
- Future: WorktreeManager, CacheService, GitHubClient, etc.

### Package Organization

- **pkg/cache**: Thread-safe generic TTL cache with cleanup
- **pkg/config**: Configuration loading, saving, and path expansion
- **pkg/gitexec**: Git command execution with context support and structured results
- **pkg/giturl**: Git URL parsing for SSH, HTTPS, and various formats
- **pkg/services**: Application-wide service container

## Installation

### Homebrew (macOS)

```bash
# Add the tap
brew tap velvee-ai/tap

# Install
brew install work

# Verify installation
work --version
```

**Note:** Pre-built binaries are currently only available for macOS through Homebrew. For Linux and Windows users, please build from source (see below).

**Windows users:** This tool requires WSL (Windows Subsystem for Linux). Install WSL by following [Microsoft's guide](https://learn.microsoft.com/en-us/windows/wsl/install).

### Build from Source

Supports **macOS**, **Linux**, and **Windows** (via WSL).

**Prerequisites:**

- Go 1.21 or later
- Git (for git commands and checkout workflow)
- GitHub CLI (`gh`) - Optional, required for GitHub issue integration and PR creation
- **Windows users**: WSL (Windows Subsystem for Linux) is required

```bash
# Clone the repository
git clone https://github.com/velvee-ai/ai-workflow.git
cd ai-workflow

# Build
go build -o work

# Or install globally
go install
```

## Quick Start

After installation, run the setup wizard to configure your environment:

```bash
# Interactive setup wizard
work setup

# Verify everything is working
work doctor
```

## Usage

### Help

```bash
# Show all available commands
work --help

# Show help for specific command
work git --help
work checkout --help
work config --help
```

### Configuration

The CLI uses a configuration file to store your preferences. You can manage settings with the `config` command:

```bash
# List all settings
work config list

# Get a specific setting
work config get default_git_folder

# Set a setting
work config set default_git_folder ~/git
work config set preferred_orgs '["myorg","other-org"]'
work config set preferred_ide cursor

# Show config file path
work config path
```

**Available Settings:**

- `default_git_folder` - Where to clone repositories (e.g., `~/git`)
- `preferred_orgs` - GitHub organizations to filter in autocomplete (JSON array)
- `preferred_ide` - IDE to open after checkout (`vscode`, `cursor`, or `none`)

### Setup and Health Check

```bash
# Run interactive setup wizard
work setup

# Check your environment and configuration
work doctor
```

The `doctor` command verifies:

- Git installation
- GitHub CLI (`gh`) installation and authentication
- Default git folder exists and is writable
- GitHub organization access
- IDE availability

### Git Commands

```bash
# Show git status
work git status

# List all branches
work git branch
```

### Checkout Commands (Git Worktree Workflow)

The checkout commands provide a powerful workflow for managing multiple branches using git worktrees, with intelligent autocomplete for repositories and branches:

```bash
# Clone a repository into structured layout
work checkout root https://github.com/user/repo.git

# This creates:
# repo/
#   └── main/  (cloned repository)

# Create/switch to a branch worktree (with tab completion!)
work checkout <TAB>        # Lists your repos from preferred orgs
work checkout myrepo <TAB>  # Lists branches in that repo

# Examples:
work checkout myrepo feature-123
work checkout myrepo main

# Create a NEW remote branch and checkout locally (with tab completion!)
work checkout new <TAB>              # Lists your repos from preferred orgs
work checkout new myrepo feature-api # Creates branch remotely, then checks out

# This will:
# - Create the branch remotely on GitHub from the base branch (default: main)
# - Create a local worktree for the new branch
# - Open in your configured IDE (if set)

# Create branch from GitHub issue
work checkout branch https://github.com/user/repo/issues/42

# This will:
# - Create a branch from the issue (using gh CLI)
# - Assign the issue to you
# - Create a worktree in default_git_folder/repo/branch-name/
# - Open in your configured IDE (if set)
```

**Autocomplete Features:**

- Repository names from your configured GitHub organizations (cached persistently)
- Branch names for the selected repository (fetched fresh from GitHub on every tab)
- Run `work reload` to refresh repository cache
- Always shows up-to-date branch information

**Benefits of this workflow:**

- Work on multiple branches simultaneously without switching
- Each branch has its own working directory
- No need to stash changes when switching branches
- Run tests in one branch while developing in another
- Automatic IDE integration for seamless development
- Smart autocomplete saves typing and prevents errors

### Commit and PR Creation

Streamline your git workflow with a single command that handles everything from commit to PR creation:

```bash
# Add, commit, pull, push, and create PR in one command
work commit "Add new feature"
work commit "Fix authentication bug"
```

This command automatically:

1. Runs `git add .`
2. Creates a commit with your message
3. Pulls with rebase to stay up-to-date
4. Pushes to remote (with retry logic for network issues)
5. Creates a GitHub pull request using `gh` CLI

**Features:**

- Automatic PR title and body generation from commits
- Exponential backoff retry for push failures (4 attempts)
- Helpful error messages if `gh` CLI is not installed
- Handles upstream branch tracking automatically

### Cache Management

The autocomplete system uses a persistent cache for repository names and fetches branches on-demand from GitHub:

```bash
# Reload repository list from GitHub
work reload
```

The repository cache is stored in `~/.work/cache/work.db` (a bbolt database). Run `work reload`:
- After adding new repositories to GitHub
- Periodically to keep your repository list fresh

Branches are fetched fresh from GitHub on every tab completion, ensuring you always see the latest branch information.

### Shell Completion

Enable tab completion for your shell:

```bash
# Bash
work completion bash > /etc/bash_completion.d/work

# Zsh
work completion zsh > "${fpath[1]}/_work"

# Fish
work completion fish > ~/.config/fish/completions/work.fish

# PowerShell
work completion powershell > work.ps1
```

See `work completion --help` for detailed installation instructions for each shell.

## Adding New Commands

Cobra makes it easy to add new commands:

1. Create a new file in `cmd/` directory (e.g., `cmd/mycommand.go`)
2. Define your command using `cobra.Command`
3. Add it to the root command in the `init()` function

Example:

```go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Description of my command",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("Hello from my command!")
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
}
```

## Advanced Features

### Configuration File

The CLI stores configuration in `~/.work/config.json` (or `$XDG_CONFIG_HOME/work/config.json` on Linux). This file is automatically created on first run or when you use `work setup`.

### GitHub Integration

The tool integrates with GitHub in several ways:

- Uses `gh` CLI for issue-based branch creation
- Fetches repository lists from your configured organizations
- Creates pull requests automatically with the `commit` command
- Supports both SSH and HTTPS git URLs

### IDE Integration

After checking out a branch, the tool can automatically open your IDE:

- **VSCode**: Opens workspace with `code <path>`
- **Cursor**: Opens workspace with `cursor <path>`
- **None**: Skip IDE integration

Configure your preference with:

```bash
work config set preferred_ide cursor
```

### Post-checkout Automation Scripts

Repositories can provide custom automation via a `.work/post_checkout.sh` script in the worktree root. When this script exists, `work` will execute it after creating or switching to a worktree, giving teams full control over the post-checkout workflow.

**Script location:** `.work/post_checkout.sh` (relative to the worktree root)

**Behavior:**
- If the script exists, `work` runs it using your default shell (`$SHELL`, or `/bin/sh` as fallback) after checkout
- The script runs with the worktree directory as its working directory
- If the script fails, a warning is printed, but the checkout is still considered successful
- If the script does not exist, `work` falls back to opening the configured IDE (VSCode/Cursor)

**Example use cases:**
- Install dependencies automatically (`npm install`, `go mod download`, etc.)
- Run initialization scripts or setup tasks
- Open the IDE as the last step if desired

**Example script:**

```bash
#!/bin/bash
# .work/post_checkout.sh

echo "Installing dependencies..."
npm install

echo "Running database migrations..."
npm run db:migrate

# Optional: open IDE after automation
cursor .
```

This hook allows teams to standardize their development environment setup while maintaining the convenience of automatic IDE opening when no custom script is needed.

### Caching Strategy

To provide fast autocomplete:

- **Repository cache**: Persistent storage in `~/.work/cache/work.db` (bbolt database)
  - Populated by `work reload` command
  - Also includes locally cloned repositories
  - bbolt provides fast reads (no in-memory cache needed)
- **Branch data**: No caching, always fresh
  - Fetched from GitHub API on every tab completion
  - Shows the 100 most recently updated branches
  - Falls back to local git repo if GitHub is unavailable

This approach keeps autocomplete fast while ensuring data is always current.

## Contributing

### Adding New Commands

Cobra makes it easy to add new commands:

1. Create a new file in `cmd/` directory (e.g., `cmd/mycommand.go`)
2. Define your command using `cobra.Command`
3. Add it to the root command in the `init()` function
4. Add autocomplete if needed using `ValidArgsFunction`

Example:

```go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Description of my command",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("Hello from my command!")
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
}
```

### Features of Cobra Framework

- **Automatic help generation**: `-h` and `--help` flags automatically
- **Nested subcommands**: Commands can have subcommands (e.g., `git status`)
- **Flags support**: Persistent and local flags with type safety
- **Suggestions**: Automatic suggestions for mistyped commands
- **Shell completions**: Bash, Zsh, Fish, PowerShell support
- **Man pages**: Automatic generation

## Command Reference

Quick reference for all available commands:

| Command                         | Description                                           |
| ------------------------------- | ----------------------------------------------------- |
| `work setup`                    | Interactive setup wizard for first-time configuration |
| `work doctor`                   | Health check to verify your environment               |
| `work config list`              | Show all configuration settings                       |
| `work config get <key>`         | Get a specific configuration value                    |
| `work config set <key> <value>` | Set a configuration value                             |
| `work config path`              | Show configuration file path                          |
| `work reload`                   | Reload repository list from GitHub                    |
| `work checkout <repo> <branch>` | Checkout or create a git worktree (with autocomplete) |
| `work checkout new <repo> <branch>` | Create remote branch via GitHub and checkout locally |
| `work checkout root <url>`      | Clone a repository with worktree-ready structure      |
| `work checkout branch <branch>` | Checkout branch in current repo using worktree        |
| `work checkout branch <issue-url>` | Create branch from GitHub issue                    |
| `work commit <message>`         | Add, commit, pull, push, and create PR                |
| `work remote`                   | Open repository in browser                            |
| `work completion <shell>`       | Generate shell completion script                      |
| `work git status`               | Show git status                                       |
| `work git branch`               | List git branches                                     |

## Development

### Quick Start

```bash
# Clone the repository
git clone https://github.com/velvee-ai/ai-workflow.git
cd ai-workflow

# Build the project
make build

# Run tests
make test

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Run linters (requires golangci-lint)
make lint

# Clean build artifacts
make clean
```

### Development Workflow

1. **Make changes** to the code
2. **Run tests** to ensure nothing breaks: `make test`
3. **Format code**: `make fmt`
4. **Build locally**: `make build`
5. **Test manually**: `./work <command>`

### Testing Philosophy

- Unit tests for all `pkg/*` packages
- Use table-driven tests for comprehensive coverage
- Mock external dependencies (git, gh CLI) in tests
- Test both success and error paths

### Adding New Features

1. **Create package in `pkg/`** if it's reusable logic
2. **Add to services** if it needs to be app-wide
3. **Wire into commands** in `cmd/`
4. **Add tests** in `*_test.go` files
5. **Update README** with new functionality

### Running Locally

```bash
# Run without building
go run main.go [command]

# Example: test setup command
go run main.go setup

# Example: test checkout with autocomplete
go run main.go checkout
```

## Releases

This project uses [GoReleaser](https://goreleaser.com/) for automated releases to Homebrew.

### Creating a Release

1. Create and push a version tag:

   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

2. GitHub Actions automatically:
   - Builds binaries for macOS, Linux, and Windows/WSL (Intel & ARM)
   - Creates a GitHub Release
   - Updates the Homebrew tap

See [RELEASE.md](RELEASE.md) for detailed release instructions.

## License

MIT License - see [LICENSE](LICENSE) file for details.
