# Error Pattern Detection Implementation Plan

Related to: #47

## Overview

Add configurable error pattern detection for Gemini and Codex outputs. When patterns like "You've hit your limit" are detected, ralphex gracefully exits with an informative message instead of continuing to loop.

## Context

- **Problem**: When Gemini/Codex hits rate limits or API errors, they don't follow prompt instructions to emit signals. ralphex continues looping, falsely reporting "issues fixed".
- **Solution**: Configurable substring patterns checked after each execution. On match, return structured error and exit gracefully.

## Tasks

### 1. Add Config Fields

**Files:**
- Modify: `pkg/config/values.go`
- Modify: `pkg/config/defaults/config`

- [x] Add `GeminiErrorPatterns []string` to `Values` struct
- [x] Add `CodexErrorPatterns []string` to `Values` struct
- [x] Parse comma-separated patterns from config (trim spaces before/after each pattern)
- [x] Add embedded defaults: Gemini = "You've hit your limit", Codex = "Rate limit,quota exceeded"
- [x] Add tests for pattern parsing including whitespace trimming
- [x] Verify tests pass

### 2. Add Error Type

**Files:**
- Modify: `pkg/executor/executor.go`

- [x] Add `ErrPatternMatch` error type with `Pattern` and `HelpCmd` fields
- [x] Add `checkErrorPatterns(output string, patterns []string) *ErrPatternMatch` helper
- [x] Case-insensitive substring matching
- [x] Add tests for pattern matching
- [x] Verify tests pass

### 3. Integrate in Gemini Executor

**Files:**
- Modify: `pkg/executor/executor.go`

- [x] Add `ErrorPatterns []string` field to `GeminiOptions`
- [x] Check patterns after execution in `Run()`
- [x] Return `ErrPatternMatch` with `HelpCmd: "gemini /usage"` on match
- [x] Add tests for error pattern detection
- [x] Verify tests pass

### 4. Integrate in Codex Executor

**Files:**
- Modify: `pkg/executor/codex.go`

- [x] Add `ErrorPatterns []string` field to `CodexOptions`
- [x] Check patterns after execution in `Run()`
- [x] Return `ErrPatternMatch` with `HelpCmd: "codex /status"` on match
- [x] Add tests for error pattern detection
- [x] Verify tests pass

### 5. Pass Patterns from Config to Executors

**Files:**
- Modify: `pkg/processor/runner.go`

- [x] Pass `cfg.GeminiErrorPatterns` to `GeminiOptions.ErrorPatterns`
- [x] Pass `cfg.CodexErrorPatterns` to `CodexOptions.ErrorPatterns`
- [x] Verify tests pass

### 6. Handle Error in Runner

**Files:**
- Modify: `pkg/processor/runner.go`

- [x] Check for `ErrPatternMatch` after gemini/codex calls
- [x] Log: `error: detected "<pattern>" in <tool> output`
- [x] Log: `run '<help_cmd>' for more information`
- [x] Return error (graceful exit, not panic)
- [x] Add integration test for error pattern flow
- [x] Verify tests pass

### 7. Documentation

**Files:**
- Modify: `README.md`
- Modify: `GEMINI.md`

- [x] Document `gemini_error_patterns` and `codex_error_patterns` config options
- [x] Explain pattern matching behavior (case-insensitive substring, whitespace trimmed)
- [x] List default patterns

### 8. Final Validation

- [x] Run full test suite
- [x] Run linter
- [x] Test manually with simulated error output
- [x] Close #47
- [x] Move plan to `docs/plans/completed/`
