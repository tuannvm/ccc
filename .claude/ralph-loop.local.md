---
active: true
iteration: 1
max_iterations: 25
completion_promise: "CONFIG_REFACTOR_COMPLETE"
started_at: "2026-03-07T15:11:38Z"
---


# Refactor ccc Config Structure - Implementation Phase

## Design Approval
The refactoring design has been approved and is documented in:
docs/REFACTOR_DESIGN.md

## Implementation Tasks

Based on the updated design (incorporating Codex review feedback):

### Phase 0: Preparation (CRITICAL - Do First)
1. Add tests for existing behavior BEFORE refactoring
   - Test loadConfig() with current config.json
   - Test saveConfig() with atomic write
   - Test provider resolution functions
   - Test session lookup functions
   - Document these as baseline tests

2. Create config validation
   - Add validateConfig() function to config_load.go
   - Check required fields
   - Validate provider configs have auth_token OR auth_env_var
   - Return helpful error messages

### Phase 1: Create New Files (Incremental)
For each file, do: CREATE → go build → go test → commit

1. **types.go** - All struct definitions
   - Move all structs from main.go
   - Add comprehensive package doc
   - Add section comments in Config struct
   - Include parseHookData() helper

2. **config_paths.go** - Path utilities
   - Move from config.go: configDir(), cacheDir(), getConfigPath()
   - Move from config.go: getProjectsDir(), resolveProjectPath(), expandPath()
   - No dependencies on other new files

3. **config_validation.go** - NEW (Codex feedback)
   - validateConfig() function
   - Check required fields for configured features
   - Validate provider configs
   - Clear error messages

4. **config_load.go** - Config loading
   - Move from config.go: loadConfig()
   - Add validateConfig() call
   - Keep migration logic for old format

5. **config_save.go** - Atomic saving
   - Move from config.go: saveConfig()
   - Already has atomic write + dir sync

6. **session_lookup.go** - NEW (Codex feedback)
   - getSessionByTopic() - from session.go
   - findSessionByClaudeID() - from hooks.go
   - findSessionByCwd() - from hooks.go
   - findSession() - combined lookup

7. **session_persist.go** - NEW (Codex feedback)
   - persistClaudeSessionID() - from hooks.go
   - Any other session write operations

8. **provider.go** - Enhance
   - Already has Provider interface and implementations
   - Add getActiveProvider() - from config.go
   - Add getProvider() - from config.go
   - Add getProviderNames() - from config.go
   - Add ensureProviderSettings() - from config.go

### Phase 2: Update Main & Clean Up
1. Update main.go - Remove struct definitions
2. Update all import statements across codebase
3. Remove old config.go
4. Update package documentation

### Phase 3: Testing & Validation
1. Run full test suite
2. Verify config.json loads correctly
3. Test atomic save still works
4. Test provider resolution
5. Test session lookups
6. Manual testing checklist

## Success Criteria

1. ✅ All tests pass (baseline tests + new tests)
2. ✅ go build succeeds
3. ✅ No circular dependencies
4. ✅ Config loads without migration
5. ✅ Each file has single responsibility
6. ✅ Package documentation complete
7. ✅ Code review passes (Claude + Gemini + Codex)

## Implementation Order (CRITICAL - Follow This)

For each file:
1. CREATE the new file
2. RUN `go build` → fix errors
3. RUN `go test -run TestBaseline` → ensure existing behavior works
4. COMMIT with descriptive message
5. Proceed to next file

## Files to Create/Modify Summary

CREATE:
- types.go
- config_paths.go
- config_validation.go
- config_load.go
- config_save.go
- session_lookup.go
- session_persist.go

MODIFY:
- provider.go (enhance)
- main.go (remove structs, update imports)
- hooks.go (update imports)
- commands.go (update imports)
- session.go (update imports)
- All other files importing from main.go or config.go

DELETE:
- config.go (after migration)

## Completion Promise
"CONFIG_REFACTOR_COMPLETE" - Output ONLY when ALL tasks done AND reviews pass

## Review Requirements

After implementation, get reviews from:
1. **Claude (self-review)** - Verify requirements met, no regressions
2. **Gemini CLI** (gemini-3.1-pro, high reasoning) - Code review
3. **Codex CLI** (gpt-5.3-codex, high reasoning) - Final verification

Use:
- Self-review after implementation
- /gemini skill for Gemini review
- /codex skill for Codex review


