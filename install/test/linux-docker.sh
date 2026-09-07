#!/bin/sh
set -eu

root="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/overgent-linux-release.XXXXXX")"
container="overgent-lanef-$$"
plain_container="overgent-lanef-no-keyring-$$"
cleanup() {
  docker rm -f "$container" "$plain_container" >/dev/null 2>&1 || true
  rm -rf "$temporary"
}
trap cleanup EXIT HUP INT TERM

case "$(uname -m)" in
  arm64|aarch64) goarch=arm64; platform=linux_arm64 ;;
  x86_64|amd64) goarch=amd64; platform=linux_amd64 ;;
  *) echo "unsupported Docker host architecture" >&2; exit 1 ;;
esac

mkdir -p "$temporary/v1" "$temporary/v2" "$temporary/backend" "$temporary/build"
GOCACHE="${GOCACHE:-/tmp/overgent-gocache}" go run "$root/cmd/release-keygen" -private-file "$temporary/private-key" > "$temporary/public-key"
public_key="$(tr -d '\r\n' < "$temporary/public-key")"

build_cli() {
  version=$1
  output=$2
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOCACHE="${GOCACHE:-/tmp/overgent-gocache}" \
    go build -trimpath -ldflags "-s -w -X main.version=$version -X main.updatePublicKey=$public_key" -o "$output" "$root/cmd/overgent"
}
build_cli v0.1.0 "$temporary/build/overgent-v1"
build_cli v0.1.1 "$temporary/build/overgent-v2"
cp "$temporary/build/overgent-v1" "$temporary/v1/overgent"
cp "$root/LICENSE" "$root/NOTICE" "$root/README.md" "$temporary/v1/"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$temporary/v1/overgent_0.1.0_${platform}.tar.gz" -C "$temporary/v1" LICENSE NOTICE README.md overgent
cp "$temporary/build/overgent-v2" "$temporary/v2/overgent"
cp "$root/LICENSE" "$root/NOTICE" "$root/README.md" "$temporary/v2/"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$temporary/v2/overgent_0.1.1_${platform}.tar.gz" -C "$temporary/v2" LICENSE NOTICE README.md overgent

printf '#!/bin/sh\nexit 0\n' > "$temporary/backend/convex-local-backend"
chmod 755 "$temporary/backend/convex-local-backend"
printf '{"schemaVersion":1,"adminKey":"fixture","functions":[]}\n' > "$temporary/backend/backend-push.json"
printf 'Functional Source License fixture for installer lifecycle tests.\n' > "$temporary/backend/LICENSE.md"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$temporary/backend/overgent_backend_v0.1.0_${platform}.tar.gz" \
  -C "$temporary/backend" convex-local-backend backend-push.json LICENSE.md

GOCACHE="${GOCACHE:-/tmp/overgent-gocache}" OVERGENT_UPDATE_SIGNING_KEY_FILE="$temporary/private-key" \
  go run "$root/cmd/release-metadata" -dist "$temporary/v1" -version v0.1.0 \
  -base-url https://localhost:8443/v1 -output "$temporary/update-manifest.json"
GOCACHE="${GOCACHE:-/tmp/overgent-gocache}" OVERGENT_UPDATE_SIGNING_KEY_FILE="$temporary/private-key" \
  go run "$root/cmd/release-metadata" -dist "$temporary/v2" -version v0.1.1 \
  -base-url https://localhost:8443/v2 -output "$temporary/update-v2.json"
GOCACHE="${GOCACHE:-/tmp/overgent-gocache}" OVERGENT_UPDATE_SIGNING_KEY_FILE="$temporary/private-key" \
  go run "$root/cmd/release-metadata" -dist "$temporary/backend" -version v0.1.0 \
  -base-url https://localhost:8443/backend -output "$temporary/v1/backend-manifest.json"

cp "$root/install/install.sh" "$temporary/install.sh"
sed -i.bak -e "s|__OVERGENT_UPDATE_PUBLIC_KEY__|$public_key|g" -e 's|__OVERGENT_APPLE_TEAM_ID__|TESTTEAM|g' "$temporary/install.sh"
rm -f "$temporary/install.sh.bak"
cp "$root/install/uninstall.sh" "$temporary/uninstall.sh"
cp "$root/install/test/https_server.py" "$temporary/https_server.py"
chmod 755 "$temporary/install.sh" "$temporary/uninstall.sh"

openssl req -x509 -newkey rsa:2048 -sha256 -days 2 -nodes -keyout "$temporary/ca.key" -out "$temporary/ca.crt" -subj '/CN=Overgent Lane F CA' >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -keyout "$temporary/server.key" -out "$temporary/server.csr" -subj '/CN=localhost' >/dev/null 2>&1
printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\n' > "$temporary/san.cnf"
openssl x509 -req -in "$temporary/server.csr" -CA "$temporary/ca.crt" -CAkey "$temporary/ca.key" -CAcreateserial \
  -out "$temporary/server.crt" -days 2 -sha256 -extfile "$temporary/san.cnf" >/dev/null 2>&1

image="$(docker build -q -f "$root/install/test/Dockerfile.linux" "$root/install/test")"
docker run -d --privileged --name "$container" -v "$temporary:/release" "$image" >/dev/null
docker exec "$container" sh -c 'useradd -m -u 1000 tester; loginctl enable-linger tester; systemctl start user@1000.service; cp /release/ca.crt /usr/local/share/ca-certificates/overgent-lanef.crt; update-ca-certificates >/dev/null'
docker exec -d "$container" python3 /release/https_server.py /release /release/server.crt /release/server.key
docker exec "$container" sh -c 'for i in $(seq 1 50); do curl -fsS https://localhost:8443/update-manifest.json >/dev/null && exit 0; sleep .1; done; exit 1'
docker exec "$container" runuser -u tester -- env HOME=/home/tester XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus \
  sh -c 'printf lane-f | gnome-keyring-daemon --unlock --components=secrets >/tmp/keyring-env'

as_tester() {
  docker exec "$container" runuser -u tester -- env HOME=/home/tester XDG_RUNTIME_DIR=/run/user/1000 \
    DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus PATH=/home/tester/.local/bin:/usr/local/bin:/usr/bin:/bin "$@"
}

as_tester sh /release/install.sh --manifest https://localhost:8443/update-manifest.json
as_tester overgent service status | grep -Eq '"service":"running"|"installed":true'
as_tester overgent version --json | grep -q 'v0.1.0'
as_tester test -f /home/tester/.config/Overgent/backend/backend.json

as_tester overgent update --manifest https://localhost:8443/update-v2.json
as_tester overgent version --json | grep -q 'v0.1.1'
as_tester overgent update rollback
as_tester overgent version --json | grep -q 'v0.1.0'

as_tester sh /release/install.sh --manifest https://localhost:8443/update-manifest.json --download-only /home/tester/offline
as_tester test -f /home/tester/offline/update-manifest.json
as_tester test -f /home/tester/offline/backend-manifest.json

as_tester sh /release/uninstall.sh
as_tester test ! -e /home/tester/.local/bin/overgent
as_tester test -d /home/tester/.config/Overgent

cli_archive="$(basename "$(find "$temporary/v1" -name 'overgent_*.tar.gz' -print -quit)")"
backend_archive="$(basename "$(find "$temporary/backend" -name 'overgent_backend_*.tar.gz' -print -quit)")"
as_tester sh /release/install.sh --manifest /home/tester/offline/update-manifest.json \
  --archive "/home/tester/offline/$cli_archive" --backend-manifest /home/tester/offline/backend-manifest.json \
  --backend-archive "/home/tester/offline/$backend_archive"
as_tester sh /release/uninstall.sh --purge-local-state
as_tester test ! -d /home/tester/.config/Overgent

cp "$temporary/update-manifest.json" "$temporary/tampered-manifest.json"
sed -i.bak 's/2026-/2025-/' "$temporary/tampered-manifest.json" 2>/dev/null || true
rm -f "$temporary/tampered-manifest.json.bak"
if as_tester sh /release/install.sh --manifest https://localhost:8443/tampered-manifest.json --skip-backend >/tmp/tampered.out 2>&1; then
  echo "tampered signed manifest was accepted" >&2; exit 1
fi
grep -q 'signature verification failed' /tmp/tampered.out

if as_tester sh /release/install.sh --manifest https://localhost:9443/missing.json --skip-backend >/tmp/offline.out 2>&1; then
  echo "offline fetch unexpectedly succeeded" >&2; exit 1
fi
grep -q -- '--download-only' /tmp/offline.out

docker run --name "$plain_container" -v "$temporary:/release:ro" "$image" sh -c \
  'HOME=/root sh /release/install.sh --manifest /release/update-manifest.json --archive /release/v1/'"$cli_archive"' --skip-backend' \
  >/tmp/no-keyring.out 2>&1 && { echo "install without Secret Service unexpectedly succeeded" >&2; exit 1; }
grep -q 'requires a running Secret Service keyring' /tmp/no-keyring.out

echo "Linux Docker lifecycle passed: signed install, backend fetch/configure, systemd service, update, rollback, prefetch/offline install, uninstall/purge, tamper/offline/keyring failures."
