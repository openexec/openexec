// mock_claude is a test helper that simulates Claude Code's stream-JSON output.
// It reads a scenario name from the first argument and outputs predefined JSON lines.
//
// Scenarios:
//   ok              — outputs a minimal valid session, exits 0
//   full            — outputs text + tool_use + tool_result, exits 0
//   crash           — outputs partial, exits 1
//   slow            — outputs text then sleeps 10s (for kill/cancel tests), exits 0
//   signal-complete — outputs text + openexec_signal phase-complete, exits 0
//   signal-progress — outputs text + openexec_signal progress, exits 0
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
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__openexec_signal","input":{"type":"phase-complete","reason":"All tests passing"}}]}}`)
		fmt.Println(`{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: phase-complete"}`)
		fmt.Println(`{"type":"result","result":{"content":[]}}`)
		os.Exit(0)

	case "signal-progress":
		fmt.Println(`{"type":"system","subtype":"init","session_id":"mock"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Making progress."}]}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__openexec_signal","input":{"type":"progress","reason":"Step done"}}]}}`)
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

	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %s\n", scenario)
		os.Exit(2)
	}
}
