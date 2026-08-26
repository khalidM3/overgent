#!/bin/sh
set -eu

target_os="$1"
artifact="$2"
if [ "$target_os" != "darwin" ]; then exit 0; fi
if [ -z "${STICKGUY_CODESIGN_IDENTITY:-}" ]; then
  echo "STICKGUY_CODESIGN_IDENTITY is required for Darwin release artifacts" >&2
  exit 1
fi
/usr/bin/codesign --force --options runtime --timestamp --sign "$STICKGUY_CODESIGN_IDENTITY" "$artifact"
/usr/bin/codesign --verify --strict --verbose=2 "$artifact"
