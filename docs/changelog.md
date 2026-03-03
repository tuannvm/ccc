# Changelog

All notable changes to ccc (Claude Code Companion) will be documented in this file.

## [Unreleased]

### Added
- Documentation website with architecture, configuration, and usage guides

## [1.2.1] - 2026-03-03

### Fixed
- **Session restart bug**: Prevented unnecessary session restarts when Claude is already running
  - Added fallback detection using Claude prompt in pane content
  - Made shell detection more conservative when `skipRestart=true`
  - Avoided pane respawn when session is in unknown state but might be active
  - See PR #2 for details

### Changed
- Updated `tmuxSafeName` test to match actual behavior (double underscores for dots)

## [1.2.0] - 2026-02-28

### Added
- **Per-project hooks**: Hooks are now installed per-project instead of globally
  - `ccc install-hooks` - Install hooks in current project
  - `ccc cleanup-hooks` - Remove hooks from current project
  - Hooks are automatically installed when creating sessions
  - See PR #1 for details

### Changed
- **Provider system**: Refactored to use provider abstraction layer
  - Multiple providers can be configured
  - Active provider can be set globally
  - Providers can be selected per-session
  - Atomic config writes to prevent corruption

### Fixed
- Improved Claude detection for npm-installed versions
- Better multi-line send handling in tmux

## [1.1.0] - 2026-01-04

### Fixed
- **tmux socket path**: Added auto-detection for Linux vs macOS socket paths
- **Claude binary detection**: Added `PATH` lookup before checking fallback paths
- **Session startup**: Fixed tmux session creation to use proper TTY
- **Project directory creation**: Fixed issue where directories weren't created
- **Hook session matching**: Fixed path suffix matching for sessions

### Changed
- Configuration auto-migration from old `~/.ccc.json` format
- Improved error messages and diagnostics

## [1.0.0] - 2025-12-15

### Added
- Initial release of ccc
- Telegram bot integration
- Multi-session support with topics
- Voice message transcription
- Image support
- File transfer with relay
- OTP permission mode
- Provider abstraction
- tmux integration
- Claude Code hooks

## Version History

| Version | Date | Description |
|---------|------|-------------|
| 1.2.1 | 2026-03-03 | Session restart bug fix |
| 1.2.0 | 2026-02-28 | Per-project hooks, provider refactoring |
| 1.1.0 | 2026-01-04 | Major bug fixes (tmux, Claude detection, paths) |
| 1.0.0 | 2025-12-15 | Initial release |

## Migration Guides

### From 1.1.x to 1.2.0

**Per-project hooks:**

Previously, hooks were installed globally. Now they're per-project:

```bash
# Old (global hooks)
~/.claude/hooks/*  # Removed

# New (per-project)
~/myproject/.claude/hooks/*
```

**Migration:**

Run `ccc install-hooks` in each project:
```bash
cd ~/myproject
ccc install-hooks
```

**Provider configuration:**

Old single-provider config:
```json
{
  "provider": {
    "base_url": "https://api.example.com"
  }
}
```

New multi-provider config:
```json
{
  "active_provider": "custom",
  "providers": {
    "custom": {
      "base_url": "https://api.example.com"
    }
  }
}
```

Auto-migration happens on first run.

### From 1.0.x to 1.1.0

No special migration needed. Configuration auto-migrates from `~/.ccc.json` to `~/.config/ccc/config.json`.

## Release Process

1. Update version in `main.go`
2. Update CHANGELOG.md
3. Commit changes
4. Create git tag:
   ```bash
   git tag -a v1.2.1 -m "Release v1.2.1"
   ```
5. Push tag:
   ```bash
   git push origin v1.2.1
   ```

## Support

For issues, questions, or contributions:
- GitHub: https://github.com/kidandcat/ccc
- Issues: https://github.com/kidandcat/ccc/issues
