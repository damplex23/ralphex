# gemini-as-gemini

Gemini CLI wrapper for ralphex, allowing Gemini to replace Gemini CLI in task and review phases.

## Scripts

### gemini-as-gemini.sh

Wraps Gemini CLI to produce Gemini-compatible stream-json output. Acts as a drop-in replacement for `gemini` in task and review phases. Since Gemini outputs plain text, this script wraps each line in a `content_block_delta` JSON event.

**Configuration** (`~/.config/ralphex/config` or `.ralphex/config`):

```ini
gemini_command = /path/to/scripts/gemini-as-gemini/gemini-as-gemini.sh
gemini_args =
```

**Environment variables:**

- `GEMINI_MODEL` — model to use (passed as `--model` flag when set)

## Testing

```bash
bash scripts/gemini-as-gemini/gemini-as-gemini_test.sh
```

## Requirements

- `gemini` CLI installed and accessible
- `jq` for JSON translation

## Troubleshooting

### YOLO mode is disabled

If you see an error like:
```
[26-03-13 21:44:54] YOLO mode is disabled by the "disableYolo" setting.
[26-03-13 21:44:54] YOLO mode is disabled by your administrator. To enable it, please request an update to the settings at: https://goo.gle/manage-gemini-cli
```
The right way to fix it is:
1. Run `gemini` to start the interactive session.
2. Type `/settings` and press Enter.
3. Look for a **Security** or **General** section.
4. Toggle the **"Disable YOLO mode"** setting if it is visible there.
