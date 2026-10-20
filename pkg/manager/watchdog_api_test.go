package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/pkg/db/state"
)

// Exercise the standalone native API path: HTTP provider -> APIRunner -> real
// buffered shell tool -> apiLoopAdapter -> pipeline -> manager watchdog/SQLite.
// No admitted executor or private event consumer supplies activity.
func TestNativeWatchdogAPIShell(t *testing.T) {
	for _, mode := range []string{"healthy", "owner", "timeout", "tool-timeout", "then-silent"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			bpDir := filepath.Join(dir, ".openexec", "blueprints")
			if err := os.MkdirAll(bpDir, 0700); err != nil {
				t.Fatal(err)
			}
			body := "id: api_probe\nname: API probe\ninitial_stage: prepare\nstages:\n  prepare:\n    name: prepare\n    type: deterministic\n    commands: ['printf retained > completed.txt']\n    create_checkpoint: true\n    on_success: implement\n  implement:\n    name: implement\n    type: agentic\n    on_success: complete\n"
			if err := os.WriteFile(filepath.Join(bpDir, "api_probe.yaml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			command := "sleep 0.8; printf checked > checked.txt"
			timeout := 3000
			if mode == "owner" || mode == "timeout" || mode == "tool-timeout" {
				command = "exec sleep 30"
			}
			if mode == "tool-timeout" {
				timeout = 800
			}
			releaseProvider := make(chan struct{})
			calls := 0 // net/http serial requests from one runner
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					input, _ := json.Marshal(map[string]any{"command": command, "timeout_ms": timeout})
					fmt.Fprintf(w, `{"id":"probe","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"shell","type":"function","function":{"name":"run_shell_command","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, string(input))
					return
				}
				if mode == "then-silent" {
					select {
					case <-r.Context().Done():
					case <-releaseProvider:
					}
					return
				}
				fmt.Fprint(w, `{"id":"done","choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			defer close(releaseProvider)
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
			defer func() { cancel(); _ = m.Wait(context.Background(), "api-probe") }()
			err = m.Start(ctx, "api-probe", WithBlueprint("api_probe"), func(c *pipeline.Config) {
				c.APIProvider = "fixture"
				c.APIBaseURL = server.URL
				c.APIKey = "disposable-fixture"
				c.APIModel = "gpt-4o"
				c.DefaultMaxIterations = 2
				c.MaxRetries = 0
			})
			if err != nil {
				t.Fatal(err)
			}
			events, unsub, err := m.Subscribe("api-probe")
			if err != nil {
				t.Fatal(err)
			}
			defer unsub()
			started, ended, terminal := false, false, ""
			toolTimedOut := false
			for event := range events {
				if event.Type == loop.EventToolResult && strings.Contains(event.Text, "Exit code:") {
					toolTimedOut = true
				}
				if event.Type == loop.EventExecutionActivity {
					switch event.ExecutionActivity {
					case "check_start":
						started = true
						if mode == "owner" {
							if err := m.Stop("api-probe"); err != nil {
								t.Fatal(err)
							}
						}
					case "check_end":
						ended = true
					}
				}
				if event.Type == loop.EventBlueprintFailed || event.Type == loop.EventBlueprintComplete {
					terminal = event.CancellationCause
				}
			}
			if err := m.Wait(context.Background(), "api-probe"); err != nil {
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
			info, err := m.Status("api-probe")
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
			checkpoint, err := os.ReadFile(filepath.Join(dir, ".openexec", "checkpoints", "api-probe.jsonl"))
			if err != nil || !strings.Contains(string(checkpoint), "prepare") {
				t.Fatalf("checkpoint lost: %s %v", checkpoint, err)
			}
			fresh, err := state.NewStore(filepath.Join(dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			run, err := fresh.GetRun(context.Background(), "api-probe")
			if err != nil || run == nil || run.Status != string(info.Status) || (want != "" && run.ErrorMessage.String != want) {
				t.Fatalf("persisted receipt: %+v %v", run, err)
			}
			t.Logf("native API shell %s: status=%s cause=%q; shell scope closed and completed work/SQLite receipt retained", mode, info.Status, want)
		})
	}
}
