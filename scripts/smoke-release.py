#!/usr/bin/env python3
"""Accept an installed native release binary, not a source-tree development build."""

import argparse
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]


def run(command, *, cwd=ROOT, expected=0):
    result = subprocess.run([str(arg) for arg in command], cwd=cwd,
                            capture_output=True, text=True, encoding="utf-8", timeout=120)
    if result.returncode != expected:
        raise RuntimeError(f"Exit {result.returncode}, expected {expected}: {command}\n"
                           f"{result.stdout}\n{result.stderr}")
    return result


def diagram(binary, root, name, arguments, markers, *, cwd=ROOT, expected=0):
    output = root / f"{name}.puml"
    result = run([binary, *arguments, "--explain", "--output", output],
                 cwd=cwd, expected=expected)
    (root / f"{name}.log").write_text(result.stdout + result.stderr, encoding="utf-8")
    text = output.read_text(encoding="utf-8")
    if not text.startswith("@startuml\n") or not text.rstrip().endswith("@enduml"):
        raise RuntimeError(f"Invalid PlantUML boundaries: {name}")
    for marker in markers:
        if marker not in text:
            raise RuntimeError(f"Missing behavior {marker!r} in {name}:\n{text}")
    print(f"PASS {name}: exit {expected}, diagram semantics")
    return text, result.stdout + result.stderr


def immutable_diff(binary, root):
    project = root / "git-project"
    project.mkdir()
    (project / "model").mkdir()
    (project / "go.mod").write_text("module example.test/release\n\ngo 1.25\n", encoding="utf-8")
    (project / "model" / "item.go").write_text("package model\ntype Item struct{}\n", encoding="utf-8")
    app = project / "app.go"
    app.write_text('package app\nimport "example.test/release/model"\n'
                   'type Before struct { Item model.Item }\n', encoding="utf-8")
    run(["git", "-c", "init.templateDir=", "init", "--quiet"], cwd=project)

    def commit(message):
        run(["git", "add", "--all"], cwd=project)
        run(["git", "-c", "user.name=Release Smoke", "-c", "user.email=release@example.invalid",
             "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", message], cwd=project)

    commit("before")
    (project / "model" / "item.go").unlink()
    (project / "account").mkdir()
    (project / "account" / "account.go").write_text("package account\ntype Account struct{}\n", encoding="utf-8")
    app.write_text('package app\nimport "example.test/release/account"\n'
                   'type After struct { Item account.Account }\n', encoding="utf-8")
    commit("after")
    diagram(binary, root, "class-diff", ["diff", "HEAD~1..HEAD", ".", "--language", "go"],
            ["After", "Account", "Item", "#palegreen", "#lightcoral"], cwd=project)
    args = ["diff", "HEAD~1..HEAD", ".", "--diagram", "package", "--language", "go",
            "--dependency-source", "both"]
    clean, _ = diagram(binary, root, "package-diff", args, ["model", "account"], cwd=project)
    app.write_text("package DirtyPackage\ntype DirtyType struct{}\n", encoding="utf-8")
    (project / "go.mod").write_text("module example.test/dirty\n\ngo 1.25\n", encoding="utf-8")
    (project / "untracked.go").write_text("package DirtyPackage\ntype UntrackedType struct{}\n", encoding="utf-8")
    dirty, explanation = diagram(binary, root, "package-diff-dirty", args,
                                  ["model", "account"], cwd=project)
    if clean != dirty or any(value in dirty + explanation for value in
                             ("DirtyPackage", "DirtyType", "UntrackedType", "example.test/dirty")):
        raise RuntimeError("Package diff included mutable working-tree content")
    print("PASS package diff: immutable commits despite dirty source, go.mod and untracked file")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--work-dir", type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve()
    root = args.work_dir.resolve()
    root.mkdir(parents=True, exist_ok=True)
    version = run([binary, "version"]).stdout.strip()
    if version != f"umlgen version {args.version}":
        raise RuntimeError(f"Installed version mismatch: {version!r}")
    print(f"PASS installed version: {version}")

    for language in ("go", "java"):
        class_target = "testdata/go/compatibility" if language == "go" else "testdata/java/compatibility/project"
        class_args = ["class", class_target, "--language", language]
        cold, _ = diagram(binary, root, f"{language}-class", class_args,
                          ["UserService", "<|.."])
        warm, _ = diagram(binary, root, f"{language}-class-warm", class_args,
                          ["UserService", "<|.."])
        uncached, _ = diagram(binary, root, f"{language}-class-uncached",
                              [*class_args, "--no-cache"], ["UserService", "<|.."])
        if cold != warm or cold != uncached:
            raise RuntimeError(f"{language} class cache changed diagram semantics")
        diagram(binary, root, f"{language}-package",
                ["package", f"testdata/package/{language}", "--language", language],
                ["declared cycle in scope", "types=1"])
        semantic = ["--java-release", "17"] if language == "java" else []
        diagram(binary, root, f"{language}-sequence",
                ["sequence", f"testdata/sequence/{language}", "--language", language,
                 "--entry", "PlaceOrder", *semantic],
                ["participant ", "pricing" if language == "go" else "Pricing", " -> "])
        diagram(binary, root, f"{language}-activity",
                ["activity", f"testdata/activity/{language}", "--language", language,
                 "--function", "ProcessOrder" if language == "go" else "processOrder", *semantic],
                ["decision", "[true]", "[return]"])
        diagram(binary, root, f"{language}-state",
                ["state", f"testdata/state/{language}", "--language", language,
                 "--state-config", f"testdata/state/{language}/machine.yaml", *semantic],
                ["configured initial", "configured<U+003A>", "Unknown previous state"])
        diagram(binary, root, f"{language}-component",
                ["component", f"testdata/package/{language}", "--language", language,
                 "--boundary-config", f"testdata/component/{language}/architecture.yaml"],
                ["Application", "configured provides", "provider binding not specified"])

    for kind, expected, markers in (
            ("compose", 0, ["declared Compose service", "declared image"]),
            ("kubernetes", 3, ["desired replicas 2 (not observed)", "unknown endpoint"])):
        text, explanation = diagram(binary, root, f"{kind}-deployment",
                ["deployment", "--manifest", f"testdata/deployment/{kind}.yaml",
                 "--boundary-config", "testdata/deployment/architecture.yaml",
                 "--mapping-config", f"testdata/deployment/{kind}-mapping.yaml"],
                markers, expected=expected)
        visible = re.sub(r"<U\+([0-9A-F]{4,6})>", lambda match: chr(int(match[1], 16)),
                         text + explanation)
        for private_value in ("fixture-only-do-not-display", "fixture-command-never-executed", "EXAMPLE_PRIVATE_VALUE"):
            if private_value in visible:
                raise RuntimeError(f"Deployment leaked private manifest data: {private_value}")
    immutable_diff(binary, root)
    result = run([sys.executable, ROOT / "scripts" / "smoke-watch.py", "--binary", binary,
                  "--work-dir", root / "watch"])
    print(result.stdout, end="")
    print("PASS release acceptance: installed binary, all diagram kinds, cache, Git diff, privacy, watch")


if __name__ == "__main__":
    main()
