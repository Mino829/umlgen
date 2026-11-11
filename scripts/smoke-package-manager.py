#!/usr/bin/env python3
"""Check the package-manager-installed command without JDK or renderer dependencies."""

import argparse
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--work-dir", required=True, type=Path)
    args = parser.parse_args()
    args.work_dir.mkdir(parents=True, exist_ok=True)
    source = args.work_dir / "PackageSmoke.java"
    source.write_text("public class PackageSmoke { private String name; }\n", encoding="utf-8")
    version = subprocess.run([args.binary, "version"], capture_output=True, text=True, check=True, timeout=60)
    if not version.stdout.startswith(f"umlgen version {args.version}\n"):
        raise SystemExit(f"unexpected installed version: {version.stdout!r}")
    output = args.work_dir / "class.puml"
    subprocess.run([args.binary, "class", str(source), "--output", str(output), "--quiet"], check=True, timeout=60)
    diagram = output.read_text(encoding="utf-8")
    if not diagram.startswith("@startuml\n") or "PackageSmoke" not in diagram or not diagram.rstrip().endswith("@enduml"):
        raise SystemExit("installed package did not generate the expected class diagram")
    print(f"PASS installed package {args.version}: version and Java class diagram")


if __name__ == "__main__":
    main()
