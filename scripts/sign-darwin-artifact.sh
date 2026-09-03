#!/bin/sh
set -eu

target_os="$1"
artifact="$2"
if [ "$target_os" != "darwin" ]; then exit 0; fi
if [ -z "${OVERGENT_CODESIGN_IDENTITY:-}" ]; then
  echo "OVERGENT_CODESIGN_IDENTITY is required for Darwin release artifacts" >&2
  exit 1
fi
/usr/bin/codesign --force --options runtime --timestamp --sign "$OVERGENT_CODESIGN_IDENTITY" "$artifact"
/usr/bin/codesign --verify --strict --verbose=2 "$artifact"
