# Contributing to OpenExec

We welcome contributions to the OpenExec execution engine!

## Development Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/openexec/openexec.git
   cd openexec
   ```

2. **Install Go:**
   Ensure you have Go 1.25 or higher installed.

3. **Build the binary:**
   ```bash
   go build -o bin/openexec ./cmd/openexec
   ```

4. **UI Setup:**
   ```bash
   cd ui
   npm install
   ```

## Workflow

1. **Create a branch** for your changes.
2. **Implement your changes.**
3. **Run tests:**
   ```bash
   go test ./...
   ```
4. **Run UI tests** (if applicable):
   ```bash
   cd ui && npm test
   ```
5. **Lint your changes:**
   ```bash
   golangci-lint run
   ```
6. **Submit a Pull Request.**

## Standards

- **Go Code:** Follow Effective Go and idiomatic patterns; format with `gofmt`.
- **UI Code:** Use TypeScript and React best practices.
- **Commit Messages:** Clear, concise subjects; reference issues when applicable.

## Security

Please report security vulnerabilities to hello@openexec.io.
