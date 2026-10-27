"""Exercise qualification decisions against real base revisions."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("runtime-qualification.py").resolve()


class QualificationTest(unittest.TestCase):
    def decision(self, previous, current):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(["git", "init", "-q", directory], check=True)
            inventory = root / "internal/core/runtime/versions.json"
            inventory.parent.mkdir(parents=True)
            if previous is not None:
                inventory.write_text(json.dumps(previous), encoding="utf-8")
                subprocess.run(["git", "-C", directory, "add", "."], check=True)
            subprocess.run(["git", "-C", directory, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-qm", "base"], check=True)
            inventory.write_text(json.dumps(current), encoding="utf-8")
            output = root / "output"
            subprocess.run(["python3", str(SCRIPT), "HEAD"], cwd=root, env={**os.environ, "GITHUB_OUTPUT": str(output)}, check=True, capture_output=True)
            return output.read_text(encoding="utf-8")

    def test_existing_version_keeps_its_introduction_exception(self):
        self.assertIn("required=false", self.decision(None, {"opencode": "opencode v2.1.4"}))

    def test_version_upgrade_requires_live_tests(self):
        result = self.decision({"opencode": "opencode v2.1.4"}, {"opencode": "opencode v2.1.5"})
        self.assertIn("required=true", result)
        self.assertIn("runtimes=opencode", result)

    def test_new_runtime_does_not_inherit_the_exception(self):
        result = self.decision(None, {"opencode": "opencode v2.1.4", "new": "new v1"})
        self.assertIn("required=true", result)
        self.assertIn("runtimes=new", result)

    def test_unchanged_qualified_inventory_needs_no_requalification(self):
        versions = {"opencode": "opencode v2.1.5"}
        self.assertIn("required=false", self.decision(versions, versions))
