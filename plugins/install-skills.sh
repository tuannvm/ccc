#!/bin/bash
# Install CCC Team Skills Plugin
# Copies skills to ~/.claude/skills/

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_DIR="$SCRIPT_DIR"
SKILLS_DIR="$HOME/.claude/skills"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "========================================"
echo "CCC Team Skills Plugin Installer"
echo "========================================"
echo ""

# Check if ~/.claude exists
if [[ ! -d "$HOME/.claude" ]]; then
    echo -e "${YELLOW}Creating ~/.claude directory...${NC}"
    mkdir -p "$HOME/.claude"
fi

# Create skills directory if needed
if [[ ! -d "$SKILLS_DIR" ]]; then
    echo -e "${YELLOW}Creating ~/.claude/skills directory...${NC}"
    mkdir -p "$SKILLS_DIR"
fi

# Install each skill
install_skill() {
    local skill_name="$1"
    local skill_dir="$PLUGIN_DIR/$skill_name"
    local target_dir="$SKILLS_DIR/$skill_name"

    if [[ ! -d "$skill_dir" ]]; then
        echo -e "${RED}✗ Skill not found: $skill_name${NC}"
        return 1
    fi

    echo -e "${YELLOW}Installing $skill_name...${NC}"

    # Remove existing installation
    if [[ -d "$target_dir" ]]; then
        rm -rf "$target_dir"
    fi

    # Copy skill
    cp -r "$skill_dir" "$target_dir"
    chmod +x "$target_dir"/*.sh 2>/dev/null || true

    echo -e "${GREEN}✓ Installed $skill_name${NC}"
}

# Try ccc-team-skills structure first (proper plugin format)
if [[ -d "$PLUGIN_DIR/ccc-team-skills/skills" ]] && compgen -G "$PLUGIN_DIR/ccc-team-skills/skills"/* > /dev/null 2>&1; then
    echo "Installing from ccc-team-skills plugin..."
    for skill in "$PLUGIN_DIR/ccc-team-skills/skills"/*; do
        if [[ -d "$skill" ]]; then
            skill_name=$(basename "$skill")
            target_dir="$SKILLS_DIR/$skill_name"
            if [[ -d "$target_dir" ]]; then
                rm -rf "$target_dir"
            fi
            cp -r "$skill" "$target_dir"
            chmod +x "$target_dir"/*.sh 2>/dev/null || true
            echo -e "${GREEN}✓ Installed $skill_name${NC}"
        fi
    done
elif [[ -d "$PLUGIN_DIR/ccc-interpane" ]]; then
    # Flat structure
    echo "Installing from flat structure..."
    install_skill "ccc-interpane"
    install_skill "ccc-team-session"
else
    echo -e "${RED}✗ No skills found in plugin directory${NC}"
    exit 1
fi

echo ""
echo "========================================"
echo "Installation Complete!"
echo "========================================"
echo ""
echo "Skills installed to: $SKILLS_DIR"
echo ""
echo "The SessionStart hook for auto-load is managed by CCC (run 'ccc install')."
echo ""
echo "To use:"
echo "  1. Run 'ccc install' to set up CCC hooks"
echo "  2. Start Claude Code in a 3-pane tmux session"
echo "  3. Set CCC_ROLE=planner|executor|reviewer in each pane"
echo "  4. Skills auto-load when CCC_ROLE is set or via @mentions"
echo ""
echo "To validate:"
echo "  ~/.claude/skills/ccc-interpane/test.sh"
echo "  ~/.claude/skills/ccc-team-session/test.sh"
echo ""
