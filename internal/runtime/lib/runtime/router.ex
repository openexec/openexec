defmodule OpenExecRuntime.Router do
  @moduledoc """
  JSON-RPC bridge between Go and Elixir.
  """
  use Plug.Router
  require Logger

  plug(Plug.Logger)
  plug(Plug.Parsers, parsers: [:json], pass: ["*/*"], json_decoder: Jason)
  plug(:match)
  plug(:dispatch)

  post "/rpc" do
    case conn.body_params do
      %{"method" => "start_task", "params" => %{"task_id" => id, "path" => path}} ->
        Logger.info("[RPC] Spawning self-healing worker for #{id}")
        DynamicSupervisor.start_child(OpenExecRuntime.TaskSupervisor, {OpenExecRuntime.TaskWorker, [id, path]})
        send_resp(conn, 200, Jason.encode!(%{result: "ok", task_id: id}))

      %{"method" => "run_iteration", "params" => %{"task_id" => id}} ->
        Logger.info("[RPC] Triggering surgical iteration for #{id}")
        OpenExecRuntime.TaskWorker.run_iteration(id)
        send_resp(conn, 200, Jason.encode!(%{result: "ok"}))

      _ ->
        send_resp(conn, 400, Jason.encode!(%{error: "invalid_method"}))
    end
  end

  match _ do
    send_resp(conn, 404, "Not Found")
  end
end
