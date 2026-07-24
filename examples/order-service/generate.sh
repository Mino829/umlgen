#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
output_dir="${1:-$script_dir/assets}"
umlgen_bin="${UMLGEN_BIN:-}"

if [[ "$output_dir" != /* ]]; then
  output_dir="$PWD/$output_dir"
fi

if [[ -z "$umlgen_bin" ]]; then
  if command -v umlgen >/dev/null 2>&1; then
    umlgen_bin="$(command -v umlgen)"
  elif [[ -x "$repo_root/umlgen" ]]; then
    umlgen_bin="$repo_root/umlgen"
  else
    echo "umlgen was not found. Install it or set UMLGEN_BIN." >&2
    exit 1
  fi
fi

if ! command -v plantuml >/dev/null 2>&1; then
  echo "PlantUML was not found. Install it before generating the SVG assets." >&2
  exit 1
fi

mkdir -p "$output_dir"
demo_repo="$(mktemp -d "${TMPDIR:-/tmp}/umlgen-order-demo.XXXXXX")"

cleanup() {
  if [[ -d "$demo_repo" && "$(basename "$demo_repo")" == umlgen-order-demo.* ]]; then
    rm -rf -- "$demo_repo"
  fi
}
trap cleanup EXIT

cp -R "$script_dir/scenarios/before/src" "$demo_repo/src"
git -C "$demo_repo" init --quiet
git -C "$demo_repo" config user.name "umlgen demo"
git -C "$demo_repo" config user.email "demo@umlgen.invalid"
git -C "$demo_repo" add src
git -C "$demo_repo" commit --quiet -m "Before payment integration"
base_sha="$(git -C "$demo_repo" rev-parse HEAD)"

git -C "$demo_repo" rm --quiet -r src
cp -R "$script_dir/scenarios/after/src" "$demo_repo/src"
git -C "$demo_repo" add src
git -C "$demo_repo" commit --quiet -m "Add payment integration"
head_sha="$(git -C "$demo_repo" rev-parse HEAD)"

"$umlgen_bin" class "$script_dir/scenarios/before/src/main/java" \
  --format svg \
  --output "$output_dir/before.puml" \
  --no-cache \
  --quiet

"$umlgen_bin" class "$script_dir/scenarios/after/src/main/java" \
  --format svg \
  --output "$output_dir/after.puml" \
  --no-cache \
  --quiet

(
  cd "$demo_repo"
  "$umlgen_bin" diff "$base_sha...$head_sha" src/main/java \
    --format svg \
    --output "$output_dir/diff.puml" \
    --no-cache \
    --quiet
)

plantuml -tsvg -pipe < "$script_dir/workflow.puml" > "$output_dir/workflow.svg"

for asset in before.puml before.svg after.puml after.svg diff.puml diff.svg workflow.svg; do
  test -s "$output_dir/$asset"
done

echo "Generated demo assets in $output_dir"
