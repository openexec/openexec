defmodule OpenExecRuntime.TaskWorker do
  @moduledoc """
  A stateful worker representing an isolated AI agent for a specific task.
  """
  use GenServer, restart: :temporary
  require Logger

  # --- Client API ---

  def start_link(task_id, project_path) do
    name = {:via, Registry, {OpenExecRuntime.TaskRegistry, task_id}}
    GenServer.start_link(__MODULE__, %{task_id: task_id, path: project_path}, name: name)
  end

  @doc "Triggers one iteration of the AI agent loop for this task."
  def run_iteration(task_id) do
    case Registry.lookup(OpenExecRuntime.TaskRegistry, task_id) do
      [{pid, _}] -> GenServer.call(pid, :execute)
      [] -> {:error, :not_found}
    end
  end

  # --- Server Callbacks ---

  @impl true
  def init(args) do
    Logger.info("[BEAM] Initializing worker for task #{args.task_id} in #{args.path}")
    
    # State holds our "Surgical Context" and iteration count
    state = %{
      task_id: args.task_id,
      project_path: args.path,
      iterations: 0,
      status: :idle,
      last_activity: DateTime.utc_now()
    }
    
    {:ok, state}
  end

  @impl true
  def handle_call(:execute, _from, state) do
    new_count = state.iterations + 1
    Logger.info("[BEAM] Executing iteration #{new_count} for #{state.task_id}")
    
    # Here we would call the local BitNet router or external LLM
    # result = OpenExecRuntime.DCP.route_and_execute(state)
    
    {:reply, :ok, %{state | iterations: new_count, status: :running}}
  end
end
