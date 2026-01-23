# Changelog

## v0.2.3 - 2026-01-22

### Fixed

- Cleanup minor code smells (unused variable, gitignore pattern) (88d9272)

### Added

- `llms.txt` for LLM agent consumption (117dcec)

## v0.2.2 - 2026-01-22

### Fixed

- Install prompts/agents into empty directories (314ad3b)

### Added

- Copy default prompts on first run (5cd13e6)
- Tests for `determineMode`, `checkClaudeDep`, `preparePlanFile`, `createRunner` (b403eb1)

## v0.2.1 - 2026-01-21

### Fixed

- Increase bufio.Scanner buffer to 16MB for large outputs (#12)
- Preserve untracked files during branch checkout (#11)
- Support git worktrees (#10)
- Add early dirty worktree check before branch operations (#9)

### Removed

- Docker support (#13)

## v0.2.0 - 2026-01-21

### Added

- Configurable colors (#7)
- Scalar config fallback to embedded defaults (#8)

## v0.1.0 - 2026-01-21

Initial release of ralphex - autonomous plan execution with Claude Code.

### Added

- Autonomous task execution with fresh context per task
- Multi-phase code review pipeline (5 agents → Codex → 2 agents)
- Custom review agents with `{{agent:name}}` template system
- Automatic git branch creation from plan filename
- Automatic commits after each task and review fix
- Plan completion tracking (moves to `completed/` folder)
- Streaming output with timestamps and colors
- Multiple execution modes: full, review-only, codex-only
- Zero configuration required - works out of the box
