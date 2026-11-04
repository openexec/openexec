#!/usr/bin/env python3
"""Prove each readable-root identity guard with immutable Go source overlays.

For every guard: the named tests pass on the real source (restored-green), then
fail on a copy with only that guard disabled (removal-red). The failure must be
a runtime failure of the named test, never a build failure, and no other
top-level test may fail.
"""
import json, os, pathlib, re, subprocess, tempfile

root = pathlib.Path(__file__).resolve().parents[1]
env = dict(os.environ, GOWORK='off')
EXEC, CLI = 'pkg/execution', 'internal/cli'
CONNECTED_UNSAFE = 'TestConnectedUnsafeRootsFailSafely'
CONNECTED_VALID = 'TestConnectedValidRootServesReadListSearchThroughPinnedDescriptor'
CONNECTED_V4 = 'TestConnectedV4RefusesMissingIdentityAndProviderThatCannotEnforce'
PINS = 'TestPinReadableRootsRefusesBadIdentities'

# label, directory, file, before, after, failing test, assertion fragment
cases = [
 ('v4-requires-identity', CLI, 'execution_stdio.go',
  'if request.Version == executionProtocolBoundRoots && (len(roots) == 0 ||',
  'if false && (len(roots) == 0 ||',
  CONNECTED_V4, 'requires an identity'),
 ('identity-requires-v4', CLI, 'execution_stdio.go',
  'if request.Version != executionProtocolBoundRoots && len(identities) > 0 {',
  'if false && len(identities) > 0 {',
  'TestConnectedV1ToV3KeepTheirBehaviour', 'accepted identities'),
 ('provider-must-enforce', CLI, 'execution_stdio.go',
  'if !ok || !bound.BoundReadableRootsAvailable() {',
  'if false && (!ok || !bound.BoundReadableRootsAvailable()) {',
  CONNECTED_V4, 'provider cannot enforce'),
 ('version-ceiling', CLI, 'execution_stdio.go',
  'request.Version < executionProtocolVersionLegacy || request.Version > newest {',
  'request.Version < executionProtocolVersionLegacy {',
  'TestConnectedRuntimeWithoutV4RefusesV4Request', 'did not refuse v4'),
 ('platform-support-advertised', EXEC, 'api_provider.go',
  'return ok && readableRootIdentitySupported()',
  'return ok',
  'TestUnsupportedPlatformRefusesBoundRootsWithoutFallback', 'advertised bound roots'),
 ('executor-cannot-bind', EXEC, 'api_provider.go',
  '} else if len(request.ReadableRootIdentities) > 0 {\n\t\t\treturn Result{}, errors.New("tool executor cannot bind',
  '} else if false {\n\t\t\treturn Result{}, errors.New("tool executor cannot bind',
  'TestAPIProviderRefusesIdentitiesItCannotBind', None),
 ('identities-without-roots', EXEC, 'api_provider.go',
  '} else if len(request.ReadableRootIdentities) > 0 {\n\t\treturn Result{}, errors.New("readable root identities without roots")',
  '} else if false {\n\t\treturn Result{}, errors.New("readable root identities without roots")',
  'TestAPIProviderRefusesIdentitiesItCannotBind', 'err=execution ID, working directory'),
 ('pinned-executor-used', EXEC, 'api_provider.go',
  'run.config.ToolExecutor = executor',
  '_ = executor',
  CONNECTED_VALID, 'outside data'),
 ('composite-pins-workspace', EXEC, 'composite_tools.go',
  'copy.workspace = workspace',
  '_ = workspace',
  CONNECTED_VALID, 'outside data'),
 ('descriptor-not-path', EXEC, 'workspace_tools.go',
  'root, pinned := e.pinnedReads[path]',
  'root, pinned := (*os.Root)(nil), false',
  CONNECTED_VALID, 'outside data'),
 ('duplicate-identity', EXEC, 'workspace_tools.go',
  'if _, duplicate := expected[identity.Path]; duplicate {',
  'if _, duplicate := expected[identity.Path]; duplicate && false {',
  PINS, 'duplicate readable root identity'),
 ('identity-count', EXEC, 'workspace_tools.go',
  'if len(identities) > 0 && len(expected) != len(paths) {',
  'if false && len(identities) > 0 && len(expected) != len(paths) {',
  PINS, 'do not match roots'),
 ('canonical-root', EXEC, 'workspace_tools.go',
  'if err != nil || !filepath.IsAbs(path) || resolved != filepath.Clean(path) {',
  'if err != nil || !filepath.IsAbs(path) || (false && resolved != filepath.Clean(path)) {',
  CONNECTED_UNSAFE, 'not a stable canonical directory'),
 ('identity-equality', EXEC, 'workspace_tools.go',
  'if !ok || identityErr != nil || actual != want {',
  'if !ok || identityErr != nil || (false && actual != want) {',
  CONNECTED_UNSAFE, 'changed since caller admission'),
]

def home(test):
    # Connected tests live beside the stdio adapter and compile pkg/execution in.
    return CLI if test.startswith('TestConnected') else EXEC

def go_test(overlay, directory, tests):
    cmd = ['go', 'test']
    if overlay:
        cmd += ['-overlay', str(overlay)]
    cmd += ['./' + directory, '-run', '^(' + '|'.join(tests) + ')$', '-count=1']
    return subprocess.run(cmd, cwd=root, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

# Restored-green: the tests named by the controls pass on the real source.
for directory in (EXEC, CLI):
    names = sorted({c[5] for c in cases if home(c[5]) == directory})
    run = go_test(None, directory, names)
    assert run.returncode == 0, (directory, run.stdout)
    print(directory, 'RESTORED GREEN:', ', '.join(names), flush=True)

results = []
with tempfile.TemporaryDirectory(prefix='root-identity-controls-') as td:
    for label, directory, name, before, after, test, fragment in cases:
        source = root / directory / name
        original = source.read_text()
        assert original.count(before) == 1, (label, original.count(before))
        replacement = pathlib.Path(td) / (label + '-' + name)
        replacement.write_text(original.replace(before, after))
        overlay = pathlib.Path(td) / (label + '.json')
        overlay.write_text(json.dumps({'Replace': {str(source): str(replacement)}}))
        run = go_test(overlay, home(test), [test])
        out = run.stdout
        assert run.returncode != 0, (label, 'guard removal still passed', out)
        assert 'build failed' not in out, (label, out)
        failed = set(re.findall(r'^--- FAIL: (\w+)', out, re.M))
        assert failed == {test}, (label, 'unexpected failing set', failed, out)
        if fragment:
            assert fragment in out, (label, fragment, out)
        print(label, 'REMOVAL RED:', test, flush=True)
        results.append(label)
print(f'{len(results)} readable-root identity guard removals failed their named tests')
