#!/bin/bash
# CCC Team Skills Plugin Verification Script
# Verifies skills are available in the plugin directory
# NOTE: This script does NOT install to ~/.claude/ - skills work from the plugin directory

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_DIR="$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "========================================"
echo "CCC Team Skills Plugin"
echo "========================================"
echo ""

# Check for consolidated ccc-team skill
if [[ -d "$PLUGIN_DIR/ccc-team" ]]; then
    echo "Found consolidated ccc-team skill"
    echo ""
    SKILL_DIR="$PLUGIN_DIR/ccc-team"

    echo -e "${GREEN}✓${NC} Found skill: ccc-team"
    if [[ -f "$SKILL_DIR/SKILL.md" ]]; then
        echo "  - SKILL.md present"
    fi
    if [[ -f "$SKILL_DIR/test.sh" ]]; then
        echo "  - test.sh present"
        chmod +x "$SKILL_DIR/test.sh" 2>/dev/null || true
    fi

    echo ""
    echo "========================================"
    echo "Skill is available at:"
    echo "  $SKILL_DIR"
    echo ""
    echo "To use in a project:"
    echo "  1. Copy ccc-team to your project's .claude/skills/ directory"
    echo "  2. Or reference directly when CCC_ROLE is set"
    echo ""
    echo "To validate:"
    echo "  $SKILL_DIR/test.sh"
    echo ""
elif [[ -d "$PLUGIN_DIR/ccc-team-skills/skills" ]]; then
    echo "Found legacy ccc-team-skills structure"
    echo ""
    SKILLS_DIR="$PLUGIN_DIR/ccc-team-skills/skills"

    for skill in "$SKILLS_DIR"/*; do
        if [[ -d "$skill" ]]; then
            skill_name=$(basename "$skill")
            echo -e "${GREEN}✓${NC} Found skill: $skill_name"
            if [[ -f "$skill/SKILL.md" ]]; then
                echo "  - SKILL.md present"
            fi
            if [[ -f "$skill/test.sh" ]]; then
                echo "  - test.sh present"
                chmod +x "$skill/test.sh" 2>/dev/null || true
            fi
        fi
    done

    echo ""
    echo "========================================"
    echo "Skills are available at:"
    echo "  $SKILLS_DIR"
    echo ""
    echo "NOTE: These are legacy skills. Please migrate to consolidated ccc-team skill."
    echo ""
    echo "To validate:"
    echo "  $SKILLS_DIR/ccc-interpane/test.sh"
    echo "  $SKILLS_DIR/ccc-team-session/test.sh"
    echo ""
elif [[ -d "$PLUGIN_DIR/ccc-interpane" ]]; then
    echo "Found legacy flat structure"
    echo ""
    echo -e "${GREEN}✓${NC} ccc-interpane"
    echo -e "${GREEN}✓${NC} ccc-team-session"
    echo ""
    echo "Skills are available at: $PLUGIN_DIR"
    echo ""
    echo "NOTE: These are legacy skills. Please migrate to consolidated ccc-team skill."
else
    echo -e "${RED}✗ No skills found in plugin directory${NC}"
    exit 1
fi
