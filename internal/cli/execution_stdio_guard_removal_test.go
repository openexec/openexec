//go:build linux || darwin

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Guard-removal controls for the readable-root identity guards. Each control
// copies one source file with exactly one guard disabled and runs only the test
// that names that guard through `go test -overlay`: the real files on disk are
// never touched. The named test must fail at run time (not at build time), it
// must be the only test that fails, and it must fail on its own assertion.
// The same tests are first run unmodified, which is the restored-green half.

const (
	execPackage = "pkg/execution"
	cliPackage  = "internal/cli"

	connectedUnsafe = "TestConnectedUnsafeRootsFailSafely"
	connectedValid  = "TestConnectedValidRootServesReadListSearchThroughPinnedDescriptor"
	connectedV4     = "TestConnectedV4RefusesMissingIdentityAndProviderThatCannotEnforce"
	connectedOld    = "TestConnectedRuntimeWithoutV4RefusesV4Request"
	pinBadIdents    = "TestPinReadableRootsRefusesBadIdentities"
	cannotBind      = "TestAPIProviderRefusesIdentitiesItCannotBind"
)

type guardRemoval struct {
	label, pkg, file, before, after, test, fragment string
}

var guardRemovals = []guardRemoval{
	{"v4-requires-identity", cliPackage, "execution_stdio.go",
		"if request.Version == executionProtocolBoundRoots && (len(roots) == 0 ||",
		"if false && (len(roots) == 0 ||",
		connectedV4, "requires an identity"},
	{"identity-requires-v4", cliPackage, "execution_stdio.go",
		"if request.Version != executionProtocolBoundRoots && len(identities) > 0 {",
		"if false && len(identities) > 0 {",
		"TestConnectedV1ToV3KeepTheirBehaviour", "accepted identities"},
	{"provider-must-enforce", cliPackage, "execution_stdio.go",
		"if !ok || !bound.BoundReadableRootsAvailable() {",
		"if false && (!ok || !bound.BoundReadableRootsAvailable()) {",
		connectedV4, "provider cannot enforce"},
	{"version-ceiling", cliPackage, "execution_stdio.go",
		"request.Version < executionProtocolVersionLegacy || request.Version > newest {",
		"request.Version < executionProtocolVersionLegacy {",
		connectedOld, "did not refuse v4"},
	{"typed-version-refusal", cliPackage, "execution_stdio.go",
		"return &unsupportedProtocolVersionError{Version: request.Version}",
		`return fmt.Errorf("unsupported execution protocol version %d", request.Version)`,
		connectedOld, "not typed"},
	{"platform-support-advertised", execPackage, "api_provider.go",
		"return ok && readableRootIdentitySupported()",
		"return ok",
		"TestUnsupportedPlatformRefusesBoundRootsWithoutFallback", "advertised bound roots"},
	{"executor-cannot-bind", execPackage, "api_provider.go",
		"} else if len(request.ReadableRootIdentities) > 0 {\n\t\t\treturn Result{}, errors.New(\"tool executor cannot bind",
		"} else if false {\n\t\t\treturn Result{}, errors.New(\"tool executor cannot bind",
		cannotBind, "err="},
	{"identities-without-roots", execPackage, "api_provider.go",
		"} else if len(request.ReadableRootIdentities) > 0 {\n\t\treturn Result{}, errors.New(\"readable root identities without roots\")",
		"} else if false {\n\t\treturn Result{}, errors.New(\"readable root identities without roots\")",
		cannotBind, "inference requests="},
	{"pinned-executor-used", execPackage, "api_provider.go",
		"run.config.ToolExecutor = executor",
		"_ = executor",
		connectedValid, "outside data"},
	{"composite-pins-workspace", execPackage, "composite_tools.go",
		"copy.workspace = workspace",
		"_ = workspace",
		connectedValid, "outside data"},
	{"descriptor-not-path", execPackage, "workspace_tools.go",
		"root, pinned := e.pinnedReads[path]",
		"root, pinned := (*os.Root)(nil), false",
		connectedValid, "outside data"},
	{"duplicate-identity", execPackage, "workspace_tools.go",
		"if _, duplicate := expected[identity.Path]; duplicate {",
		"if _, duplicate := expected[identity.Path]; duplicate && false {",
		pinBadIdents, "duplicate readable root identity"},
	{"identity-count", execPackage, "workspace_tools.go",
		"if len(identities) > 0 && len(expected) != len(paths) {",
		"if false && len(identities) > 0 && len(expected) != len(paths) {",
		pinBadIdents, "do not match roots"},
	{"canonical-root", execPackage, "workspace_tools.go",
		"if err != nil || !filepath.IsAbs(path) || resolved != filepath.Clean(path) {",
		"if err != nil || !filepath.IsAbs(path) || (false && resolved != filepath.Clean(path)) {",
		connectedUnsafe, "not a stable canonical directory"},
	{"admission-same-file", execPackage, "workspace_tools.go",
		"|| !os.SameFile(opened, named) {",
		"|| (false && !os.SameFile(opened, named)) {",
		"TestPinReadableRootsRefusesRootSwappedDuringAdmission", "swapped root admitted"},
	{"post-admission-root-recheck", execPackage, "workspace_tools.go",
		"stillNamed := func() error { return pinnedRootStillNamed(root, path) }",
		"stillNamed := func() error { return nil }",
		connectedValid, "swap after admission was not refused"},
	{"post-operation-root-recheck", execPackage, "workspace_tools.go",
		"if stillErr := stillNamed(); stillErr != nil {",
		"if stillErr := error(nil); stillErr != nil {",
		"TestPinnedRootSwappedAfterNameCheckIsRefused", "swap served"},
	{"post-admission-root-same-object", execPackage, "root_walk_unix.go",
		"if !ok || uint64(held.Dev) != uint64(walked.Dev) || uint64(held.Ino) != uint64(walked.Ino) {\n\t\treturn stale",
		"if !ok || (false && uint64(held.Ino) != uint64(walked.Ino)) {\n\t\treturn stale",
		connectedValid, "swap after admission was not refused"},
	{"noproc-walk-runs", execPackage, "workspace_tools.go",
		"if err := openedFileByDescriptorWalk(root, file, cleaned); err != nil {",
		"if err := error(nil); err != nil {",
		"TestNoProcSwapPlusSymlinkDoesNotBlessProtectedTarget", "served"},
	{"noproc-walk-no-follow", execPackage, "root_walk_unix.go",
		"flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC",
		"flags := unix.O_RDONLY | unix.O_CLOEXEC",
		"TestNoProcInterleavedLstatStatSwapIsRefused", "served through interleaved swap"},
	{"noproc-walk-same-object", execPackage, "root_walk_unix.go",
		"if !ok || uint64(held.Dev) != uint64(walked.Dev) || uint64(held.Ino) != uint64(walked.Ino) {\n\t\treturn errors.New(\"the path changed while it was being opened\")",
		"if !ok || (false && uint64(held.Ino) != uint64(walked.Ino)) {\n\t\treturn errors.New(\"the path changed while it was being opened\")",
		"TestNoProcRefusesFileReplacedAfterOpen", "served"},
	{"identity-computation-error", execPackage, "workspace_tools.go",
		"if !ok || identityErr != nil || actual != want {",
		"if !ok || (false && identityErr != nil) || actual != want {",
		"TestPinReadableRootsRefusesWhenIdentityCannotBeComputed", "zero identity admitted"},
	{"identity-equality", execPackage, "workspace_tools.go",
		"if !ok || identityErr != nil || actual != want {",
		"if !ok || identityErr != nil || (false && actual != want) {",
		connectedUnsafe, "changed since caller admission"},
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found from %s: %v", wd, err)
	}
	return root
}

func packageOf(test string) string {
	// Connected tests live beside the stdio adapter and compile pkg/execution in.
	if strings.HasPrefix(test, "TestConnected") || strings.HasPrefix(test, "TestLegacy") {
		return cliPackage
	}
	return execPackage
}

// nestedGoTest runs a focused `go test` in a child process. An overlay of "" is
// an unmodified run.
func nestedGoTest(t *testing.T, root, overlay, pkg string, tests []string) (string, bool) {
	t.Helper()
	goBinary, err := exec.LookPath("go")
	if err != nil {
		goBinary = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	args := []string{"test"}
	if overlay != "" {
		args = append(args, "-overlay", overlay)
	}
	args = append(args, "./"+pkg, "-run", "^("+strings.Join(tests, "|")+")$", "-count=1", "-timeout", "120s")
	command := exec.Command(goBinary, args...)
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	return string(output), err == nil
}

func writeOverlay(t *testing.T, dir, name string, replace map[string]string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func failedTopLevel(output string) []string {
	var failed []string
	for _, match := range regexp.MustCompile(`(?m)^--- FAIL: (\w+)`).FindAllStringSubmatch(output, -1) {
		failed = append(failed, match[1])
	}
	sort.Strings(failed)
	return failed
}

func TestReadableRootIdentityGuardRemovalControls(t *testing.T) {
	root := moduleRoot(t)
	scratch := t.TempDir()

	t.Run("restored-green", func(t *testing.T) {
		byPackage := map[string]map[string]bool{}
		for _, control := range guardRemovals {
			if byPackage[packageOf(control.test)] == nil {
				byPackage[packageOf(control.test)] = map[string]bool{}
			}
			byPackage[packageOf(control.test)][control.test] = true
		}
		for pkg, set := range byPackage {
			var names []string
			for name := range set {
				names = append(names, name)
			}
			sort.Strings(names)
			if output, ok := nestedGoTest(t, root, "", pkg, names); !ok {
				t.Fatalf("%s: named tests are not green on the real source:\n%s", pkg, output)
			}
		}
	})

	for _, control := range guardRemovals {
		t.Run(control.label, func(t *testing.T) {
			source := filepath.Join(root, control.pkg, control.file)
			original, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if count := strings.Count(string(original), control.before); count != 1 {
				t.Fatalf("guard text found %d times in %s, want exactly 1", count, control.file)
			}
			replacement := filepath.Join(scratch, control.label+"-"+control.file)
			if err := os.WriteFile(replacement, []byte(strings.Replace(string(original), control.before, control.after, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			overlay := writeOverlay(t, scratch, control.label, map[string]string{source: replacement})
			output, ok := nestedGoTest(t, root, overlay, packageOf(control.test), []string{control.test})
			if ok {
				t.Fatalf("removing the guard still passed %s:\n%s", control.test, output)
			}
			if strings.Contains(output, "build failed") || strings.Contains(output, "[setup failed]") {
				t.Fatalf("removal broke the build instead of the behaviour:\n%s", output)
			}
			if failed := failedTopLevel(output); len(failed) != 1 || failed[0] != control.test {
				t.Fatalf("failing tests = %v, want only %s:\n%s", failed, control.test, output)
			}
			if !strings.Contains(output, control.fragment) {
				t.Fatalf("%s failed, but not on its %q assertion:\n%s", control.test, control.fragment, output)
			}
		})
	}
}

// The reader as it was on main before version 4 (testdata/legacy_runtime, taken
// verbatim from d4e65423) runs a v4 request. The overlay swaps in that reader and
// its tests, drops the tests that name version-4 symbols, and adds the legacy
// refusal test, so the old code itself answers.
func TestLegacyReaderRefusesV4Request(t *testing.T) {
	root := moduleRoot(t)
	legacy := filepath.Join(root, cliPackage, "testdata", "legacy_runtime")
	scratch := t.TempDir()
	reader, err := os.ReadFile(filepath.Join(legacy, "execution_stdio.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reader), "executionProtocolBoundRoots") || !strings.Contains(string(reader), "executionProtocolVersion       = 3") {
		t.Fatal("legacy snapshot is not the pre-v4 reader")
	}
	overlay := writeOverlay(t, scratch, "legacy", map[string]string{
		filepath.Join(root, cliPackage, "execution_stdio.go"):                        filepath.Join(legacy, "execution_stdio.go.txt"),
		filepath.Join(root, cliPackage, "execution_stdio_test.go"):                   filepath.Join(legacy, "execution_stdio_test.go.txt"),
		filepath.Join(root, cliPackage, "execution_stdio_root_identity_test.go"):     "",
		filepath.Join(root, cliPackage, "execution_stdio_guard_removal_test.go"):     "",
		filepath.Join(root, cliPackage, "execution_stdio_command_boundary_test.go"):  "",
		filepath.Join(root, cliPackage, "execution_stdio_legacy_v4_refusal_test.go"): filepath.Join(legacy, "legacy_v4_refusal_test.go.txt"),
	})
	output, ok := nestedGoTest(t, root, overlay, cliPackage, []string{"TestLegacyRuntimeRefusesV4Request", "TestLegacyRuntimeCommandRefusesV4Request", "TestLegacyRuntimeStillServesV3"})
	if !ok {
		t.Fatalf("the legacy reader did not refuse the v4 request:\n%s", output)
	}
}
