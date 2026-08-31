#!/bin/sh
# Builds and publishes the closed-test distribution: one origin serving the
# installer, the app bundle, the CLI, the dashboard, and a proxy to Convex.
#
#   STICKGUY_DOGFOOD_ORIGIN=https://example.vercel.app \
#   STICKGUY_DOGFOOD_CONVEX=https://your-deployment.convex.site \
#   ./install/dogfood/publish.sh [--deploy]
#
# Without --deploy it stages dist-dogfood/ and stops, so the artifacts can be
# inspected before anything is served. See README.md in this directory for what
# this channel is and, more importantly, what it is not.
set -eu

root="$(cd "$(dirname "$0")/../.." && pwd)"
staging="$root/dist-dogfood"
deploy=false
[ "${1:-}" = "--deploy" ] && deploy=true

origin="${STICKGUY_DOGFOOD_ORIGIN:?set STICKGUY_DOGFOOD_ORIGIN to the public origin}"
convex="${STICKGUY_DOGFOOD_CONVEX:?set STICKGUY_DOGFOOD_CONVEX to the Convex site URL}"
project="${STICKGUY_DOGFOOD_VERCEL_PROJECT:-stickguy-dogfood}"
origin="${origin%/}"
convex="${convex%/}"

if [ "$(uname -s)" != Darwin ]; then
  echo "The desktop bundle can only be built on macOS." >&2
  exit 1
fi
# Dashboard activation only accepts an HTTPS origin, and the app bakes this in
# at link time, so a bad value here fails much later and much less clearly.
for value in "$origin" "$convex"; do
  case "$value" in
    https://*[!/]) ;;
    *) echo "Not a clean HTTPS origin: $value" >&2; exit 1 ;;
  esac
done
node_major="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
if [ "$node_major" -lt 22 ]; then
  echo "Node 22 or newer is required (found ${node_major}.x). Try: nvm use 22" >&2
  exit 1
fi
# Checked before the build rather than after it. The build takes minutes, and
# a missing CLI discovered at the deploy step wastes all of them. nvm installs
# global binaries per Node version, so requiring Node 22 above can itself be
# what takes vercel off PATH.
if [ "$deploy" = true ]; then
  vercel_bin="${STICKGUY_DOGFOOD_VERCEL_BIN:-vercel}"
  if ! command -v "$vercel_bin" >/dev/null 2>&1; then
    echo "The vercel CLI is not on PATH for Node ${node_major}.x." >&2
    echo "Install it for this version:  npm i -g vercel" >&2
    echo "Or point at an existing one:  STICKGUY_DOGFOOD_VERCEL_BIN=\$(nvm which 20 >/dev/null 2>&1 && dirname \$(nvm which 20))/vercel" >&2
    echo "Or stage without deploying by re-running without --deploy." >&2
    exit 1
  fi
fi

commit="$(git -C "$root" rev-parse HEAD)"
short="$(git -C "$root" rev-parse --short HEAD)"
if [ -n "$(git -C "$root" status --porcelain)" ]; then
  # The point of stamping the commit into the binary is that someone can later
  # ask what a member actually installed. Shipping a dirty tree quietly breaks
  # that, so say so rather than stamping a commit the artifacts do not match.
  echo "WARNING: the working tree is dirty; published artifacts will not match $short." >&2
fi

echo "Building Stickguy $short against $origin ..."
STICKGUY_PRODUCTION_API_ORIGIN="$origin" \
  STICKGUY_VERSION="v0.1.0-dogfood-$short" \
  STICKGUY_COMMIT="$commit" \
  pnpm --dir "$root" desktop:build >/dev/null
pnpm --dir "$root/apps/dashboard" build >/dev/null

app="$root/apps/desktop/build/bin/Stickguy.app"
rm -rf "$staging"
mkdir -p "$staging/public"
cp "$root/apps/dashboard/dist/index.html" "$staging/public/index.html"
cp -R "$root/apps/dashboard/dist/assets" "$staging/public/assets"
cp "$app/Contents/Resources/stickguy" "$staging/public/stickguy"
ditto -c -k --keepParent "$app" "$staging/public/Stickguy.zip"

app_sha="$(shasum -a 256 "$staging/public/Stickguy.zip" | awk '{print $1}')"
cli_sha="$(shasum -a 256 "$staging/public/stickguy" | awk '{print $1}')"
sed -e "s|__ORIGIN__|$origin|g" -e "s|__APP_SHA256__|$app_sha|g" -e "s|__SHA256__|$cli_sha|g" \
  "$root/install/dogfood/install.template.sh" > "$staging/public/install.sh"
chmod 755 "$staging/public/install.sh"

# A single origin is load-bearing, not a convenience: dashboard activation
# replies with a 303 to /dashboard on whichever host authenticated the ticket,
# so the SPA and the API have to answer on the same one.
cat > "$staging/vercel.json" <<JSON
{
  "\$schema": "https://openapi.vercel.sh/vercel.json",
  "rewrites": [
    { "source": "/api/(.*)", "destination": "$convex/\$1" },
    { "source": "/v1/(.*)", "destination": "$convex/v1/\$1" },
    { "source": "/dashboard", "destination": "/index.html" }
  ],
  "headers": [
    { "source": "/install.sh", "headers": [{ "key": "content-type", "value": "text/plain; charset=utf-8" }, { "key": "cache-control", "value": "no-store" }] },
    { "source": "/stickguy", "headers": [{ "key": "content-type", "value": "application/octet-stream" }, { "key": "cache-control", "value": "no-store" }] },
    { "source": "/Stickguy.zip", "headers": [{ "key": "content-type", "value": "application/zip" }, { "key": "cache-control", "value": "no-store" }] }
  ]
}
JSON

echo
echo "Staged $staging"
echo "  commit      $short"
echo "  app         $app_sha"
echo "  cli         $cli_sha"
echo "  convex      $convex"

if [ "$deploy" != true ]; then
  echo
  echo "Re-run with --deploy to publish, or: cd $staging && vercel deploy --prod"
  exit 0
fi

echo
echo "Deploying to $origin (project $project) ..."
# The staging directory is recreated on every run and so is never linked to a
# project. Without an explicit --project, vercel takes the directory name and
# helpfully creates a brand new project, which deploys the build to a URL
# nobody has and leaves the real origin serving the previous one.
(cd "$staging" && "$vercel_bin" deploy --prod --yes --project "$project")

# The installer pins a checksum of the binary served beside it. If a cached
# edge copy of one outlives the other, members get a checksum failure rather
# than a wrong binary, but it is still worth catching here.
served_app="$(curl -fsSL "$origin/Stickguy.zip?cachebust=$$" | shasum -a 256 | awk '{print $1}')"
served_pin="$(curl -fsSL "$origin/install.sh?cachebust=$$" | sed -n "s/^expected_sha='\\(.*\\)'/\\1/p")"
if [ "$served_app" != "$app_sha" ] || [ "$served_pin" != "$app_sha" ]; then
  echo "WARNING: served artifacts do not agree yet (pin=$served_pin app=$served_app); re-check in a minute." >&2
  exit 1
fi
echo "Verified: the served installer pins the served app ($app_sha)."
