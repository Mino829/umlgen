"""Offline regression tests for unsafe package-manager update inputs."""

import copy
import io
import json
from pathlib import Path
import runpy
import tempfile
import unittest
from unittest.mock import patch


UPDATER = runpy.run_path(str(Path(__file__).with_name("update-package-managers.py")))
ARCHIVES = UPDATER["ARCHIVES"]
VERSION = "0.6.0"
BASE = f"{UPDATER['HOMEPAGE']}/releases/download/v{VERSION}"


def manifest_text():
    return "".join(f"{'a' * 64}  {name}\n" for name in ARCHIVES)


def release_metadata():
    return {
        "tag_name": f"v{VERSION}", "draft": False, "prerelease": False,
        "assets": [{"name": name, "browser_download_url": f"{BASE}/{name}"}
                   for name in (*ARCHIVES, "SHA256SUMS.txt")],
    }


class PackageManagerInputs(unittest.TestCase):
    def test_exact_stable_version_only(self):
        validate = UPDATER["stable_version"]
        self.assertEqual(validate("v0.6.0"), VERSION)
        self.assertEqual(validate(VERSION), VERSION)
        for value in ("latest", "v0.06.0", "0.6", "0.6.0-rc.1", "0.6.0+build", "0.6.0\n", "../0.6.0"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate(value)

    def test_strict_complete_checksums(self):
        parse = UPDATER["parse_checksums"]
        self.assertEqual(set(parse(manifest_text())), set(ARCHIVES))
        duplicate = f"{'b' * 64}  {ARCHIVES[0]}\n"
        for text in (
            manifest_text() + duplicate,
            manifest_text() + manifest_text().splitlines(keepends=True)[0],
            "".join(manifest_text().splitlines(keepends=True)[1:]),
            manifest_text().replace("a" * 64, "a" * 63, 1),
            manifest_text().replace("a" * 64, "g" * 64, 1),
            manifest_text().replace(ARCHIVES[0], "../" + ARCHIVES[0]),
            manifest_text() + "\n",
            "",
        ):
            with self.subTest(text=text), self.assertRaises(ValueError):
                parse(text)

    def test_release_must_match_published_exact_tag(self):
        validate = UPDATER["validate_release"]
        self.assertEqual(len(validate(release_metadata(), VERSION)), 5)
        for field, value in (("draft", True), ("prerelease", True), ("tag_name", "v0.7.0")):
            metadata = release_metadata()
            metadata[field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                validate(metadata, VERSION)
        for mutation in ("missing", "duplicate", "foreign-url"):
            metadata = release_metadata()
            if mutation == "missing":
                metadata["assets"].pop()
            elif mutation == "duplicate":
                metadata["assets"].append(copy.deepcopy(metadata["assets"][0]))
            else:
                metadata["assets"][0]["browser_download_url"] = "https://example.com/package.tar.gz"
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                validate(metadata, VERSION)

    def test_download_and_api_hashes_must_agree(self):
        verify = UPDATER["verify_digest"]
        asset = {"name": ARCHIVES[0], "digest": "sha256:" + "a" * 64}
        verify(asset, "a" * 64, "a" * 64)
        with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
            verify(asset, "b" * 64, "a" * 64)
        with self.assertRaisesRegex(ValueError, "API digest mismatch"):
            verify(asset, "b" * 64, "b" * 64)

    def test_bad_download_preserves_both_definitions(self):
        metadata = release_metadata()
        checksums = manifest_text().encode("ascii")
        responses = [io.BytesIO(json.dumps(metadata).encode()), io.BytesIO(checksums), io.BytesIO(b"corrupt archive")]
        globals_ = UPDATER["main"].__globals__
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("Formula/umlgen.rb", "bucket/umlgen.json"):
                (root / name).parent.mkdir()
                (root / name).write_text("original", encoding="utf-8")
            with patch.dict(globals_, request=lambda *args, **kwargs: responses.pop(0)), patch(
                "sys.argv", ["update-package-managers.py", "--version", VERSION, "--output-dir", directory]
            ), self.assertRaises(SystemExit) as result:
                UPDATER["main"]()
            self.assertEqual(result.exception.code, 1)
            for name in ("Formula/umlgen.rb", "bucket/umlgen.json"):
                self.assertEqual((root / name).read_text(encoding="utf-8"), "original")

    def test_manifest_digest_is_verified_before_archive_download(self):
        metadata = release_metadata()
        metadata["assets"][-1]["digest"] = "sha256:" + "0" * 64
        responses = [io.BytesIO(json.dumps(metadata).encode()), io.BytesIO(manifest_text().encode())]
        with patch.dict(UPDATER["released_checksums"].__globals__, request=lambda *args, **kwargs: responses.pop(0)):
            with self.assertRaisesRegex(ValueError, "API digest mismatch"):
                UPDATER["released_checksums"](VERSION)
        self.assertEqual(responses, [])


if __name__ == "__main__":
    unittest.main()
