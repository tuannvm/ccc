#!/bin/bash
# CCC Team Skill Validation Suite
# Tests the consolidated ccc-team skill (session + interpane)

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
echo "CCC Team Skill Validation Suite"
echo "========================================"
echo ""

# ==========================================
# SECTION 1: Skill File Validation
# ==========================================
log_info "SECTION 1: Skill File Validation"
echo "----------------------------------------"

test_num "Skill file exists"
if [[ -f "$SKILL_FILE" ]]; then
    log_pass "SKILL.md exists at $SKILL_FILE"
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
    if [[ "$NAME" == "ccc-team" ]]; then
        log_pass "Name field correct: $NAME"
    else
        log_fail "Name field is '$NAME', expected 'ccc-team'"
    fi
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
if tmux list-sessions &> /dev/null; then
    SESSION_COUNT=$(tmux list-sessions 2>/dev/null | wc -l)
    log_pass "tmux responsive, $SESSION_COUNT session(s)"
else
    log_skip "No tmux access"
fi

# ==========================================
# SECTION 3: 1-Based Pane Indexing
# ==========================================
log_info "SECTION 3: Pane Index Mapping (1-based)"
echo "----------------------------------------"

test_num "Planner pane index mapping"
if grep -q "Pane 1.*Planner" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Planner maps to :.1"
else
    log_fail "Planner pane index not documented correctly"
fi

test_num "Executor pane index mapping"
if grep -q "Pane 2.*Executor" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Executor maps to :.2"
else
    log_fail "Executor pane index not documented correctly"
fi

test_num "Reviewer pane index mapping"
if grep -q "Pane 3.*Reviewer" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Reviewer maps to :.3"
else
    log_fail "Reviewer pane index not documented correctly"
fi

# ==========================================
# SECTION 4: Role Documentation
# ==========================================
log_info "SECTION 4: Role Documentation"
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
# SECTION 5: Session Lifecycle
# ==========================================
log_info "SECTION 5: Session Lifecycle Documentation"
echo "----------------------------------------"

test_num "Create session documented"
if grep -qi "ccc team new\|tmux new-window" "$SKILL_FILE" 2>/dev/null; then
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
# SECTION 6: CCC_ROLE Environment
# ==========================================
log_info "SECTION 6: CCC_ROLE Environment"
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
# SECTION 7: Inter-Pane Communication
# ==========================================
log_info "SECTION 7: Inter-Pane Communication"
echo "----------------------------------------"

test_num "@mention syntax documented"
if grep -q "@planner\|@executor\|@reviewer" "$SKILL_FILE" 2>/dev/null; then
    log_pass "@mention syntax documented"
else
    log_fail "@mention syntax not documented"
fi

test_num "tmux target mapping documented"
if grep -q ":.1\|:.2\|:.3" "$SKILL_FILE" 2>/dev/null; then
    log_pass "tmux target mapping documented"
else
    log_fail "tmux target mapping not documented"
fi

test_num "Heredoc safety documented"
if grep -q "<< 'EOF'\|<< 'EOF'" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Heredoc safety documented"
else
    log_fail "Heredoc safety not documented"
fi

test_num "ACK protocol documented"
if grep -qi "ACK\|Done\|NACK" "$SKILL_FILE" 2>/dev/null; then
    log_pass "ACK protocol documented"
else
    log_fail "ACK protocol not documented"
fi

test_num "Message submit commands documented"
if grep -q "Escape\|send-keys.*Enter" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Message submit commands documented"
else
    log_fail "Double-Enter submit not documented"
fi

# ==========================================
# SECTION 8: Pane Management
# ==========================================
log_info "SECTION 8: Pane Management"
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
# SECTION 9: Troubleshooting
# ==========================================
log_info "SECTION 9: Troubleshooting"
echo "----------------------------------------"

test_num "Troubleshooting section exists"
if grep -qi "troubleshoot\|error\|issue" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Troubleshooting documented"
else
    log_fail "Troubleshooting not documented"
fi

# ==========================================
# SECTION 10: Communication Rules
# ==========================================
log_info "SECTION 10: Communication Rules"
echo "----------------------------------------"

test_num "Planner directs communication"
if grep -q "Only.*Planner\|Planner.*communicates" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Planner communication direction documented"
else
    log_skip "Communication direction not explicitly documented"
fi

test_num "Executor reports to Planner"
if grep -qi "executor.*planner\|reports.*planner" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Executor-to-Planner reporting documented"
else
    log_skip "Executor-to-Planner reporting not explicitly documented"
fi

test_num "Reviewer reports to Planner"
if grep -qi "reviewer.*planner\|reports.*planner" "$SKILL_FILE" 2>/dev/null; then
    log_pass "Reviewer-to-Planner reporting documented"
else
    log_skip "Reviewer-to-Planner reporting not explicitly documented"
fi

# ==========================================
# SECTION 11: Integration Test
# ==========================================
if command -v tmux &> /dev/null; then
    log_info "SECTION 11: Integration Test (Live tmux)"
    echo "----------------------------------------"

    test_num "Create test buffer"
    if echo "test" | tmux load-buffer -b ccc-team-test /dev/stdin 2>/dev/null; then
        log_pass "tmux load-buffer works"
        tmux delete-buffer -b ccc-team-test 2>/dev/null || true
    else
        log_skip "load-buffer not testable"
    fi

    test_num "Heredoc message creation"
    if cat > /tmp/ccc-team-test.txt << 'EOF'
test message
EOF
        [[ -f /tmp/ccc-team-test.txt ]]; then
        log_pass "Heredoc works correctly"
        rm -f /tmp/ccc-team-test.txt
    else
        log_fail "Heredoc failed"
    fi

    test_num "Check for active panes in current window"
    PANE_COUNT=$(tmux list-panes 2>/dev/null | wc -l)
    if [[ $PANE_COUNT -ge 3 ]]; then
        log_pass "Current window has $PANE_COUNT pane(s)"
    else
        log_info "Current window has $PANE_COUNT pane(s)"
        log_skip "Partial window ($PANE_COUNT panes)"
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
    echo "The ccc-team skill is properly configured."
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo "Please review failed tests and fix the skill."
    exit 1
fi
