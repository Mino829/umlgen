#!/usr/bin/env python3
"""Prove rejected candidate installs preserve the real installed binary and PATH."""

import argparse
import hashlib
import io
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import zipfile


ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def user_path():
    if os.name == "nt":
        import winreg
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment") as key:
            try:
                return winreg.QueryValueEx(key, "Path")
            except FileNotFoundError:
                return None
    profile = Path.home() / ".zprofile"
    return profile.read_bytes() if profile.exists() else None


def unsafe_archive(original, destination, member, duplicate):
    if original.name.endswith(".zip"):
        with zipfile.ZipFile(original) as source, zipfile.ZipFile(destination, "w") as target:
            with source.open(member) as incoming, target.open(member, "w") as outgoing:
                shutil.copyfileobj(incoming, outgoing)
            target.writestr(member if duplicate else "../umlgen-escaped.txt", b"must not extract")
    else:
        with tarfile.open(original, "r:gz") as source, tarfile.open(destination, "w:gz") as target:
            entry = source.getmember(member)
            with source.extractfile(entry) as data:
                target.addfile(entry, data)
            extra = tarfile.TarInfo(member if duplicate else "../umlgen-escaped.txt")
            payload = b"must not extract"
            extra.size = len(payload)
            target.addfile(extra, io.BytesIO(payload))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive-dir", type=Path, required=True)
    parser.add_argument("--install-dir", type=Path, required=True)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    install = args.install_dir.resolve()
    if os.name == "nt":
        member = "umlgen-windows-amd64.exe"
        name = "umlgen-windows-amd64.zip"
        binary = install / "umlgen.exe"
        installer = ["powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
                     str(ROOT / "scripts/install-windows.ps1")]
        flags = ["-InstallDir", str(install), "-SkipPathUpdate"]
        directory_flag, version_flag = "-ArchiveDirectory", "-Version"
    elif platform.system() == "Darwin":
        architecture = "arm64" if platform.machine().lower() in ("arm64", "aarch64") else "amd64"
        member = f"umlgen-darwin-{architecture}"
        name = member + ".tar.gz"
        binary = install / "umlgen"
        installer = ["bash", str(ROOT / "scripts/install-macos.sh")]
        flags = ["--install-dir", str(install), "--skip-path-update"]
        directory_flag, version_flag = "--archive-dir", "--version"
    else:
        parser.error("installer acceptance requires native macOS or Windows")
    original = args.archive_dir.resolve() / name
    before_binary, before_path = digest(binary), user_path()
    with tempfile.TemporaryDirectory(prefix="umlgen-installer-smoke-") as temporary:
        directory = Path(temporary) / "archives"
        directory.mkdir()
        archive = directory / name
        checksums = directory / "SHA256SUMS.txt"
        for case in ("missing-version", "wrong-version", "duplicate-checksum", "corrupt-archive",
                     "traversal-member", "duplicate-member"):
            shutil.copyfile(original, archive)
            if case in ("traversal-member", "duplicate-member"):
                unsafe_archive(original, archive, member, case == "duplicate-member")
            checksum = f"{digest(archive)}  {name}\n"
            checksums.write_text(checksum * (2 if case == "duplicate-checksum" else 1), encoding="ascii")
            if case == "corrupt-archive":
                with archive.open("ab") as output:
                    output.write(b"tampered")
            command = [*installer, directory_flag, str(directory), *flags]
            if case != "missing-version":
                command += [version_flag, "0.0.0" if case == "wrong-version" else args.version]
            result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True,
                                    encoding="utf-8", errors="replace", timeout=60)
            if result.returncode == 0:
                raise RuntimeError(f"Installer accepted {case}: {result.stdout}\n{result.stderr}")
            if digest(binary) != before_binary or user_path() != before_path:
                raise RuntimeError(f"Rejected {case} changed the installed binary or user PATH")
            if list(install.glob(".umlgen-install-*")) or (directory.parent / "umlgen-escaped.txt").exists():
                raise RuntimeError(f"Rejected {case} leaked a staged/extracted file")
            print(f"PASS installer {case}: rejected, existing executable and PATH unchanged")


if __name__ == "__main__":
    main()
