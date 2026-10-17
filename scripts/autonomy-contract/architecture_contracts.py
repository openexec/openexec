"""Check the discovery document against this checkout; no deployment claims."""
import re
import sys
from pathlib import Path

SECTIONS = (
    'Evidence scope', 'Restored resources', 'Module and API map',
    'Producer, persistence and reload', 'Binding validation',
    'Native recovery and receipts', 'Related tests and verification',
    'Historical claims and external evidence',
)
RESTORED = (
    'pkg/runtime/execution.go', 'internal/execution/gates/failure.go',
    'pkg/manager/task_failure.go',
)
REQUIRED_SOURCES = (
    'pkg/execution/execution.go', 'internal/cli/execution_stdio.go',
    'internal/blueprint/executor.go', 'internal/blueprint/engine.go',
    'internal/execution/gates/runner.go', 'pkg/manager/scheduler.go',
    'pkg/manager/events.go', 'pkg/manager/checkpoints.go',
    'pkg/manager/manager.go', 'pkg/db/state/store.go',
    'pkg/db/state/verification.go',
)


def verify(root):
    doc = root / 'docs/ARCHITECTURE.md'
    content = doc.read_text()
    for section in SECTIONS:
        if not re.search(r'^## ' + re.escape(section) + r'\n\s*\S', content, re.M):
            raise ValueError(f'missing or empty section: {section}')
    for path in RESTORED:
        if f'`{path}`' not in content:
            raise ValueError(f'missing restored resource observation: {path}')
        if not (root / path).is_file():
            raise ValueError(f'missing restored resource: {path}')
    refs = re.findall(r'\]\((\.\./[^)]+)\)', content)
    seen = set()
    for ref in refs:
        path, separator, symbol = ref.partition('#')
        source = (doc.parent / path).resolve()
        source.relative_to(root)
        if not separator or not re.fullmatch(r'\w+', symbol):
            raise ValueError(f'source reference needs declaration: {ref}')
        text = source.read_text()
        # Optional receiver followed by the exact declared identifier.
        declaration = r'^(?:type\s+|func\s+(?:\([^\n]*\)\s+)?)'
        if not re.search(declaration + re.escape(symbol) + r'\b', text, re.M):
            raise ValueError(f'unresolved declaration: {ref}')
        seen.add(source.relative_to(root).as_posix())
    missing = (set(REQUIRED_SOURCES) | set(RESTORED)) - seen
    if missing:
        raise ValueError(f'missing required source references: {sorted(missing)}')
    return len(refs)


if __name__ == '__main__':
    try:
        count = verify(Path(sys.argv[1]).resolve())
    except (OSError, ValueError, IndexError) as error:
        print(f'architecture-contracts: FAIL: {error}', file=sys.stderr)
        sys.exit(1)
    print(f'architecture-contracts: PASS ({count} source declarations; discovery only)')
