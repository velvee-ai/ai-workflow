# Release Guide

This document explains how to release new versions of AI Workflow CLI to Homebrew.

## Prerequisites

Before creating releases, you need to:

1. **Create a Homebrew Tap Repository**

   ```bash
   # On GitHub, create a new repository named: homebrew-tap
   # Under: velvee-ai/homebrew-tap
   ```

2. **Set up GitHub Token for Homebrew**
   - Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
   - Generate new token with `repo` scope
   - Add it as a repository secret named `HOMEBREW_TAP_GITHUB_TOKEN`
   - Go to: Settings → Secrets and variables → Actions → New repository secret

## Release Process

### 1. Update Version

Make sure your changes are committed and pushed to the main branch.

### 2. Create and Push a Tag

```bash
# Create a new version tag (e.g., v1.0.0, v1.1.0, v2.0.0)
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag to GitHub
git push origin v1.0.0
```

### 3. Automated Release

Once you push the tag:

- GitHub Actions automatically triggers the release workflow
- GoReleaser builds binaries for macOS (Intel and Apple Silicon)
- Creates a GitHub Release with release notes
- Publishes the formula to `velvee-ai/homebrew-tap`

### 4. Verify Release

After the workflow completes:

1. **Check GitHub Release**: Visit https://github.com/velvee-ai/ai-workflow/releases
2. **Check Homebrew Tap**: Visit https://github.com/velvee-ai/homebrew-tap
3. **Test Installation**:
   ```bash
   brew tap velvee-ai/tap
   brew install ai-workflow
   ai-workflow --help
   ```

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- `v1.0.0` - Major version (breaking changes)
- `v1.1.0` - Minor version (new features, backward compatible)
- `v1.0.1` - Patch version (bug fixes)

## Workflow Files

- `.goreleaser.yaml` - GoReleaser configuration
- `.github/workflows/release.yaml` - GitHub Actions release workflow

## Manual Testing Before Release

```bash
# Test build locally
go build -o ai-workflow

# Test locally with GoReleaser (requires GoReleaser installed)
goreleaser release --snapshot --clean

# Install GoReleaser
brew install goreleaser
```

## Troubleshooting

### Release workflow fails

1. Check that `HOMEBREW_TAP_GITHUB_TOKEN` secret is set
2. Verify the homebrew-tap repository exists
3. Check GitHub Actions logs for specific errors

### Formula not updating in Homebrew tap

1. Verify the token has `repo` permissions
2. Check that the repository name matches: `velvee-ai/homebrew-tap`
3. Look at GoReleaser logs in GitHub Actions

## First Release Checklist

- [ ] Create `velvee-ai/homebrew-tap` repository on GitHub
- [ ] Generate GitHub Personal Access Token with `repo` scope
- [ ] Add token as `HOMEBREW_TAP_GITHUB_TOKEN` secret
- [ ] Create and push first version tag (e.g., `v1.0.0`)
- [ ] Verify GitHub Actions workflow completes successfully
- [ ] Test Homebrew installation on macOS

## Example: Creating First Release

```bash
# 1. Ensure you're on main branch
git checkout main
git pull origin main

# 2. Create and push tag
git tag -a v1.0.0 -m "Initial release"
git push origin v1.0.0

# 3. Wait for GitHub Actions to complete
# Visit: https://github.com/velvee-ai/ai-workflow/actions

# 4. Test installation
brew tap velvee-ai/tap
brew install ai-workflow
ai-workflow --version
```
