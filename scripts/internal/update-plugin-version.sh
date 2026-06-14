#!/usr/bin/env bash
set -euo pipefail

# Extract version from git tag (removes 'v' prefix)
VERSION="${1#v}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

# Update plugin.json
if [ -f ".gemini-plugin/plugin.json" ]; then
  # Use jq if available, otherwise sed
  if command -v jq &> /dev/null; then
    jq --arg v "$VERSION" '.version = $v' .gemini-plugin/plugin.json > .gemini-plugin/plugin.json.tmp
    mv .gemini-plugin/plugin.json.tmp .gemini-plugin/plugin.json
  else
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" .gemini-plugin/plugin.json
    rm .gemini-plugin/plugin.json.bak
  fi
  echo "Updated plugin.json to version $VERSION"
fi

# Update marketplace.json
if [ -f ".gemini-plugin/marketplace.json" ]; then
  if command -v jq &> /dev/null; then
    jq --arg v "$VERSION" '.plugins[0].version = $v' .gemini-plugin/marketplace.json > .gemini-plugin/marketplace.json.tmp
    mv .gemini-plugin/marketplace.json.tmp .gemini-plugin/marketplace.json
  else
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" .gemini-plugin/marketplace.json
    rm .gemini-plugin/marketplace.json.bak
  fi
  echo "Updated marketplace.json to version $VERSION"
fi
