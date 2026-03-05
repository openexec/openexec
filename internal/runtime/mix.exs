defmodule OpenExecRuntime.MixProject do
  use Mix.Project

  def project do
    [
      app: :open_exec_runtime,
      version: "0.1.0",
      elixir: "~> 1.14",
      start_permanent: Mix.env() == :prod,
      deps: deps()
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {OpenExecRuntime.Application, []}
    ]
  end

  defp deps do
    [
      {:jason, "~> 1.4"},           # High-speed JSON
      {:ecto_sqlite3, "~> 0.10"},   # For surgical pointer lookups in knowledge.db
      {:telemetry, "~> 1.0"},       # Observability
      {:uuid, "~> 1.1"},            # For tracking multi-project session IDs
      {:plug_cowboy, "~> 2.6"}      # HTTP Bridge for Go <-> Elixir communication
    ]
  end
end
