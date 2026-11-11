#!/usr/bin/env python3
"""Check an installed package and optionally render real SVG/PNG images."""

import argparse
from pathlib import Path
import struct
import subprocess
import xml.etree.ElementTree as ET


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--render", action="store_true", help="also render SVG and PNG using the configured renderer")
    args = parser.parse_args()
    args.work_dir.mkdir(parents=True, exist_ok=True)
    source = args.work_dir / "PackageSmoke.java"
    source.write_text("public class PackageSmoke { private String name; }\n", encoding="utf-8")
    version = subprocess.run([args.binary, "version"], capture_output=True, text=True, check=True, timeout=60)
    if not version.stdout.startswith(f"umlgen version {args.version}\n"):
        raise SystemExit(f"unexpected installed version: {version.stdout!r}")
    output = args.work_dir / "class.puml"
    subprocess.run([args.binary, "class", str(source), "--output", str(output), "--quiet"], check=True, timeout=60)
    diagram_bytes = output.read_bytes()
    diagram = diagram_bytes.decode("utf-8")
    if not diagram.startswith("@startuml\n") or "PackageSmoke" not in diagram or not diagram.rstrip().endswith("@enduml"):
        raise SystemExit("installed package did not generate the expected class diagram")
    print(f"PASS installed package {args.version}: version and Java class diagram")
    if args.render:
        for format in ("svg", "png"):
            rendered = args.work_dir / f"class-{format}.puml"
            subprocess.run([args.binary, "class", str(source), "--format", format,
                            "--output", str(rendered), "--quiet"], check=True, timeout=120)
            if rendered.read_bytes() != diagram_bytes:
                raise SystemExit(f"{format} rendering changed the class diagram")
            image = rendered.with_suffix(f".{format}").read_bytes()
            if format == "svg":
                root = ET.fromstring(image)
                if "PackageSmoke" not in "".join(root.itertext()):
                    raise SystemExit("rendered SVG does not contain the class")
            else:
                if len(image) < 33 or not image.startswith(b"\x89PNG\r\n\x1a\n") or image[12:16] != b"IHDR":
                    raise SystemExit("renderer did not produce a PNG image")
                width, height = struct.unpack(">II", image[16:24])
                if width == 0 or height == 0:
                    raise SystemExit("rendered PNG has an empty image size")
            print(f"PASS installed package {args.version}: actual {format.upper()} image, unchanged PlantUML")


if __name__ == "__main__":
    main()
