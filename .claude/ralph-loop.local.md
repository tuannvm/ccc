---
active: true
iteration: 1
max_iterations: 10
completion_promise: "WORKTREE_COLORS_IMPLEMENTED"
started_at: "2026-03-17T20:52:51Z"
---

Implement worktree topic color feature for ccc (Claude Code Companion):

## Requirements

Add visual color coding to Telegram forum topics for worktree sessions so all worktrees belonging to the same base project share the same color. This creates visual grouping and helps distinguish worktrees from base sessions.

## Desired Behavior

Given a base project 'myproject':
- myproject (base session) → default/random Telegram color
- myproject_main (worktree) → purple (all myproject worktrees are purple)
- myproject_feature-auth (worktree) → purple
- myproject_hotfix (worktree) → purple

Given a different project 'otherproject':
- otherproject (base session) → default/random Telegram color
- otherproject_dev (worktree) → orange (all otherproject worktrees are orange)
- otherproject_fix (worktree) → orange

## Implementation Details

1. Add a color generation function in telegram.go:
   - Input: base session name (e.g., 'myproject')
   - Output: consistent hex color string for that base name
   - Use hash-based approach to ensure same base always gets same color
   - Use visually distinct colors from a predefined palette

2. Modify createForumTopic function:
   - Add new parameter: isWorktree bool (or similar context)
   - When creating a worktree topic, add icon_color to the Telegram API params
   - Base sessions (non-worktree) get default Telegram behavior (no explicit color)

3. Update all callers of createForumTopic:
   - Find all locations where createForumTopic is called
   - Pass appropriate worktree context so the function knows whether to apply color
   - Ensure the base session name is available for color generation

4. Color palette (hex format for Telegram):
   - Use these distinct colors: 9B59B6 (purple), E67E22 (orange), 3498DB (blue), 1ABC9C (teal), E74C3C (red), F39C12 (yellow-orange)

## Success Criteria

- All worktree sessions for the same base project have the same icon color
- Different base projects get different colors (consistent across runs)
- Base sessions (non-worktree) are not affected (use Telegram's default)
- No breaking changes to existing functionality
- Code follows existing project conventions and patterns

## Notes

- Telegram Bot API createForumTopic accepts 'icon_color' parameter (RGB hex like '9B59B6')
- Worktree sessions are identified by is_worktree=true and have worktree_name in SessionInfo
- Session names for worktrees follow pattern: {base}_{worktree_name}
- The base session name needs to be extracted from the worktree session name for color generation

Test by creating multiple worktree sessions for the same project and verify they all have the same colored icon in Telegram.
