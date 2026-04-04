#!/bin/bash
# CCC Team Session Skill Validation Suite
# Tests the team session management skill setup

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
SKIP=0
TEST=0

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILL_FILE="$SCRIPT_DIR/SKILL.md"

log_pass() { echo -e "${GREEN}✅ PASS${NC}: $1"; PASS=$((PASS+1)); }
log_fail() { echo -e "${RED}❌ FAIL${NC}: $1"; FAIL=$((FAIL+1)); }
log_skip() { echo -e "${YELLOW}⏭️  SKIP${NC}: $1"; SKIP=$((SKIP+1)); }
log_info() { echo -e "${BLUE}ℹ️  INFO${NC}: $1"; }
test_num() { TEST=$((TEST+1)); echo -e "\n${BLUE}[Test $TEST]${NC} $1"; }

echo "========================================"
echo "CCC Team Session Skill Validation Suite"
echo "========================================"
echo ""

# ==========================================
# SECTION 1: Skill File Validation
# ==========================================
log_info "SECTION 1: Skill File Validation"
echo "----------------------------------------"

test_num "Skill file exists"
if [[ -f "$SKILL_FILE" ]]; then
    log_pass "SKILL.md exists"
else
    log_fail "SKILL.md not found at $SKILL_FILE"
fi

test_num "Frontmatter validation"
if head -1 "$SKILL_FILE" 2>/dev/null | grep -q "^---$"; then
    log_pass "Frontmatter starts correctly"
else
    log_fail "Frontmatter missing"
fi

test_num "Name field validation"
if grep -q "^name: " "$SKILL_FILE" 2>/dev/null; then
    NAME=$(grep "^name: " "$SKILL_FILE" | cut -d' ' -f2-)
    log_pass "Name field present: $NAME"
else
    log_fail "Name field missing"
fi

test_num "Description field validation"
if grep -q "^description: " "$SKILL_FILE" 2>/dev/null; then
    log_pass "Description field present"
else
    log_fail "Description field missing"
fi

# ==========================================
# SECTION 2: tmux Command Validation
# ==========================================
log_info "SECTION 2: tmux Command Validation"
echo "----------------------------------------"

test_num "tmux is installed"
if command -v tmux &> /dev/null; then
    TMUX_VERSION=$(tmux -V 2>/dev/null || echo "unknown")
    log_pass "tmux installed: $TMUX_VERSION"
else
    log_fail "tmux not found"
fi

# ==========================================
# SECTION 3: Role Documentation
# ==========================================
log_info "SECTION 3: Role Documentation"
echo "----------------------------------------"

test_num "Planner role documented"
if grep -qi "planner" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Planner role documented"
else
    log_fail "Planner role not documented"
fi

test_num "Executor role documented"
if grep -qi "executor" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Executor role documented"
else
    log_fail "Executor role not documented"
fi

test_num "Reviewer role documented"
if grep -qi "reviewer" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Reviewer role documented"
else
    log_fail "Reviewer role not documented"
fi

# ==========================================
# SECTION 4: Session Lifecycle
# ==========================================
log_info "SECTION 4: Session Lifecycle Documentation"
echo "----------------------------------------"

test_num "Create session documented"
if grep -qi "create" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Session creation documented"
else
    log_fail "Session creation not documented"
fi

test_num "Attach/detach documented"
if grep -qi "attach\|detach" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Attach/detach documented"
else
    log_fail "Attach/detach not documented"
fi

test_num "Close session documented"
if grep -qi "close\|kill" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Session close documented"
else
    log_fail "Session close not documented"
fi

# ==========================================
# SECTION 5: CCC_ROLE Environment
# ==========================================
log_info "SECTION 5: CCC_ROLE Environment"
echo "----------------------------------------"

test_num "CCC_ROLE environment variable documented"
if grep -q "CCC_ROLE" "$SKILL_FILE" 2>/dev/null; then
    log_pass "CCC_ROLE documented"
else
    log_fail "CCC_ROLE not documented"
fi

test_num "Role setup commands documented"
if grep -qi "CCC_ROLE=planner\|CCC_ROLE=executor\|CCC_ROLE=reviewer" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Role setup commands documented"
else
    log_fail "Role setup commands not documented"
fi

# ==========================================
# SECTION 6: Pane Management
# ==========================================
log_info "SECTION 6: Pane Management"
echo "----------------------------------------"

test_num "Pane naming documented"
if grep -qi "select-pane\|pane.*title\|T " "$SKILL_FILE" 2>/dev/null; then
    log_pass "Pane naming documented"
else
    log_fail "Pane naming not documented"
fi

test_num "Pane layout documented"
if grep -qi "select-layout\|layout" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Pane layout documented"
else
    log_fail "Pane layout not documented"
fi

# ==========================================
# SECTION 7: Troubleshooting
# ==========================================
log_info "SECTION 7: Troubleshooting"
echo "----------------------------------------"

test_num "Troubleshooting section exists"
if grep -qi "troubleshoot\|error\|issue" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Troubleshooting documented"
else
    log_fail "Troubleshooting not documented"
fi

# ==========================================
# SECTION 8: Related Skills
# ==========================================
log_info "SECTION 8: Related Skills"
echo "----------------------------------------"

test_num "ccc-interpane skill exists (messaging)"
CCC_INTERPANE_PATHS=(
    "$SCRIPT_DIR/../ccc-interpane/SKILL.md"
    "/home/tuannvm/.claude/skills/ccc-interpane/SKILL.md"
    "$HOME/.claude/skills/ccc-interpane/SKILL.md"
)
FOUND=0
for path in "${CCC_INTERPANE_PATHS[@]}"; do
    if [[ -f "$path" ]]; then
        FOUND=1
        break
    fi
done
if [[ $FOUND -eq 1 ]]; then
    log_pass "ccc-interpane skill present"
else
    log_fail "ccc-interpane skill missing (messaging skill)"
fi

# ==========================================
# SECTION 9: Integration Test
# ==========================================
if command -v tmux &> /dev/null; then
    log_info "SECTION 9: Integration Test (Live tmux)"
    echo "----------------------------------------"

    test_num "tmux list-sessions command"
    if tmux list-sessions &> /dev/null; then
        SESSION_COUNT=$(tmux list-sessions 2>/dev/null | wc -l)
        log_pass "tmux responsive, $SESSION_COUNT session(s)"
    else
        log_skip "No tmux access"
    fi

    test_num "tmux select-pane command (syntax check)"
    if tmux select-pane -t 0 -T "Test" 2>/dev/null; then
        log_pass "select-pane works"
        tmux select-pane -t 0 -T "" 2>/dev/null || true
    else
        log_skip "select-pane not testable in current context"
    fi
fi

# ==========================================
# SUMMARY
# ==========================================
echo ""
echo "========================================"
echo "VALIDATION SUMMARY"
echo "========================================"
echo -e "Tests: $TEST | ${GREEN}Passed: $PASS${NC} | ${RED}Failed: $FAIL${NC} | ${YELLOW}Skipped: $SKIP${NC}"
echo ""

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}✅ ALL TESTS PASSED${NC}"
    echo "The team session skill is properly configured."
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo "Please review failed tests and fix the skill."
    exit 1
fi
