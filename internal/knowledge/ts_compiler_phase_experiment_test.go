package knowledge

// Finite, opt-in test harness only. No production instrumentation or policy.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

var compilerPhaseOutput = flag.String("compiler-phase-output", "", "new private experiment directory, separate from original capture")
var compilerPhaseContext = flag.String("compiler-phase-context", "", "supervisor-retained historical context inventory; never synthesize from current worker")

// The supervisor supplies this historical inventory separately from the four
// unchanged capture files. A current reconstruction inventory cannot authenticate
// history. Every tree includes directories, missing roots and symlink targets;
// symlink destinations must also be inventoried under external roots. Paths and
// digests are PRIVATE. This is a test-input format, not a runtime context schema.
type compilerPhaseContextRecord struct {
	Complete bool                                    `json:"supervisor_attests_complete_historical_closure"`
	Request  string                                  `json:"request_sha256"`
	Manifest string                                  `json:"manifest_hash"`
	Trees    map[string]map[string]map[string]string `json:"trees"`
}

var compilerPhaseDomains = []string{"dependencies", "generated", "inherited_configuration", "default_libraries"}

func phaseTree(root string) (map[string]string, error) {
	result := map[string]string{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(root string) error {
		if visited[root] {
			return nil
		}
		visited[root] = true
		return filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if os.IsNotExist(e) && p == root {
				result[p] = "absent"
				return nil
			}
			if e != nil {
				return e
			}
			if d.Name() == ".git" && d.IsDir() {
				return filepath.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(p)
				if err != nil {
					return err
				}
				result[p] = "link:" + target
				canonical, err := filepath.EvalSymlinks(p)
				if err != nil {
					return errors.New("unresolved context symlink")
				}
				if err := visit(canonical); err != nil {
					return err
				}
			} else if d.IsDir() {
				result[p] = "directory"
			} else {
				h := captureFileHash(p)
				if strings.HasPrefix(h, "unknown:") {
					return errors.New("unreadable context input")
				}
				result[p] = h
			}
			return nil
		})
	}
	err := visit(root)
	return result, err
}

func phaseContextDifferences(name string, request []byte, identity compilerReplayIdentity, args []string) []string {
	var c compilerPhaseContextRecord
	b, err := os.ReadFile(name)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return []string{"context.dependencies", "context.generated", "context.inherited_configuration", "context.default_libraries"}
	}
	var differences []string
	if !c.Complete { // phase-guard: complete-context
		differences = append(differences, "context.complete_historical_closure")
	}
	if c.Request != captureHash(request) || c.Manifest != identity.Manifest.ManifestHash { // phase-guard: binding
		differences = append(differences, "context.capture_binding")
	}
	for _, domain := range compilerPhaseDomains {
		trees := c.Trees[domain]
		if len(trees) == 0 { // phase-guard: missing-context
			differences = append(differences, "context."+domain)
			continue
		}
		requiredRoot := ""
		switch domain {
		case "generated":
			requiredRoot = args[3]
		case "dependencies":
			requiredRoot = filepath.Join(args[3], "node_modules")
		case "default_libraries":
			requiredRoot = filepath.Dir(args[2])
		}
		if requiredRoot != "" && trees[requiredRoot] == nil { // phase-guard: context-coverage
			differences = append(differences, "context."+domain+".coverage")
			continue
		}
		for root, expected := range trees {
			actual, err := phaseTree(root)
			if !filepath.IsAbs(root) || err != nil || !reflect.DeepEqual(actual, expected) { // phase-guard: context-drift
				differences = append(differences, "context."+domain)
				break
			}
		}
	}
	return differences
}

// Numeric rows only: phase, group, entry(1)/exit(2), monotonic ns,
// cumulative user/system CPU us, diagnostic count, selected(-1/0/1),
// cumulative time spent sampling/serializing/writing telemetry ns. Entry/exit
// timestamps include previous instrumentation overhead; overhead is not a
// counterfactual uninstrumented runtime estimate.
type compilerPhaseRow [9]int64
type compilerPhaseDelta struct {
	Phase  int64 `json:"phase"`
	Group  int64 `json:"group"`
	Wall   int64 `json:"wall_ns"`
	User   int64 `json:"user_us"`
	System int64 `json:"system_us"`
}
type compilerPhaseSummary struct {
	Completed          []compilerPhaseDelta `json:"completed"`
	Unfinished         *compilerPhaseRow    `json:"unfinished,omitempty"`
	OverheadLowerBound int64                `json:"telemetry_overhead_lower_bound_ns"`
}

func instrumentCompilerPhases(script []byte) ([]byte, error) {
	s := string(script)
	// Exact, single anchors refuse a changed extractor rather than silently
	// instrumenting only some phases. Production bytes are never edited.
	patches := [][2]string{
		{`const require = createRequire(import.meta.url);`, `const phaseFD = 3;
const phaseStart = process.hrtime.bigint();
let phaseOverhead = 0n;
function phaseEvent(p,g,e,b=0,s=-1) {
  const begin = process.hrtime.bigint();
  const cpu = process.cpuUsage();
  fs.writeSync(phaseFD, JSON.stringify([p,g,e,Number(begin-phaseStart),cpu.user,cpu.system,b,s,Number(phaseOverhead)])+"\n");
  phaseOverhead += process.hrtime.bigint()-begin;
}
phaseEvent(1,0,1);
const require = createRequire(import.meta.url);`},
		{`const ts = require(compilerPath);`, "const ts = require(compilerPath);\nphaseEvent(1,0,2);\nphaseEvent(2,0,1);"},
		{`function exported(node) {`, "phaseEvent(2,0,2,output.diagnostics.length);\nfunction exported(node) {"},
		{`for (const group of groups.values()) {`, "let phaseGroup = 0;\nfor (const group of groups.values()) {\n  phaseGroup++;\n  phaseEvent(3,phaseGroup,1,output.diagnostics.length);"},
		{`  const checker = program.getTypeChecker();`, "  phaseEvent(3,phaseGroup,2,output.diagnostics.length);\n  phaseEvent(4,phaseGroup,1,output.diagnostics.length);\n  const checker = program.getTypeChecker();\n  phaseEvent(4,phaseGroup,2,output.diagnostics.length);"},
		{`  for (const diagnostic of ` + diagnosticBudgetGuard + `) {`, "  const phaseSelected = output.diagnostics.length < 20 ? 1 : 0;\n  phaseEvent(5,phaseGroup,1,output.diagnostics.length,phaseSelected);\n  for (const diagnostic of " + diagnosticBudgetGuard + ") {"},
		{`  for (const sourceFile of program.getSourceFiles()) {`, "  phaseEvent(5,phaseGroup,2,output.diagnostics.length,phaseSelected);\n  phaseEvent(6,phaseGroup,1,output.diagnostics.length);\n  for (const sourceFile of program.getSourceFiles()) {"},
		{"\n}\n\noutput.files.sort", "\n  phaseEvent(6,phaseGroup,2,output.diagnostics.length);\n}\n\nphaseEvent(7,0,1,output.diagnostics.length);\noutput.files.sort"},
		{`process.stdout.write(JSON.stringify(output));`, "process.stdout.write(JSON.stringify(output));\nphaseEvent(7,0,2,output.diagnostics.length);"},
	}
	for _, p := range patches {
		if strings.Count(s, p[0]) != 1 { // phase-guard: required-anchor
			return nil, errors.New("required phase instrumentation absent")
		}
		s = strings.Replace(s, p[0], p[1], 1)
	}
	return []byte(s), nil
}

func summarizeCompilerPhases(data []byte, success bool, emptyRequest bool) (compilerPhaseSummary, error) {
	var summary compilerPhaseSummary
	var previous compilerPhaseRow
	nextPhase, group := int64(1), int64(0)
	var open *compilerPhaseRow
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var numbers []int64
		decodeErr := json.Unmarshal(line, &numbers)
		if decodeErr != nil || len(numbers) != 9 || bytes.Contains(line, []byte("null")) { // phase-guard: numeric-privacy
			return summary, errors.New("refused nonnumeric phase telemetry")
		}
		var r compilerPhaseRow
		copy(r[:], numbers)
		valid := r[0] == nextPhase && r[1] == group && r[2] == 1
		if open != nil {
			valid = r[0] == open[0] && r[1] == open[1] && r[2] == 2
		}
		valid = valid && r[3] >= previous[3] && r[4] >= previous[4] && r[5] >= previous[5] && r[6] >= previous[6] && r[8] >= previous[8]
		valid = valid && r[3] >= 0 && r[4] >= 0 && r[5] >= 0 && r[6] >= 0 && r[8] >= 0
		if r[0] == 5 {
			if open == nil {
				valid = valid && ((r[6] < 20 && r[7] == 1) || (r[6] >= 20 && r[7] == 0))
			} else {
				valid = valid && r[7] == open[7]
			}
		} else {
			valid = valid && r[7] == -1
		}
		// Serialization follows a completed traversal, or an explicitly empty
		// request. A missing entire program group must not masquerade as empty.
		if open == nil && nextPhase == 3 && (group > 1 || emptyRequest) && r[0] == 7 && r[1] == 0 && r[2] == 1 && r[7] == -1 {
			valid = r[3] >= previous[3] && r[4] >= previous[4] && r[5] >= previous[5] && r[6] >= previous[6] && r[8] >= previous[8]
		}
		if !valid { // phase-guard: event-order
			return summary, errors.New("refused invalid or missing phase event")
		}
		if open == nil {
			entry := r
			open = &entry
		} else {
			summary.Completed = append(summary.Completed, compilerPhaseDelta{r[0], r[1], r[3] - open[3], r[4] - open[4], r[5] - open[5]})
			open = nil
			nextPhase = r[0] + 1
			if r[0] == 2 {
				nextPhase, group = 3, 1
			}
			if r[0] == 6 {
				nextPhase, group = 3, group+1
			}
		}
		previous = r
		summary.OverheadLowerBound = r[8]
		summary.Unfinished = open
	}
	summary.Unfinished = open
	if success && (open != nil || len(summary.Completed) == 0 || previous[0] != 7 || previous[2] != 2) { // phase-guard: complete-success
		return summary, errors.New("successful child lacks required phases")
	}
	return summary, nil
}

func privatePhaseFile(name string, directory bool) bool {
	i, err := os.Lstat(name)
	mode := os.FileMode(0o600)
	if directory {
		mode = 0o700
	}
	return err == nil && i.Mode()&os.ModeSymlink == 0 && i.IsDir() == directory && i.Mode().Perm() == mode
}

func runCompilerPhaseExperiment(dir, contextFile, destination string) error {
	// All refusals are value-free. Never return OS errors that embed paths.
	if !filepath.IsAbs(dir) || !privatePhaseFile(dir, true) { // phase-guard: capture-mode
		return errors.New("private capture directory required")
	}
	for _, name := range []string{"request.json", "extractor.mjs", "identity.json", "result.json"} {
		if !privatePhaseFile(filepath.Join(dir, name), false) { // phase-guard: artifact-mode
			return errors.New("private capture artifact missing or unsafe")
		}
	}
	request, _ := os.ReadFile(filepath.Join(dir, "request.json"))
	script, _ := os.ReadFile(filepath.Join(dir, "extractor.mjs"))
	b, _ := os.ReadFile(filepath.Join(dir, "identity.json"))
	var identity compilerReplayIdentity
	if json.Unmarshal(b, &identity) != nil {
		return errors.New("invalid identity")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "result.json"))
	var result struct {
		Child nativeChildRecord `json:"child"`
	}
	if json.Unmarshal(b, &result) != nil {
		return errors.New("invalid result")
	}
	r := result.Child
	if len(r.Arguments) != 4 || r.Bound != "45s" || typeScriptExtractorTimeout != 45*time.Second || captureHash(request) != r.RequestSHA256 || captureHash(script) != r.ScriptSHA256 || !bytes.Equal(script, typeScriptCompilerScript) { // phase-guard: capture
		return errors.New("capture request/script/argv/bound mismatch")
	}
	// Refuse unavailable historical context BEFORE touching a source root. This
	// permits an honest context-only refusal on retained records without scanning
	// a busy worker or manufacturing a present-day reconstruction.
	if _, err := os.Stat(contextFile); err != nil { // phase-guard: unavailable-context
		return errors.New("faithful context unavailable (context.dependencies, context.generated, context.inherited_configuration, context.default_libraries)")
	}
	if !privatePhaseFile(contextFile, false) { // phase-guard: context-mode
		return errors.New("private historical context required")
	}
	manifest, err := BuildScanManifest(r.Arguments[3])
	if err != nil {
		return errors.New("manifest unavailable")
	}
	if differences := compilerReplayIdentityDifferences(manifest, identity, r); len(differences) != 0 { // phase-guard: identity
		return fmt.Errorf("source/configuration/toolchain changed (%s)", strings.Join(differences, ", "))
	}
	if differences := phaseContextDifferences(contextFile, request, identity, r.Arguments); len(differences) != 0 { // phase-guard: context-admission
		return fmt.Errorf("faithful context unavailable (%s)", strings.Join(differences, ", "))
	}
	instrumented, err := instrumentCompilerPhases(script)
	if err != nil {
		return err
	}
	// A separate experiment destination plus a single reservation on this
	// capture prevents changing output directories to repeat an admitted child.
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination || !privatePhaseFile(filepath.Dir(destination), true) { // phase-guard: destination-mode
		return errors.New("new private experiment destination required")
	}
	canonicalParent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return errors.New("experiment parent unavailable")
	}
	destination = filepath.Join(canonicalParent, filepath.Base(destination))
	for _, root := range []string{dir, r.Arguments[3]} {
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return errors.New("experiment boundary unavailable")
		}
		rel, e := filepath.Rel(root, destination)
		if e != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) { // phase-guard: destination-separate
			return errors.New("experiment destination must be separate")
		}
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("experiment destination exists or unavailable")
	}
	c := &compilerCapture{dir: dir}
	if err := c.write("replay.json", []byte(`{"status":"instrumented experiment reserved; never delete or retry"}`)); err != nil { // phase-guard: experiment-reservation
		return errors.New("capture already reserved")
	}
	if os.Mkdir(destination, 0o700) != nil {
		return errors.New("reserved experiment destination unavailable; never retry")
	}
	return executeCompilerPhaseChild(destination, r, request, instrumented, 45*time.Second)
}

func executeCompilerPhaseChild(destination string, original nativeChildRecord, request, script []byte, bound time.Duration) error {
	c := &compilerCapture{dir: destination}
	if c.write("instrumented.mjs", script) != nil {
		return errors.New("cannot retain private instrumented script")
	}
	telemetry, err := os.OpenFile(filepath.Join(destination, "phases.jsonl"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("cannot create private phase channel")
	}
	defer telemetry.Close()
	args := append([]string(nil), original.Arguments...)
	args[1] = filepath.Join(destination, "instrumented.mjs")
	parent := context.Background()
	child, cancel := context.WithTimeout(parent, bound)
	defer cancel()
	cmd := exec.CommandContext(child, args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(request)
	cmd.ExtraFiles = []*os.File{telemetry}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	r := childMeasurement(compilerObservation{Request: request, Command: cmd, Started: start, Elapsed: time.Since(start), Parent: parent, Child: child, RunError: runErr, StdoutBytes: stdout.Len(), StderrBytes: stderr.Len(), Stdout: stdout.Bytes()}, args[3])
	r.Bound = bound.String()
	r.ScriptSHA256 = captureHash(script)
	r.NodeSHA256, r.CompilerSHA256 = original.NodeSHA256, original.CompilerSHA256
	if _, err := telemetry.Seek(0, 0); err != nil {
		return errors.New("phase reread failed")
	}
	data, err := io.ReadAll(io.LimitReader(telemetry, (4<<20)+1))
	if err != nil {
		return errors.New("phase reread failed")
	}
	var req tsCompilerRequest
	requestErr := json.Unmarshal(request, &req)
	summary, phaseErr := summarizeCompilerPhases(data, r.Ending == "exit_0", requestErr == nil && len(req.Files) == 0)
	if len(data) > 4<<20 { // phase-guard: telemetry-bound
		phaseErr = errors.New("phase telemetry exceeds finite bound")
	}
	// Never retain malformed telemetry (could contain a path/body), even on
	// nonzero exit. The child record and fixed refusal persist without it.
	if phaseErr != nil {
		if telemetry.Truncate(0) != nil {
			return errors.New("cannot quarantine invalid telemetry")
		}
	}
	status := "valid numeric telemetry"
	if phaseErr != nil {
		status = phaseErr.Error()
	}
	if c.writeJSON("measurement.json", struct {
		Child      nativeChildRecord    `json:"child"`
		Summary    compilerPhaseSummary `json:"phases"`
		Status     string               `json:"telemetry_status"`
		ScriptHash string               `json:"modified_script_sha256"`
		Note       string               `json:"interpretation"`
	}{r, summary, status, captureHash(script), "changed script/path/mtime and telemetry overhead; cache/load not controlled; success is nonreproduction, not cure"}) != nil {
		return errors.New("cannot persist measurement")
	}
	if phaseErr != nil {
		return phaseErr
	}
	if r.Ending != "exit_0" || !r.ResponseDecoded {
		return errors.New("instrumented child incomplete; private phases and ending retained")
	}
	return nil
}
