// An external module: only exported OpenExec imports are permitted here.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/openexec/openexec/pkg/runtime"
)

func main() {
	ctx := context.Background()
	binding := runtime.TerminalBinding{TaskID: "external-task", Stage: "verify", TaskAttempt: 1, StageAttempt: 1, Source: "deterministic-runner"}
	code := 1
	terminal := runtime.TerminalCompletion{ID: "external-terminal", Binding: binding, Outcome: "exited", ExitCode: &code}
	dir, err := os.MkdirTemp("", "external-terminal-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := dir + "/terminal.json"
	persist := func() {
		data, err := json.Marshal(terminal)
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			panic(err)
		}
	}
	// Isolated caller-owned fixture, not a production authentication adapter.
	load := func(_ context.Context, id string) (runtime.TerminalCompletion, error) {
		var saved runtime.TerminalCompletion
		data, err := os.ReadFile(path)
		if err != nil {
			return saved, err
		}
		err = json.Unmarshal(data, &saved)
		if saved.ID != id {
			return saved, errors.New("wrong terminal")
		}
		return saved, err
	}
	persist()
	err = runtime.VerificationTerminalFailure(ctx, binding, terminal.ID, load)
	if err == nil || err.Error() != "verification verify exited 1" {
		panic(fmt.Sprintf("failed exit: %v", err))
	}
	code = 0
	persist()
	if err = runtime.VerificationTerminalFailure(ctx, binding, terminal.ID, load); err != nil {
		panic(err)
	}
	binding.TaskAttempt++
	if runtime.VerificationTerminalFailure(ctx, binding, terminal.ID, load) == nil {
		panic("stale binding accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(runtime.VerificationTerminalFailure(cancelled, binding, terminal.ID, load), context.Canceled) {
		panic("cancellation lost")
	}
	if runtime.VerificationCommandFailure(ctx, "verify", nil) != nil {
		panic("success lost")
	}
	fmt.Println("external-consumer: PASS (exported API, persisted fixture reload, stale binding and cancellation)")
}
