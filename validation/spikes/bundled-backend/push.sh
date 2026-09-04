#!/bin/sh
set -eu

usage() {
  echo "usage: $0 build <backend-url> <admin-key> <output-dir>" >&2
  echo "       $0 replay <backend-url> <admin-key> <backend-push.json>" >&2
  exit 2
}

[ "$#" -eq 4 ] || usage
mode="$1"
backend_url="${2%/}"
admin_key="$3"
artifact="$4"

case "$backend_url" in
  http://127.0.0.1:*|http://localhost:*) ;;
  *) echo "backend URL must be a loopback HTTP origin" >&2; exit 2 ;;
esac
case "$admin_key" in
  *'|'*) ;;
  *) echo "admin key does not have the expected instance-name form" >&2; exit 2 ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/overgent-backend-push.XXXXXX")
cleanup() { rm -rf "$scratch"; }
trap cleanup EXIT HUP INT TERM

case "$mode" in
  build)
    output_dir="$artifact"
    mkdir -p "$output_dir"
    (
      cd "$repo_root/convex"
      ./node_modules/.bin/convex deploy \
        --url "$backend_url" \
        --admin-key "$admin_key" \
        --typecheck disable \
        --codegen disable \
        --skip-workos-check \
        --push-all-modules \
        --write-push-request "$scratch/backend-push"
    )
    jq -c '.adminKey = "__OVERGENT_ADMIN_KEY__"' "$scratch/backend-push.json" > "$output_dir/backend-push.json"
    echo "$output_dir/backend-push.json"
    ;;
  replay)
    [ -f "$artifact" ] || { echo "push artifact does not exist: $artifact" >&2; exit 2; }
    jq --arg key "$admin_key" '.adminKey = $key' "$artifact" > "$scratch/start.json"
    brotli -q 4 -o "$scratch/start.json.br" "$scratch/start.json"
    curl -fsS \
      -H "Authorization: Convex $admin_key" \
      -H "Convex-Client: npm-cli-1.45.0" \
      -H "Content-Type: application/json" \
      -H "Content-Encoding: br" \
      --data-binary "@$scratch/start.json.br" \
      "$backend_url/api/deploy2/start_push" > "$scratch/start-response.json"

    while :; do
      jq --arg key "$admin_key" '{adminKey:$key,schemaChange:.schemaChange,timeoutMs:10000,dryRun:false}' \
        "$scratch/start-response.json" > "$scratch/wait.json"
      curl -fsS \
        -H "Authorization: Convex $admin_key" \
        -H "Convex-Client: npm-cli-1.45.0" \
        -H "Content-Type: application/json" \
        --data-binary "@$scratch/wait.json" \
        "$backend_url/api/deploy2/wait_for_schema" > "$scratch/wait-response.json"
      schema_state=$(jq -r '.type' "$scratch/wait-response.json")
      case "$schema_state" in
        complete) break ;;
        inProgress) continue ;;
        *) cat "$scratch/wait-response.json" >&2; exit 1 ;;
      esac
    done

    jq --arg key "$admin_key" '{adminKey:$key,startPush:.,dryRun:false}' \
      "$scratch/start-response.json" > "$scratch/finish.json"
    brotli -q 4 -o "$scratch/finish.json.br" "$scratch/finish.json"
    curl -fsS \
      -H "Authorization: Convex $admin_key" \
      -H "Convex-Client: npm-cli-1.45.0" \
      -H "Content-Type: application/json" \
      -H "Content-Encoding: br" \
      --data-binary "@$scratch/finish.json.br" \
      "$backend_url/api/deploy2/finish_push" > "$scratch/finish-response.json"
    jq '{
      authAdded: (.authDiff.added | length),
      authRemoved: (.authDiff.removed | length),
      definitions: (.definitionDiffs | length),
      components: (.componentDiffs | to_entries | map({
        path: .key,
        diffType: .value.diffType,
        modulesAdded: (.value.moduleDiff.added // []),
        modulesRemoved: (.value.moduleDiff.removed // [])
      }))
    }' \
      "$scratch/finish-response.json"
    ;;
  *) usage ;;
esac
