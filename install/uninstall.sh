#!/bin/sh
set -eu

binary="${STICKGUY_BINARY:-$HOME/.local/bin/stickguy}"
if [ -x "$binary" ]; then
  "$binary" service remove || true
fi
rm -f "$binary" "$binary.previous"

if [ "${1:-}" = "--purge-local-state" ]; then
  state="$HOME/Library/Application Support/Stickguy"
  if [ -d "$state" ]; then
    stamp="$(date +%Y%m%d%H%M%S)"
    trash="$HOME/.Trash/Stickguy-state-$stamp"
    mv "$state" "$trash"
    echo "Local Stickguy state moved to $trash (recoverable from Trash)."
  fi
else
  echo "Stickguy removed. Local state and Keychain credentials were preserved."
  echo "Run this script with --purge-local-state only if you also want recoverable local-state removal."
fi
