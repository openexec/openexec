import subprocess, hashlib

R = 'pkg/execution/context_recovery.go'
T = 'pkg/execution/token_budget.go'
A = "TestSearchOmissionRequiresPositionBoundScanProvenance|TestOmissionPreservationClasses"
BIND = 'complete := completeScans[position] != "" && completeScans[position] == sha256Hex(block.ToolOutput)'
G = [
    ("G1 position+hash binding", R, BIND, 'complete := completeScans[position] != ""', A),
    ("G2 provenance required", R, BIND, 'complete := true', A),
    ("G3 unambiguous call-ID pairing", R, 'complete && !ambiguous[block.ToolResultID]', 'complete', "TestSearchOmissionRequiresPositionBoundScanProvenance"),
    ("G4 only list_directory/search_files", R, '''						case "search_files":
							labels[position] = "complete search result"
''', '''						case "search_files":
							labels[position] = "complete search result"
						default:
							labels[position] = "result"
''', A),
    ("G5 no tool error", R, 'labels[[2]int{i, j}] == "" || b.ToolError != ""', 'labels[[2]int{i, j}] == ""', A),
    ("G6 minimum size 256", R, 'b.ToolError != "" || len(b.ToolOutput) < 256', 'b.ToolError != ""', "TestOmissionEligibilityRequiresPairingNoErrorAndSize"),
    ("G7 newest result kept", R, 'if kept <= 1 {', 'if kept <= 0 {', A),
    ("G8 recorder: no tool error", T, '	if toolErr != nil {\n		return\n	}\n	if (tool', '	if (tool', "TestOmissionPreservationClasses"),
    ("G9 recorder: scan-complete flag", T, '(tool == "search_files" && scanComplete)', '(tool == "search_files")', "TestOmissionPreservationClasses"),
    ("G10 recorder: vouching executor", T, '(tool == "list_directory" && p.trustedDirectoryResults)', '(tool == "list_directory")', "TestListingProvenanceComesFromVouchingExecutor"),
]


def sh(c):
    p = subprocess.run(c, shell=True, capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def h(f):
    return hashlib.sha256(open(f, 'rb').read()).hexdigest()


for name, f, old, new, tests in G:
    src = open(f).read()
    assert src.count(old) == 1, (name, src.count(old))
    print("=== GUARD", name, "| file", f, "| sha256(original)", h(f))
    print("tests:", tests)
    open(f, 'w').write(src.replace(old, new))
    print("sha256(mutated)", h(f))
    rc, o = sh("go build ./pkg/execution/ && echo BUILD_OK")
    print("compile:", o.strip())
    rc, o = sh("go test ./pkg/execution/ -count=1 -run '%s' 2>&1 | grep -E '^(---|FAIL|ok)|omitted|recorded|changed' | head -8" % tests)
    print("RED (guard removed):\n" + o)
    open(f, 'w').write(src)
    print("sha256(restored)", h(f))
    rc, o = sh("go test ./pkg/execution/ -count=1 -run '%s' 2>&1 | tail -3" % tests)
    print("GREEN (guard restored):\n" + o)
