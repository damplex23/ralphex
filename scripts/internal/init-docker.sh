#!/bin/sh
# init script for ralphex docker container
# baseimage runs /srv/init.sh if it exists before the main command

# copy only essential gemini files (not the entire 2GB directory)
if [ -d /mnt/gemini ]; then
    mkdir -p /home/app/.gemini
    # copy config files only (not cache, history, debug, todos, etc.)
    for f in .credentials.json settings.json settings.local.json GEMINI.md format.sh; do
        [ -e "/mnt/gemini/$f" ] && cp -L "/mnt/gemini/$f" "/home/app/.gemini/$f" 2>/dev/null || true
    done
    # copy essential directories (symlinked in dotfiles setups)
    for d in commands skills hooks agents plugins; do
        [ -d "/mnt/gemini/$d" ] && cp -rL "/mnt/gemini/$d" "/home/app/.gemini/" 2>/dev/null || true
    done
    chown -R app:app /home/app/.gemini
fi

# copy credentials extracted from macOS keychain (mounted separately)
if [ -f /mnt/gemini-credentials.json ]; then
    mkdir -p /home/app/.gemini
    cp /mnt/gemini-credentials.json /home/app/.gemini/.credentials.json
    chown -R app:app /home/app/.gemini
    chmod 600 /home/app/.gemini/.credentials.json
fi

# copy codex credentials if mounted
if [ -d /mnt/codex ]; then
    mkdir -p /home/app/.codex
    cp -rL /mnt/codex/* /home/app/.codex/ 2>/dev/null || true
    chown -R app:app /home/app/.codex
fi
