#!/bin/sh
set -eu

root="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/overgent-shell-test.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

sh -n "$root/install/install.sh"
sh -n "$root/install/uninstall.sh"
awk '/^cat > .*manifest.awk.*AWK/{copy=1;next} copy && /^AWK$/{exit} copy{print}' \
  "$root/install/install.sh" > "$temporary/manifest.awk"
[ -s "$temporary/manifest.awk" ]

for awk_bin in awk mawk gawk nawk; do
  if command -v "$awk_bin" >/dev/null 2>&1; then
    "$root/install/test/parser-cases.sh" "$(command -v "$awk_bin")" "$temporary/manifest.awk"
  fi
done

if OVERGENT_MANIFEST_URL=https://example.invalid sh "$root/install/install.sh" >"$temporary/output" 2>&1; then
  echo "source installer unexpectedly accepted unrendered trust anchors" >&2
  exit 1
fi
grep -q "source template has no production trust anchors" "$temporary/output"

echo "installer shell tests passed"
