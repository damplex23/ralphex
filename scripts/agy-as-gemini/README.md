# agy-as-gemini

Antigravity CLI wrapper for ralphex, allowing `agy` to replace Gemini CLI in task and review phases.

## Scripts

### agy-as-gemini.sh

Wraps `agy` CLI to produce Gemini-compatible stream-json output. Acts as a drop-in replacement for `gemini` in task and review phases. Since Antigravity outputs plain text, this script wraps each line in a `content_block_delta` JSON event.

Additionally, to prevent deadlock/recursion loop issues when running the wrapper inside an active Antigravity agent process, the wrapper unsets **every** `ANTIGRAVITY_*` environment variable (prefix-wide cleanup via `unset ${!ANTIGRAVITY_@}`, not a fixed list) before invoking `agy`. This is intentional and means new Antigravity-managed vars are cleaned automatically without wrapper updates.

## Compatibility

Tested with `agy` 1.0.2. The wrapper requires three `agy` flags to be present:
- `--dangerously-skip-permissions` — auto-approve tool/command permissions for unattended runs
- `--print-timeout` — override agy's 5-minute print-mode default
- `-p` / `--print` / `--prompt` — non-interactive single-prompt mode

Model selection is **not** exposed (no `AGY_MODEL` env var) — `agy` 1.0.2 has no `--model` flag.

**Configuration** (`~/.config/ralphex/config` or `.ralphex/config`):

```ini
gemini_command = /path/to/scripts/agy-as-gemini/agy-as-gemini.sh
gemini_args =
```

## Testing

```bash
bash scripts/agy-as-gemini/agy-as-gemini_test.sh
```

## Requirements

- `agy` (Antigravity) CLI installed and accessible in the system PATH
- `jq` for JSON translation
