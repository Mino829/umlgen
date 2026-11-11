#!/usr/bin/env bash
# Use only on a machine without an existing umlgen Homebrew installation/tap.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
export HOMEBREW_NO_AUTO_UPDATE=1
export HOMEBREW_NO_INSTALL_CLEANUP=1
formula=mino829/umlgen/umlgen
case "$(brew tap)" in
  *mino829/umlgen*) echo 'Refusing to replace an existing mino829/umlgen tap.' >&2; exit 1 ;;
esac
if brew list --formula --versions umlgen >/dev/null 2>&1; then
  echo 'Refusing to replace an existing umlgen formula.' >&2
  exit 1
fi
work="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/umlgen-homebrew.XXXXXX")"
tap_repo="$work/tap"
cleanup() {
  brew uninstall --force "$formula" >/dev/null 2>&1 || true
  brew untap mino829/umlgen >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

# Install a genuine older release, never a fabricated version of the new binary.
python3 "$root/scripts/update-package-managers.py" --version v0.6.0 --output-dir "$tap_repo"
git -C "$tap_repo" init --quiet
git -C "$tap_repo" add Formula/umlgen.rb
git -C "$tap_repo" -c user.name=PackageSmoke -c user.email=smoke@example.invalid \
  -c commit.gpgsign=false commit --quiet -m 'Published v0.6.0 baseline'
brew tap mino829/umlgen "$tap_repo"
brew install "$formula"
binary="$(brew --prefix)/bin/umlgen"
python3 "$root/scripts/smoke-package-manager.py" --binary "$binary" --version 0.6.0 --work-dir "$work/baseline"
brew test "$formula"

# Update the local Git remote as a real tap update would, testing this checkout's definition.
cp "$root/Formula/umlgen.rb" "$tap_repo/Formula/umlgen.rb"
git -C "$tap_repo" add Formula/umlgen.rb
if ! git -C "$tap_repo" diff --cached --quiet; then
  git -C "$tap_repo" -c user.name=PackageSmoke -c user.email=smoke@example.invalid \
    -c commit.gpgsign=false commit --quiet -m 'Package definition under test'
fi
brew update
brew upgrade "$formula"
version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$root/bucket/umlgen.json")"
if [ "$version" = 0.6.0 ]; then
  # Initial tap has no newer release yet; also exercise replacement of the same version.
  brew reinstall "$formula"
fi
python3 "$root/scripts/smoke-package-manager.py" --binary "$binary" --version "$version" --work-dir "$work/current"
brew test "$formula"
brew uninstall "$formula"
test ! -e "$binary"
brew untap mino829/umlgen
trap - EXIT
rm -rf "$work"
