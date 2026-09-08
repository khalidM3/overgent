#!/bin/sh
# shellcheck disable=SC2016 # The literal backticks are malicious test data.
set -eu

awk_bin=$1
parser=$2
temporary="$(mktemp -d "${TMPDIR:-/tmp}/overgent-parser-test.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
sha='2f0a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708'
url='https://example.invalid/overgent_v0.1.0_linux_amd64.tar.gz'
pass=0
fail=0

manifest() {
  printf '{"schemaVersion":1,"version":"v0.1.0","publishedAt":"2026-01-01T00:00:00Z","assets":{%s},"signature":"AAAA"}' "$1"
}
asset() {
  printf '"%s":{"url":"%s","sha256":"%s","size":%s}' "$1" "$2" "$3" "$4"
}
expect_ok() {
  name=$1
  body=$2
  expected=$3
  printf '%s' "$body" > "$temporary/manifest.json"
  if output="$($awk_bin -v platform=linux_amd64 -f "$parser" "$temporary/manifest.json" 2>"$temporary/error")" &&
     [ "$(printf '%s\n' "$output" | sed -n '1,4p')" = "$expected" ]; then
    pass=$((pass + 1)); printf 'ok   %s\n' "$name"
  else
    fail=$((fail + 1)); printf 'FAIL %s\n%s\n%s\n' "$name" "$output" "$(cat "$temporary/error")"
  fi
}
expect_reject() {
  name=$1
  body=$2
  printf '%s' "$body" > "$temporary/manifest.json"
  if $awk_bin -v platform=linux_amd64 -f "$parser" "$temporary/manifest.json" >"$temporary/output" 2>"$temporary/error"; then
    fail=$((fail + 1)); printf 'FAIL %s: accepted invalid input\n' "$name"
  else
    pass=$((pass + 1)); printf 'ok   %s\n' "$name"
  fi
}

selected="$(asset linux_amd64 "$url" "$sha" 7)"
expected="$url
$sha
7
v0.1.0"
expect_ok valid "$(manifest "$selected")" "$expected"
expect_ok sorted-map-canonicalisation "$(manifest "$(asset windows_amd64 https://example.invalid/w.zip "$sha" 8),$selected,$(asset darwin_arm64 https://example.invalid/d.tar.gz "$sha" 9)")" "$expected"
expect_ok escaped-solidus "$(manifest "$(asset linux_amd64 'https:\/\/example.invalid\/overgent.tar.gz' "$sha" 7)")" "https://example.invalid/overgent.tar.gz
$sha
7
v0.1.0"

expect_reject missing-platform "$(manifest "$(asset darwin_arm64 https://example.invalid/d.tar.gz "$sha" 7)")"
expect_reject wrong-schema '{"schemaVersion":2,"version":"v0.1.0","publishedAt":"2026-01-01T00:00:00Z","assets":{},"signature":"AAAA"}'
expect_reject missing-version '{"schemaVersion":1,"publishedAt":"2026-01-01T00:00:00Z","assets":{},"signature":"AAAA"}'
expect_reject bad-published-at '{"schemaVersion":1,"version":"v0.1.0","publishedAt":"yesterday","assets":{},"signature":"AAAA"}'
expect_reject missing-signature '{"schemaVersion":1,"version":"v0.1.0","publishedAt":"2026-01-01T00:00:00Z","assets":{}}'
expect_reject unknown-top-level '{"schemaVersion":1,"version":"v0.1.0","publishedAt":"2026-01-01T00:00:00Z","assets":{},"signature":"AAAA","extra":true}'
expect_reject unknown-asset-field "$(manifest "\"linux_amd64\":{\"url\":\"$url\",\"sha256\":\"$sha\",\"size\":7,\"extra\":true}")"
expect_reject duplicate-top-level '{"schemaVersion":1,"schemaVersion":1,"version":"v0.1.0","publishedAt":"2026-01-01T00:00:00Z","assets":{},"signature":"AAAA"}'
expect_reject duplicate-asset "$(manifest "$selected,$selected")"
expect_reject http-url "$(manifest "$(asset linux_amd64 http://example.invalid/x "$sha" 7)")"
expect_reject shell-character-url "$(manifest "$(asset linux_amd64 'https://example.invalid/`id`' "$sha" 7)")"
expect_reject uppercase-checksum "$(manifest "$(asset linux_amd64 "$url" "$(printf '%s' "$sha" | tr '[:lower:]' '[:upper:]')" 7)")"
expect_reject short-checksum "$(manifest "$(asset linux_amd64 "$url" "${sha%?}" 7)")"
expect_reject zero-size "$(manifest "$(asset linux_amd64 "$url" "$sha" 0)")"
expect_reject fractional-size "$(manifest "$(asset linux_amd64 "$url" "$sha" 7.5)")"
expect_reject oversized "$(manifest "$(asset linux_amd64 "$url" "$sha" 262144001)")"
expect_reject trailing-content "$(manifest "$selected"){}"
expect_reject truncated '{"schemaVersion":1,"version":"v0.1.0"'
expect_reject raw-newline "$(manifest "$(asset linux_amd64 'https://example.invalid/a
b' "$sha" 7)")"

printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
