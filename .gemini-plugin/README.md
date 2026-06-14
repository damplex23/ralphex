# ralphex Gemini CLI Plugin

This directory contains the Gemini CLI plugin configuration for ralphex.

## Files

- `plugin.json` - Plugin manifest with metadata and version
- `marketplace.json` - Marketplace catalog for single-plugin distribution

## Installation

Users can install via the plugin marketplace:

```bash
/plugin marketplace add umputun/ralphex
/plugin install ralphex@ralphex
```

## Versioning

The `version` field in both JSON files is automatically updated during releases by `scripts/internal/update-plugin-version.sh`, triggered by goreleaser.

## Marketplace Structure

This repository serves as both:
1. The ralphex CLI tool source code
2. A single-plugin Gemini CLI marketplace

The marketplace references `./` as the plugin source. Plugin skills are located in `assets/gemini/skills/`, keeping all Gemini CLI related files organized together.
