#!/bin/bash
# CCC Inter-Pane Skill Validation Suite
# Tests the inter-pane communication skill setup and tmux integration

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
echo "CCC Inter-Pane Skill Validation Suite"
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
    log_fail "SKILL.md not found"
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

test_num "tmux can list sessions"
if command -v tmux &> /dev/null && tmux list-sessions &> /dev/null; then
    SESSION_COUNT=$(tmux list-sessions 2>/dev/null | wc -l)
    log_pass "tmux responsive, $SESSION_COUNT session(s)"
else
    log_skip "No tmux sessions (OK for skill validation)"
fi

# ==========================================
# SECTION 3: Pane Index Mapping Validation
# ==========================================
log_info "SECTION 3: Pane Index Mapping"
echo "----------------------------------------"

test_num "Planner pane index mapping"
if grep -q "| @planner  | :.1" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Planner maps to :.1"
else
    log_fail "Planner pane index mapping incorrect"
fi

test_num "Executor pane index mapping"
if grep -q "| @executor | :.2" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Executor maps to :.2"
else
    log_fail "Executor pane index mapping incorrect"
fi

test_num "Reviewer pane index mapping"
if grep -q "| @reviewer | :.3" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Reviewer maps to :.3"
else
    log_fail "Reviewer pane index mapping incorrect"
fi

# ==========================================
# SECTION 4: CCC_ROLE Bootstrap Validation
# ==========================================
log_info "SECTION 4: CCC_ROLE Bootstrap Mechanism"
echo "----------------------------------------"

test_num "CCC_ROLE detection documented"
if grep -q "CCC_ROLE" "$SKILL_FILE" 2>/dev/null; then
    log_pass "CCC_ROLE mentioned in skill"
else
    log_fail "CCC_ROLE not documented"
fi

test_num "CCC_ROLE env var check command"
if grep -q 'echo \$CCC_ROLE' "$SKILL_FILE" 2>/dev/null; then
    log_pass "CCC_ROLE check command documented"
else
    log_fail "CCC_ROLE check command missing"
fi

test_num "tmux pane title check documented"
if grep -q "pane_title" "$SKILL_FILE" 2>/dev/null; then
    log_pass "tmux pane title check documented"
else
    log_fail "tmux pane title check missing"
fi

# ==========================================
# SECTION 5: Security Validation
# ==========================================
log_info "SECTION 5: Security Validation"
echo "----------------------------------------"

test_num "Heredoc safety documented"
if grep -q "<< 'EOF'" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Heredoc ('<< 'EOF'') safety documented"
else
    log_fail "Heredoc safety not documented"
fi

test_num "Current window constraint documented"
if grep -qi "current window" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Current window constraint documented"
else
    log_fail "Current window constraint missing"
fi

# ==========================================
# SECTION 6: ACK Protocol Validation
# ==========================================
log_info "SECTION 6: ACK Protocol"
echo "----------------------------------------"

test_num "ACK protocol documented"
if grep -q "@{sender} ACK" "$SKILL_FILE" 2>/dev/null; then
    log_pass "ACK protocol present"
else
    log_fail "ACK protocol missing"
fi

test_num "Done response documented"
if grep -q "@{sender} Done" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Done response documented"
else
    log_fail "Done response missing"
fi

# ==========================================
# SECTION 7: Integration Test (if tmux available)
# ==========================================
if command -v tmux &> /dev/null && tmux list-sessions &> /dev/null; then
    log_info "SECTION 7: Integration Test (Live tmux)"
    echo "----------------------------------------"

    test_num "Create test buffer"
    echo "test message $(date)" > /tmp/ccc-test-msg.txt
    if tmux load-buffer -b ccc-test-buffer /tmp/ccc-test-msg.txt 2>/dev/null; then
        log_pass "tmux load-buffer works"
        tmux delete-buffer -b ccc-test-buffer 2>/dev/null || true
    else
        log_fail "tmux load-buffer failed"
    fi

    test_num "Heredoc message creation"
    if cat > /tmp/ccc-heredoc-test.txt << 'ENDOFFILE'
test message
ENDOFFILE
    then
        log_pass "Heredoc works correctly"
    else
        log_fail "Heredoc failed"
    fi

    test_num "Check for active panes in current window"
    CURRENT_WINDOW_PANES=$(tmux list-panes 2>/dev/null | wc -l || echo "0")
    log_info "Current window has $CURRENT_WINDOW_PANES pane(s)"
    if [[ "$CURRENT_WINDOW_PANES" -eq 3 ]]; then
        log_pass "3-pane window detected (team session ready)"
    elif [[ "$CURRENT_WINDOW_PANES" -gt 0 ]]; then
        log_skip "Partial window ($CURRENT_WINDOW_PANES panes)"
    else
        log_skip "No panes in current window"
    fi
else
    log_info "SECTION 7: Integration Test (Live tmux)"
    echo "----------------------------------------"
    test_num "Integration prerequisites"
    log_skip "Skipping live tmux checks (tmux server/session not active)"
fi

# ==========================================
# SECTION 8: Related Skills
# ==========================================
log_info "SECTION 8: Related Skills"
echo "----------------------------------------"

test_num "ccc-team-session skill exists"
# Check local skills directory (relative to script), global, and common paths
CCC_TEAM_SESSION_PATHS=(
    "$SCRIPT_DIR/../ccc-team-session/SKILL.md"
    "$HOME/.claude/skills/ccc-team-session/SKILL.md"
)
FOUND=0
for path in "${CCC_TEAM_SESSION_PATHS[@]}"; do
    if [[ -f "$path" ]]; then
        FOUND=1
        break
    fi
done
if [[ $FOUND -eq 1 ]]; then
    log_pass "ccc-team-session skill present"
else
    log_fail "ccc-team-session skill missing"
fi

test_num "tmux-intercom skill exists (reference)"
TMUX_INTERCOM_PATHS=(
    "$SCRIPT_DIR/../tmux-intercom/SKILL.md"
    "$HOME/.claude/skills/tmux-intercom/SKILL.md"
)
FOUND=0
for path in "${TMUX_INTERCOM_PATHS[@]}"; do
    if [[ -f "$path" ]]; then
        FOUND=1
        break
    fi
done
if [[ $FOUND -eq 1 ]]; then
    log_pass "tmux-intercom skill present (baseline)"
else
    log_skip "tmux-intercom not found (optional)"
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
    echo "The inter-pane skill is properly configured."
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo "Please review failed tests and fix the skill."
    exit 1
fi
