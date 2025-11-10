# Publishing Work CLI to Homebrew

This guide walks you through the complete process of publishing the Work CLI tool to Homebrew, making it installable via `brew install work`.

## Current Status

The infrastructure is **ready** but **not yet published**:

✅ **Already configured:**

- `.goreleaser.yaml` - Release automation config
- `.github/workflows/release.yaml` - GitHub Actions workflow
- Binary name: `work`
- Homebrew formula name: `work`

❌ **Still needed:**

- Create `velvee-ai/homebrew-tap` repository
- Set up GitHub token for Homebrew publishing
- Create first release tag

## Prerequisites

Before starting, ensure you have:

- Admin access to `velvee-ai` GitHub organization
- Ability to create repositories in the organization
- Ability to create repository secrets

## Step-by-Step Setup

### Step 1: Create the Homebrew Tap Repository

1. Go to https://github.com/organizations/velvee-ai/repositories/new
2. Create a new repository with these settings:
   - **Repository name:** `homebrew-tap`
   - **Description:** "Homebrew formulae for Saya Ventures tools"
   - **Visibility:** Public
   - **Initialize:** Do NOT add README, .gitignore, or license (will be auto-created)
3. Click "Create repository"

The final URL should be: `https://github.com/velvee-ai/homebrew-tap`

### Step 2: Create GitHub Personal Access Token

This token allows GoReleaser to push formula updates to the tap repository.

1. Go to https://github.com/settings/tokens/new
2. Configure the token:
   - **Note:** "GoReleaser Homebrew Tap Access"
   - **Expiration:** Choose based on your security policy (90 days recommended)
   - **Scopes:** Select `repo` (Full control of private repositories)
     - This includes: `repo:status`, `repo_deployment`, `public_repo`, `repo:invite`, `security_events`
3. Click "Generate token"
4. **IMPORTANT:** Copy the token immediately - you won't be able to see it again!

### Step 3: Add Token as Repository Secret

1. Go to https://github.com/velvee-ai/ai-workflow/settings/secrets/actions
2. Click "New repository secret"
3. Configure the secret:
   - **Name:** `HOMEBREW_TAP_GITHUB_TOKEN`
   - **Secret:** Paste the token from Step 2
4. Click "Add secret"

### Step 4: Verify GitHub Actions is Enabled

1. Go to https://github.com/velvee-ai/ai-workflow/settings/actions
2. Ensure "Actions permissions" is set to "Allow all actions and reusable workflows"
3. Verify "Workflow permissions" has "Read and write permissions" enabled

### Step 5: Create and Push First Release Tag

Now you're ready to create your first release!

```bash
# 1. Ensure you're on the main branch with latest changes
git checkout main
git pull origin main

# 2. Create the release tag
git tag -a v1.0.0 -m "Initial release of Work CLI

Features:
- Git worktree management with intelligent autocomplete
- Streamlined commit and PR creation
- Setup wizard and health checks
- IDE integration (VSCode/Cursor)
- GitHub organization support
- Configuration system
- Shell completion for Bash/Zsh/Fish/PowerShell"

# 3. Push the tag to trigger the release
git push origin v1.0.0
```

### Step 6: Monitor the Release Process

1. Go to https://github.com/velvee-ai/ai-workflow/actions
2. You should see a "Release" workflow running
3. Click on it to monitor progress
4. The workflow will:
   - Build binaries for macOS (Intel & Apple Silicon)
   - Create GitHub Release with artifacts
   - Generate and push Homebrew formula to `homebrew-tap`

Expected duration: 2-5 minutes

### Step 7: Verify the Release

Once the workflow completes successfully:

#### Check GitHub Release

1. Go to https://github.com/velvee-ai/ai-workflow/releases
2. You should see "v1.0.0" release with:
   - Release notes
   - Binary archives for macOS (Intel and ARM64)
   - Checksums file

#### Check Homebrew Tap

1. Go to https://github.com/velvee-ai/homebrew-tap
2. You should see a `Formula/work.rb` file
3. This file contains the Homebrew formula for installation

#### Test Installation

On a macOS machine (or ask a team member to test):

```bash
# Add the tap
brew tap velvee-ai/tap

# Install work
brew install work

# Verify installation
work --version
work --help

# Run setup
work setup

# Run health check
work doctor
```

## Publishing Future Releases

After the initial setup, publishing new releases is simple:

```bash
# 1. Ensure all changes are committed and pushed to main
git checkout main
git pull origin main

# 2. Create a new version tag (follow semantic versioning)
git tag -a v1.1.0 -m "Release v1.1.0

New features:
- Feature A
- Feature B

Bug fixes:
- Fix issue X
- Fix issue Y"

# 3. Push the tag
git push origin v1.1.0

# 4. GitHub Actions automatically handles the rest!
```

The workflow will automatically:

- Build new binaries
- Create GitHub Release
- Update Homebrew formula in the tap
- Users can then run `brew upgrade work` to get the new version

## Semantic Versioning Guide

Follow [Semantic Versioning](https://semver.org/) for version numbers:

- **v1.0.0 → v2.0.0** - Major version (breaking changes)
  - Example: Removing commands, changing command syntax
- **v1.0.0 → v1.1.0** - Minor version (new features, backward compatible)
  - Example: Adding new commands, new options
- **v1.0.0 → v1.0.1** - Patch version (bug fixes)
  - Example: Fixing bugs, improving documentation

## Troubleshooting

### Workflow fails with "failed to publish artifacts"

**Cause:** Token doesn't have correct permissions or isn't set

**Solution:**

1. Verify `HOMEBREW_TAP_GITHUB_TOKEN` secret exists
2. Verify token has `repo` scope
3. Generate a new token if needed and update the secret

### Formula not appearing in homebrew-tap

**Cause:** Repository doesn't exist or wrong name

**Solution:**

1. Verify repository exists at `https://github.com/velvee-ai/homebrew-tap`
2. Check repository name is exactly `homebrew-tap` (lowercase)
3. Ensure repository is public

### "Permission denied" when pushing to tap

**Cause:** Token lacks write permissions

**Solution:**

1. Ensure token has `repo` scope (not just `public_repo`)
2. Verify token hasn't expired
3. Generate new token with correct permissions

### Release tag already exists

**Cause:** Trying to recreate an existing tag

**Solution:**

```bash
# Delete the tag locally and remotely
git tag -d v1.0.0
git push --delete origin v1.0.0

# Create it again
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### Homebrew installation fails on user's machine

**Cause:** Formula might have errors

**Solution:**

1. Check the formula at `https://github.com/velvee-ai/homebrew-tap/blob/main/Formula/work.rb`
2. Verify SHA256 checksums match the release artifacts
3. Test formula locally: `brew audit --strict work`

## Testing Before Release (Optional)

You can test the release process locally without publishing:

```bash
# Install GoReleaser
brew install goreleaser

# Run a local snapshot build (doesn't publish)
goreleaser release --snapshot --clean

# Check the dist/ folder for built binaries
ls -la dist/

# Test the binary
./dist/work_darwin_arm64/work --help
```

## Updating README After First Release

After successfully publishing to Homebrew, the README installation instructions are already correct:

```bash
brew tap velvee-ai/tap
brew install work
```

No changes needed - the Homebrew section can remain as the recommended installation method.

## Rollback a Release

If a release has critical issues:

### Option 1: Release a Patch Version (Recommended)

```bash
# Fix the issue, commit, then:
git tag -a v1.0.1 -m "Hotfix release"
git push origin v1.0.1
```

### Option 2: Delete the Release (Not Recommended)

```bash
# Delete GitHub release manually from web UI
# Delete the tag
git tag -d v1.0.0
git push --delete origin v1.0.0

# Note: Users who already installed will keep that version
# until they run: brew upgrade work
```

## Additional Resources

- [GoReleaser Documentation](https://goreleaser.com/quick-start/)
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Semantic Versioning](https://semver.org/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)

## Checklist for First Release

Use this checklist to ensure everything is set up correctly:

- [ ] Created `velvee-ai/homebrew-tap` repository
- [ ] Generated GitHub Personal Access Token with `repo` scope
- [ ] Added token as `HOMEBREW_TAP_GITHUB_TOKEN` secret in ai-workflow repo
- [ ] Verified GitHub Actions is enabled with read/write permissions
- [ ] All changes committed and pushed to main branch
- [ ] Created and pushed v1.0.0 tag
- [ ] Verified GitHub Actions workflow completed successfully
- [ ] Checked GitHub Release was created with binaries
- [ ] Verified Formula/work.rb exists in homebrew-tap
- [ ] Tested Homebrew installation on macOS
- [ ] Announced release to team (optional)

## Support

If you encounter issues not covered in this guide:

1. Check GitHub Actions logs for detailed error messages
2. Review GoReleaser documentation
3. Check existing GitHub issues in goreleaser/goreleaser
4. Open an issue in this repository with workflow logs

---

**Once you complete the first release, you can delete or archive this document, or keep it for reference when onboarding new maintainers.**
