# Config Structure Refactoring - Design Document

## Current State Analysis

### File: `config.go` (427 lines)
**Mixed responsibilities:**
- Directory utilities (`configDir()`, `cacheDir()`)
- Path management (`getConfigPath()` with migration)
- Config I/O (`loadConfig()`, `saveConfig()`)
- Path resolution (`getProjectsDir()`, `resolveProjectPath()`, `expandPath()`)
- Provider management (`getActiveProvider()`, `getProvider()`, `getProviderNames()`, `ensureProviderSettings()`)

### File: `main.go` (527 lines)
**Struct definitions mixed with CLI:**
- `SessionInfo` (lines 13-22)
- `ProviderConfig` (lines 25-40)
- `Config` (lines 43-57)
- Telegram types (lines 59-105)
- CLI command handlers

### File: `provider.go` (247 lines)
**Already well-organized:**
- `Provider` interface
- `BuiltinProvider` implementation
- `ConfiguredProvider` implementation

### File: `ledger.go` (189 lines)
**Self-contained:**
- `MessageRecord` type
- Ledger operations

### Files with scattered session logic:
- `session.go` - `getSessionByTopic()`
- `hooks.go` - `findSessionByClaudeID()`, `findSessionByCwd()`, `persistClaudeSessionID()`
- `commands.go` - Session lookup helpers

---

## Proposed Structure

### Principle: Single Responsibility per File

```
┌─────────────────────────────────────────────────────────────┐
│                      NEW FILE STRUCTURE                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  types.go              - All struct definitions             │
│  ├── Config            - Main config struct                 │
│  ├── SessionInfo       - Session data                       │
│  ├── ProviderConfig    - Provider settings                  │
│  ├── Telegram*         - Telegram types                    │
│  ├── HookData          - Hook types                        │
│  └── MessageRecord     - Ledger types                      │
│                                                              │
│  config/               - Config package (internal)          │
│  ├── load.go           - Config loading & migration         │
│  ├── save.go           - Atomic config saving               │
│  └── paths.go          - Path utilities                     │
│                                                              │
│  provider.go           - Provider interface & impls        │
│  ├── Provider interface                                      │
│  ├── BuiltinProvider                                         │
│  └── ConfiguredProvider                                      │
│                                                              │
│  session.go            - Session utilities                  │
│  ├── getSessionByTopic()                                     │
│  ├── findSessionBy*()                                       │
│  └── persistClaudeSessionID()                                │
│                                                              │
│  main.go               - CLI entry point only                │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Design Decisions

### Decision 1: Flat vs Nested Config Structure

**Recommendation: Keep FLAT structure**

Reasons:
- ✅ JSON compatibility - no migration needed
- ✅ Simpler serialization
- ✅ Easier to understand for users editing config
- ✅ No breaking changes

**Instead, use clear section comments:**
```go
type Config struct {
    // ========== Telegram Integration ==========
    BotToken string `json:"bot_token"`
    ChatID   int64  `json:"chat_id"`
    GroupID  int64  `json:"group_id,omitempty"`

    // ========== Sessions ==========
    Sessions map[string]*SessionInfo `json:"sessions,omitempty"`

    // ========== AI Providers ==========
    ActiveProvider string                     `json:"active_provider,omitempty"`
    Providers     map[string]*ProviderConfig `json:"providers,omitempty"`

    // ========== User Preferences ==========
    ProjectsDir       string `json:"projects_dir,omitempty"`
    TranscriptionLang string `json:"transcription_lang,omitempty"`
    RelayURL          string `json:"relay_url,omitempty"`
    Away              bool   `json:"away"`

    // ========== Authentication ==========
    OAuthToken string `json:"oauth_token,omitempty"`
    OTPSecret  string `json:"otp_secret,omitempty"`

    // ========== Legacy ==========
    Provider *ProviderConfig `json:"provider,omitempty"` // Deprecated
}
```

### Decision 2: Subpackage for Config or Not?

**Recommendation: NO subpackage**

Reasons:
- ✅ Simpler import paths (`config.Config` vs just `Config`)
- ✅ No circular import risk
- ✅ All config functions remain at package level
- ✅ Smaller project doesn't need subpackages

**Alternative: Use `_config.go` suffix to group:**
- `types.go` - Struct definitions
- `config_load.go` - Loading
- `config_save.go` - Saving
- `config_paths.go` - Paths

### Decision 3: Provider Interface Location

**Recommendation: Keep in `provider.go`**

Already well-organized with:
- Interface definition
- Two implementations (Builtin, Configured)
- Helper functions could be moved here

### Decision 4: Session Utilities Consolidation

**Recommendation: Create `session.go` with:**

From `session.go`:
- `getSessionByTopic()` - already here

From `hooks.go`:
- `findSessionByClaudeID()`
- `findSessionByCwd()`
- `persistClaudeSessionID()`

From `commands.go`:
- Any session lookup helpers

---

## Migration Plan

### Phase 1: Create New Files (Non-Breaking)

1. **types.go** - Extract all structs from main.go
2. **config_load.go** - Extract loadConfig() and migration
3. **config_save.go** - Extract saveConfig()
4. **config_paths.go** - Extract path utilities
5. **session.go** - Consolidate session utilities

### Phase 2: Update Imports

1. Update all files to import from new locations
2. Remove duplicate struct definitions from main.go
3. Run `go build` to check for errors

### Phase 3: Clean Up

1. Delete old `config.go`
2. Remove unused functions
3. Update package documentation

### Phase 4: Testing

1. Run full test suite
2. Verify config loads correctly
3. Test atomic save
4. Test provider resolution

---

## File-by-File Breakdown

### 1. types.go (New)

**Contents:**
- All struct definitions
- Helper functions for parsing (e.g., `parseHookData`)

**Exports:**
- `Config`, `SessionInfo`, `ProviderConfig`
- `TelegramMessage`, `CallbackQuery`, `TelegramUpdate`, etc.
- `HookData`, `MessageRecord`
- `parseHookData()`

---

### 2. config_load.go (New)

**Contents:**
- `loadConfig()` - Load with migration logic
- Old format migration (map[string]int64 → SessionInfo)

**Dependencies:**
- `types.go` - for Config struct
- `config_paths.go` - for getConfigPath()

---

### 3. config_save.go (New)

**Contents:**
- `saveConfig()` - Atomic write with directory sync

**Dependencies:**
- `types.go` - for Config struct
- `config_paths.go` - for getConfigPath()

---

### 4. config_paths.go (New)

**Contents:**
- `configDir()` - ~/.config/ccc
- `cacheDir()` - ~/Library/Caches/ccc
- `getConfigPath()` - With migration from old path
- `getProjectsDir()` - Expand projects_dir
- `resolveProjectPath()` - Resolve project paths
- `expandPath()` - Expand ~

**Dependencies:**
- None (pure path utilities)

---

### 5. session.go (New/Enhanced)

**Contents:**
- `getSessionByTopic(config, topicID)` - Already here
- `findSessionByClaudeID(config, claudeSessionID)` - From hooks.go
- `findSessionByCwd(config, cwd)` - From hooks.go
- `persistClaudeSessionID(config, sessName, claudeSessionID)` - From hooks.go
- `findSession(config, cwd, claudeSessionID)` - Combines the above

**Dependencies:**
- `types.go` - for Config, SessionInfo
- `config_paths.go` - for path utilities

---

### 6. provider.go (Enhanced)

**Current:**
- `Provider` interface ✓
- `BuiltinProvider` ✓
- `ConfiguredProvider` ✓

**Add:**
- `getActiveProvider(config)` - From config.go
- `getProvider(config, name)` - From config.go
- `getProviderNames(config)` - From config.go

**Remove from config.go:**
- These functions move to provider.go

**Dependencies:**
- `types.go` - for Config, ProviderConfig
- `config_paths.go` - for expandPath

---

### 7. main.go (Cleaned)

**Remove:**
- All struct definitions (moved to types.go)
- Provider functions (moved to provider.go)
- Config I/O functions (moved to config_*.go)

**Keep:**
- `const version`
- CLI command handlers
- `main()` function

---

## Risk Assessment

### Low Risk
- Creating new files (non-breaking)
- Moving functions between files
- Adding comments

### Medium Risk
- Import updates across many files
- Potential circular dependencies

### High Risk
- Changing JSON struct tags (avoid this!)
- Breaking existing configs

---

## Success Metrics

1. ✅ All tests pass
2. ✅ `go build` succeeds
3. ✅ No circular dependencies
4. ✅ Config loads without migration
5. ✅ Each file has single responsibility
6. ✅ No dead code

---

## Open Questions

1. **Should we create a `config/` subpackage?**
   - Pros: Better organization
   - Cons: More imports, potential circular deps

2. **Should `ensureProviderSettings()` stay in config package or move to provider.go?**
   - It writes to provider's settings.json
   - Currently in config.go

3. **Should we consolidate the 5 config_* files into a subpackage?**
   - Option A: `config.Load()`, `config.Save()`
   - Option B: Keep as package-level functions

---

## Codex Review Feedback (Incorporated)

### Priority 1 Updates (Must Fix)

1. **Add Config Validation**
   - New file: `config_validation.go` or integrate into `config_load.go`
   - Validate required fields
   - Validate provider configs (auth_token OR auth_env_var required)
   - Validate session paths exist

2. **Test Coverage Baseline**
   - Phase 0: Add tests for EXISTING behavior first
   - Document current config format
   - Create migration tests

3. **Rollback Plan**
   - Keep git commits granular for easy revert
   - Document rollback procedure
   - Test rollback path

4. **Split Session Utilities**
   - `session_lookup.go` - Query functions only
   - `session_persist.go` - Write operations

### Priority 2 Updates (Should Fix)

1. **Reconsider Subpackage Approach**
   - Current: No subpackage (namespace pollution)
   - Alternative: `config/` subpackage with clean API
   - Decision: **Keep no subpackage for now**, but structure to allow future migration

2. **Move ensureProviderSettings**
   - Currently in config.go
   - Should move to provider.go
   - Rationale: Provider-specific logic

### Updated File Structure

```
┌─────────────────────────────────────────────────────────────┐
│                      UPDATED FILE STRUCTURE                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  types.go                 - All struct definitions            │
│  ├── Config, SessionInfo, ProviderConfig, etc.            │
│                                                              │
│  config_paths.go          - Path utilities (no deps)        │
│  ├── configDir(), cacheDir(), expandPath()                  │
│                                                              │
│  config_load.go           - Load & validation              │
│  ├── loadConfig(), validateConfig()                        │
│                                                              │
│  config_save.go           - Atomic saving                   │
│  ├── saveConfig()                                          │
│                                                              │
│  session_lookup.go        - Session queries                 │
│  ├── getSessionByTopic(), findSessionBy*()                  │
│                                                              │
│  session_persist.go       - Session writes                  │
│  ├── persistClaudeSessionID()                              │
│                                                              │
│  provider.go              - Provider interface & helpers    │
│  ├── getActiveProvider(), ensureProviderSettings()         │
│                                                              │
│  main.go                  - CLI only                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Updated Migration Plan

**Phase 0: Preparation (NEW)**
- Add tests for existing behavior (test coverage baseline)
- Document current config format
- Create config validation tests
- Set up git branch for refactoring

**Phase 1: Create New Files**
- Create each new file incrementally
- Run `go build` after EACH file
- Run `go test` after EACH file

**Phase 2: Move Functions**
- Move functions one at a time
- Update imports incrementally
- Build and test after each move

**Phase 3: Clean Up**
- Remove old config.go
- Remove duplicate struct definitions from main.go
- Update all imports

**Phase 4: Validation**
- Full test suite
- Manual testing checklist
- Config migration testing (load old configs)

### Config Validation Strategy

Add `validateConfig()` function that checks:
- Required fields for configured features
- Provider configs have auth_token OR auth_env_var
- Session paths exist (optional, warn if not)
- No unknown fields (allow forward compatibility)

---

## Next Steps

1. ✅ Design document created
2. ✅ Codex review incorporated
3. ⏳ User approval of updated design
4. ⏳ Ralph loop implementation with Codex review
