# codex-as-gemini

Wraps the Codex CLI to produce Gemini-compatible `stream-json` output, allowing Codex to be used as a drop-in replacement for Gemini CLI in ralphex task and review phases.

## How it works

The script translates Codex JSONL events into Gemini's `stream-json` format that ralphex's `GeminiExecutor` can parse. It extracts the prompt from `-p` flag and ignores all other Gemini-specific flags gracefully.

Event mapping:

| Codex event | Gemini event | Notes |
|---|---|---|
| `item.completed` (agent_message) | `content_block_delta` (text_delta) | Always emitted |
| `item.completed` (command_execution) | `content_block_delta` (text_delta) | Only when `CODEX_VERBOSE=1` |
| `turn.completed` | `result` | End of execution |
| Other events | Skipped | |

## Configuration

Add to `~/.config/ralphex/config` or `.ralphex/config`:

```ini
gemini_command = /path/to/scripts/codex-as-gemini/codex-as-gemini.sh
gemini_args =
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `CODEX_MODEL` | (codex default) | Model to use |
| `CODEX_SANDBOX` | `danger-full-access` | Sandbox mode |
| `CODEX_VERBOSE` | `0` | Set to `1` to include command execution output |

## Requirements

- `codex` CLI installed and configured
- `jq` for JSON translation
