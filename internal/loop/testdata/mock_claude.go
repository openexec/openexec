// mock_claude is a test helper that simulates Claude Code's stream-JSON output.
// It reads a scenario name from the first argument and outputs predefined JSON lines.
//
// Scenarios:
//
//	ok              — outputs a minimal valid session, exits 0
//	full            — outputs text + tool_use + tool_result, exits 0
//	crash           — outputs partial, exits 1
//	slow            — outputs text then sleeps 10s (for kill/cancel tests), exits 0
//	signal-complete — outputs text + openexec_signal phase-complete, exits 0
//	signal-progress — outputs text + openexec_signal progress, exits 0
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	scenario := "ok"
	if len(os.Args) > 1 {
		scenario = os.Args[1]
	}

	switch scenario {
	case "ok":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from mock."}]}}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"Done."}]}}`)
		os.Exit(0)

	case "full":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"I will write a file."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Write","input":{"file_path":"test.go","content":"package main"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"File created successfully"}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"All done."}]}}`)
		os.Exit(0)

	case "crash":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Fprintln(os.Stderr, "segfault or something")
		os.Exit(1)

	case "slow":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Working..."}]}}`)
		time.Sleep(10 * time.Second)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-complete":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"All tests passing."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"openexec_signal","input":{"type":"phase-complete","reason":"All tests passing"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: phase-complete"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-progress":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Making progress."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"openexec_signal","input":{"type":"progress","reason":"Step done"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: progress"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-route-spark":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Review complete. Routing to Spark for rework."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__openexec_signal","input":{"type":"route","reason":"Implementation needs rework","target":"spark"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: route"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-route-hon":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Review approved. Routing to Hon for refactoring."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__openexec_signal","input":{"type":"route","reason":"Code approved, proceed to refactor","target":"hon"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: route"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-blocked":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Cannot proceed without API credentials."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__openexec_signal","input":{"type":"blocked","reason":"Missing API credentials"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: blocked"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "no-progress":
		// Scenario for testing C-003 exit strategy: completes without sending progress signals
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Working without progress signals."}]}}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"Done but no signal."}]}}`)
		os.Exit(0)

	case "build-fail-recoverable":
		// Scenario for G-005: Simulates a recoverable build failure that emits diagnostics
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Running go build..."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"go build ./..."}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"main.go:10:5: undefined: someFunc"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Build failed. Attempting to fix..."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu2","name":"openexec_signal","input":{"type":"progress","reason":"Captured build diagnostic, attempting fix"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu2","content":"Signal received: progress"}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"Captured diagnostic and continuing."}]}}`)
		os.Exit(0)

	case "build-fail-then-recover":
		// Scenario for G-005: First fails, subsequent retry succeeds (simulates checkpoint/recovery)
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Attempting recovery after build failure."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"openexec_signal","input":{"type":"progress","reason":"Recovery attempt in progress"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: progress"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu2","name":"openexec_signal","input":{"type":"phase-complete","reason":"Recovery successful"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu2","content":"Signal received: phase-complete"}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"Recovered successfully."}]}}`)
		os.Exit(0)

	case "soft-fail-diagnostic":
		// Scenario for G-005: Emits build failure diagnostic via stderr but exits 0 (soft-fail)
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Build soft-failed, capturing diagnostics."}]}}`)
		fmt.Fprintln(os.Stderr, "go build: main.go:15:2: syntax error: unexpected EOF")
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"openexec_signal","input":{"type":"progress","reason":"Build soft-failed, diagnostic captured"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: progress"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu2","name":"openexec_signal","input":{"type":"phase-complete","reason":"Soft-fail handled gracefully"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu2","content":"Signal received: phase-complete"}`)
		fmt.Println(`{"type":"result","result":{"content":[{"type":"text","text":"Soft-fail complete."}]}}`)
		os.Exit(0)

	case "crash-then-recover":
		// Scenario for G-005: First iteration crashes (exit 1), used with retry to test recovery
		// This simulates a hard crash that will trigger retry logic
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Build crashed unexpectedly."}]}}`)
		fmt.Fprintln(os.Stderr, "FATAL: internal compiler error at line 42")
		os.Exit(1)

	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %s\n", scenario)
		os.Exit(2)
	}
}
