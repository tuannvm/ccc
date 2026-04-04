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

### Option 1: Clone and Install

```bash
# Clone the ccc repo
git clone https://github.com/tuannvm/ccc.git
cd ccc

# Run the install script
./plugins/install-skills.sh
```

### Option 2: Manual Installation

```bash
# Copy skills to ~/.claude/skills/
cp -r skills/* ~/.claude/skills/
chmod +x ~/.claude/skills/*/test.sh
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
# Run validation tests
~/.claude/skills/ccc-interpane/test.sh
~/.claude/skills/ccc-team-session/test.sh
```

## Architecture

```
Session (tmux)
  └─ Window
       ├─ Pane 0: Planner  (CCC_ROLE=planner)
       ├─ Pane 1: Executor (CCC_ROLE=executor)
       └─ Pane 2: Reviewer (CCC_ROLE=reviewer)
```

## Usage

1. Create tmux window with 3 panes
2. Set `CCC_ROLE` in each pane
3. Start Claude Code
4. Skills auto-load based on role
5. Communicate via @mentions

See individual SKILL.md files for detailed usage.
