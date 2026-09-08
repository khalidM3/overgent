#!/bin/sh
# shellcheck disable=SC2016 # Single-quoted $ expressions below are awk programs.
set -eu

# These public trust anchors are rendered into the copy attached to a release.
# The source template refuses to install so a repository checkout can never
# silently become a production distribution channel.
#
update_public_key='__OVERGENT_UPDATE_PUBLIC_KEY__'
apple_team_id='__OVERGENT_APPLE_TEAM_ID__'
manifest_url="${OVERGENT_MANIFEST_URL:-https://releases.overgent.com/current/update-manifest.json}"
backend_manifest_url="${OVERGENT_BACKEND_MANIFEST_URL:-}"
manifest_source=''
archive_source=''
backend_manifest_source=''
backend_archive_source=''
download_only=''
install_backend=true

usage() {
  cat <<'EOF'
Usage: install.sh [options]
  --manifest URL|PATH          signed CLI manifest (default: release channel)
  --archive PATH               prefetched CLI archive
  --backend-manifest URL|PATH  signed backend manifest (default: release channel)
  --backend-archive PATH       prefetched backend archive
  --download-only DIRECTORY    verify and copy artifacts without installing
  --skip-backend               install only the CLI/service; local Projects stay unavailable
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --manifest|--archive|--backend-manifest|--backend-archive|--download-only)
      [ "$#" -ge 2 ] || { echo "$1 requires a value." >&2; exit 2; }
      case "$1" in
        --manifest) manifest_source="$2" ;;
        --archive) archive_source="$2" ;;
        --backend-manifest) backend_manifest_source="$2" ;;
        --backend-archive) backend_archive_source="$2" ;;
        --download-only) download_only="$2" ;;
      esac
      shift 2
      ;;
    --skip-backend) install_backend=false; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown installer option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# Per-OS tool selection. macOS keeps absolute /usr/bin paths so a hostile PATH
# cannot substitute the verification tools; Linux distributions disagree about
# whether these live in /bin or /usr/bin, so they are resolved through PATH and
# then checked for existence.
os="$(uname -s)"
case "$os" in
  Darwin)
    case "$(uname -m)" in
      arm64) platform='darwin_arm64' ;;
      *) echo "Overgent beta is currently qualified only on Apple Silicon Macs." >&2; exit 1 ;;
    esac
    curl_bin='/usr/bin/curl'
    awk_bin='/usr/bin/awk'
    tar_bin='/usr/bin/tar'
    ;;
  Linux)
    case "$(uname -m)" in
      x86_64|amd64) platform='linux_amd64' ;;
      aarch64|arm64) platform='linux_arm64' ;;
      *) echo "Overgent publishes no Linux archive for $(uname -m)." >&2; exit 1 ;;
    esac
    curl_bin="$(command -v curl || true)"
    awk_bin="$(command -v awk || true)"
    tar_bin="$(command -v tar || true)"
    openssl_bin="$(command -v openssl || true)"
    busctl_bin="$(command -v busctl || true)"
    for required in "$curl_bin" "$awk_bin" "$tar_bin" "$openssl_bin" "$busctl_bin"; do
      [ -n "$required" ] || { echo "Overgent's Linux installer needs curl, awk, tar, openssl, and busctl on PATH." >&2; exit 1; }
    done
    command -v sha256sum >/dev/null 2>&1 || { echo "Overgent's installer needs sha256sum (coreutils) to verify the release." >&2; exit 1; }
    ;;
  *)
    echo "Overgent has no installer for $os. Qualified platforms are macOS and Linux; Windows uses install.ps1." >&2
    exit 1
    ;;
esac

# An anchor is unrendered if it still has the placeholder's shape. This is
# deliberately a shape test rather than a comparison against the literal
# placeholder: the release step renders with an unanchored global substitution,
# so a literal here would be rewritten to the real value too and the guard would
# then compare the real key against itself and refuse every rendered install.
# No base64 Ed25519 key or Apple Team ID can match this pattern.
unrendered() {
  case "$1" in __OVERGENT_*__) return 0 ;; *) return 1 ;; esac
}
if unrendered "$update_public_key"; then
  echo "Use install.sh attached to a signed Overgent release; this source template has no production trust anchors." >&2
  exit 1
fi
# The Apple Team ID is the signer anchor for the macOS chain only, so it is
# required exactly where it is used rather than on every platform.
if [ "$os" = Darwin ] && unrendered "$apple_team_id"; then
  echo "Use install.sh attached to a signed Overgent release; this source template has no production trust anchors." >&2
  exit 1
fi

temporary="$(mktemp -d "${TMPDIR:-/tmp}/overgent-install.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

# --proto/--proto-redir pin the whole request chain to HTTPS, so a redirect
# cannot silently downgrade a fetch to cleartext.
fetch() {
  "$curl_bin" --fail --silent --show-error --location \
    --proto '=https' --proto-redir '=https' \
    --max-time 300 "$1" --output "$2"
}

copy_or_fetch() {
  source=$1
  fallback=$2
  destination=$3
  label=$4
  [ -n "$source" ] || source=$fallback
  case "$source" in
    https://*)
      if ! fetch "$source" "$destination"; then
        echo "Could not download the $label." >&2
        echo "On a connected machine use --download-only, copy the verified files here, then pass their paths explicitly." >&2
        exit 1
      fi
      ;;
    http://*|file://*) echo "$label must use HTTPS or be a local path." >&2; exit 1 ;;
    *)
      [ -f "$source" ] || { echo "$label was not found at $source." >&2; exit 1; }
      cp "$source" "$destination"
      ;;
  esac
}

# Strict, bounded JSON. The macOS installer used to parse the manifest with JXA
# (osascript -l JavaScript), which does not exist on Linux; this awk recursive
# descent parser replaces it on both platforms without asking for jq, Python or
# Node. It is a real parser, not a regex scrape: it validates the whole document
# against the JSON grammar, rejects trailing content, rejects duplicate keys,
# bounds depth, node count and document size, and then applies the same
# per-field checks the JXA version relied on the shell to apply. Brace-interval
# regexes are avoided deliberately - the awk shipped with macOS predates them.
cat > "$temporary/manifest.awk" <<'AWK'
BEGIN {
  for (i = 1; i < 32; i++) control[sprintf("%c", i)] = 1
  maxDocument = 1048576
  maxNodes = 20000
  maxDepth = 16
  maxArtifact = 262144000
}
{ document = document $0 "\n" }

function fail(message) {
  print "overgent installer: " message > "/dev/stderr"
  exit 1
}
function skipSpace(   c) {
  while (position <= documentLength) {
    c = substr(document, position, 1)
    if (c == " " || c == "\t" || c == "\n" || c == "\r") position++
    else return
  }
}
function expect(want) {
  skipSpace()
  if (substr(document, position, 1) != want) fail("update manifest is not valid JSON")
  position++
}
function hexValue(text,   i, c, value, digit) {
  value = 0
  for (i = 1; i <= 4; i++) {
    c = tolower(substr(text, i, 1))
    digit = index("0123456789abcdef", c)
    if (digit == 0) fail("update manifest is not valid JSON")
    value = value * 16 + digit - 1
  }
  return value
}
function parseString(   out, c, escape, hex, code) {
  skipSpace()
  if (substr(document, position, 1) != "\"") fail("update manifest is not valid JSON")
  position++
  out = ""
  while (1) {
    if (position > documentLength) fail("update manifest ends inside a string")
    c = substr(document, position, 1)
    position++
    if (c == "\"") return out
    if (c == "\\") {
      escape = substr(document, position, 1)
      position++
      if (escape == "\"") out = out "\""
      else if (escape == "\\") out = out "\\"
      else if (escape == "/") out = out "/"
      else if (escape == "b") out = out sprintf("%c", 8)
      else if (escape == "f") out = out sprintf("%c", 12)
      else if (escape == "n") out = out "\n"
      else if (escape == "r") out = out "\r"
      else if (escape == "t") out = out "\t"
      else if (escape == "u") {
        hex = substr(document, position, 4)
        position += 4
        code = hexValue(hex)
        # Only printable ASCII survives an escape. Everything this installer
        # reads out of the manifest is ASCII by construction, and refusing the
        # rest keeps a control or multi-byte payload out of the shell.
        if (code < 32 || code > 126) fail("update manifest contains an unsupported escape")
        out = out sprintf("%c", code)
      }
      else fail("update manifest contains an invalid escape")
      continue
    }
    if (c in control) fail("update manifest contains a raw control character")
    out = out c
  }
}
function parseNumber(   start, c, text) {
  skipSpace()
  start = position
  while (position <= documentLength) {
    c = substr(document, position, 1)
    if (index("-+.eE0123456789", c) == 0) break
    position++
  }
  text = substr(document, start, position - start)
  if (text !~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$/) fail("update manifest is not valid JSON")
  return text
}
function budget(depth) {
  if (depth > maxDepth) fail("update manifest is nested too deeply")
  nodes++
  if (nodes > maxNodes) fail("update manifest has too many values")
}
# skipValue validates a value this installer does not need, so an unknown field
# is still held to the grammar instead of being skipped by scanning ahead for a
# delimiter.
function skipValue(depth,   c) {
  budget(depth)
  skipSpace()
  c = substr(document, position, 1)
  if (c == "\"") { parseString(); return }
  if (c == "{") {
    position++
    skipSpace()
    if (substr(document, position, 1) == "}") { position++; return }
    while (1) {
      parseString()
      expect(":")
      skipValue(depth + 1)
      skipSpace()
      c = substr(document, position, 1)
      position++
      if (c == ",") continue
      if (c == "}") return
      fail("update manifest is not valid JSON")
    }
  }
  if (c == "[") {
    position++
    skipSpace()
    if (substr(document, position, 1) == "]") { position++; return }
    while (1) {
      skipValue(depth + 1)
      skipSpace()
      c = substr(document, position, 1)
      position++
      if (c == ",") continue
      if (c == "]") return
      fail("update manifest is not valid JSON")
    }
  }
  if (substr(document, position, 4) == "true") { position += 4; return }
  if (substr(document, position, 5) == "false") { position += 5; return }
  if (substr(document, position, 4) == "null") { position += 4; return }
  parseNumber()
}
function parseAsset(assetKey,   c, key, fieldKey) {
  expect("{")
  skipSpace()
  if (substr(document, position, 1) == "}") { position++; return }
  while (1) {
    key = parseString()
    fieldKey = assetKey SUBSEP key
    if (seenAssetField[fieldKey]++) fail("update manifest repeats the asset field \"" key "\"")
    expect(":")
    if (key == "url") assetURL[assetKey] = parseString()
    else if (key == "sha256") assetSHA[assetKey] = parseString()
    else if (key == "size") assetSize[assetKey] = parseNumber()
    else fail("update manifest contains an unknown asset field \"" key "\"")
    skipSpace()
    c = substr(document, position, 1)
    position++
    if (c == ",") continue
    if (c == "}") return
    fail("update manifest is not valid JSON")
  }
}
function parseAssets(   c, key) {
  expect("{")
  skipSpace()
  if (substr(document, position, 1) == "}") { position++; return }
  while (1) {
    key = parseString()
    if (seenAsset[key]++) fail("update manifest repeats the asset key \"" key "\"")
    if (key !~ /^[a-z0-9_]+$/) fail("update manifest contains an invalid asset key")
    assetCount++
    if (assetCount > 12) fail("update manifest publishes too many assets")
    assetKeys[assetCount] = key
    expect(":")
    parseAsset(key)
    if (key == platform) haveAsset = 1
    skipSpace()
    c = substr(document, position, 1)
    position++
    if (c == ",") continue
    if (c == "}") return
    fail("update manifest is not valid JSON")
  }
}
END {
  if (platform == "") fail("no platform was requested")
  documentLength = length(document)
  if (documentLength > maxDocument) fail("update manifest exceeds the 1 MiB limit")
  position = 1
  schemaVersion = ""
  expect("{")
  skipSpace()
  if (substr(document, position, 1) == "}") position++
  else {
    while (1) {
      key = parseString()
      if (seenTop[key]++) fail("update manifest repeats the \"" key "\" field")
      expect(":")
      if (key == "schemaVersion") schemaVersion = parseNumber()
      else if (key == "version") version = parseString()
      else if (key == "publishedAt") publishedAt = parseString()
      else if (key == "assets") { haveAssets = 1; parseAssets() }
      else if (key == "signature") signature = parseString()
      else fail("update manifest contains an unknown top-level field \"" key "\"")
      skipSpace()
      c = substr(document, position, 1)
      position++
      if (c == ",") continue
      if (c == "}") break
      fail("update manifest is not valid JSON")
    }
  }
  skipSpace()
  if (position <= documentLength) fail("update manifest has trailing content")

  if (schemaVersion == "") fail("update manifest declares no schema version")
  if (schemaVersion != "1") fail("update manifest schema version " schemaVersion " is not supported by this installer")
  if (version !~ /^v[0-9][0-9A-Za-z.+-]*$/) fail("update manifest contains an invalid version")
  if (publishedAt !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z$/) fail("update manifest contains an invalid publish time")
  if (signature == "" || signature !~ /^[A-Za-z0-9+\/=]+$/) fail("update manifest contains an invalid signature")
  if (!haveAssets) fail("update manifest publishes no assets")
  if (!haveAsset) fail("this release publishes no asset for " platform)
  for (i = 1; i <= assetCount; i++) {
    key = assetKeys[i]
    if (assetURL[key] == "") fail("release asset has no URL")
    if (length(assetURL[key]) > 2000) fail("release asset URL is too long")
    if (assetURL[key] !~ /^https:\/\/[A-Za-z0-9._~:\/?#@!$&'()*+,;=%-]+$/) fail("release asset URL is not an HTTPS URL")
    if (length(assetSHA[key]) != 64 || assetSHA[key] !~ /^[0-9a-f]+$/) fail("release checksum is not 64 lowercase hex characters")
    if (assetSize[key] !~ /^[1-9][0-9]*$/) fail("release size is not a positive integer")
    if (length(assetSize[key]) > 12 || assetSize[key] + 0 > maxArtifact) fail("release size exceeds the 250 MiB artifact limit")
  }
  for (i = 2; i <= assetCount; i++) {
    value = assetKeys[i]
    j = i - 1
    while (j >= 1 && assetKeys[j] > value) { assetKeys[j + 1] = assetKeys[j]; j-- }
    assetKeys[j + 1] = value
  }
  payload = "{\"schemaVersion\":" schemaVersion ",\"version\":\"" version "\",\"publishedAt\":\"" publishedAt "\",\"assets\":{"
  for (i = 1; i <= assetCount; i++) {
    key = assetKeys[i]
    if (i > 1) payload = payload ","
    payload = payload "\"" key "\":{\"url\":\"" assetURL[key] "\",\"sha256\":\"" assetSHA[key] "\",\"size\":" assetSize[key] "}"
  }
  payload = payload "}}"

  print assetURL[platform]
  print assetSHA[platform]
  print assetSize[platform]
  print version
  print signature
  print payload
}
AWK

verify_manifest() {
  manifest=$1
  output=$2
  "$awk_bin" -v platform="$platform" -f "$temporary/manifest.awk" "$manifest" > "$output"
  if [ "$os" = Linux ]; then
    signature="$(sed -n '5p' "$output")"
    payload="$(sed -n '6p' "$output")"
    printf '\060\052\060\005\006\003\053\145\160\003\041\000' > "$temporary/public.der"
    printf '%s' "$update_public_key" | "$openssl_bin" base64 -d -A >> "$temporary/public.der" 2>/dev/null || {
      echo "Release update public key is invalid." >&2; exit 1;
    }
    [ "$(wc -c < "$temporary/public.der" | tr -d ' ')" = 44 ] || { echo "Release update public key is invalid." >&2; exit 1; }
    printf '%s' "$signature" | "$openssl_bin" base64 -d -A > "$temporary/signature" 2>/dev/null || {
      echo "Release manifest signature is invalid." >&2; exit 1;
    }
    printf '%s' "$payload" > "$temporary/payload"
    if ! "$openssl_bin" pkeyutl -verify -pubin -inkey "$temporary/public.der" -keyform DER \
        -rawin -in "$temporary/payload" -sigfile "$temporary/signature" >/dev/null 2>&1; then
      echo "Release manifest signature verification failed." >&2
      exit 1
    fi
  fi
}

copy_or_fetch "$manifest_source" "$manifest_url" "$temporary/manifest.json" "signed CLI manifest"
verify_manifest "$temporary/manifest.json" "$temporary/asset"

asset_url="$(sed -n '1p' "$temporary/asset")"
asset_sha="$(sed -n '2p' "$temporary/asset")"
asset_size="$(sed -n '3p' "$temporary/asset")"
release_version="$(sed -n '4p' "$temporary/asset")"
# The parser already enforced all three. Re-checking here is deliberate: this
# chain is the security boundary of the distribution, and these three facts are
# cheap enough to assert twice rather than trust to one implementation.
case "$asset_url" in https://*) ;; *) echo "Release asset URL is not HTTPS." >&2; exit 1 ;; esac
case "$asset_sha" in *[!a-f0-9]*|'') echo "Release checksum is invalid." >&2; exit 1 ;; esac
[ "${#asset_sha}" -eq 64 ] || { echo "Release checksum is invalid." >&2; exit 1; }
case "$asset_size" in *[!0-9]*|'') echo "Release size is invalid." >&2; exit 1 ;; esac

if [ -n "$archive_source" ]; then
  copy_or_fetch "$archive_source" '' "$temporary/archive.tar.gz" "CLI archive"
else
  copy_or_fetch '' "$asset_url" "$temporary/archive.tar.gz" "CLI archive"
fi
if [ "$os" = Darwin ]; then
  actual_size="$(/usr/bin/stat -f '%z' "$temporary/archive.tar.gz")"
  actual_sha="$(/usr/bin/shasum -a 256 "$temporary/archive.tar.gz" | /usr/bin/awk '{print $1}')"
else
  actual_size="$(stat -c '%s' "$temporary/archive.tar.gz")"
  actual_sha="$(sha256sum "$temporary/archive.tar.gz" | "$awk_bin" '{print $1}')"
fi
[ "$actual_size" = "$asset_size" ] || { echo "Release size verification failed." >&2; exit 1; }
[ "$actual_sha" = "$asset_sha" ] || { echo "Release checksum verification failed." >&2; exit 1; }

cli_entries="$("$tar_bin" -tzf "$temporary/archive.tar.gz")"
cli_expected='LICENSE
NOTICE
README.md
overgent'
[ "$(printf '%s\n' "$cli_entries" | LC_ALL=C sort)" = "$cli_expected" ] || {
  echo "CLI archive contains unexpected entries." >&2; exit 1;
}
"$tar_bin" -tvzf "$temporary/archive.tar.gz" overgent | "$awk_bin" 'NR == 1 && substr($1, 1, 1) == "-" { ok = 1 } END { exit !ok }' || {
  echo "CLI archive does not contain a regular overgent executable." >&2; exit 1;
}
"$tar_bin" -xzf "$temporary/archive.tar.gz" -C "$temporary" overgent

if [ "$os" = Darwin ]; then
  /usr/bin/codesign --verify --strict --verbose=2 "$temporary/overgent"
  observed_team="$(/usr/bin/codesign -dvv "$temporary/overgent" 2>&1 | /usr/bin/sed -n 's/^TeamIdentifier=//p')"
  [ "$observed_team" = "$apple_team_id" ] || { echo "Release signer identity verification failed." >&2; exit 1; }
else
  echo "Linux install verified the Ed25519-signed release manifest, size, and SHA-256." >&2
fi

backend_binary=''
backend_bundle=''
if [ "$install_backend" = true ]; then
  [ -n "$backend_manifest_url" ] || backend_manifest_url="$(dirname "$asset_url")/backend-manifest.json"
  copy_or_fetch "$backend_manifest_source" "$backend_manifest_url" "$temporary/backend-manifest.json" "signed backend manifest"
  verify_manifest "$temporary/backend-manifest.json" "$temporary/backend-asset"
  backend_url="$(sed -n '1p' "$temporary/backend-asset")"
  backend_sha="$(sed -n '2p' "$temporary/backend-asset")"
  backend_size="$(sed -n '3p' "$temporary/backend-asset")"
  backend_version="$(sed -n '4p' "$temporary/backend-asset")"
  [ "$backend_version" = "$release_version" ] || { echo "Backend manifest version does not match the CLI release." >&2; exit 1; }
  if [ -n "$backend_archive_source" ]; then
    copy_or_fetch "$backend_archive_source" '' "$temporary/backend.tar.gz" "backend archive"
  else
    copy_or_fetch '' "$backend_url" "$temporary/backend.tar.gz" "backend archive"
  fi
  if [ "$os" = Darwin ]; then
    actual_backend_size="$(/usr/bin/stat -f '%z' "$temporary/backend.tar.gz")"
    actual_backend_sha="$(/usr/bin/shasum -a 256 "$temporary/backend.tar.gz" | /usr/bin/awk '{print $1}')"
  else
    actual_backend_size="$(stat -c '%s' "$temporary/backend.tar.gz")"
    actual_backend_sha="$(sha256sum "$temporary/backend.tar.gz" | "$awk_bin" '{print $1}')"
  fi
  [ "$actual_backend_size" = "$backend_size" ] || { echo "Backend size verification failed." >&2; exit 1; }
  [ "$actual_backend_sha" = "$backend_sha" ] || { echo "Backend checksum verification failed." >&2; exit 1; }
fi

if [ -n "$download_only" ]; then
  mkdir -p "$download_only"
  chmod 700 "$download_only"
  cp "$temporary/manifest.json" "$download_only/update-manifest.json"
  cp "$temporary/archive.tar.gz" "$download_only/$(basename "$asset_url")"
  if [ "$install_backend" = true ]; then
    cp "$temporary/backend-manifest.json" "$download_only/backend-manifest.json"
    cp "$temporary/backend.tar.gz" "$download_only/$(basename "$backend_url")"
  fi
  echo "Verified release files copied to $download_only. Copy that directory to the offline machine and pass the local paths to this installer."
  exit 0
fi

if [ "$os" = Linux ] && ! "$busctl_bin" --user introspect org.freedesktop.secrets /org/freedesktop/secrets >/dev/null 2>&1; then
  echo "Overgent requires a running Secret Service keyring and never stores credentials in a plaintext or encrypted file." >&2
  echo "Start GNOME Keyring, KWallet, or KeePassXC Secret Service in this login session, then run the installer again." >&2
  exit 1
fi

destination="$HOME/.local/bin"
mkdir -p "$destination"
chmod 700 "$destination"
chmod 755 "$temporary/overgent"
if [ "$install_backend" = true ]; then
  if [ "$os" = Darwin ]; then
    runtime_root="$HOME/Library/Application Support/Overgent/runtime/$backend_version"
    backend_name='convex-local-backend'
  else
    config_home="$HOME/.config"
    case "${XDG_CONFIG_HOME:-}" in /*) config_home="$XDG_CONFIG_HOME" ;; esac
    runtime_root="$config_home/Overgent/runtime/$backend_version"
    backend_name='convex-local-backend'
  fi
  runtime_stage="$temporary/backend-runtime"
  mkdir -p "$runtime_stage"
  entries="$("$tar_bin" -tzf "$temporary/backend.tar.gz")"
  expected='LICENSE.md
backend-push.json
convex-local-backend'
  [ "$(printf '%s\n' "$entries" | LC_ALL=C sort)" = "$expected" ] || {
    echo "Backend archive contains unexpected entries." >&2; exit 1;
  }
  "$tar_bin" -tvzf "$temporary/backend.tar.gz" convex-local-backend | "$awk_bin" 'NR == 1 && substr($1, 1, 1) == "-" { ok = 1 } END { exit !ok }' || {
    echo "Backend archive does not contain a regular executable." >&2; exit 1;
  }
  "$tar_bin" -xzf "$temporary/backend.tar.gz" -C "$runtime_stage" LICENSE.md backend-push.json convex-local-backend
  chmod 755 "$runtime_stage/$backend_name"
  if [ "$os" = Darwin ]; then
    /usr/bin/codesign --verify --strict --verbose=2 "$runtime_stage/$backend_name"
    backend_team="$(/usr/bin/codesign -dvv "$runtime_stage/$backend_name" 2>&1 | /usr/bin/sed -n 's/^TeamIdentifier=//p')"
    [ "$backend_team" = "$apple_team_id" ] || { echo "Backend signer identity verification failed." >&2; exit 1; }
  fi
  if [ ! -d "$runtime_root" ]; then
    mkdir -p "$(dirname "$runtime_root")"
    mv "$runtime_stage" "$runtime_root"
  else
    for runtime_file in LICENSE.md backend-push.json "$backend_name"; do
      if [ "$os" = Darwin ]; then
        staged_hash="$(/usr/bin/shasum -a 256 "$runtime_stage/$runtime_file" | /usr/bin/awk '{print $1}')"
        installed_hash="$(/usr/bin/shasum -a 256 "$runtime_root/$runtime_file" 2>/dev/null | /usr/bin/awk '{print $1}' || true)"
      else
        staged_hash="$(sha256sum "$runtime_stage/$runtime_file" | "$awk_bin" '{print $1}')"
        installed_hash="$(sha256sum "$runtime_root/$runtime_file" 2>/dev/null | "$awk_bin" '{print $1}' || true)"
      fi
      [ "$staged_hash" = "$installed_hash" ] || {
        echo "Installed backend runtime differs from the signed $backend_version release; move $runtime_root aside and reinstall." >&2
        exit 1
      }
    done
  fi
  backend_binary="$runtime_root/$backend_name"
  backend_bundle="$runtime_root/backend-push.json"
fi

installed="$destination/overgent"
previous="$installed.previous"
had_previous=false
if [ -f "$installed" ]; then
  rm -f "$previous"
  mv "$installed" "$previous"
  had_previous=true
fi
mv "$temporary/overgent" "$installed"
if { [ "$install_backend" = false ] || "$installed" backend install --binary "$backend_binary" --bundle "$backend_bundle"; } &&
   "$installed" service install; then
  :
else
  echo "Installation did not become healthy; restoring the previous executable when available." >&2
  rm -f "$installed"
  if [ "$had_previous" = true ]; then
    mv "$previous" "$installed"
    "$installed" service install >/dev/null 2>&1 || echo "The previous executable was restored, but its service needs manual repair." >&2
  fi
  exit 1
fi

echo "Overgent $release_version installed and its per-user service started."
[ "$install_backend" = false ] || echo "The $backend_version local backend was fetched separately and configured for local Projects."
echo "Add $destination to PATH if your shell does not already include it."
