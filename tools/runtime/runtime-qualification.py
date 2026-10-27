#!/usr/bin/env python3
"""Require live qualification whenever the enabled runtime/version set grows."""
import json
import os
import subprocess
import sys

path = "internal/core/runtime/versions.json"
current = json.load(open(path, encoding="utf-8"))
# The existing adapter predates qualification. This exact version alone retains
# its documented exception when the base branch has no version inventory yet.
legacy = {"opencode": "opencode v2.1.4"}
result = subprocess.run(["git", "show", f"{sys.argv[1]}:{path}"], capture_output=True, text=True)
if result.returncode:
    # A missing file is expected on the introduction PR; an unknown base is not.
    subprocess.run(["git", "rev-parse", "--verify", sys.argv[1]], check=True, capture_output=True)
    previous = legacy
else:
    previous = json.loads(result.stdout)
changed = [name for name, version in current.items() if previous.get(name) != version]
if not current or any(not name or not version for name, version in current.items()):
    raise SystemExit("runtime inventory must contain nonempty runtime/version pairs")
print("Live qualification required: " + (", ".join(changed) or "none (enabled versions unchanged)"))
if "GITHUB_OUTPUT" in os.environ:
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        output.write("required=" + ("true" if changed else "false") + "\n")
        output.write("runtimes=" + ",".join(changed) + "\n")
