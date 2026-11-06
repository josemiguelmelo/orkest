import unittest
from check_dependencies import forbidden, violations


class DependencyRulesTest(unittest.TestCase):
    def test_rejects_outward_and_cross_context_imports(self):
        for source, target in [
            ('server/task', 'adapters/postgres'), ('agentctl/run', 'adapters/runtime/acp'),
            ('server/orchestration', 'agentctl/run'), ('agentctl/run', 'server/task'),
            ('contracts/agentctl/v1', 'agentctl/config'), ('contracts/agentctl/v1', 'platform/ids'),
            ('platform/ids', 'server/task'), ('platform/ids', 'contracts/agentctl/v1'),
        ]:
            with self.subTest(source=source, target=target):
                self.assertTrue(forbidden('internal/' + source, 'internal/' + target))
        self.assertTrue(forbidden('internal/contracts/agentctl/v1', 'github.com/coder/acp-go-sdk'))
        self.assertTrue(forbidden('internal/contracts/agentctl/v1', 'net/http'))
        self.assertTrue(forbidden('internal/platform/ids', 'github.com/jackc/pgx/v5'))

    def test_accepts_inward_dependencies(self):
        for source, target in [('cmd/server', 'internal/adapters/http/server'),
                               ('internal/adapters/postgres', 'internal/server/task'),
                               ('internal/server/task', 'internal/platform/ids'),
                               ('internal/agentctl/run', 'internal/agentctl/runtime'),
                               ('internal/contracts/agentctl/v1', 'encoding/json')]:
            self.assertFalse(forbidden(source, target))

    def test_exact_exceptions_do_not_allow_new_violations(self):
        packages = [{'ImportPath': 'internal/server/task', 'Imports': ['internal/adapters/postgres']}]
        key = 'internal/server/task -> internal/adapters/postgres'
        self.assertEqual(violations(packages, {}, {}), [key])
        self.assertEqual(violations(packages, {}, {key: 'phase 4'}), [])
        self.assertEqual(violations([], {}, {key: 'phase 4'}), ['stale exception: ' + key])
