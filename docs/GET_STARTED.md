# Getting Started with OpenExec

Welcome to OpenExec! This guide will take you from zero to running your first AI-orchestrated project.

## 1. Local Setup

### Prerequisites
- **Go 1.21+**: [Install Go](https://go.dev/doc/install)
- **Node.js 18+ & npm**: (Only for UI development) [Install Node.js](https://nodejs.org/)
- **Git**: Required for version control integration.

### Installation
You can install the pre-built binary using our installation script:

```bash
# Default installation (installs to /usr/local/bin or ~/.local/bin)
curl -sSfL https://openexec.io/install.sh | sh

# Non-sudo / Custom path installation
curl -sSfL https://openexec.io/install.sh | INSTALL_DIR=$HOME/bin sh
```

The script automatically detects your OS and architecture. If it cannot write to `/usr/local/bin`, it will try to install to `~/.local/bin` to avoid requiring `sudo`.

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

OpenExec uses Git to track changes and manage safety guardrails. **Your project directory must be a Git repository.**

If you haven't already, initialize Git in your project folder:

```bash
git init
```

Then, run the OpenExec initialization:

```bash
./openexec init
```

Follow the interactive prompts to configure your **Task-Specific Brains**. OpenExec allows you to choose different models for different stages of the lifecycle, using either **Cloud APIs** (Claude, GPT, Gemini) or **Local LLMs** (via Ollama/OpenCode):

- **Planner:** The high-level architect that turns intent into stories.
- **Executor:** The agent that writes the actual code.
- **Reviewer:** The quality gatekeeper that verifies the implementation.

**Why this matters:** You can use a powerful cloud model like Claude 4.6 Sonnet for complex planning, while using a fast local model for repetitive implementation tasks.

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

### Safe Daemon Mode
For continuous background execution, use the `--daemon` flag:

```bash
./openexec start --daemon
```

OpenExec v0.1.6 includes **Automated PID Tracking**:
- It writes a process ID file to `.openexec/openexec.pid`.
- It redirects all background output to `.openexec/daemon.log`.
- `openexec run` and `openexec stop` automatically use this file to manage the background engine.

### Executing Tasks
You can run tasks individually or let the daemon handle them:

```bash
# Execute the next pending task (automatically connects to background engine)
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

## 7. Updating OpenExec

To update to the latest version, simply run:

```bash
./openexec update
```

This will check the latest version on openexec.io and replace your current binary with the latest one for your platform.

## 8. Troubleshooting

- **Logs:** Check `.openexec/daemon.log` for background process output.
- **Audit Database:** All AI decisions and tool calls are stored in `.openexec/data/audit.db`. You can inspect this with any SQLite browser.
- **Missing Directory Error:** If the server fails to start, ensure `.openexec/data` exists (this is fixed in v0.1.1+).

---
Next: [Read the Architecture Guide](docs/KNOWLEDGE_BASE.md)
