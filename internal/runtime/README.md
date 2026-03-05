# OpenExec BEAM Runtime

This is the fault-tolerant orchestration layer for OpenExec, built with Elixir and OTP.

## Purpose
The BEAM Runtime manages long-running, stateful AI agent tasks. It provides:
1.  **Process Isolation:** Each agent (TaskWorker) runs in its own memory space.
2.  **Self-Healing:** Supervision trees automatically restart agents if they crash.
3.  **Location Transparency:** Native support for distributed orchestration across multiple servers.

## Installation
You must have Elixir 1.14+ installed.

```bash
cd internal/runtime
mix deps.get
```

## Running
To start the runtime in interactive mode:
```bash
iex -S mix
```

To spawn a new task worker manually:
```elixir
DynamicSupervisor.start_child(OpenExecRuntime.TaskSupervisor, {OpenExecRuntime.TaskWorker, ["T-001", "/path/to/project"]})
```

To trigger an iteration:
```elixir
OpenExecRuntime.TaskWorker.run_iteration("T-001")
```
