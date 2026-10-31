package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindCompatibleNodeSkipsObsoleteRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executables")
	}
	root := t.TempDir()
	oldDirectory := filepath.Join(root, "old")
	newDirectory := filepath.Join(root, "new")
	for _, directory := range []string{oldDirectory, newDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldDirectory, "node"), []byte("#!/bin/sh\nprintf 6\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newNode := filepath.Join(newDirectory, "node")
	if err := os.WriteFile(newNode, []byte("#!/bin/sh\nprintf 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", oldDirectory+string(os.PathListSeparator)+newDirectory)
	t.Setenv("NVM_BIN", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("VOLTA_HOME", "")

	found, err := findCompatibleNode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found != newNode {
		t.Fatalf("compatible Node = %q, want %q", found, newNode)
	}
}

// fakeNodeExtractor installs a POSIX "node" that reports major version 20 for
// the probe and runs body for the extractor invocation.
func fakeNodeExtractor(t *testing.T, body string) (root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executables")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"-e\" ]; then printf 20; exit 0; fi\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NVM_BIN", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("VOLTA_HOME", "")
	root = t.TempDir()
	compiler := filepath.Join(root, "node_modules", "typescript", "lib")
	if err := os.MkdirAll(compiler, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compiler, "typescript.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExtractorFailureNamesWhoEndedIt(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		timeout time.Duration
		cancel  bool
		want    string
		notWant string
	}{
		{name: "deadline", body: "exec sleep 30", timeout: 300 * time.Millisecond, want: "extractor deadline 300ms exceeded", notWant: "killed from outside"},
		{name: "external kill", body: "kill -9 $$", timeout: 30 * time.Second, want: "killed from outside this process", notWant: "exceeded"},
		{name: "caller cancel", body: "exec sleep 30", timeout: 30 * time.Second, cancel: true, want: "caller context ended", notWant: "killed from outside"},
		{name: "plain failure", body: "echo boom >&2; exit 3", timeout: 30 * time.Second, want: "exited on its own", notWant: "killed from outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeNodeExtractor(t, tc.body)
			old := typeScriptExtractorTimeout
			typeScriptExtractorTimeout = tc.timeout
			t.Cleanup(func() { typeScriptExtractorTimeout = old })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				time.AfterFunc(200*time.Millisecond, cancel)
			}
			_, _, _, err := extractTypeScriptWithCompiler(ctx, root, []string{"a.ts"})
			if err == nil {
				t.Fatal("expected extractor failure")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error %q lacks %q", msg, tc.want)
			}
			if tc.notWant != "" && strings.Contains(msg, tc.notWant) {
				t.Fatalf("error %q must not contain %q", msg, tc.notWant)
			}
			if tc.name == "plain failure" && (!strings.Contains(msg, "exit status 3") || strings.Contains(msg, "signal")) {
				t.Fatalf("exit 3 misdescribed: %q", msg)
			}
			if tc.name == "plain failure" && !strings.Contains(msg, "boom") {
				t.Fatalf("stderr dropped: %q", msg)
			}
		})
	}
}

const diagnosticBudgetGuard = "output.diagnostics.length < 20 ? ts.getPreEmitDiagnostics(program) : []"

// Entirely synthetic: no captured request, source, paths or diagnostics. The
// second configuration forces real TypeScript to evaluate a costly union only
// during semantic diagnostics. Symbol/reference extraction does not need that
// calculation. Eight hundred members keep the eager control beyond 45s even
// on an otherwise idle worker, without manufacturing a sleep or a fake compiler.
func diagnosticBudgetFixture(t *testing.T, errors, members int) (string, []string) {
	t.Helper()
	root := t.TempDir()
	compiler, err := filepath.Abs("../../ui/node_modules/typescript")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compiler, "lib/typescript.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compiler, filepath.Join(root, "node_modules/typescript")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"early", "later"} {
		writeTestFile(t, root, dir+"/tsconfig.json", `{"compilerOptions":{"skipLibCheck":true},"include":["*.ts"]}`)
	}
	var early strings.Builder
	for i := 0; i < errors; i++ {
		fmt.Fprintf(&early, "export const early%d: string = %d;\n", i, i)
	}
	writeTestFile(t, root, "early/target.ts", "export function target(value: number) { return value + 1 }\n")
	writeTestFile(t, root, "early/errors.ts", early.String())
	var union []string
	for i := 0; i < members; i++ {
		union = append(union, fmt.Sprint(i))
	}
	writeTestFile(t, root, "later/caller.ts", `import { target } from '../early/target';
export function caller() { return target(1) }
export const laterError: number = 'wrong';
type Intersection<U> = (U extends unknown ? (x: U) => void : never) extends (x: infer I) => void ? I : never;
type Last<U> = Intersection<U extends unknown ? () => U : never> extends () => infer L ? L : never;
type Tuple<U, T extends unknown[] = []> = [U] extends [never] ? T : Tuple<Exclude<U, Last<U>>, [Last<U>, ...T]>;
export type Expensive = Tuple<`+strings.Join(union, "|")+">;\n")
	return root, []string{"early/errors.ts", "early/target.ts", "later/caller.ts"}
}

func checkBudgetExtraction(t *testing.T, root string, files []string) {
	t.Helper()
	if typeScriptExtractorTimeout != 45*time.Second {
		t.Fatal("production deadline changed")
	}
	extracted, configuration, limitations, err := extractTypeScriptWithCompiler(t.Context(), root, files)
	if err != nil {
		t.Fatal(err)
	}
	if configuration != "early/tsconfig.json,later/tsconfig.json" || len(limitations) != 21 || limitations[20] != "additional TypeScript diagnostics truncated" {
		t.Fatalf("retained diagnostics/configuration changed: %s %v", configuration, limitations)
	}
	found := false
	for _, ref := range extracted["later/caller.ts"].References {
		if ref.TargetName == "target" && ref.TargetPath == "early/target.ts" && ref.EdgeType == "calls" && ref.Resolution == ResolutionCompilerExact {
			found = true
		}
	}
	if !found {
		t.Fatal("later program lost its real compiler-resolved call")
	}
}

// This assertion runs unchanged against the base tree through declared go-test
// base=true. It must fail there at the REAL, unchanged 45-second deadline.
func TestCompilerDiagnosticBudgetDeadline(t *testing.T) {
	root, files := diagnosticBudgetFixture(t, 20, 800)
	checkBudgetExtraction(t, root, files)
}

func TestCompilerDiagnosticBudgetFixRemoval(t *testing.T) {
	mutateTypeScriptExtractor(t, diagnosticBudgetGuard, "ts.getPreEmitDiagnostics(program)")
	root, files := diagnosticBudgetFixture(t, 20, 800)
	var record nativeChildRecord
	ctx := contextWithCompilerRecord(t, &record)
	_, _, _, err := extractTypeScriptWithCompiler(ctx, root, files)
	if err == nil || record.Ending != "extractor_deadline" || record.ParentError != "none" || record.ChildError != "context deadline exceeded" || record.Bound != "45s" || record.StdoutBytes != 0 {
		t.Fatalf("fix removal must reproduce real compiler deadline: record=%+v error=%v", record, err)
	}
	t.Logf("only guard removed: %s, bound=%s, elapsed=%dns, script=%s", record.Ending, record.Bound, record.ElapsedNS, record.ScriptSHA256)
}

func TestCompilerDiagnosticBudgetPreservesResults(t *testing.T) {
	for _, errors := range []int{0, 19, 20, 21} {
		t.Run(fmt.Sprint(errors), func(t *testing.T) {
			root, files := diagnosticBudgetFixture(t, errors, 6)
			actual, config, limits, err := extractTypeScriptWithCompiler(t.Context(), root, files)
			if err != nil {
				t.Fatal(err)
			}
			t.Run("eager equivalence", func(t *testing.T) {
				mutateTypeScriptExtractor(t, diagnosticBudgetGuard, "ts.getPreEmitDiagnostics(program)")
				eager, eagerConfig, eagerLimits, err := extractTypeScriptWithCompiler(t.Context(), root, files)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, eager) || config != eagerConfig || !reflect.DeepEqual(limits, eagerLimits) {
					t.Fatal("bounded collection changed extracted output or retained limitations")
				}
			})
			later := strings.Contains(strings.Join(limits, "\n"), "later/caller.ts")
			if later != (errors < 20) {
				t.Fatalf("later diagnostic visibility=%v with %d earlier errors: %v", later, errors, limits)
			}
		})
	}
	t.Run("configuration diagnostics consume same budget", func(t *testing.T) {
		root, files := diagnosticBudgetFixture(t, 0, 6)
		var options []string
		for i := 0; i < 20; i++ {
			options = append(options, fmt.Sprintf(`"unknownOption%d":true`, i))
		}
		writeTestFile(t, root, "early/tsconfig.json", `{"compilerOptions":{`+strings.Join(options, ",")+`},"include":["*.ts"]}`)
		actual, config, limits, err := extractTypeScriptWithCompiler(t.Context(), root, files)
		if err != nil {
			t.Fatal(err)
		}
		mutateTypeScriptExtractor(t, diagnosticBudgetGuard, "ts.getPreEmitDiagnostics(program)")
		eager, eagerConfig, eagerLimits, err := extractTypeScriptWithCompiler(t.Context(), root, files)
		if err != nil || !reflect.DeepEqual(actual, eager) || config != eagerConfig || !reflect.DeepEqual(limits, eagerLimits) || len(limits) != 21 {
			t.Fatalf("configuration diagnostic budget differs: %v", err)
		}
	})
}

func contextWithCompilerRecord(t *testing.T, record *nativeChildRecord) context.Context {
	t.Helper()
	return context.WithValue(t.Context(), compilerObservationKey{}, func(o compilerObservation) {
		*record = childMeasurement(o, o.Command.Args[3])
	})
}
