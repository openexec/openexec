defmodule OpenExecRuntime.Application do
  @moduledoc """
  The entry point for the OpenExec BEAM Runtime.
  Sets up the Supervision tree for autonomous self-healing.
  """
  use Application

  @impl true
  def start(_type, _args) do
    children = [
      # DynamicSupervisor allows us to spawn workers for each task (T-001, etc) on demand
      {DynamicSupervisor, name: OpenExecRuntime.TaskSupervisor, strategy: :one_for_one},
      
      # Registry allows us to find workers by their Task ID (e.g. "T-001") across nodes
      {Registry, keys: :unique, name: OpenExecRuntime.TaskRegistry}
    ]

    opts = [strategy: :one_for_one, name: OpenExecRuntime.Supervisor]
    Supervisor.start_link(children, opts)
  end
end
