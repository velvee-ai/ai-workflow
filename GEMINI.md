# GEMINI.md - Project Context for AI Assistants

This file provides high-level architectural context, design patterns, and development guidelines for AI assistants working on the Work CLI project.

## Project Purpose

**Work CLI** is a powerful Go-based command-line tool designed to orchestrate git workflows using **git worktrees**. It enables developers to work on multiple branches simultaneously in separate directories, provides intelligent autocomplete for repositories and branches, and streamlines the process of committing and creating pull requests.

## Tech Stack

- **Language**: Go (1.21+)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **Database**: [bbolt](https://github.com/etcd-io/bbolt) (Persistent cache for repository metadata)
- **External Integrations**: GitHub CLI (`gh`), Git

## Architecture & Design Patterns

### Service Singleton Pattern

The application follows a service singleton pattern to manage dependencies and global state without relying on `init()` functions or global variables in logic packages.

- Base: [pkg/services/services.go](file:///Users/sathish/git/ai-workflow/add-git-task/pkg/services/services.go)
- **Initialization**: Triggered in `cmd/root.go` via `services.MustInit()`.
- **Usage**: Access services using `services.Get()`.

### Package Breakdown

#### `cmd/` (CLI Layer)
Contains Cobra command definitions. Each file typically corresponds to a command (e.g., `checkout.go`, `commit.go`).
- [root.go](file:///Users/sathish/git/ai-workflow/add-git-task/cmd/root.go): Entry point, initializes services, and defines common flags.
- [checkout.go](file:///Users/sathish/git/ai-workflow/add-git-task/cmd/checkout.go): Implementation of worktree-based branch management.

#### `pkg/` (Logic Layer)
- [cache/](file:///Users/sathish/git/ai-workflow/add-git-task/pkg/cache/): Wrapper around `bbolt` for persistent repository/branch metadata.
- [config/](file:///Users/sathish/git/ai-workflow/add-git-task/pkg/config/): Logic for loading/saving `~/.work/config.yaml`.
- [gitexec/](file:///Users/sathish/git/ai-workflow/add-git-task/pkg/gitexec/): Context-aware wrapper for executing git commands.
- [giturl/](file:///Users/sathish/git/ai-workflow/add-git-task/pkg/giturl/): Parsers for varied git URL formats (SSH, HTTPS).

## Development Guidelines

### Adding a New Command

1.  Create a new file in `cmd/` (e.g., `cmd/mycmd.go`).
2.  Define the command struct using `cobra.Command`.
3.  Implement the `Run` or `RunE` function.
4.  Add the command to `rootCmd` in the `init()` section of your new file.
5.  If the command requires complex logic, implement it in a new package under `pkg/` and expose it via the `services` container.

### Testing

The project uses Go's standard testing library along with `testify`.
- **Unit Tests**: Co-located with code as `_test.go` files.
- **Mocks**: When testing commands, ensure you mock external calls to `git` or `gh`.

```bash
# Run all tests
make test
```

### Common Gotchas

- **Config Path**: Configuration is stored in `~/.work/config.yaml` by default.
- **Cache Path**: The `bbolt` database is located at `~/.work/cache/work.db`.
- **gh CLI Requirement**: Many features (like `work commit` or branch creation from issues) depend on the GitHub CLI (`gh`) being authenticated.
- **Worktree Layout**: The tool assumes a specific directory structure: `default_git_folder/repo_name/branch_name/`.

## Core Workflows

### Checkout Workflow
1.  Verify if the base repository is cloned.
2.  Create a new directory for the branch.
3.  Add a git worktree to that directory.
4.  Execute `.work/post_checkout.sh` if it exists.
5.  Open the IDE (VSCode/Cursor) if configured.
