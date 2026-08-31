#!/bin/sh
# Stickguy closed-test installer - UNSIGNED channel.
#
# This is deliberately NOT install/install.sh from the repository. That one is
# the production channel and refuses to run without Apple notarization and the
# Ed25519 update trust anchors. This channel verifies a pinned SHA-256 of a
# build from a known commit, so install it only from someone you trust.
#
#   curl -fsSL __ORIGIN__/install.sh | sh
set -eu

origin='__ORIGIN__'
expected_sha='__APP_SHA256__'

if [ "$(uname -s)" != Darwin ]; then
  echo "Stickguy is only validated on macOS." >&2
  exit 1
fi
if [ "$(uname -m)" != arm64 ]; then
  echo "Stickguy is only validated on Apple Silicon Macs." >&2
  exit 1
fi

temporary="$(mktemp -d "${TMPDIR:-/tmp}/stickguy-install.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

echo "Downloading Stickguy..."
/usr/bin/curl --fail --silent --show-error --location "$origin/Stickguy.zip" --output "$temporary/Stickguy.zip"
actual_sha="$(/usr/bin/shasum -a 256 "$temporary/Stickguy.zip" | /usr/bin/awk '{print $1}')"
if [ "$actual_sha" != "$expected_sha" ]; then
  echo "Checksum verification failed: expected $expected_sha, got $actual_sha" >&2
  exit 1
fi

/usr/bin/ditto -x -k "$temporary/Stickguy.zip" "$temporary/extracted"
app="$temporary/extracted/Stickguy.app"
if [ ! -d "$app" ] || [ ! -x "$app/Contents/MacOS/stickguy-desktop" ]; then
  echo "The downloaded archive is not a usable Stickguy app bundle." >&2
  exit 1
fi
# Ad-hoc signed, not notarized. --verify still proves the bundle was not
# modified or truncated in transit, which is what this check is for.
/usr/bin/codesign --verify --strict "$app" 2>/dev/null || {
  echo "The downloaded app bundle failed its own signature check." >&2
  exit 1
}

# Prefer /Applications, but never fail the install just because this account
# cannot write there.
destination_directory="/Applications"
[ -w "$destination_directory" ] || destination_directory="$HOME/Applications"
/bin/mkdir -p "$destination_directory"
destination="$destination_directory/Stickguy.app"

if [ -d "$destination" ]; then
  echo "Replacing the existing install at $destination ..."
  /usr/bin/osascript -e 'tell application "System Events" to if exists (application process "Stickguy") then tell application "Stickguy" to quit' >/dev/null 2>&1 || true
  /bin/rm -rf "$destination"
fi
/bin/mv "$app" "$destination"
# curl does not set the quarantine attribute, but a re-download through a
# browser would; clearing it keeps the app openable either way.
/usr/bin/xattr -dr com.apple.quarantine "$destination" 2>/dev/null || true

echo "Installed $destination"
echo "Opening Stickguy - first run walks you through connecting a repository."
/usr/bin/open "$destination"
echo
echo "In the app:"
echo "  * Create Project - to test on a repository of your own, or"
echo "  * Join a Project - paste the invite code you were sent"
echo "Then pick the repository, tick the coding agents you use, and connect."
echo "Restart that agent afterwards; sessions already open are not observed."
