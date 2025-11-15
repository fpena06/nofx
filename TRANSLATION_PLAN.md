# NOFX Repository Translation Plan: Chinese to English

**Version:** 1.0
**Date:** 2025-11-15
**Status:** In Progress

---

## Overview

This document outlines the phased approach to translating the NOFX repository from Chinese to English. The translation focuses on developer-facing content (logs, code comments) first, with UI translation as the lowest priority since browsers can handle that automatically.

**Scope:** 181 files containing Chinese content
**Estimated Effort:** 125-180 hours (3-4 weeks)

---

## Translation Phases

### ✅ Phase 0: Planning (CURRENT)
- [x] Analyze codebase for Chinese content
- [x] Create translation plan
- [ ] Commit plan to feature branch

---

### 🔴 **Phase 1: Logs and Terminal Messages (HIGH PRIORITY)**

**Goal:** Translate all console output, logging statements, and terminal messages

#### 1.1 Go Backend Logs (Priority: CRITICAL)
**Files to translate:** ~56 Go files
**Estimated time:** 15-20 hours

**Key files:**
- `main.go` - Startup logs, system initialization messages
- `trader/auto_trader.go` - Trading operation logs
- `trader/signal_checker.go` - Signal processing logs
- `api/utils.go` - API error messages
- `logger/config.go` - Logger configuration messages
- `cmd/` - All CLI command output
- `database/` - Database operation logs
- `exchange/` - Exchange connection logs

**Focus areas:**
- `log.Info()`, `log.Error()`, `log.Warn()` statements
- `fmt.Printf()`, `fmt.Println()` terminal output
- Error messages in `errors.New()` and `fmt.Errorf()`
- Panic messages and stack traces

#### 1.2 TypeScript/React Logs (Priority: HIGH)
**Files to translate:** ~64 TypeScript/React files
**Estimated time:** 10-15 hours

**Key files:**
- `web/src/services/` - API service error messages
- `web/src/utils/logger.ts` - Logging utility messages
- `web/src/hooks/` - React hooks error messages
- `web/src/components/` - Component error states
- Console.log, console.error, console.warn statements

**Focus areas:**
- `console.log()`, `console.error()`, `console.warn()` statements
- `throw new Error()` messages
- Toast/notification messages (user-visible errors)
- Debug messages

---

### 🟡 **Phase 2: Codebase Translation (MEDIUM PRIORITY)**

**Goal:** Translate code comments, documentation strings, and inline explanations

#### 2.1 Go Codebase Comments (Priority: MEDIUM)
**Files to translate:** ~56 Go files
**Estimated time:** 25-35 hours

**Focus areas:**
- Function documentation comments
- Struct field comments
- Complex logic explanations
- TODO/FIXME comments
- Package documentation

**Key modules:**
- `trader/` - Core trading logic (12 files)
- `exchange/` - Exchange integrations (8 files)
- `database/` - Database models (6 files)
- `api/` - API handlers (10 files)
- `strategy/` - Trading strategies (5 files)

#### 2.2 TypeScript/React Codebase Comments (Priority: MEDIUM)
**Files to translate:** ~64 TypeScript/React files
**Estimated time:** 15-20 hours

**Focus areas:**
- JSDoc comments
- Interface/Type documentation
- Component prop descriptions
- Function explanations
- TODO/FIXME comments

**Key directories:**
- `web/src/components/` - React components
- `web/src/services/` - API services
- `web/src/hooks/` - Custom React hooks
- `web/src/utils/` - Utility functions
- `web/src/types/` - TypeScript type definitions

---

### 🟢 **Phase 3: UI Translation (LOW PRIORITY)**

**Goal:** Ensure all UI strings have proper English translations

#### 3.1 Translation Dictionary QA (Priority: LOW)
**File:** `web/src/i18n/translations.ts` (1,660 lines)
**Estimated time:** 2-4 hours

**Status:** Already has English-Chinese pairs, needs QA review

**Tasks:**
- Verify all English translations are accurate
- Check for missing translations
- Ensure consistency in terminology
- Fix any grammatical issues

#### 3.2 Hardcoded UI Strings (Priority: LOW)
**Files to check:** UI components
**Estimated time:** 5-10 hours

**Tasks:**
- Find hardcoded Chinese strings in components
- Move to translation dictionary
- Ensure all user-facing text uses i18n

**Note:** This is lowest priority since Chrome browser can auto-translate the UI.

---

## Out of Scope

### ❌ NOT Translating (Per Requirements)

1. **Prompts** - Keep as-is, no translation needed
   - AI/LLM prompt templates
   - System prompts
   - Trading strategy prompts

2. **Documentation** (Defer to later)
   - `CHANGELOG.zh-CN.md`
   - `docs/prompt-guide.zh-CN.md`
   - Chinese-specific documentation files
   - Can be handled separately as content translation project

---

## Implementation Strategy

### Translation Workflow
1. **Identify** - Use grep to find Chinese characters in target files
2. **Translate** - Convert Chinese to English while preserving:
   - Technical accuracy
   - Context and meaning
   - Code functionality
3. **Test** - Ensure no functionality breaks
4. **Commit** - Commit changes in logical groups
5. **Review** - Code review for translation quality

### Quality Guidelines
- **Accuracy:** Preserve technical meaning
- **Consistency:** Use consistent terminology across codebase
- **Clarity:** Prefer clear, professional English
- **Context:** Maintain context from original Chinese
- **Testing:** Verify no functionality breaks

### Terminology Standards
Create and maintain a glossary for consistent translation:
- Trading terms (e.g., 开仓 → "open position", 平仓 → "close position")
- System terms (e.g., 配置 → "configuration", 日志 → "log")
- UI terms (e.g., 按钮 → "button", 确认 → "confirm")

---

## Progress Tracking

### Phase 1: Logs and Terminal Messages
- [ ] Go backend logs (0/56 files)
- [ ] TypeScript/React logs (0/64 files)

### Phase 2: Codebase Translation
- [ ] Go codebase comments (0/56 files)
- [ ] TypeScript codebase comments (0/64 files)

### Phase 3: UI Translation
- [ ] Translation dictionary QA
- [ ] Hardcoded UI strings

---

## File Inventory

### Phase 1 Priority Files (Logs & Terminal Messages)

#### Go Files with Logs (56 files)
```
main.go
trader/auto_trader.go
trader/signal_checker.go
trader/position_manager.go
api/utils.go
logger/config.go
cmd/root.go
cmd/start.go
database/db.go
exchange/binance.go
exchange/okx.go
[... see detailed file list in analysis]
```

#### TypeScript Files with Logs (64 files)
```
web/src/services/api.ts
web/src/utils/logger.ts
web/src/hooks/useTrader.ts
web/src/components/TraderDashboard.tsx
[... see detailed file list in analysis]
```

---

## Success Criteria

### Phase 1 Complete When:
- ✅ All console/terminal output is in English
- ✅ All log messages are in English
- ✅ All error messages shown to developers are in English
- ✅ Application runs without issues
- ✅ Logs are still informative and clear

### Phase 2 Complete When:
- ✅ All code comments are in English
- ✅ Function/class documentation is in English
- ✅ Code is readable for English-speaking developers
- ✅ Technical accuracy is preserved

### Phase 3 Complete When:
- ✅ Translation dictionary is reviewed and accurate
- ✅ No hardcoded Chinese strings in UI
- ✅ UI displays correctly in English

---

## Notes

- **Prompts:** No translation needed (per requirements)
- **UI:** Lowest priority - Chrome can auto-translate
- **Focus:** Developer experience first (logs, code comments)
- **Testing:** Run application after each phase to ensure nothing breaks
- **Commits:** Make atomic commits per file or logical group
- **Branch:** `claude/translate-repo-chinese-to-english-01UidL5odAorN6X6md1wvmt4`

---

## Next Steps

1. ✅ Review and approve this plan
2. ⏳ Start Phase 1.1: Go backend logs translation
3. ⏳ Start Phase 1.2: TypeScript/React logs translation
4. ⏳ Test and verify Phase 1 completion
5. ⏳ Begin Phase 2: Codebase translation
6. ⏳ Complete Phase 3: UI translation QA

---

**Ready to begin Phase 1!** 🚀
