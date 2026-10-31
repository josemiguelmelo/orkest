#!/usr/bin/env python3
"""Enforce agentctl domain and concrete runtime dependency boundaries."""

from pathlib import Path
import re
import sys


PROFILE_FORBIDDEN_IMPORTS = (
    "os", "os/exec", "syscall", "net/http", "internal/postgres",
    "internal/orchestration", "internal/app", "internal/core/durablefile",
    "internal/agentctl/runtime/acp", "internal/agentctl/runtimehost",
    "internal/agentctl/run", "internal/agentctl/httpapi",
    "internal/agentctl/runtime/opencode", "internal/agentctl/runtime/claudecode",
    "internal/agentctl/runtime/codex",
    "internal/agentctl/runtime/githubcopilot",
)
COMMUNICATION_FORBIDDEN_IMPORTS = (
    "os", "os/exec", "syscall", "path/filepath", "net/http",
    "internal/postgres", "internal/orchestration", "internal/app",
    "internal/core/durablefile", "internal/agentctl/runtime/opencode",
    "internal/agentctl/runtime/claudecode",
    "internal/agentctl/runtime/codex",
    "internal/agentctl/runtime/githubcopilot",
    "internal/agentctl/runtimehost",
    "internal/agentctl/run", "internal/agentctl/httpapi",
)
HOST_FORBIDDEN_IMPORTS = (
    "internal/postgres", "internal/orchestration", "internal/app",
    "internal/agentctl/httpapi", "internal/agentctl/runtime/acp",
    "internal/agentctl/runtime/opencode",
    "internal/agentctl/runtime/claudecode",
    "internal/agentctl/runtime/codex",
    "internal/agentctl/runtime/githubcopilot",
    "internal/agentctl/run",
)
APPLICATION_FORBIDDEN_IMPORTS = (
    "internal/agentctl/runtimehost", "internal/agentctl/runtime/acp",
    "internal/agentctl/runtime/opencode",
    "internal/agentctl/runtime/claudecode",
    "internal/agentctl/runtime/codex",
    "internal/agentctl/runtime/githubcopilot",
)
DEFINITION_FORBIDDEN_IMPORTS = (
    "os", "os/exec", "syscall", "net/http", "internal/postgres",
    "internal/orchestration", "internal/app", "internal/core/durablefile",
    "internal/agentctl/run", "internal/agentctl/httpapi",
    "internal/agentctl/runtimehost", "internal/agentctl/runtime/acp",
    "internal/agentctl/runtime/opencode", "internal/agentctl/runtime/claudecode",
    "internal/agentctl/runtime/codex",
    "internal/agentctl/runtime/githubcopilot",
)
MODULE_PREFIX = "github.com/josemiguelmelo/orkest/"
OPERATIONAL_CALLS = re.compile(
    r"\b(?:exec\.(?:Command|CommandContext|LookPath)|StartChildProcess|"
    r"os\.(?:Mkdir|MkdirAll|Open|OpenFile|ReadFile|WriteFile|Remove)|"
    r"syscall\.)"
)


def imports(source: str) -> list[str]:
    return re.findall(r'^\s*(?:[\w.]+\s+)?"([^"]+)"', source, re.MULTILINE)


def check_file(path: Path, kind: str) -> list[str]:
    source = path.read_text(encoding="utf-8")
    forbidden = {
        "profile": PROFILE_FORBIDDEN_IMPORTS,
        "communication": COMMUNICATION_FORBIDDEN_IMPORTS,
        "host": HOST_FORBIDDEN_IMPORTS,
        "application": APPLICATION_FORBIDDEN_IMPORTS,
        "definition": DEFINITION_FORBIDDEN_IMPORTS,
    }[kind]
    failures = []
    for name in imports(source):
        local_name = name.removeprefix(MODULE_PREFIX)
        if any(local_name == item or local_name.startswith(item + "/") for item in forbidden):
            failures.append(f"{path}: forbidden import {name}")
    if kind == "communication" and OPERATIONAL_CALLS.search(source):
        failures.append(f"{path}: process or filesystem operation in communication implementation")
    if kind == "profile" and OPERATIONAL_CALLS.search(source):
        failures.append(f"{path}: operational I/O in runtime profile")
    return failures


def violations(root: Path) -> list[str]:
    targets = (
        (root / "internal/agentctl/runtime/opencode", "profile"),
        (root / "internal/agentctl/runtime/claudecode", "profile"),
        (root / "internal/agentctl/runtime/codex", "profile"),
        (root / "internal/agentctl/runtime/githubcopilot", "profile"),
        (root / "internal/agentctl/runtime/acp", "communication"),
        (root / "internal/agentctl/runtimehost", "host"),
        (root / "internal/agentctl/run", "application"),
        (root / "internal/agentctl/config", "definition"),
        (root / "internal/agentctl/interaction", "definition"),
    )
    failures: list[str] = []
    for directory, kind in targets:
        for path in sorted(directory.rglob("*.go")):
            if path.name.endswith("_test.go") or "opencodetest" in path.parts:
                continue
            failures.extend(check_file(path, kind))
    # Only the runtime package root contains contracts; child adapters have
    # their own rules above and may perform protocol or native rendering work.
    for path in sorted((root / "internal/agentctl/runtime").glob("*.go")):
        if not path.name.endswith("_test.go"):
            failures.extend(check_file(path, "definition"))
    for path in sorted((root / "internal/agentctl").glob("*.go")):
        failures.append(f"{path}: place Go code in an agentctl domain package")
    return failures


def main() -> int:
    root = Path(__file__).resolve().parents[2]
    failures = violations(root)
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("Agentctl runtime boundaries are clean.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
