#!/usr/bin/env python3
"""Exercise the real watch update/delete/cancellation lifecycle on native runners."""

import argparse
import os
from pathlib import Path
import signal
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--work-dir", type=Path, required=True)
    args = parser.parse_args()
    root = args.work_dir.resolve()
    source = root / "src"
    source.mkdir(parents=True, exist_ok=False)
    code = source / "app.go"
    code.write_text("package app\ntype WatchInitial struct{}\n", encoding="utf-8")
    output = root / "model.puml"
    errors_path = root / "stderr.log"
    flags = subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0
    with errors_path.open("w", encoding="utf-8") as errors:
        process = subprocess.Popen(
            [str(args.binary.resolve()), "watch", str(source), "--language", "go",
             "--interval", "25ms", "--debounce", "75ms", "--quiet",
             "--output", str(output)],
            stdout=subprocess.DEVNULL, stderr=errors, creationflags=flags)

        def await_output(predicate):
            deadline = time.monotonic() + 30
            while True:
                if process.poll() is not None:
                    raise RuntimeError(errors_path.read_text(encoding="utf-8"))
                text = output.read_text(encoding="utf-8") if output.exists() else ""
                if predicate(text):
                    return
                if time.monotonic() >= deadline:
                    raise RuntimeError(f"Watch update deadline exceeded: {text}\n"
                                       f"{errors_path.read_text(encoding='utf-8')}")
                time.sleep(0.025)

        try:
            await_output(lambda text: "WatchInitial" in text)
            code.write_text("package app\ntype WatchUpdated struct{}\n", encoding="utf-8")
            await_output(lambda text: "WatchUpdated" in text and "WatchInitial" not in text)
            code.unlink()
            await_output(lambda text: "@enduml" in text and "WatchUpdated" not in text)
            process.send_signal(signal.CTRL_BREAK_EVENT if os.name == "nt" else signal.SIGINT)
            if process.wait(timeout=10) != 0:
                raise RuntimeError(errors_path.read_text(encoding="utf-8"))
            if list(root.glob(".umlgen-watch-*")):
                raise RuntimeError("Watch cancellation left unpublished staging files")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=10)
    print("PASS watch: initial, update, final source removal, native cancellation, stage cleanup")


if __name__ == "__main__":
    main()
