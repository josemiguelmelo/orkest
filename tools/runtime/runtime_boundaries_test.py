import importlib.util
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("check_runtime_boundaries.py").resolve()
SPEC = importlib.util.spec_from_file_location("check_runtime_boundaries", SCRIPT)
BOUNDARIES = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BOUNDARIES)


class RuntimeBoundaryTests(unittest.TestCase):
    def test_pure_profile_is_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "profile.go"
            path.write_text('package profile\nimport "path/filepath"\nfunc pathFor(s string) string { return filepath.Clean(s) }\n', encoding="utf-8")
            self.assertEqual(BOUNDARIES.check_file(path, "profile"), [])

    def test_profile_filesystem_io_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "profile.go"
            path.write_text('package profile\nimport "os"\nfunc write() { os.WriteFile("x", nil, 0600) }\n', encoding="utf-8")
            self.assertTrue(BOUNDARIES.check_file(path, "profile"))

    def test_communication_process_and_persistence_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "client.go"
            path.write_text('package client\nimport "os/exec"\nfunc start() { exec.Command("agent") }\n', encoding="utf-8")
            self.assertTrue(BOUNDARIES.check_file(path, "communication"))

    def test_runtime_implementations_cannot_import_the_host(self):
        for kind in ("profile", "communication", "application", "definition"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "example.go"
                path.write_text(
                    'package example\nimport "github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"\n',
                    encoding="utf-8",
                )
                self.assertTrue(BOUNDARIES.check_file(path, kind))

    def test_host_depends_on_contracts_without_concrete_runtimes(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "host.go"
            path.write_text(
                'package host\nimport "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"\n',
                encoding="utf-8",
            )
            self.assertEqual(BOUNDARIES.check_file(path, "host"), [])
            path.write_text(
                'package host\nimport "github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"\n',
                encoding="utf-8",
            )
            self.assertTrue(BOUNDARIES.check_file(path, "host"))

    def test_domain_scan_rejects_dependencies_on_concrete_implementations(self):
        for domain in ("config", "interaction", "runtime", "run"):
            with self.subTest(domain=domain), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                package = root / "internal/agentctl" / domain
                package.mkdir(parents=True)
                source = package / "example.go"
                source.write_text(
                    f'package {domain}\nimport "github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"\n',
                    encoding="utf-8",
                )
                self.assertTrue(BOUNDARIES.violations(root))

    def test_runtime_implementation_cannot_depend_on_run_service(self):
        for kind in ("profile", "communication", "host", "definition"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "example.go"
                path.write_text(
                    'package example\nimport "github.com/josemiguelmelo/orkest/internal/agentctl/run"\n',
                    encoding="utf-8",
                )
                self.assertTrue(BOUNDARIES.check_file(path, kind))

    def test_all_profiles_including_kiro_are_scanned(self):
        for runtime in ("opencode", "claudecode", "codex", "githubcopilot", "kiro"):
            with self.subTest(runtime=runtime), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                package = root / "internal/adapters/runtime/profiles" / runtime
                package.mkdir(parents=True)
                (package / "profile.go").write_text(
                    'package profile\nimport "github.com/josemiguelmelo/orkest/internal/server/task"\n',
                    encoding="utf-8",
                )
                self.assertTrue(BOUNDARIES.violations(root))

    def test_flat_root_package_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            package = root / "internal/agentctl"
            package.mkdir(parents=True)
            (package / "example.go").write_text("package agentctl\n", encoding="utf-8")
            self.assertTrue(BOUNDARIES.violations(root))


if __name__ == "__main__":
    unittest.main()
