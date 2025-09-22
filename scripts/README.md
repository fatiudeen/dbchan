# Version Bumping Scripts

This directory contains scripts to help manage version bumping for the Helm chart.

## Scripts

### `bump-version.sh`
The main version bumping script that handles both chart and app version updates.

**Usage:**
```bash
./scripts/bump-version.sh [chart|app|both] [patch|minor|major]
```

**Examples:**
```bash
# Bump chart version patch (0.1.0 -> 0.1.1)
./scripts/bump-version.sh chart patch

# Bump app version minor (v.0.1.0 -> v.0.2.0)
./scripts/bump-version.sh app minor

# Bump both versions major
./scripts/bump-version.sh both major
```

### `version-bump`
A convenience wrapper that provides shorter commands for common operations.

**Usage:**
```bash
# Quick commands (bumps both versions)
./scripts/version-bump patch    # 0.1.0 -> 0.1.1, v.0.1.0 -> v.0.1.1
./scripts/version-bump minor    # 0.1.0 -> 0.2.0, v.0.1.0 -> v.0.2.0
./scripts/version-bump major    # 0.1.0 -> 1.0.0, v.0.1.0 -> v.1.0.0

# Specific targets
./scripts/version-bump chart patch    # Bump only chart version
./scripts/version-bump app minor      # Bump only app version
./scripts/version-bump both major     # Bump both versions
```

## Version Bump Types

- **patch**: Increment the patch version (0.1.0 -> 0.1.1)
- **minor**: Increment the minor version and reset patch (0.1.0 -> 0.2.0)
- **major**: Increment the major version and reset minor/patch (0.1.0 -> 1.0.0)

## Features

- ✅ Bumps both chart version and app version
- ✅ Supports patch, minor, and major version bumps
- ✅ Can target specific versions (chart, app, or both)
- ✅ Colored output for better readability
- ✅ Validates input arguments
- ✅ Shows current and new versions
- ✅ Handles version format differences (chart: `0.1.0`, app: `v.0.1.0`)
- ✅ Creates backup files during updates
- ✅ Error handling and validation

## Requirements

- Bash shell
- `sed` command (usually available on Unix-like systems)
- The script must be run from the project root directory

## File Structure

The scripts expect the following file structure:
```
project-root/
├── scripts/
│   ├── bump-version.sh
│   ├── version-bump
│   └── README.md
└── charts/
    └── dbchan/
        └── Chart.yaml
```

## Examples

### Bump patch version for both chart and app
```bash
./scripts/version-bump patch
```

### Bump minor version for chart only
```bash
./scripts/version-bump chart minor
```

### Bump major version for app only
```bash
./scripts/version-bump app major
```

### Show help
```bash
./scripts/version-bump
./scripts/bump-version.sh
```
