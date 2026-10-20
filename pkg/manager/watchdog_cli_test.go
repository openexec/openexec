package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/pkg/db/state"
)

// Exercise the actual standalone CLI process/parser and MCP shell handler.
// Only the provider CLI is a disposable protocol fixture; no private event
// consumer or admitted executor injects activity into the manager.
func TestNativeWatchdogCLITool(t *testing.T) {
	// The Go helper stands in for a provider CLI. Race instrumentation's
	// artificial one-second exit delay is not provider inference or tool work.
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	for _, mode := range []string{"healthy", "owner", "timeout", "tool-timeout", "then-silent"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			bpDir := filepath.Join(dir, ".openexec", "blueprints")
			if err := os.MkdirAll(bpDir, 0700); err != nil {
				t.Fatal(err)
			}
			body := "id: cli_probe\nname: CLI probe\ninitial_stage: prepare\nstages:\n  prepare:\n    name: prepare\n    type: deterministic\n    commands: ['printf retained > completed.txt']\n    create_checkpoint: true\n    on_success: implement\n  implement:\n    name: implement\n    type: agentic\n    on_success: complete\n"
			if err := os.WriteFile(filepath.Join(bpDir, "cli_probe.yaml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			store, err := state.NewStore(filepath.Join(dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m, err := New(Config{WorkDir: dir, StateStore: store, WatchdogStallThreshold: 250 * time.Millisecond, WatchdogCheckInterval: 10 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			duration := 5 * time.Second
			if mode == "timeout" {
				duration = 1200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()
			defer func() { cancel(); _ = m.Wait(context.Background(), "cli-probe") }()
			err = m.Start(ctx, "cli-probe", WithBlueprint("cli_probe"), func(c *pipeline.Config) {
				c.CommandName = executable
				c.CommandArgs = []string{"-test.run=^TestNativeWatchdogCLIHelper$", "--", "watchdog-cli-fixture", mode}
				c.DefaultMaxIterations = 2
				c.MaxRetries = 0
			})
			if err != nil {
				t.Fatal(err)
			}
			events, unsub, err := m.Subscribe("cli-probe")
			if err != nil {
				t.Fatal(err)
			}
			defer unsub()
			started, ended, terminal := false, false, ""
			toolTimedOut := false
			for event := range events {
				if event.Type == loop.EventToolResult && strings.Contains(event.Text, "command timed out") {
					toolTimedOut = true
				}
				if event.Type == loop.EventExecutionActivity {
					switch event.ExecutionActivity {
					case "check_start":
						started = true
						if mode == "owner" {
							// Let the real silent tool exceed the stall threshold
							// before exercising authenticated-boundary Stop.
							time.AfterFunc(600*time.Millisecond, func() { _ = m.Stop("cli-probe") })
						}
					case "check_end":
						ended = true
					}
				}
				if event.Type == loop.EventBlueprintFailed || event.Type == loop.EventBlueprintComplete {
					terminal = event.CancellationCause
				}
			}
			if err := m.Wait(context.Background(), "cli-probe"); err != nil {
				t.Fatal(err)
			}
			if mode == "tool-timeout" && !toolTimedOut {
				t.Fatal("tool timeout did not reach provider feedback")
			}
			want := ""
			switch mode {
			case "owner":
				want = "owner_stop"
			case "timeout":
				want = "timeout"
			case "then-silent":
				want = "watchdog_stall"
			}
			info, err := m.Status("cli-probe")
			if err != nil {
				t.Fatal(err)
			}
			if info.CancellationCause != want || terminal != want {
				t.Fatalf("cause=%q terminal=%q want=%q status=%+v", info.CancellationCause, terminal, want, info)
			}
			if want == "" && info.Status != StatusComplete {
				t.Fatalf("legitimate buffered shell killed: %+v", info)
			}
			if !started || !ended {
				t.Fatalf("shell scope missing: start=%v end=%v", started, ended)
			}
			if want == "" && info.PendingChecks != 0 {
				t.Fatalf("shell scope leaked: %+v", info)
			}
			if mode == "healthy" || mode == "then-silent" {
				raw, err := os.ReadFile(filepath.Join(dir, "checked.txt"))
				if err != nil || string(raw) != "checked" {
					t.Fatalf("check did not finish: %s %v", raw, err)
				}
			}
			raw, err := os.ReadFile(filepath.Join(dir, "completed.txt"))
			if err != nil || string(raw) != "retained" {
				t.Fatalf("completed work lost: %s %v", raw, err)
			}
			checkpoint, err := os.ReadFile(filepath.Join(dir, ".openexec", "checkpoints", "cli-probe.jsonl"))
			if err != nil || !strings.Contains(string(checkpoint), "prepare") {
				t.Fatalf("checkpoint lost: %s %v", checkpoint, err)
			}
			fresh, err := state.NewStore(filepath.Join(dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			run, err := fresh.GetRun(context.Background(), "cli-probe")
			if err != nil || run == nil || run.Status != string(info.Status) || (want != "" && run.ErrorMessage.String != want) {
				t.Fatalf("persisted receipt: %+v %v", run, err)
			}
			t.Logf("native CLI/MCP tool %s: status=%s cause=%q; shell scope closed and completed work/SQLite receipt retained", mode, info.Status, want)
		})
	}
}

// TestNativeWatchdogCLIHelper runs only as a child process selected by CommandArgs.
// It emits Claude stream-json around a real tools/call to OpenExec's MCP server,
// whose shell handler buffers a real subprocess's stdout/stderr until exit.
func TestNativeWatchdogCLIHelper(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "watchdog-cli-fixture" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	command, timeout := "sleep 0.8; printf checked > checked.txt", 3000
	if mode == "owner" || mode == "timeout" {
		command = "sleep 2; printf finished > interrupted.txt"
	}
	if mode == "tool-timeout" {
		command, timeout = "exec sleep 3", 800
	}
	input := map[string]any{"command": command, "timeout_ms": timeout}
	emit := func(value any) {
		if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
			os.Exit(2)
		}
	}
	emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_use", "id": "shell", "name": "mcp__openexec-signal__run_shell_command", "input": input},
	}}})
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "run_shell_command", "arguments": input}})
	var response bytes.Buffer
	dir, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	server, err := mcp.NewServerWithConfig(bytes.NewReader(append(req, '\n')), &response, mcp.ServerConfig{WorkDir: dir, Mode: "danger-full-access"})
	if err == nil {
		err = server.Serve()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var rpc struct {
		Result struct {
			Content json.RawMessage `json:"content"`
			IsError bool            `json:"isError"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(response.Bytes(), &rpc); err != nil || rpc.Error != nil || len(rpc.Result.Content) == 0 {
		fmt.Fprintln(os.Stderr, "invalid MCP reply:", response.String(), err)
		os.Exit(2)
	}
	emit(map[string]any{"type": "user", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": "shell", "content": rpc.Result.Content, "is_error": rpc.Result.IsError},
	}}})
	if mode == "then-silent" {
		// Silent inference resumes after the tool, with no fabricated events.
		time.Sleep(30 * time.Second)
	}
	emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "done"}}}})
	emit(map[string]any{"type": "result", "subtype": "success"})
	os.Exit(0)
}
