#!/usr/bin/env bash

set -euo pipefail

version="latest"
install_dir="${HOME}/.local/bin"
skip_path_update=false

usage() {
    cat <<'EOF'
Usage: install-macos.sh [options]

Options:
  --version VERSION       Install latest or a version such as v0.3.0
  --install-dir PATH      Install directory (default: ~/.local/bin)
  --skip-path-update      Do not add the install directory to ~/.zprofile
  -h, --help              Show this help
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)
            [[ $# -ge 2 ]] || fail "--version requires a value"
            version="$2"
            shift 2
            ;;
        --install-dir)
            [[ $# -ge 2 ]] || fail "--install-dir requires a value"
            install_dir="$2"
            shift 2
            ;;
        --skip-path-update)
            skip_path_update=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

[[ "$(uname -s)" == "Darwin" ]] || fail "this installer supports macOS only"

case "$(uname -m)" in
    arm64|aarch64)
        architecture="arm64"
        ;;
    x86_64|amd64)
        architecture="amd64"
        ;;
    *)
        fail "unsupported Mac architecture: $(uname -m)"
        ;;
esac

if [[ "$version" == "latest" ]]; then
    release_url="$(
        curl --proto '=https' --tlsv1.2 -fsSL \
            -o /dev/null \
            -w '%{url_effective}' \
            https://github.com/Mino829/umlgen/releases/latest
    )"
    tag="${release_url##*/}"
else
    tag="$version"
    [[ "$tag" == v* ]] || tag="v${tag}"
fi

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "invalid release version: $tag"

case "$install_dir" in
    /*)
        ;;
    *)
        install_dir="${PWD}/${install_dir}"
        ;;
esac

if [[ "$install_dir" == *$'\n'* || "$install_dir" == *'"'* ||
      "$install_dir" == *'`'* || "$install_dir" == *'$'* ]]; then
    fail "the install directory contains unsupported characters"
fi

asset="umlgen-darwin-${architecture}"
archive_name="${asset}.tar.gz"
download_base="https://github.com/Mino829/umlgen/releases/download/${tag}"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/umlgen-install.XXXXXX")"

cleanup() {
    if [[ -n "${temp_dir:-}" && -d "$temp_dir" ]]; then
        rm -rf -- "$temp_dir"
    fi
}
trap cleanup EXIT

echo "Downloading umlgen ${tag} for macOS ${architecture}..."
curl --proto '=https' --tlsv1.2 -fsSL \
    "${download_base}/${archive_name}" \
    -o "${temp_dir}/${archive_name}"
curl --proto '=https' --tlsv1.2 -fsSL \
    "${download_base}/SHA256SUMS.txt" \
    -o "${temp_dir}/SHA256SUMS.txt"

expected_hash="$(
    awk -v asset="$archive_name" '$2 == asset { print $1; exit }' \
        "${temp_dir}/SHA256SUMS.txt"
)"
[[ "$expected_hash" =~ ^[0-9a-fA-F]{64}$ ]] ||
    fail "a valid SHA-256 checksum was not found for ${archive_name}"

actual_hash="$(
    shasum -a 256 "${temp_dir}/${archive_name}" | awk '{ print $1 }'
)"
actual_hash="$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')"
expected_hash="$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')"
[[ "$actual_hash" == "$expected_hash" ]] ||
    fail "SHA-256 verification failed for ${archive_name}"

tar -xzf "${temp_dir}/${archive_name}" -C "$temp_dir"
[[ -f "${temp_dir}/${asset}" ]] ||
    fail "the umlgen executable was not found in the downloaded archive"

mkdir -p "$install_dir"
install -m 0755 "${temp_dir}/${asset}" "${install_dir}/umlgen"

if [[ "$skip_path_update" == false ]]; then
    profile="${HOME}/.zprofile"
    path_line="export PATH=\"${install_dir}:\$PATH\""
    touch "$profile"
    if ! grep -Fqx "$path_line" "$profile"; then
        printf '\n# Added by umlgen installer\n%s\n' "$path_line" >> "$profile"
    fi
fi

echo
echo "Installed umlgen ${tag} in:"
echo "  ${install_dir}"
if [[ "$skip_path_update" == false ]]; then
    echo "Open a new Terminal window, then run: umlgen version"
fi
