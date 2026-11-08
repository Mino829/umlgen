#!/usr/bin/env bash

set -euo pipefail

version="latest"
version_explicit=false
archive_dir=""
install_dir="${HOME}/.local/bin"
skip_path_update=false

usage() {
    cat <<'EOF'
Usage: install-macos.sh [options]

Options:
  --version VERSION       Install latest or a SemVer version (optional v prefix)
  --archive-dir PATH      Install offline from PATH (requires explicit --version)
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
            version_explicit=true
            shift 2
            ;;
        --archive-dir)
            [[ $# -ge 2 && -n "$2" ]] || fail "--archive-dir requires a value"
            archive_dir="$2"
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

if [[ -n "$archive_dir" ]]; then
    [[ "$version_explicit" == true && "$version" != latest ]] ||
        fail "--archive-dir requires an explicit release --version"
    [[ -d "$archive_dir" ]] || fail "the archive directory does not exist"
fi

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

# Strict SemVer, including prerelease identifiers; numeric identifiers cannot
# have leading zeroes. Build metadata is preserved when checking the binary.
release_version="${tag#v}"
semver_pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?(\+([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?$'
[[ "$tag" == v* && "$release_version" =~ $semver_pattern ]] ||
    fail "invalid release version"
prerelease="${release_version%%+*}"
if [[ "$prerelease" == *-* ]]; then
    prerelease="${prerelease#*-}"
    old_ifs="$IFS"
    IFS=.
    read -r -a identifiers <<< "$prerelease"
    IFS="$old_ifs"
    for identifier in "${identifiers[@]}"; do
        [[ ! "$identifier" =~ ^0[0-9]+$ ]] || fail "invalid numeric prerelease identifier"
    done
fi

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
staged_install=""

cleanup() {
    if [[ -n "${staged_install:-}" ]]; then
        rm -f -- "$staged_install"
    fi
    if [[ -n "${temp_dir:-}" && -d "$temp_dir" ]]; then
        rm -rf -- "$temp_dir"
    fi
}
trap cleanup EXIT

if [[ -n "$archive_dir" ]]; then
    echo "Installing local umlgen ${tag} for macOS ${architecture}..."
    [[ -f "${archive_dir}/${archive_name}" && -f "${archive_dir}/SHA256SUMS.txt" ]] ||
        fail "the archive directory must contain ${archive_name} and SHA256SUMS.txt"
    # Work from a private snapshot so the file checked is the file extracted.
    cp "${archive_dir}/${archive_name}" "${temp_dir}/${archive_name}"
    cp "${archive_dir}/SHA256SUMS.txt" "${temp_dir}/SHA256SUMS.txt"
else
    echo "Downloading umlgen ${tag} for macOS ${architecture}..."
    curl --proto '=https' --tlsv1.2 -fsSL \
        "${download_base}/${archive_name}" \
        -o "${temp_dir}/${archive_name}"
    curl --proto '=https' --tlsv1.2 -fsSL \
        "${download_base}/SHA256SUMS.txt" \
        -o "${temp_dir}/SHA256SUMS.txt"
fi

# Require one exact filename entry, not the first matching digest. Accept the
# standard sha256sum text and binary markers, but not paths or extra columns.
expected_hash=""
checksum_count=0
while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    if [[ "$line" =~ ^([^[:space:]]+)[[:space:]]+(\*?)([^[:space:]]+)$ ]]; then
        hash="${BASH_REMATCH[1]}"
        checksum_name="${BASH_REMATCH[3]}"
        if [[ "$checksum_name" == "$archive_name" ]]; then
            checksum_count=$((checksum_count + 1))
            [[ "$hash" =~ ^[0-9a-fA-F]{64}$ ]] ||
                fail "invalid SHA-256 checksum for ${archive_name}"
            expected_hash="$hash"
        fi
    elif [[ "$line" == *"$archive_name"* ]]; then
        fail "malformed SHA-256 checksum entry for ${archive_name}"
    fi
done < "${temp_dir}/SHA256SUMS.txt"
[[ "$checksum_count" == 1 ]] ||
    fail "expected exactly one SHA-256 checksum for ${archive_name}"

actual_hash="$(
    shasum -a 256 "${temp_dir}/${archive_name}" | awk '{ print $1 }'
)"
actual_hash="$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')"
expected_hash="$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')"
[[ "$actual_hash" == "$expected_hash" ]] ||
    fail "SHA-256 verification failed for ${archive_name}"

# Native release archives contain exactly one regular executable. List before
# streaming that exact member; never let tar create archive-controlled paths,
# links, permissions, or files on disk.
tar -tzf "${temp_dir}/${archive_name}" > "${temp_dir}/members"
member_count=0
while IFS= read -r member || [[ -n "$member" ]]; do
    [[ "$member" == "$asset" ]] || fail "unexpected archive member"
    member_count=$((member_count + 1))
done < "${temp_dir}/members"
[[ "$member_count" == 1 ]] || fail "expected exactly one umlgen executable"
member_details="$(tar -tvzf "${temp_dir}/${archive_name}")"
[[ "$member_details" == -* ]] || fail "the archive executable must be a regular file"
tar -xOzf "${temp_dir}/${archive_name}" -- "$asset" > "${temp_dir}/${asset}"
[[ -s "${temp_dir}/${asset}" ]] || fail "the archive executable is empty"
chmod 0700 "${temp_dir}/${asset}"
if ! installed_version="$("${temp_dir}/${asset}" version)"; then
    fail "the archive executable could not report its version"
fi
[[ "$installed_version" == "umlgen version ${release_version}" ]] ||
    fail "the archive executable version does not match ${release_version}"

# Stage on the target filesystem, then rename; validation failures and failed
# copies never truncate or follow a symlink at the old executable.
mkdir -p "$install_dir"
[[ ! -d "${install_dir}/umlgen" ]] || fail "the install target is a directory"
staged_install="$(mktemp "${install_dir}/.umlgen-install.XXXXXX")"
install -m 0755 "${temp_dir}/${asset}" "$staged_install"
mv -f -- "$staged_install" "${install_dir}/umlgen"
staged_install=""

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
