#!/bin/sh
set -eu

binary="${OVERGENT_BINARY:-$HOME/.local/bin/overgent}"
bindings_removed=true
if [ -x "$binary" ]; then
  if ! "$binary" setup remove-all; then
    bindings_removed=false
  fi
  "$binary" service remove || true
fi
rm -f "$binary" "$binary.previous"

if [ "${1:-}" = "--purge-local-state" ]; then
  state="$HOME/Library/Application Support/Overgent"
  # A local Project's coordination history lives in the bundled backend's
  # database under this directory, so it travels with the rest of the state
  # into the Trash rather than being deleted outright.
  if [ -d "$state/backend" ] && [ -x "$binary" ]; then
    "$binary" backend stop >/dev/null 2>&1 || true
  fi
  if [ -d "$state" ]; then
    stamp="$(date +%Y%m%d%H%M%S)"
    trash="$HOME/.Trash/Overgent-state-$stamp"
    mv "$state" "$trash"
    echo "Local Overgent state moved to $trash (recoverable from Trash)."
  fi
  # The backend's instance secret and deployment secrets key are Keychain
  # items, not files, so moving the state directory does not remove them. They
  # are useless without the database that just went to the Trash, and leaving
  # named credentials behind after an uninstall is its own problem.
  for account in $(/usr/bin/security dump-keychain 2>/dev/null | /usr/bin/sed -n 's/.*"acct"<blob>="\(overgent\.local-backend\.[^"]*\)".*/\1/p' | /usr/bin/sort -u); do
    /usr/bin/security delete-generic-password -s com.overgent.comice -a "$account" >/dev/null 2>&1 || true
  done
else
  echo "Overgent removed. Local state and Keychain credentials were preserved."
  echo "Run this script with --purge-local-state only if you also want recoverable local-state removal."
fi
if [ "$bindings_removed" = false ]; then
  echo "One or more managed agent bindings had drifted and were left untouched for safety. Review them with Overgent before deleting the preserved state." >&2
fi
