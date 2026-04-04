# CCC Team Skills Plugin

Claude Code plugin for 3-pane team sessions with Planner/Executor/Reviewer roles.

## Structure

```
ccc-team-skills/
├── .claude-plugin/
│   └── plugin.json          # Plugin metadata
├── skills/
│   ├── ccc-interpane/      # Inter-pane messaging skill
│   │   ├── SKILL.md
│   │   └── test.sh
│   └── ccc-team-session/   # Team session management skill
│       ├── SKILL.md
│       └── test.sh
└── README.md
```

## Installation

The CCC team skills are part of the CCC plugin structure and are accessed directly from the plugin directory. No global installation is required.

### Using Skills in a Project

1. **For project-local skills**: Copy the skills directory to your project's `.claude/` directory:
   ```bash
   # In your project directory
   mkdir -p .claude/skills
   cp -r /path/to/ccc/plugins/ccc-team-skills/skills/* .claude/skills/
   chmod +x .claude/skills/*/test.sh
   ```

2. **Or reference directly**: When working in a CCC project, the skills in `plugins/ccc-team-skills/skills/` are accessible to Claude Code when CCC_ROLE is set.

### Using the Plugin During Development

```bash
# Clone the ccc repo
git clone https://github.com/tuannvm/ccc.git
cd ccc

# Skills are in plugins/ccc-team-skills/skills/
# Reference them directly when CCC_ROLE is set
```

## Skills

### ccc-interpane

Inter-pane communication via @mentions and tmux.

**Auto-loads when**: `CCC_ROLE` env var is set to `planner|executor|reviewer`

**Commands**:
- Send messages using tmux buffer approach
- ACK protocol for message acknowledgment
- Validation test suite

### ccc-team-session

Team session creation and management.

**Provides**:
- 3-pane tmux layout setup
- Role-based pane naming
- Session lifecycle management

## Verification

```bash
# Run validation tests from the skills directory
./skills/ccc-interpane/test.sh
./skills/ccc-team-session/test.sh

# Or from project-local installation
.your-project/.claude/skills/ccc-interpane/test.sh
```

## Architecture

```
Session (tmux)
  └─ Window
       ├─ Pane 1: Planner  (CCC_ROLE=planner)
       ├─ Pane 2: Executor (CCC_ROLE=executor)
       └─ Pane 3: Reviewer (CCC_ROLE=reviewer)
```

## Usage

1. Create tmux window with 3 panes
2. Set `CCC_ROLE` in each pane
3. Start Claude Code
4. Skills auto-load based on role
5. Communicate via @mentions

See individual SKILL.md files for detailed usage.
