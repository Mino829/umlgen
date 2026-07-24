#!/usr/bin/env bash

set -euo pipefail

install_dir="${HOME}/.local/bin"
skip_path_update=false

fail() {
    echo "Error: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
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
            echo "Usage: uninstall-macos.sh [--install-dir PATH] [--skip-path-update]"
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

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

binary="${install_dir}/umlgen"
if [[ -e "$binary" ]]; then
    rm -f -- "$binary"
fi

if [[ "$skip_path_update" == false ]]; then
    profile="${HOME}/.zprofile"
    if [[ -f "$profile" ]]; then
        path_line="export PATH=\"${install_dir}:\$PATH\""
        temp_profile="$(mktemp "${TMPDIR:-/tmp}/umlgen-profile.XXXXXX")"
        trap 'rm -f -- "$temp_profile"' EXIT
        awk -v path_line="$path_line" '
            $0 == path_line { next }
            $0 == "# Added by umlgen installer" { next }
            { print }
        ' "$profile" > "$temp_profile"
        mv "$temp_profile" "$profile"
        trap - EXIT
    fi
fi

echo "Removed umlgen from: ${binary}"
if [[ "$skip_path_update" == false ]]; then
    echo "Open a new Terminal window to refresh PATH."
fi
