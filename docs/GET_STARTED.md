# Getting Started with OpenExec

Welcome to OpenExec! This guide will take you from zero to running your first AI-orchestrated project.

## 1. Local Setup

### Prerequisites
- **Go 1.21+**: [Install Go](https://go.dev/doc/install)
- **Node.js 18+ & npm**: (Only for UI development) [Install Node.js](https://nodejs.org/)
- **Git**: Required for version control integration.

### Building from Source
OpenExec is a single binary that embeds its UI. To build it locally:

1. **Build the UI (Optional if `dist` exists):**
   ```bash
   cd ui
   npm install
   npm run build
   cd ..
   ```

2. **Build the CLI:**
   ```bash
   # From the project root
   go build -o openexec ./cmd/openexec
   # Add to your PATH or use ./openexec
   ```

## 2. Initialize Your Project

Go to your project directory (it must be a git repo) and run:

```bash
./openexec init
```

Follow the interactive prompts to configure your **Task-Specific Brains**. OpenExec allows you to choose different models for different stages of the lifecycle, using either **Cloud APIs** (Claude, GPT, Gemini) or **Local LLMs** (via Ollama/OpenCode):

- **Planner:** The high-level architect that turns intent into stories.
- **Executor:** The agent that writes the actual code.
- **Reviewer:** The quality gatekeeper that verifies the implementation.

**Why this matters:** You can use a powerful cloud model like Claude 3.5 Sonnet for complex planning, while using a fast local model for repetitive implementation tasks.

## 3. The Power of the Knowledge Map

OpenExec isn't just a chat interface. It uses a **Local Knowledge Map (DCP)** that surgically indexes your code.

- **Precision:** Agents know the exact byte-offset of every function.
- **Efficiency:** Because the map is local, OpenExec only sends the *exact* snippets needed to the AI. This drastically reduces the information sent to APIs, saving you tokens and improving accuracy compared to tools that "dump" entire files into the context.

## 4. Generate a Plan

Now, turn that intent into concrete tasks:

```bash
./openexec plan INTENT.md
```

OpenExec will generate a `stories.json` file in `.openexec/` containing the execution DAG (Directed Acyclic Graph) of tasks.

## 5. Running the System

### The Integrated Server (CLI + UI)
To start the orchestration engine and host the web console:

```bash
./openexec start --ui
```

- **Server:** Runs on `http://localhost:8080` (default).
- **UI:** Automatically opens in your browser, showing the **Knowledge Hub** and task progress.

### Executing Tasks
You can run tasks individually or let the daemon handle them:

```bash
# Execute the next pending task
./openexec run
```

## 6. Development Mode (Advanced)

If you are modifying the React UI and want Hot Module Replacement (HMR):

1. Start the backend: `./openexec start`
2. Start the UI dev server:
   ```bash
   cd ui
   npm run dev -- --port 3001
   ```
3. Visit `http://localhost:3001`. The UI will proxy requests to the backend on `:8080`.

## 7. Troubleshooting

- **Logs:** Check `.openexec/daemon.log` for background process output.
- **Audit Database:** All AI decisions and tool calls are stored in `.openexec/data/audit.db`. You can inspect this with any SQLite browser.
- **Missing Directory Error:** If the server fails to start, ensure `.openexec/data` exists (this is fixed in v0.1.1+).

---
Next: [Read the Architecture Guide](docs/KNOWLEDGE_BASE.md)
