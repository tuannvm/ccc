# Changelog

All notable changes to ccc (Claude Code Companion) will be documented in this file.

## [Unreleased]

### Fixed
- **Workspace trust dialog handling**: Fixed compatibility with Claude Code 2.1.84+ which shows a workspace trust dialog on startup
  - The dialog "Yes, I trust this folder" / "No, exit" is now automatically accepted
  - Only appears in interactive mode (not with -p flag); --dangerously-skip-permissions doesn't skip it
  - Added proper detection with both dialog strings to avoid false positives
  - Added 1-second delay after accepting to ensure dialog dismissal
  - Prevents hanging sessions when Claude Code is waiting for trust confirmation

### Added
- **Telegram Bot API 9.5 streaming**: Real-time typing effect for AI responses
  - Uses `sendMessageDraft` API method for smooth character-by-character updates
  - No "edited" tag appears on messages
  - Higher rate limits than traditional editMessageText
  - Configurable via `enable_streaming` in config.json
  - Thread-safe implementation with atomic state management
- **Inter-pane communication for team sessions**: Team sessions now support @mention-based routing between planner, executor, and reviewer panes
  - `@planner`, `@executor`, `@reviewer` mentions route messages to target panes via tmux
  - Automatic deduplication prevents duplicate delivery of the same request
  - Hop count tracking prevents infinite message loops (max 5 hops)
  - Message queuing when target pane is busy (max 10 messages per role)
  - Persistent routing state survives restarts via `.config/ccc/sessions/<name>/interpane/`
  - Full test coverage for mention parsing, role inference, and message queue management
- **Worktree auto-generation**: `/worktree` command now supports auto-generating worktree names
  - Run `/worktree` in a session topic to let Claude Code generate a unique name
  - Generated names follow Claude's adjective-noun-noun pattern (e.g., `merry-wishing-crystal`)
  - `ccc run --worktree` also supports auto-generation when no name is provided
- **Visual worktree organization**: Worktree sessions now have color-coded Telegram topics
  - All worktrees for the same base project share the same color icon
  - Uses FNV-1a hash for consistent color assignment across runs
  - Makes it easy to visually identify related worktree sessions
- **Snapshot-based worktree detection**: Improved reliability when detecting newly created worktrees
  - Prevents race conditions when multiple worktrees are created concurrently
  - Confirmation check reduces false positives from transient files
  - 30-second polling timeout with helpful error messages
- Documentation website with architecture, configuration, and usage guides
- `ccc install-hooks` command for manual hook installation in current project directory
  - Installs hooks to `.claude/settings.local.json`
  - Checks if hooks are already installed before proceeding
  - Useful for troubleshooting and manual setup

### Fixed
- **Worktree session hook routing**: Fixed critical bug where reply hooks for worktree sessions were sent to the base session's Telegram topic instead of the worktree session's topic
  - Added tmux window name detection as the primary lookup method in `findSession()`
  - Handles tmux name sanitization (dots → "__") with collision detection
  - Correctly handles session switches via `ccc attach` and manual Claude ID changes via `/session`
  - See commit `53dd308` for details
- **Hooks installation documentation**: The `install-hooks` command mentioned in docs but not implemented has now been added (PR #3)
- **`/new` command requirements**: Documented that `/new` only works in supergroups, not private chats
- **GroupID requirement**: Clarified that `ccc setgroup` must be run before `/new` will work
- **Team session role display bug**: Fixed incorrect role names in Telegram messages (e.g., planner showing [Executor])
  - Root cause: Hooks couldn't determine which pane a session belonged to (transcript path inference failed, env vars unavailable in hook context)
  - Solution: Query tmux for active pane name (Planner/Executor/Reviewer) or index (1/2/3) to determine role
  - Panes are now named during creation for better UX and more reliable role detection
  - Telegram messages now show correct role prefix: `[Planner]`, `[Executor]`, `[Reviewer]`

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
