#!/usr/bin/env python3
"""Build a native release archive or assemble the four-archive checksum manifest."""

import argparse
import hashlib
import os
from pathlib import Path
import platform
import re
import subprocess
import tarfile
import zipfile


ROOT = Path(__file__).resolve().parent.parent
VERSION_PATTERN = re.compile(
    r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?",
    re.ASCII,
)
ARCHIVES = {
    "umlgen-linux-amd64.tar.gz": "umlgen-linux-amd64",
    "umlgen-darwin-arm64.tar.gz": "umlgen-darwin-arm64",
    "umlgen-darwin-amd64.tar.gz": "umlgen-darwin-amd64",
    "umlgen-windows-amd64.zip": "umlgen-windows-amd64.exe",
}
MANIFEST = "SHA256SUMS.txt"


def release_version(value):
    match = VERSION_PATTERN.fullmatch(value)
    if match is None or any(
        part.isdigit() and len(part) > 1 and part.startswith("0")
        for part in (match.group(4) or "").split(".")
    ):
        raise argparse.ArgumentTypeError("version must be strict SemVer without a v prefix")
    return value


def digest(path):
    hasher = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def write_checksums(output, names):
    text = "".join(f"{digest(output / name)}  {name}\n" for name in sorted(names))
    (output / MANIFEST).write_text(text, encoding="ascii", newline="\n")


def aggregate_checksums(output):
    entries = {entry.name for entry in output.iterdir()}
    expected = set(ARCHIVES)
    missing = expected - entries
    unexpected = entries - expected - {MANIFEST}
    if missing or unexpected:
        raise ValueError(
            f"expected exactly the four public archives; missing={sorted(missing)}, "
            f"unexpected={sorted(unexpected)}"
        )
    for name, member in ARCHIVES.items():
        archive = output / name
        if archive.is_symlink() or not archive.is_file():
            raise ValueError(f"archive must be a regular file: {name}")
        if name.endswith(".zip"):
            with zipfile.ZipFile(archive) as source:
                members = source.infolist()
                if (
                    len(members) != 1
                    or members[0].filename != member
                    or members[0].is_dir()
                    or members[0].file_size == 0
                    or (members[0].external_attr >> 16) & 0o170000 == 0o120000
                ):
                    raise ValueError(f"archive must contain only its native executable: {name}")
        else:
            with tarfile.open(archive, "r:gz") as source:
                members = source.getmembers()
                if (
                    len(members) != 1
                    or members[0].name != member
                    or not members[0].isfile()
                    or members[0].size == 0
                ):
                    raise ValueError(f"archive must contain only its native executable: {name}")
    manifest = output / MANIFEST
    if manifest.is_symlink() or (manifest.exists() and not manifest.is_file()):
        raise ValueError("checksum manifest must be a regular file")
    write_checksums(output, expected)


def build_native(version, output):
    system = {"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}.get(
        platform.system()
    )
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(machine)
    asset = f"umlgen-{system}-{arch}"
    archive_name = asset + (".zip" if system == "windows" else ".tar.gz")
    if archive_name not in ARCHIVES:
        raise ValueError(f"unsupported native release platform: {platform.system()}/{machine}")
    output.mkdir(parents=True, exist_ok=True)
    executable = output / ARCHIVES[archive_name]
    environment = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="1")
    subprocess.run(
        [
            "go", "build", "-trimpath",
            "-ldflags", f"-s -w -X github.com/Mino829/umlgen/internal/cli.Version={version}",
            "-o", str(executable), "./cmd/umlgen",
        ],
        cwd=ROOT,
        env=environment,
        check=True,
    )
    archive = output / archive_name
    if system == "windows":
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as target:
            target.write(executable, arcname=executable.name)
    else:
        with tarfile.open(archive, "w:gz") as target:
            target.add(executable, arcname=executable.name, recursive=False)
    write_checksums(output, [archive_name])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, type=release_version)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--checksums-only", action="store_true")
    args = parser.parse_args()
    output = args.output_dir.resolve()
    try:
        if args.checksums_only:
            aggregate_checksums(output)
        else:
            build_native(args.version, output)
    except (OSError, ValueError, tarfile.TarError, zipfile.BadZipFile, subprocess.CalledProcessError) as error:
        parser.exit(1, f"package-release: {error}\n")


if __name__ == "__main__":
    main()
