package knowledge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Entirely public synthetic inputs, using real Node and real TypeScript. This
// fixture is deliberately small; it is not a model of the natural deadline.
func phaseFixture(t *testing.T, diagnostics int, noLib ...bool) (string, string, string, string, nativeChildRecord) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := filepath.Abs("../../ui/node_modules/typescript/lib/typescript.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(compiler); err != nil {
		t.Fatal("locked TypeScript dependency required")
	}
	root := t.TempDir()
	options := map[string]any{"skipLibCheck": true, "types": []string{}}
	if len(noLib) > 0 && noLib[0] {
		options["noLib"] = true
	}
	config, _ := json.Marshal(map[string]any{"compilerOptions": options, "include": []string{"*.ts"}})
	for _, dir := range []string{"early", "later"} {
		writeTestFile(t, root, dir+"/tsconfig.json", string(config))
	}
	var body strings.Builder
	for i := 0; i < diagnostics; i++ {
		fmt.Fprintf(&body, "export const bad%d: string = %d;\n", i, i)
	}
	writeTestFile(t, root, "early/a.ts", body.String()+"export function target() { return 1 }\n")
	writeTestFile(t, root, "later/b.ts", "import { target } from '../early/a'; export function caller() { return target() }\n")
	writeTestFile(t, root, "node_modules/dep/index.d.ts", "export declare const context: number;\n")
	writeTestFile(t, root, ".generated/ambient.d.ts", "declare const generated: string;\n")
	writeTestFile(t, root, "inherited.json", `{"compilerOptions":{"strict":true}}`)
	parent := privateCaptureParent(t)
	dir := filepath.Join(parent, "capture")
	if os.Mkdir(dir, 0o700) != nil {
		t.Fatal("mkdir capture")
	}
	request, _ := json.Marshal(tsCompilerRequest{Files: []string{"early/a.ts", "later/b.ts"}})
	manifest, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	r := nativeChildRecord{Arguments: []string{node, filepath.Join(parent, "absent-original.mjs"), compiler, root}, Bound: "45s", RequestSHA256: captureHash(request), ScriptSHA256: captureHash(typeScriptCompilerScript), NodeSHA256: captureFileHash(node), CompilerSHA256: captureFileHash(compiler)}
	c := &compilerCapture{dir: dir}
	for name, v := range map[string]any{"identity.json": compilerReplayIdentity{manifest, r.NodeSHA256, r.CompilerSHA256}, "result.json": map[string]any{"child": r}} {
		if err := c.writeJSON(name, v); err != nil {
			t.Fatal(err)
		}
	}
	if c.write("request.json", request) != nil || c.write("extractor.mjs", typeScriptCompilerScript) != nil {
		t.Fatal("write fixture")
	}
	contextRecord := compilerPhaseContextRecord{Complete: true, Request: r.RequestSHA256, Manifest: manifest.ManifestHash, Trees: map[string]map[string]map[string]string{}}
	for domain, treeRoot := range map[string]string{"dependencies": filepath.Join(root, "node_modules"), "generated": root, "inherited_configuration": filepath.Join(root, "inherited.json"), "default_libraries": filepath.Dir(compiler)} {
		tree, err := phaseTree(treeRoot)
		if err != nil {
			t.Fatal(err)
		}
		contextRecord.Trees[domain] = map[string]map[string]string{treeRoot: tree}
	}
	contextFile := filepath.Join(parent, "historical-context.json")
	b, _ := json.Marshal(contextRecord)
	if os.WriteFile(contextFile, b, 0o600) != nil {
		t.Fatal("write context")
	}
	return dir, contextFile, filepath.Join(parent, "experiment"), root, r
}

func phaseInvoke(t *testing.T, dir, contextFile, dest string) ([]byte, error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exec.Command(executable, "-test.run=^TestConsoleNativeRequestCapture$", "-compiler-private-capture="+dir, "-compiler-phase-context="+contextFile, "-compiler-phase-output="+dest).CombinedOutput()
}

func phaseRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCompilerPhaseNativeJourney(t *testing.T) {
	for _, count := range []int{0, 20, 21} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			dir, ctx, dest, _, r := phaseFixture(t, count)
			originals := map[string][]byte{}
			for _, name := range []string{"request.json", "extractor.mjs", "identity.json", "result.json"} {
				originals[name] = phaseRead(t, filepath.Join(dir, name))
			}
			// Mtime-only changes are a near miss, not source drift.
			if err := os.Chtimes(filepath.Join(r.Arguments[3], "early/a.ts"), time.Now(), time.Now()); err != nil {
				t.Fatal(err)
			}
			out, err := phaseInvoke(t, dir, ctx, dest)
			if err != nil {
				t.Fatalf("native experiment failed: %v %s", err, out)
			}
			data := phaseRead(t, filepath.Join(dest, "phases.jsonl"))
			summary, err := summarizeCompilerPhases(data, true, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(summary.Completed) != 11 || summary.Unfinished != nil || summary.OverheadLowerBound <= 0 {
				t.Fatalf("missing phase deltas: %+v", summary)
			}
			var rows []compilerPhaseRow
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var row compilerPhaseRow
				if json.Unmarshal(line, &row) != nil {
					t.Fatal("invalid row")
				}
				rows = append(rows, row)
			}
			selected := int64(1)
			if count >= 20 {
				selected = 0
			}
			found := false
			for _, row := range rows {
				if row[0] == 5 && row[1] == 2 && row[2] == 1 {
					found = true
					if row[7] != selected {
						t.Fatal("diagnostic selection not observed")
					}
				}
			}
			if !found {
				t.Fatal("second diagnostics group missing")
			}
			var missing bytes.Buffer
			for _, row := range rows {
				if row[0] < 3 || row[0] > 6 {
					b, _ := json.Marshal(row)
					missing.Write(b)
					missing.WriteByte('\n')
				}
			}
			if _, err := summarizeCompilerPhases(missing.Bytes(), true, false); err == nil {
				t.Fatal("entire missing program phases accepted")
			}
			// Compare full response bytes, not only decoding/counts. Stdout is not
			// retained by the experiment; run the same synthetic scripts as controls.
			request := originals["request.json"]
			plain := filepath.Join(filepath.Dir(dir), "plain.mjs")
			if os.WriteFile(plain, typeScriptCompilerScript, 0o600) != nil {
				t.Fatal("plain script")
			}
			run := func(script string, channel *os.File) []byte {
				cmd := exec.Command(r.Arguments[0], script, r.Arguments[2], r.Arguments[3])
				cmd.Stdin = bytes.NewReader(request)
				if channel != nil {
					cmd.ExtraFiles = []*os.File{channel}
				}
				b, err := cmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			sink, err := os.CreateTemp(t.TempDir(), "numeric")
			if err != nil {
				t.Fatal(err)
			}
			defer sink.Close()
			plainOut := run(plain, nil)
			modifiedOut := run(filepath.Join(dest, "instrumented.mjs"), sink)
			if !bytes.Equal(plainOut, modifiedOut) {
				t.Fatal("instrumentation changed compiler response")
			}
			var response tsCompilerResponse
			if json.Unmarshal(modifiedOut, &response) != nil || len(response.Files) != 2 {
				t.Fatal("after-budget traversal lost source files")
			}
			if !bytes.Contains(modifiedOut, []byte(`"target_name":"target"`)) {
				t.Fatal("after-budget checker reference missing")
			}
			var measurement struct {
				Child   nativeChildRecord    `json:"child"`
				Summary compilerPhaseSummary `json:"phases"`
				Hash    string               `json:"modified_script_sha256"`
			}
			if json.Unmarshal(phaseRead(t, filepath.Join(dest, "measurement.json")), &measurement) != nil || measurement.Child.Bound != "45s" || !measurement.Child.ResponseDecoded || measurement.Child.ScriptSHA256 != measurement.Hash || measurement.Hash == r.ScriptSHA256 {
				t.Fatal("persisted identity/bound/response missing")
			}
			for name, b := range originals {
				if !bytes.Equal(b, phaseRead(t, filepath.Join(dir, name))) {
					t.Fatal("original artifact changed")
				}
			}
			for _, name := range []string{"", "instrumented.mjs", "phases.jsonl", "measurement.json"} {
				if !privatePhaseFile(filepath.Join(dest, name), name == "") {
					t.Fatal("private experiment mode lost")
				}
			}
			out, err = phaseInvoke(t, dir, ctx, dest+"-again")
			if err == nil || !bytes.Contains(out, []byte("capture already reserved")) {
				t.Fatal("single use admission lost")
			}
		})
	}
}

func TestCompilerPhaseEmptyRequest(t *testing.T) {
	dir, ctx, dest, _, r := phaseFixture(t, 0)
	request := []byte(`{"files":[]}`)
	r.RequestSHA256 = captureHash(request)
	if os.WriteFile(filepath.Join(dir, "request.json"), request, 0o600) != nil {
		t.Fatal("request")
	}
	rewritePhaseRecord(t, dir, r)
	editPhaseContext(t, ctx, func(c *compilerPhaseContextRecord) { c.Request = r.RequestSHA256 })
	if out, err := phaseInvoke(t, dir, ctx, dest); err != nil {
		t.Fatalf("explicit empty request refused: %v %s", err, out)
	}
	summary, err := summarizeCompilerPhases(phaseRead(t, filepath.Join(dest, "phases.jsonl")), true, true)
	if err != nil || len(summary.Completed) != 3 {
		t.Fatal("empty request phases incomplete")
	}
}

func TestCompilerPhaseAdmissionRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*testing.T, string, string, string, nativeChildRecord)
	}{
		{"source", "source/configuration/toolchain changed", func(t *testing.T, d, c, r string, n nativeChildRecord) { writeTestFile(t, r, "early/a.ts", "changed") }},
		{"missing-source", "source/configuration/toolchain changed", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Remove(filepath.Join(r, "early/a.ts")) != nil {
				t.Fatal("remove")
			}
		}},
		{"configuration", "source/configuration/toolchain changed", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			writeTestFile(t, r, "early/tsconfig.json", `{}`)
		}},
		{"missing-configuration", "source/configuration/toolchain changed", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Remove(filepath.Join(r, "early/tsconfig.json")) != nil {
				t.Fatal("remove")
			}
		}},
		{"node", "current.node_sha256/result.node_sha256", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			n.NodeSHA256 = "synthetic-change"
			rewritePhaseRecord(t, d, n)
		}},
		{"typescript", "current.typescript_sha256/result.typescript_sha256", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			n.CompilerSHA256 = "synthetic-change"
			rewritePhaseRecord(t, d, n)
		}},
		{"missing-node", "current.node_sha256/result.node_sha256", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			n.Arguments[0] = filepath.Join(r, "missing-node")
			rewritePhaseRecord(t, d, n)
		}},
		{"missing-typescript", "current.typescript_sha256/result.typescript_sha256", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			n.Arguments[2] = filepath.Join(r, "missing-typescript")
			rewritePhaseRecord(t, d, n)
		}},
		{"dependencies", "context.dependencies", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			writeTestFile(t, r, "node_modules/dep/index.d.ts", "changed")
		}},
		{"missing-dependencies", "context.dependencies", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.RemoveAll(filepath.Join(r, "node_modules")) != nil {
				t.Fatal("remove")
			}
		}},
		{"generated", "context.generated", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			writeTestFile(t, r, ".generated/ambient.d.ts", "changed")
		}},
		{"missing-generated", "context.generated", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.RemoveAll(filepath.Join(r, ".generated")) != nil {
				t.Fatal("remove")
			}
		}},
		{"added-generated", "context.generated", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			writeTestFile(t, r, ".another/ambient.d.ts", "new")
		}},
		{"missing-context", "context.dependencies, context.generated, context.inherited_configuration, context.default_libraries", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Remove(c) != nil {
				t.Fatal("remove")
			}
		}},
		{"empty-domain", "context.default_libraries", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			editPhaseContext(t, c, func(p *compilerPhaseContextRecord) { delete(p.Trees, "default_libraries") })
		}},
		{"empty-inherited", "context.inherited_configuration", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			editPhaseContext(t, c, func(p *compilerPhaseContextRecord) { delete(p.Trees, "inherited_configuration") })
		}},
		{"incomplete-closure", "context.complete_historical_closure", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			editPhaseContext(t, c, func(p *compilerPhaseContextRecord) { p.Complete = false })
		}},
		{"coverage", "context.generated.coverage", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			editPhaseContext(t, c, func(p *compilerPhaseContextRecord) {
				tree, err := phaseTree(filepath.Join(r, ".generated"))
				if err != nil {
					t.Fatal(err)
				}
				p.Trees["generated"] = map[string]map[string]string{filepath.Join(r, ".generated"): tree}
			})
		}},
		{"inherited-configuration", "context.inherited_configuration", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			alterPhaseHistoricalTree(t, c, "inherited_configuration")
		}},
		{"default-libraries", "context.default_libraries", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			alterPhaseHistoricalTree(t, c, "default_libraries")
		}},
		{"binding", "context.capture_binding", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			editPhaseContext(t, c, func(p *compilerPhaseContextRecord) { p.Request = "changed" })
		}},
		{"capture-mode", "private capture directory required", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Chmod(d, 0o755) != nil {
				t.Fatal("chmod")
			}
		}},
		{"artifact-mode", "private capture artifact missing or unsafe", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Chmod(filepath.Join(d, "request.json"), 0o644) != nil {
				t.Fatal("chmod")
			}
		}},
		{"context-mode", "private historical context required", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Chmod(c, 0o644) != nil {
				t.Fatal("chmod")
			}
		}},
		{"request", "capture request/script/argv/bound mismatch", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.WriteFile(filepath.Join(d, "request.json"), []byte(`{}`), 0o600) != nil {
				t.Fatal("write")
			}
		}},
		{"script", "capture request/script/argv/bound mismatch", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			b := append(phaseRead(t, filepath.Join(d, "extractor.mjs")), []byte("\n// changed\n")...)
			n.ScriptSHA256 = captureHash(b)
			rewritePhaseRecord(t, d, n)
			if os.WriteFile(filepath.Join(d, "extractor.mjs"), b, 0o600) != nil {
				t.Fatal("script")
			}
		}},
		{"missing-script", "private capture artifact missing or unsafe", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if os.Remove(filepath.Join(d, "extractor.mjs")) != nil {
				t.Fatal("remove")
			}
		}},
		{"bound", "capture request/script/argv/bound mismatch", func(t *testing.T, d, c, r string, n nativeChildRecord) { n.Bound = "44s"; rewritePhaseRecord(t, d, n) }},
		{"argv", "capture request/script/argv/bound mismatch", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			n.Arguments = n.Arguments[:3]
			rewritePhaseRecord(t, d, n)
		}},
		{"reserved", "capture already reserved", func(t *testing.T, d, c, r string, n nativeChildRecord) {
			if (&compilerCapture{dir: d}).write("replay.json", []byte(`{}`)) != nil {
				t.Fatal("reserve")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, ctx, dest, root, r := phaseFixture(t, 0)
			tc.change(t, dir, ctx, root, r)
			out, err := phaseInvoke(t, dir, ctx, dest)
			if err == nil || !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("expected admission refusal %s: err=%v output=%s", tc.name, err, out)
			}
			for _, secret := range []string{dir, root, r.RequestSHA256, r.NodeSHA256, "declare const generated"} {
				if bytes.Contains(out, []byte(secret)) {
					t.Fatal("private admission details leaked")
				}
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatal("refused child created experiment")
			}
			if tc.name != "reserved" {
				if _, err := os.Stat(filepath.Join(dir, "replay.json")); !os.IsNotExist(err) {
					t.Fatal("refused child reserved capture")
				}
			}
		})
	}
}

func alterPhaseHistoricalTree(t *testing.T, name, domain string) {
	t.Helper()
	editPhaseContext(t, name, func(p *compilerPhaseContextRecord) {
		for _, tree := range p.Trees[domain] {
			for name, value := range tree {
				if len(value) == 64 {
					tree[name] = "synthetic-historical-drift"
					return
				}
			}
		}
		t.Fatal("fixture domain has no file")
	})
}

func TestCompilerPhaseContextSymlinks(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	writeTestFile(t, external, "context.d.ts", "original")
	if os.Symlink(external, filepath.Join(root, "link")) != nil {
		t.Fatal("symlink")
	}
	before, err := phaseTree(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, external, "context.d.ts", "changed")
	after, err := phaseTree(root)
	if err != nil || reflect.DeepEqual(before, after) {
		t.Fatal("symlink target drift invisible")
	}
	if os.Symlink(root, filepath.Join(external, "cycle")) != nil {
		t.Fatal("symlink")
	}
	if _, err := phaseTree(root); err != nil {
		t.Fatal("resolvable directory cycle should be visited once")
	}
	if os.Symlink(filepath.Join(root, "absent"), filepath.Join(root, "broken")) != nil {
		t.Fatal("symlink")
	}
	if _, err := phaseTree(root); err == nil {
		t.Fatal("unresolved context symlink admitted")
	}
}

func TestCompilerPhaseGuardRemovalControls(t *testing.T) {
	// Real compiled mutants, never a search standing in for behaviour. Overlay
	// only this test harness, then run the same behavioural assertions against
	// its native executable. Compile/setup failures cannot count as detection.
	_, here, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(here), "ts_compiler_phase_experiment_test.go")
	original := phaseRead(t, source)
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, source, original, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	guards := map[string]*ast.IfStmt{}
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		for _, comments := range file.Comments {
			for _, c := range comments.List {
				if fs.Position(c.Pos()).Line == fs.Position(stmt.Body.Lbrace).Line && strings.HasPrefix(c.Text, "// phase-guard: ") {
					guards[strings.TrimPrefix(c.Text, "// phase-guard: ")] = stmt
				}
			}
		}
		return true
	})
	controls := []struct{ guard, test, receipt string }{
		{"binding", "TestCompilerPhaseAdmissionRefusals/binding", "expected admission refusal binding"},
		{"missing-context", "TestCompilerPhaseAdmissionRefusals/empty-inherited", "expected admission refusal empty-inherited"},
		{"context-drift", "TestCompilerPhaseAdmissionRefusals/dependencies", "expected admission refusal dependencies"},
		{"complete-context", "TestCompilerPhaseAdmissionRefusals/incomplete-closure", "expected admission refusal incomplete-closure"},
		{"context-coverage", "TestCompilerPhaseAdmissionRefusals/coverage", "expected admission refusal coverage"},
		{"capture", "TestCompilerPhaseAdmissionRefusals/request", "expected admission refusal request"},
		{"identity", "TestCompilerPhaseAdmissionRefusals/source", "expected admission refusal source"},
		{"context-admission", "TestCompilerPhaseAdmissionRefusals/binding", "expected admission refusal binding"},
		{"experiment-reservation", "TestCompilerPhaseNativeJourney/0", "single use admission lost"},
		{"required-anchor", "TestCompilerPhaseRequiredInstrumentation", "missing required phase accepted"},
		{"numeric-privacy", "TestCompilerPhaseTelemetryRefusals", "invalid telemetry accepted"},
		{"event-order", "TestCompilerPhaseTelemetryRefusals", "invalid telemetry accepted"},
		{"complete-success", "TestCompilerPhaseTelemetryRefusals", "incomplete successful child accepted"},
		{"unavailable-context", "TestCompilerPhaseUnavailableContextNeverScans", "context-only refusal lost"},
		{"capture-mode", "TestCompilerPhaseAdmissionRefusals/capture-mode", "expected admission refusal capture-mode"},
		{"artifact-mode", "TestCompilerPhaseAdmissionRefusals/artifact-mode", "expected admission refusal artifact-mode"},
		{"context-mode", "TestCompilerPhaseAdmissionRefusals/context-mode", "expected admission refusal context-mode"},
		{"destination-mode", "TestCompilerPhaseDestinationRefusals/mode", "unsafe destination accepted"},
		{"destination-separate", "TestCompilerPhaseDestinationRefusals/inside-capture", "unsafe destination accepted"},
		{"telemetry-bound", "TestCompilerPhaseTelemetryBound", "oversize telemetry not classified"},
	}
	if len(guards) != len(controls) {
		t.Fatal("every labelled guard requires a removal control")
	}
	for _, tc := range controls {
		t.Run(tc.guard, func(t *testing.T) {
			stmt := guards[tc.guard]
			if stmt == nil {
				t.Fatal("guard control is missing")
			}
			start, end := fs.Position(stmt.Cond.Pos()).Offset, fs.Position(stmt.Cond.End()).Offset
			mutant := append([]byte(nil), original[:start]...)
			mutant = append(mutant, []byte("false && (")...)
			mutant = append(mutant, original[start:end]...)
			mutant = append(mutant, ')')
			mutant = append(mutant, original[end:]...)
			dir := t.TempDir()
			replaced := filepath.Join(dir, "mutant.go")
			if os.WriteFile(replaced, mutant, 0o600) != nil {
				t.Fatal("write overlay")
			}
			b, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replaced}})
			overlay := filepath.Join(dir, "overlay.json")
			if os.WriteFile(overlay, b, 0o600) != nil {
				t.Fatal("overlay")
			}
			parts := strings.Split(tc.test, "/")
			for i := range parts {
				parts[i] = "^" + parts[i] + "$"
			}
			out, err := exec.Command("go", "test", "-overlay="+overlay, ".", "-run", strings.Join(parts, "/"), "-count=1", "-v").CombinedOutput()
			if err == nil || !bytes.Contains(out, []byte("--- FAIL: ")) || !bytes.Contains(out, []byte(tc.receipt)) || bytes.Contains(out, []byte("build failed")) {
				t.Fatalf("guard removal not detected behaviorally (%s): %v %s", tc.guard, err, out)
			}
			t.Log("removed guard rejected by behavioural assertion:", tc.guard)
		})
	}
}

func TestCompilerPhaseDestinationRefusals(t *testing.T) {
	for _, which := range []string{"mode", "inside-capture", "inside-source", "symlink-parent", "existing", "final-symlink"} {
		t.Run(which, func(t *testing.T) {
			dir, ctx, dest, root, _ := phaseFixture(t, 0)
			want := "experiment destination must be separate"
			switch which {
			case "mode":
				wide := t.TempDir()
				if os.Chmod(wide, 0o755) != nil {
					t.Fatal("chmod")
				}
				dest = filepath.Join(wide, "experiment")
				want = "new private experiment destination required"
			case "inside-capture":
				dest = filepath.Join(dir, "experiment")
			case "inside-source":
				if os.Chmod(root, 0o700) != nil {
					t.Fatal("chmod")
				}
				dest = filepath.Join(root, "experiment")
			case "symlink-parent":
				writeTestFile(t, root, ".private/keep", "synthetic")
				if os.Chmod(filepath.Join(root, ".private"), 0o700) != nil {
					t.Fatal("chmod")
				}
				// Update only the synthetic historical fixture to include the link
				// target; then exercise destination canonicalization after admission.
				editPhaseContext(t, ctx, func(p *compilerPhaseContextRecord) {
					tree, err := phaseTree(root)
					if err != nil {
						t.Fatal(err)
					}
					p.Trees["generated"][root] = tree
				})
				link := filepath.Join(filepath.Dir(dir), "alias")
				if os.Symlink(root, link) != nil {
					t.Fatal("symlink")
				}
				dest = filepath.Join(link, ".private", "experiment")
			case "existing":
				if os.Mkdir(dest, 0o700) != nil {
					t.Fatal("mkdir")
				}
				want = "experiment destination exists or unavailable"
			case "final-symlink":
				if os.Symlink(t.TempDir(), dest) != nil {
					t.Fatal("symlink")
				}
				want = "experiment destination exists or unavailable"
			}
			out, err := phaseInvoke(t, dir, ctx, dest)
			if err == nil || !bytes.Contains(out, []byte(want)) {
				t.Fatalf("unsafe destination accepted (%s): %v %s", which, err, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "replay.json")); !os.IsNotExist(err) {
				t.Fatal("unsafe destination reserved capture")
			}
		})
	}
}

func rewritePhaseRecord(t *testing.T, dir string, r nativeChildRecord) {
	t.Helper()
	var identity compilerReplayIdentity
	if json.Unmarshal(phaseRead(t, filepath.Join(dir, "identity.json")), &identity) != nil {
		t.Fatal("identity")
	}
	identity.Node, identity.Compiler = r.NodeSHA256, r.CompilerSHA256
	for name, v := range map[string]any{"identity.json": identity, "result.json": map[string]any{"child": r}} {
		b, _ := json.Marshal(v)
		if os.WriteFile(filepath.Join(dir, name), b, 0o600) != nil {
			t.Fatal("rewrite")
		}
	}
}

func editPhaseContext(t *testing.T, name string, edit func(*compilerPhaseContextRecord)) {
	t.Helper()
	var c compilerPhaseContextRecord
	if json.Unmarshal(phaseRead(t, name), &c) != nil {
		t.Fatal("context")
	}
	edit(&c)
	b, _ := json.Marshal(c)
	if os.WriteFile(name, b, 0o600) != nil {
		t.Fatal("context write")
	}
}

func TestCompilerPhaseRequiredInstrumentation(t *testing.T) {
	if _, err := instrumentCompilerPhases(typeScriptCompilerScript); err != nil {
		t.Fatal(err)
	}
	for _, anchor := range []string{"const require = createRequire(import.meta.url);", "const ts = require(compilerPath);", "function exported(node) {", "for (const group of groups.values()) {", "  const checker = program.getTypeChecker();", "  for (const diagnostic of " + diagnosticBudgetGuard + ") {", "  for (const sourceFile of program.getSourceFiles()) {", "\n}\n\noutput.files.sort", "process.stdout.write(JSON.stringify(output));"} {
		changed := bytes.Replace(typeScriptCompilerScript, []byte(anchor), []byte("/* absent synthetic phase */"), 1)
		if _, err := instrumentCompilerPhases(changed); err == nil {
			t.Fatal("missing required phase accepted")
		}
	}
}

func TestCompilerPhaseTelemetryRefusals(t *testing.T) {
	for _, data := range []string{
		`[1,0,1,0,0,0,0,-1,0,"/private/path"]`,
		`["private source body"]`,
		`[1,null,1,0,0,0,0,-1,0]`,
		`[1,0,2,0,0,0,0,-1,0]`,
		"[1,0,1,0,0,0,0,-1,0]\n[1,0,2,-1,0,0,0,-1,0]",
		"[1,0,1,0,0,0,0,-1,0]\n[1,0,2,1,0,0,0,-1,0]\n[3,1,1,2,0,0,0,-1,0]",
	} {
		if _, err := summarizeCompilerPhases([]byte(data), false, false); err == nil {
			t.Fatal("invalid telemetry accepted")
		}
	}
	data := []byte("[1,0,1,0,0,0,0,-1,0]\n[1,0,2,1,1,0,0,-1,1]\n[2,0,1,2,1,0,0,-1,1]\n")
	summary, err := summarizeCompilerPhases(data, false, false)
	if err != nil || len(summary.Completed) != 1 || summary.Unfinished == nil || summary.Unfinished[0] != 2 {
		t.Fatal("kill evidence lost")
	}
	if _, err := summarizeCompilerPhases(data, true, false); err == nil {
		t.Fatal("incomplete successful child accepted")
	}
	partial := append(append([]byte(nil), data...), []byte("[2,0,2,")...)
	summary, err = summarizeCompilerPhases(partial, false, false)
	if err == nil || summary.Unfinished == nil || summary.Unfinished[0] != 2 || len(summary.Completed) != 1 {
		t.Fatal("last valid entry lost on partial killed write")
	}
	if s, err := summarizeCompilerPhases(nil, false, false); err != nil || s.Unfinished != nil || len(s.Completed) != 0 {
		t.Fatal("startup unknown fabricated")
	}
}

func TestCompilerPhaseUnavailableContextNeverScans(t *testing.T) {
	dir, ctx, dest, root, r := phaseFixture(t, 0)
	// Make the alleged original root unreachable. A context refusal must still
	// name missing historical comparisons, not scan or write the original root.
	r.Arguments[3] = filepath.Join(root, "unavailable-worker")
	rewritePhaseRecord(t, dir, r)
	if os.Remove(ctx) != nil {
		t.Fatal("remove context")
	}
	out, err := phaseInvoke(t, dir, ctx, dest)
	if err == nil || !bytes.Contains(out, []byte("faithful context unavailable (context.dependencies, context.generated, context.inherited_configuration, context.default_libraries)")) {
		t.Fatalf("context-only refusal lost: %v %s", err, out)
	}
	for _, path := range []string{r.Arguments[3], dest, filepath.Join(dir, "replay.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("context refusal touched source or reserved experiment")
		}
	}
}

func TestCompilerPhaseRejectsRetainedTelemetryLeak(t *testing.T) {
	dir, _, dest, _, r := phaseFixture(t, 0)
	if os.Mkdir(dest, 0o700) != nil {
		t.Fatal("mkdir")
	}
	// Deliberately malicious synthetic side channel. stdout still decodes; it
	// must not turn a leaked path/body into an accepted/retained phase record.
	script := []byte(`import fs from "node:fs"; fs.writeSync(3,JSON.stringify(["PRIVATE_PATH_OR_BODY"])+"\n"); process.stdout.write('{"configuration":"synthetic","files":[]}');`)
	err := executeCompilerPhaseChild(dest, r, phaseRead(t, filepath.Join(dir, "request.json")), script, time.Second)
	if err == nil || err.Error() != "refused nonnumeric phase telemetry" {
		t.Fatalf("leak accepted: %v", err)
	}
	if len(phaseRead(t, filepath.Join(dest, "phases.jsonl"))) != 0 {
		t.Fatal("leak retained in telemetry")
	}
	if bytes.Contains(phaseRead(t, filepath.Join(dest, "measurement.json")), []byte("PRIVATE_PATH_OR_BODY")) {
		t.Fatal("leak retained in measurement")
	}
}

func TestCompilerPhaseTelemetryBound(t *testing.T) {
	dir, _, dest, _, r := phaseFixture(t, 0)
	if os.Mkdir(dest, 0o700) != nil {
		t.Fatal("mkdir")
	}
	script := []byte(`import fs from "node:fs"; fs.writeSync(3," ".repeat((4<<20)+2)+"PRIVATE_BODY"); process.stdout.write('{"configuration":"synthetic","files":[]}');`)
	err := executeCompilerPhaseChild(dest, r, phaseRead(t, filepath.Join(dir, "request.json")), script, time.Second)
	if err == nil || err.Error() != "phase telemetry exceeds finite bound" {
		t.Fatalf("oversize telemetry not classified: %v", err)
	}
	if len(phaseRead(t, filepath.Join(dest, "phases.jsonl"))) != 0 {
		t.Fatal("oversize telemetry retained")
	}
}

func TestCompilerPhaseDeadlineRetention(t *testing.T) {
	for _, tc := range []struct {
		phase, group int
		anchor       string
	}{
		{1, 0, "phaseEvent(1,0,1);"},
		{2, 0, "phaseEvent(2,0,1);"},
		{3, 1, "phaseEvent(3,phaseGroup,1,output.diagnostics.length);"},
		{4, 1, "phaseEvent(4,phaseGroup,1,output.diagnostics.length);"},
		{5, 1, "phaseEvent(5,phaseGroup,1,output.diagnostics.length,phaseSelected);"},
		{6, 2, "phaseEvent(6,phaseGroup,1,output.diagnostics.length);"},
		{7, 0, "phaseEvent(7,0,1,output.diagnostics.length);"},
	} {
		t.Run(fmt.Sprint(tc.phase), func(t *testing.T) {
			dir, _, dest, _, r := phaseFixture(t, 21, true)
			if os.Mkdir(dest, 0o700) != nil {
				t.Fatal("mkdir")
			}
			script, err := instrumentCompilerPhases(typeScriptCompilerScript)
			if err != nil {
				t.Fatal(err)
			}
			// Deterministic synthetic stall at each real source boundary. The
			// production opt-in path always keeps 45s. This small noLib fixture
			// uses 5s to reach its stall without racing default-library setup.
			condition := "true"
			if tc.group > 0 {
				condition = fmt.Sprintf("phaseGroup === %d", tc.group)
			}
			replacement := tc.anchor + "\nif (" + condition + ") Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10000);"
			script = bytes.Replace(script, []byte(tc.anchor), []byte(replacement), 1)
			err = executeCompilerPhaseChild(dest, r, phaseRead(t, filepath.Join(dir, "request.json")), script, 5*time.Second)
			if err == nil || !strings.Contains(err.Error(), "instrumented child incomplete") {
				t.Fatalf("deadline error=%v", err)
			}
			var record struct {
				Child   nativeChildRecord    `json:"child"`
				Summary compilerPhaseSummary `json:"phases"`
			}
			if json.Unmarshal(phaseRead(t, filepath.Join(dest, "measurement.json")), &record) != nil {
				t.Fatal("measurement")
			}
			summary, err := summarizeCompilerPhases(phaseRead(t, filepath.Join(dest, "phases.jsonl")), false, false)
			if err != nil || record.Child.Ending != "extractor_deadline" || record.Child.ResponseDecoded || summary.Unfinished == nil || summary.Unfinished[0] != int64(tc.phase) || summary.Unfinished[1] != int64(tc.group) || !reflect.DeepEqual(summary, record.Summary) {
				observedPhase, observedGroup := int64(0), int64(0)
				if summary.Unfinished != nil {
					observedPhase, observedGroup = summary.Unfinished[0], summary.Unfinished[1]
				}
				t.Fatalf("deadline evidence lost: target=%d/%d observed=%d/%d ending=%s decoded=%t parser_error=%v", tc.phase, tc.group, observedPhase, observedGroup, record.Child.Ending, record.Child.ResponseDecoded, err)
			}
			if tc.phase == 6 && summary.Unfinished[6] < 20 {
				t.Fatal("after-budget traversal was not reached")
			}
			if record.Child.UserNS <= 0 || record.Child.ElapsedNS < 5*int64(time.Second) {
				t.Fatal("child CPU/wall ending lost")
			}
			for _, delta := range summary.Completed {
				if delta.Wall < 0 || delta.User < 0 || delta.System < 0 {
					t.Fatal("invalid completed deltas")
				}
			}
		})
	}
}
