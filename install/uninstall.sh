#!/bin/sh
set -eu

binary="${STICKGUY_BINARY:-$HOME/.local/bin/stickguy}"
bindings_removed=true
if [ -x "$binary" ]; then
  if ! "$binary" setup remove-all; then
    bindings_removed=false
  fi
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
if [ "$bindings_removed" = false ]; then
  echo "One or more managed agent bindings had drifted and were left untouched for safety. Review them with Stickguy before deleting the preserved state." >&2
fi
