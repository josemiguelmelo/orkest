#!/usr/bin/env python3
"""Check actual Go imports, using go list's parser and build selection."""
import json
import re
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
PREFIX = 'github.com/josemiguelmelo/orkest/'


def layer(path):
    for name, prefix in [('server', 'internal/server/'), ('worker', 'internal/agentctl/'),
                         ('adapter', 'internal/adapters/'), ('contract', 'internal/contracts/'),
                         ('platform', 'internal/platform/'), ('command', 'cmd/')]:
        if path.startswith(prefix):
            return name
    return None


def forbidden(source, target):
    owner, dependency = layer(source), layer(target)
    if source.endswith('/model') and dependency in ('server', 'worker') and not target.endswith('/model'):
        return True
    if owner in ('server', 'worker'):
        return dependency in ('adapter', 'command') or (owner, dependency) in (
            ('server', 'worker'), ('worker', 'server'))
    if owner == 'contract':
        if target in ('os', 'os/exec', 'syscall', 'net/http', 'database/sql'):
            return True
        return dependency in ('server', 'worker', 'adapter', 'platform', 'command') or (
            not target.startswith('internal/') and '.' in target.split('/')[0])
    if owner == 'platform':
        if any(target.startswith(prefix) for prefix in ('github.com/jackc/pgx', 'github.com/moby/', 'github.com/coder/acp-go-sdk')):
            return True
        return dependency in ('server', 'worker', 'adapter', 'contract', 'command')
    return False


def decode_packages(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        package, end = decoder.raw_decode(raw.lstrip())
        yield package
        raw = raw.lstrip()[end:]


def violations(packages, mapping, exceptions):
    failures, seen = [], set()
    for package in packages:
        old_source = package['ImportPath'].removeprefix(PREFIX)
        source = mapping.get(old_source, old_source)
        for name in package.get('Imports', []):
            old_target = name.removeprefix(PREFIX)
            target = mapping.get(old_target, old_target)
            if forbidden(source, target):
                key = old_source + ' -> ' + old_target
                seen.add(key)
                if key not in exceptions:
                    failures.append(key)
    failures.extend('stale exception: ' + key for key in exceptions if key not in seen)
    return failures


def main():
    mapping = json.loads((ROOT / 'tools/architecture/package_mapping.json').read_text())
    exceptions = json.loads((ROOT / 'tools/architecture/exceptions.json').read_text())
    raw = subprocess.check_output(['go', 'list', '-json', './cmd/...', './internal/...', './db/...'], cwd=ROOT, text=True)
    packages=list(decode_packages(raw))
    failures = violations(packages, mapping, exceptions)
    for package in packages:
        source=package['ImportPath'].removeprefix(PREFIX)
        if source in mapping and mapping[source]!=source:
            failures.append('superseded package: '+source)
        if source.startswith('internal/server/'):
            for filename in package.get('GoFiles',[]):
                content=(Path(package['Dir'])/filename).read_text()
                if re.search(r'(?m)^type (Service|Repository)\b',content):
                    failures.append(source+': use domain-specific exported names')
    if failures:
        print('\n'.join(failures), file=sys.stderr)
        return 1
    print(f'Go dependency boundaries pass ({len(exceptions)} temporary exceptions).')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
