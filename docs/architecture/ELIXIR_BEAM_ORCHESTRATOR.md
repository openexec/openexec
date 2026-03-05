# OpenExec "Scale Layer": Elixir / BEAM Orchestrator Architecture

This document outlines the architectural shift from Go-based concurrency (goroutines + mutexes) to an Elixir-based **BEAM / OTP (Open Telecom Platform)** model for the OpenExec Deterministic Control Plane.

## 1. Why Elixir / BEAM?

The current Go implementation uses `sync.WaitGroup` and `sync.Mutex` to manage parallel task execution. While fast, this model is "fragile" when dealing with unpredictable entities like AI agents. If a worker panics or an LLM call hangs indefinitely, managing the blast radius and recovery is complex.

**The BEAM Advantage:**
*   **"Let it Crash" Philosophy:** Errors are expected. Instead of defensive `try/catch` blocks, processes crash cleanly and are instantly restarted by a **Supervisor** into a known good state.
*   **Process Isolation:** Every AI Agent runs in its own isolated memory space (a GenServer). A hallucinating agent cannot corrupt the memory of another agent.
*   **Native Distributed Queueing:** Built-in asynchronous messaging (mailboxes) eliminates the need for complex locking mechanisms.

---

## 2. The Supervision Tree

In Elixir, the orchestration is modeled as a hierarchical "Supervision Tree".

```text
[OpenExec.Application]
       │
       ├─▶ [Knowledge.Repo] (Ecto/SQLite Connection Pool)
       │
       ├─▶ [DCP.Router.Pool] (BitNet Intent Parsers)
       │
       └─▶ [Orchestration.Supervisor] (The Core Engine)
                   │
                   ├─▶ [Story.Supervisor] (US-001)
                   │       │
                   │       ├─▶ [Task.Worker] (T-001 - Implements Logic)
                   │       └─▶ [Task.Worker] (T-002 - Implements Tests)
                   │
                   └─▶ [Story.Supervisor] (US-002)
                           │
                           └─▶ [Task.Worker] (T-003)
```

### How it Self-Heals
If `Task.Worker (T-001)` gets stuck in an infinite "test-fail-fix" loop, the `Story.Supervisor` detects the timeout, kills the process, and restarts it with a "fresh context" or escalates to a more powerful model (e.g., swapping Haiku for Opus).

---

## 3. The GenServer Implementations

Here is what the Elixir implementation looks like compared to the current Go code.

### A. The Agent Worker (GenServer)
Instead of a Go `for` loop, the agent is a state machine responding to messages.

```elixir
defmodule OpenExec.TaskWorker do
  use GenServer
  require Logger

  # --- Client API ---
  def start_link(task_id) do
    GenServer.start_link(__MODULE__, task_id, name: via_tuple(task_id))
  end

  def execute_step(pid, instruction) do
    GenServer.cast(pid, {:execute, instruction})
  end

  # --- Server Callbacks ---
  @impl true
  def init(task_id) do
    # Hydrate state from the Deterministic Knowledge Store
    context = Knowledge.Repo.get_pointers_for_task(task_id)
    {:ok, %{task_id: task_id, context: context, retries: 0}}
  end

  @impl true
  def handle_cast({:execute, instruction}, state) do
    # 1. Route Intent (Local BitNet)
    tool = DCP.Router.parse_intent(instruction)

    # 2. Check Hard Policy Gates
    case DCP.Policy.check(tool, state.context) do
      :ok -> 
        result = apply_tool(tool)
        {:noreply, %{state | context: update_context(state, result)}}

      {:error, reason} ->
        Logger.warn("Policy Blocked Agent: #{reason}")
        # Send violation back to LLM to self-correct
        {:noreply, state}
    end
  end
end
```

### B. The Story Supervisor
This replaces the complex Go DAG execution logic. It manages the workers and their dependencies natively.

```elixir
defmodule OpenExec.StorySupervisor do
  use DynamicSupervisor

  def start_link(story_id) do
    DynamicSupervisor.start_link(__MODULE__, story_id, name: via_tuple(story_id))
  end

  @impl true
  def init(_story_id) do
    # If a child dies, restart just that child (one_for_one)
    DynamicSupervisor.init(strategy: :one_for_one, max_restarts: 3)
  end

  def start_task(supervisor, task_data) do
    spec = {OpenExec.TaskWorker, task_data.id}
    DynamicSupervisor.start_child(supervisor, spec)
  end
end
```

---

## 4. Architectural Upgrades (DCP Integration)

By moving the DCP Coordinator to Elixir, we gain significant operational advantages:

### 1. In-Memory "Surgical Pointers" (ETS)
Currently, Go reads from SQLite for every pointer lookup. In Elixir, we load the `symbols` and `api_docs` tables into **ETS (Erlang Term Storage)** on startup.
*   **Result:** Pointer lookups happen in memory across all agent processes with zero database locking or disk I/O.

### 2. Distributed Operations
Elixir nodes connect over a mesh network. 
*   **Current Flow:** The `DeployTool` runs `gcloud` locally to connect to a remote server.
*   **Elixir Flow:** You deploy a lightweight OpenExec node to your 10 production servers. The local Orchestrator sends an Elixir message directly to the remote node. The command executes natively on the target hardware, returning the result securely via the BEAM distribution protocol.

## Summary

The current Go/Python architecture is perfect for single-machine CLI execution. Transitioning the **Server and Execution Engine** to Elixir transforms OpenExec into a resilient, distributed, enterprise-grade AI Operating System capable of managing massive, multi-environment deployments autonomously.
