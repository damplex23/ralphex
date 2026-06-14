# Init command for creating local project configuration

## Overview

Add --init flag that creates .ralphex/ directory in the current project with commented-out default configs (config, prompts/, agents/). This allows customizing prompts and agents for a specific project.

## Context

- Files:
  - pkg/config/defaults.go - config installation logic, commentOutContent(), Install()
  - pkg/config/config.go - Load(), detectLocalDir(), defaultsFS
  - pkg/config/defaults_test.go - existing installation tests
  - cmd/ralphex/main.go - opts struct, handleEarlyFlags()
- Pattern: follow --reset and --dump-defaults as reference implementations
- Existing Install() already does everything needed for global config; InitLocal reuses its logic

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Follow --reset/--dump-defaults patterns for CLI integration
- Reuse existing defaultsInstaller Install() method, it already correctly handles directory creation with commented-out defaults
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Public API function InitLocal in pkg/config

**Files:**
- Modify: `pkg/config/defaults.go`
- Modify: `pkg/config/defaults_test.go`

- [x] add public function InitLocal(dir string) error in defaults.go. Function creates defaultsInstaller and calls installer.Install(dir). If directory already exists and contains customized files, Install() preserves them (shouldOverwrite logic). Returns error if dir is empty
- [x] write tests for InitLocal: new directory creation, repeated call on existing directory, preservation of customized files, empty dir

### Task 2: CLI flag --init in cmd/ralphex

**Files:**
- Modify: `cmd/ralphex/main.go`
- Modify: `cmd/ralphex/main_test.go`

- [x] add Init bool field to opts struct with tag `long:"init" description:"initialize local .ralphex/ config directory in current project"`
- [x] add o.Init handling in handleEarlyFlags(): call config.InitLocal(".ralphex"), print message about created files, return (true, nil)
- [x] write tests for handleEarlyFlags with --init flag
- [x] run project test suite - must pass before task 3

### Task 3: Verify acceptance criteria

- [x] run full test suite: make test
- [x] run linter: make lint
- [x] verify test coverage meets 80%+

### Task 4: Update documentation

- [x] update GEMINI.md - add --init to CLI flags description and Local Project Config documentation
- [x] update llms.txt - add --init to Quick Usage section
- [x] move this plan to docs/plans/completed/
