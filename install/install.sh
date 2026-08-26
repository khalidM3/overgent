#!/bin/sh
set -eu

# These public trust anchors are rendered into the copy attached to a release.
# The source template refuses to install so a repository checkout can never
# silently become a production distribution channel.
update_public_key='__STICKGUY_UPDATE_PUBLIC_KEY__'
apple_team_id='__STICKGUY_APPLE_TEAM_ID__'
manifest_url="${STICKGUY_MANIFEST_URL:-https://github.com/stickguy/stickguy/releases/latest/download/update-manifest.json}"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "Stickguy beta is currently qualified only on macOS. Linux and Windows archives are build artifacts, not supported installs." >&2
  exit 1
fi
if [ "$update_public_key" = '__STICKGUY_UPDATE_PUBLIC_KEY__' ] || [ "$apple_team_id" = '__STICKGUY_APPLE_TEAM_ID__' ]; then
  echo "Use install.sh attached to a signed Stickguy release; this source template has no production trust anchors." >&2
  exit 1
fi

case "$(uname -m)" in
  arm64) platform='darwin_arm64' ;;
  *) echo "Stickguy beta is currently qualified only on Apple Silicon Macs." >&2; exit 1 ;;
esac

temporary="$(mktemp -d "${TMPDIR:-/tmp}/stickguy-install.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
/usr/bin/curl --fail --silent --show-error --location "$manifest_url" --output "$temporary/manifest.json"

# JXA is part of supported macOS and gives us strict JSON parsing without asking
# the user to install jq, Python, Node, or another runtime.
asset_json="$(/usr/bin/osascript -l JavaScript - "$temporary/manifest.json" "$platform" <<'JXA'
ObjC.import('Foundation')
const args = $.NSProcessInfo.processInfo.arguments.js.slice(4)
const data = $.NSData.dataWithContentsOfFile(args[0])
if (!data) throw new Error('cannot read update manifest')
const object = $.NSJSONSerialization.JSONObjectWithDataOptionsError(data, 0, null).js
if (object.schemaVersion !== 1 || !object.assets || !object.assets[args[1]]) throw new Error('release does not support this Mac')
const asset = object.assets[args[1]]
JSON.stringify({url: asset.url, sha256: asset.sha256, size: asset.size})
JXA
)"
asset_url="$(printf '%s' "$asset_json" | /usr/bin/sed -E 's/.*"url":"([^"]+)".*/\1/')"
asset_sha="$(printf '%s' "$asset_json" | /usr/bin/sed -E 's/.*"sha256":"([a-f0-9]{64})".*/\1/')"
asset_size="$(printf '%s' "$asset_json" | /usr/bin/sed -E 's/.*"size":([0-9]+).*/\1/')"
case "$asset_url" in https://*) ;; *) echo "Release asset URL is not HTTPS." >&2; exit 1 ;; esac
case "$asset_sha" in *[!a-f0-9]*|'') echo "Release checksum is invalid." >&2; exit 1 ;; esac

/usr/bin/curl --fail --silent --show-error --location "$asset_url" --output "$temporary/archive.tar.gz"
actual_size="$(/usr/bin/stat -f '%z' "$temporary/archive.tar.gz")"
[ "$actual_size" = "$asset_size" ] || { echo "Release size verification failed." >&2; exit 1; }
actual_sha="$(/usr/bin/shasum -a 256 "$temporary/archive.tar.gz" | /usr/bin/awk '{print $1}')"
[ "$actual_sha" = "$asset_sha" ] || { echo "Release checksum verification failed." >&2; exit 1; }

/usr/bin/tar -xzf "$temporary/archive.tar.gz" -C "$temporary" stickguy
/usr/bin/codesign --verify --strict --verbose=2 "$temporary/stickguy"
observed_team="$(/usr/bin/codesign -dvv "$temporary/stickguy" 2>&1 | /usr/bin/sed -n 's/^TeamIdentifier=//p')"
[ "$observed_team" = "$apple_team_id" ] || { echo "Release signer identity verification failed." >&2; exit 1; }

destination="$HOME/.local/bin"
/bin/mkdir -p "$destination"
/bin/chmod 700 "$destination"
/bin/chmod 755 "$temporary/stickguy"
/bin/mv "$temporary/stickguy" "$destination/stickguy"
"$destination/stickguy" service install

echo "Stickguy installed and its per-user service started."
echo "Add $destination to PATH if your shell does not already include it."
