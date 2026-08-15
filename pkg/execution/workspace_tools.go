package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/pkg/agent"
)

// maxToolOutputBytes caps what one tool call may return to the model. A file
// larger than this is truncated with a marker rather than silently cut: a
// model that believes it read a whole file and did not will edit from a false
// premise.
const maxToolOutputBytes = 128 << 10

// deniedComponents are path components no tool may traverse into, whatever the
// roots say.
//
// `.openexec/config.json` holds this feature's own provider entries, and those
// entries must carry a literal API key — the client rejects an empty one and
// the console's scrubbed environment cannot resolve a `$VAR`. Every configured
// credential for every endpoint therefore sits in a file inside the checkout
// the model is reading. Containment alone would hand it over on request.
//
// `.git` is here for the same reason one level up: config there holds remote
// credentials and helpers.
var deniedComponents = map[string]bool{
	".openexec": true,
	".uaos":     true,
	".git":      true,
}

// WorkspaceToolExecutor is the bounded tool boundary for API providers.
//
// The authorization it enforces is the request's, not its own: reads are
// confined to the readable roots, writes to the writable ones, and a read-only
// request cannot reach a writing tool at all. This is what makes APIProvider's
// refusal to run workspace-write without an executor satisfiable without
// widening anything.
//
// Containment is enforced by os.Root rather than by comparing resolved path
// strings. String comparison answers "where did this path point a moment ago",
// which is a different question from "where is this open file" — a directory
// swapped for a symlink between the check and the open lands outside the root
// with the check having passed. os.Root holds a descriptor to the root and
// resolves every component beneath it, so there is no interval to race.
//
// Deliberately narrower than the pipeline's tool set in internal/loop:
//
//   - No shell. An unattended model with a shell is a wider hole than any CLI
//     provider opens, and nothing here can contain one — a command can move
//     files, fetch code, or open a socket regardless of how its argument was
//     validated. Adding one is a separate decision with its own approval path.
//   - No patch application. Containing `git apply` means understanding every
//     path a patch header can name; whole-file writes are containable in a way
//     patches are not, and small local models produce better whole files than
//     diffs anyway.
type WorkspaceToolExecutor struct{}

var _ ToolExecutor = (*WorkspaceToolExecutor)(nil)

func NewWorkspaceToolExecutor() *WorkspaceToolExecutor { return &WorkspaceToolExecutor{} }

// SupportsWorkspaceWrite is true: this executor owns files, and editing them
// under a granted root is what it is for.
func (e *WorkspaceToolExecutor) SupportsWorkspaceWrite() bool { return true }

// WorkspaceTools returns the definitions offered to the model for a sandbox
// mode. Read-only sessions are never shown a writing tool: refusing at
// dispatch would work, but only after the model has spent a turn proposing
// something it was never allowed to do.
//
// This is advertisement, not enforcement. ExecuteTool re-checks the mode, so a
// mistake here cannot grant a write.
func WorkspaceTools(sandbox Sandbox) []agent.ToolDefinition {
	tools := []agent.ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read a file's contents. Paths are relative to the working directory unless absolute.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {"path": {"type": "string", "description": "Path to the file to read."}},
				"required": ["path"]
			}`),
		},
		{
			Name:        "list_directory",
			Description: "List the entries of a directory. Paths are relative to the working directory unless absolute.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {"path": {"type": "string", "description": "Directory to list. Defaults to the working directory."}}
			}`),
		},
	}
	if sandbox.Mode == SandboxWorkspaceWrite {
		tools = append(tools, agent.ToolDefinition{
			Name:        "write_file",
			Description: "Write a file, creating it if absent and replacing its contents if present.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Path to the file to write."},
					"content": {"type": "string", "description": "The complete new contents of the file."},
					"create_directories": {"type": "boolean", "description": "Create missing parent directories.", "default": false}
				},
				"required": ["path", "content"]
			}`),
		})
	}
	return tools
}

// ValidateAccess fails closed: the executor either can enforce the request's
// authorization exactly, or the run does not start.
func (e *WorkspaceToolExecutor) ValidateAccess(workingDir string, sandbox Sandbox, writableRoots []string) error {
	if strings.TrimSpace(workingDir) == "" {
		return errors.New("working directory is required")
	}
	if root, err := os.OpenRoot(workingDir); err != nil {
		return fmt.Errorf("working directory: %w", err)
	} else {
		_ = root.Close()
	}
	for _, path := range writableRoots {
		root, err := os.OpenRoot(path)
		if err != nil {
			return fmt.Errorf("writable root %q: %w", path, err)
		}
		_ = root.Close()
	}
	switch sandbox.Mode {
	case SandboxReadOnly:
		return nil
	case SandboxWorkspaceWrite:
		if len(writableRoots) == 0 {
			// Without a root there is nothing to contain writes to, and
			// defaulting to the working directory would invent an
			// authorization the caller never granted.
			return errors.New("workspace-write requires at least one writable root")
		}
		return nil
	default:
		return fmt.Errorf("unsupported sandbox mode %q", sandbox.Mode)
	}
}

func (e *WorkspaceToolExecutor) ExecuteTool(_ context.Context, request ToolRequest) (string, error) {
	if err := e.ValidateAccess(request.WorkingDir, request.Sandbox, request.WritableRoots); err != nil {
		return "", err
	}
	switch request.Name {
	case "read_file":
		return e.readFile(request)
	case "list_directory":
		return e.listDirectory(request)
	case "write_file":
		return e.writeFile(request)
	default:
		return "", fmt.Errorf("unknown tool %q", request.Name)
	}
}

func (e *WorkspaceToolExecutor) readFile(request ToolRequest) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	return withinRoots(input.Path, request.WorkingDir, readableRoots(request), "read", func(root *os.Root, relative string) (string, error) {
		file, err := root.Open(relative)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		defer file.Close()
		// One byte past the cap distinguishes "exactly at the limit" from
		// "truncated", so the marker is never a lie in either direction.
		data, err := io.ReadAll(io.LimitReader(file, maxToolOutputBytes+1))
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		if len(data) > maxToolOutputBytes {
			return string(data[:maxToolOutputBytes]) +
				fmt.Sprintf("\n\n[truncated at %d bytes]", maxToolOutputBytes), nil
		}
		return string(data), nil
	})
}

func (e *WorkspaceToolExecutor) listDirectory(request ToolRequest) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if len(request.Input) > 0 {
		if err := json.Unmarshal(request.Input, &input); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}
	if input.Path == "" {
		input.Path = "."
	}
	return withinRoots(input.Path, request.WorkingDir, readableRoots(request), "read", func(root *os.Root, relative string) (string, error) {
		directory, err := root.Open(relative)
		if err != nil {
			return "", fmt.Errorf("list directory: %w", err)
		}
		defer directory.Close()
		entries, err := directory.ReadDir(-1)
		if err != nil {
			return "", fmt.Errorf("list directory: %w", err)
		}
		var out strings.Builder
		for _, entry := range entries {
			name := entry.Name()
			// Named but not enterable: hiding them would have the model
			// theorise about a directory it can see in git output anyway.
			if entry.IsDir() {
				name += "/"
			}
			out.WriteString(name)
			out.WriteString("\n")
		}
		return out.String(), nil
	})
}

func (e *WorkspaceToolExecutor) writeFile(request ToolRequest) (string, error) {
	if request.Sandbox.Mode != SandboxWorkspaceWrite {
		// Reachable: the model was not offered this tool, but a provider that
		// invented the call must not be answered by writing a file.
		return "", fmt.Errorf("write_file is not available in %s mode", request.Sandbox.Mode)
	}
	var input struct {
		Path              string `json:"path"`
		Content           string `json:"content"`
		CreateDirectories bool   `json:"create_directories"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	// Writes are contained by the writable roots alone. The working directory
	// is readable by virtue of being where the run happens; that does not make
	// it writable.
	return withinRoots(input.Path, request.WorkingDir, request.WritableRoots, "write", func(root *os.Root, relative string) (string, error) {
		if input.CreateDirectories {
			if parent := filepath.Dir(relative); parent != "." && parent != "" {
				if err := root.MkdirAll(parent, 0o755); err != nil {
					return "", fmt.Errorf("create parent directories: %w", err)
				}
			}
		}
		file, err := root.Create(relative)
		if err != nil {
			return "", fmt.Errorf("write file: %w", err)
		}
		defer file.Close()
		if _, err := file.WriteString(input.Content); err != nil {
			return "", fmt.Errorf("write file: %w", err)
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(input.Content), input.Path), nil
	})
}

// readableRoots is what a call may open: the roots granted for reading, the
// ones granted for writing, and the directory the run happens in. A reviewer
// that cannot read its own checkout is useless, and one that cannot read the
// sibling repository the console told it about is worse — it reports the work
// as missing rather than the repository.
func readableRoots(request ToolRequest) []string {
	roots := make([]string, 0, len(request.ReadableRoots)+len(request.WritableRoots)+1)
	roots = append(roots, request.ReadableRoots...)
	roots = append(roots, request.WritableRoots...)
	return append(roots, request.WorkingDir)
}

// withinRoots runs an operation against the first authorized root that
// contains the path, and refuses when none does.
//
// The root is opened, the path is made relative to it, and every component is
// resolved by the kernel beneath that descriptor. A path naming a denied
// component is refused before any of that: those files are inside the roots by
// construction, so containment is not what keeps them out.
func withinRoots(argument, workingDir string, roots []string, action string, run func(*os.Root, string) (string, error)) (string, error) {
	if strings.TrimSpace(argument) == "" {
		return "", errors.New("path is required")
	}
	if len(roots) == 0 {
		return "", errors.New("no authorized roots for this request")
	}
	if component, denied := deniedComponent(argument); denied {
		return "", fmt.Errorf("%q is not readable: %s holds configuration and credentials for this workspace",
			argument, component)
	}
	var firstErr error
	for _, path := range roots {
		if path == "" {
			continue
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			continue
		}
		relative, ok := relativeTo(root.Name(), workingDir, argument)
		if !ok {
			_ = root.Close()
			continue
		}
		output, err := run(root, relative)
		_ = root.Close()
		if err == nil {
			return output, nil
		}
		// A path that resolves into this root but fails to open (absent file,
		// or an escape the kernel refused) is reported from the first root
		// that claimed it, not silently retried against the others.
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return "", firstErr
	}
	return "", fmt.Errorf("path %q is outside the authorized roots for this request (%s denied)", argument, action)
}

// relativeTo expresses a path relative to a root, without resolving symlinks:
// resolution is the kernel's job through os.Root, and doing it here would
// reintroduce the gap between deciding and opening.
//
// A relative argument is anchored to the working directory, never to whichever
// root is being tested. Anchoring it to the root would make a bare filename
// mean a different file per root, so a write refused against the checkout
// would land in some other granted tree instead.
func relativeTo(root, workingDir, argument string) (string, bool) {
	if !filepath.IsAbs(argument) {
		argument = filepath.Join(workingDir, argument)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(absoluteRoot, filepath.Clean(argument))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

func deniedComponent(path string) (string, bool) {
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if deniedComponents[component] {
			return component, true
		}
	}
	return "", false
}
