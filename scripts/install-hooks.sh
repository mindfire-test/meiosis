#!/usr/bin/env bash

# Create hooks directory if it doesn't exist
mkdir -p .git/hooks

# Copy the commit-msg hook and make it executable
cp scripts/commit-msg .git/hooks/commit-msg
chmod +x .git/hooks/commit-msg

echo "Git hooks successfully installed!"
