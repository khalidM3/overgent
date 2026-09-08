#!/bin/sh
set -eu

os="$(uname -s)"
case "$os" in
  Darwin|Linux) ;;
  *) echo "Overgent has no uninstaller for $os. macOS and Linux use this script; Windows uses uninstall.ps1." >&2; exit 1 ;;
esac

binary="${OVERGENT_BINARY:-$HOME/.local/bin/overgent}"
purge=false
case "$#:${1:-}" in
  0:) ;;
  1:--purge-local-state) purge=true ;;
  *) echo "Usage: uninstall.sh [--purge-local-state]" >&2; exit 2 ;;
esac

# The profile root the service and the CLI actually use. config.DefaultRoot is
# os.UserConfigDir + "/Overgent", which is "Library/Application Support" on
# macOS and $XDG_CONFIG_HOME (default ~/.config) on Linux. Go ignores a relative
# XDG_CONFIG_HOME, so this ignores one too, or the uninstaller would look
# somewhere the service never wrote.
state_directory() {
  if [ "$os" = Darwin ]; then
    echo "$HOME/Library/Application Support/Overgent"
    return
  fi
  config_home="$HOME/.config"
  case "${XDG_CONFIG_HOME:-}" in /*) config_home="$XDG_CONFIG_HOME" ;; esac
  echo "$config_home/Overgent"
}

# Percent-encode a path for a .trashinfo Path field, per the freedesktop trash
# specification. Bytes are read through od so a home directory with non-ASCII
# characters encodes correctly rather than being mangled.
percent_encode() {
  printf '%s' "$1" | od -An -tu1 -v | tr -s ' ' '\n' | awk 'NF {
    b = $1 + 0
    if ((b >= 48 && b <= 57) || (b >= 65 && b <= 90) || (b >= 97 && b <= 122) ||
        b == 45 || b == 46 || b == 95 || b == 126 || b == 47) printf "%s", sprintf("%c", b)
    else printf "%%%02X", b
  }'
}

# Move the profile root somewhere the member can get it back from. macOS has
# ~/.Trash; Linux has the freedesktop trash, which needs a matching .trashinfo
# or a file manager will not offer to restore it. If neither is reachable the
# state is still moved, never deleted, and the new location is printed.
trash_state() {
  state="$1"
  stamp="$2"
  name="Overgent-state-$stamp"
  if [ "$os" = Darwin ]; then
    trash="$HOME/.Trash/$name"
    mv "$state" "$trash"
    echo "Local Overgent state moved to $trash (recoverable from Trash)."
    return
  fi
  data_home="$HOME/.local/share"
  case "${XDG_DATA_HOME:-}" in /*) data_home="$XDG_DATA_HOME" ;; esac
  trash_root="$data_home/Trash"
  if mkdir -p "$trash_root/files" "$trash_root/info" 2>/dev/null; then
    # The info file is written first: an entry in files/ with no info/ record is
    # an orphan a file manager will not restore, while the reverse is harmless.
    {
      echo "[Trash Info]"
      echo "Path=$(percent_encode "$state")"
      echo "DeletionDate=$(date +%Y-%m-%dT%H:%M:%S)"
    } > "$trash_root/info/$name.trashinfo"
    if mv "$state" "$trash_root/files/$name" 2>/dev/null; then
      echo "Local Overgent state moved to $trash_root/files/$name (recoverable from Trash)."
      return
    fi
    rm -f "$trash_root/info/$name.trashinfo"
  fi
  fallback="$HOME/$name"
  mv "$state" "$fallback"
  echo "The desktop trash was not reachable, so local Overgent state was moved to $fallback instead of being deleted."
}

# The backend's instance secret and deployment secrets key are credential-store
# items, not files, so moving the state directory does not remove them. They are
# useless without the database that just went to the Trash, and leaving named
# credentials behind after an uninstall is its own problem. The hosted device
# credential is deliberately left alone: revoking a device is a separate
# authorized operation.
purge_darwin_credentials() {
  for account in $(/usr/bin/security dump-keychain 2>/dev/null | /usr/bin/sed -n 's/.*"acct"<blob>="\(overgent\.local-backend\.[^"]*\)".*/\1/p' | /usr/bin/sort -u); do
    /usr/bin/security delete-generic-password -s com.overgent.comice -a "$account" >/dev/null 2>&1 || true
  done
}

# Linux keeps the same items in the Secret Service, keyed on the same two
# attributes ("service" and "account"). SearchItems matches attributes exactly
# and cannot match the "overgent.local-backend." prefix, so every item for this
# service is listed and then filtered on its account attribute. busctl is used
# rather than secret-tool because it ships with systemd, which is already
# required for the service unit, whereas libsecret-tools usually is not.
purge_linux_credentials() {
  if ! command -v busctl >/dev/null 2>&1; then
    echo "busctl is not available, so the local backend's keyring items were left in place." >&2
    echo "Remove the 'com.overgent.comice' entries whose account starts with 'overgent.local-backend.' from your keyring by hand." >&2
    return
  fi
  items="$(busctl --user call org.freedesktop.secrets /org/freedesktop/secrets \
      org.freedesktop.Secret.Service SearchItems 'a{ss}' 1 service com.overgent.comice 2>/dev/null \
    | tr ' ' '\n' | sed -n 's|^"\(/org/freedesktop/secrets/[^"]*\)"$|\1|p' || true)"
  if [ -z "$items" ]; then
    return
  fi
  removed=0
  left=0
  for item in $items; do
    attributes="$(busctl --user get-property org.freedesktop.secrets "$item" \
      org.freedesktop.Secret.Item Attributes 2>/dev/null || true)"
    case "$attributes" in
      *'"account" "overgent.local-backend.'*) ;;
      *) continue ;;
    esac
    if busctl --user call org.freedesktop.secrets "$item" \
      org.freedesktop.Secret.Item Delete >/dev/null 2>&1; then
      removed=$((removed + 1))
    else
      left=$((left + 1))
    fi
  done
  [ "$removed" -eq 0 ] || echo "Removed $removed local-backend keyring item(s)."
  if [ "$left" -gt 0 ]; then
    echo "$left local-backend keyring item(s) could not be removed; a locked keyring cannot be changed without an interactive prompt. Unlock it and re-run this script." >&2
  fi
}

state="$(state_directory)"

bindings_removed=true
if [ -x "$binary" ]; then
  if ! "$binary" setup remove-all; then
    bindings_removed=false
  fi
  "$binary" service remove || true
  # A local Project's coordination history lives in the bundled backend's
  # database under this directory, so it travels with the rest of the state
  # into the Trash rather than being deleted outright. Stop it here, while the
  # executable still exists and after the service that supervises it is gone:
  # this used to run after "rm -f $binary", where the [ -x "$binary" ] guard
  # could never be true, so the database was always moved out from under a
  # live backend.
  if [ "$purge" = true ] && [ -d "$state/backend" ]; then
    "$binary" backend stop >/dev/null 2>&1 || true
  fi
fi
rm -f "$binary" "$binary.previous"

if [ "$purge" = true ]; then
  if [ -d "$state" ]; then
    trash_state "$state" "$(date +%Y%m%d%H%M%S)"
  fi
  if [ "$os" = Darwin ]; then
    purge_darwin_credentials
  else
    purge_linux_credentials
  fi
else
  if [ "$os" = Darwin ]; then
    echo "Overgent removed. Local state and Keychain credentials were preserved."
  else
    echo "Overgent removed. Local state and keyring credentials were preserved."
  fi
  echo "Run this script with --purge-local-state only if you also want recoverable local-state removal."
fi
if [ "$bindings_removed" = false ]; then
  echo "One or more managed agent bindings had drifted and were left untouched for safety. Review them with Overgent before deleting the preserved state." >&2
fi
