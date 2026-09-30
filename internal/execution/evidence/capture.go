// Package evidence retains private command evidence separately from repair classification.
package evidence

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const StreamLimit = 4096
const metadataLimit = 128
const maxEvidenceBytes = 4 << 20
const directory = ".openexec/data/verification"

// Buffer drains the entire stream, retaining its first and last halves on overflow.
// Use one buffer per stream; exec.Cmd serializes writes to each writer.
type Buffer struct {
	data      []byte
	Truncated bool
}

func (b *Buffer) Len() int       { return len(b.data) }
func (b *Buffer) String() string { return string(b.data) }
func (b *Buffer) Write(p []byte) (int, error) {
	n := len(p)
	if n <= StreamLimit-b.Len() {
		b.data = append(b.data, p...)
		return n, nil
	}
	b.Truncated = true
	const half = StreamLimit / 2
	// Fill the prefix once. Keep only the latest half of the suffix without
	// allocating in proportion to the incoming write.
	if len(b.data) < half {
		take := half - len(b.data)
		b.data = append(b.data, p[:take]...)
		p = p[take:]
	}
	if len(p) >= half {
		b.data = append(b.data[:half], p[len(p)-half:]...)
	} else {
		start := len(b.data) - (half - len(p))
		copy(b.data[half:], b.data[start:])
		b.data = append(b.data[:StreamLimit-len(p)], p...)
	}
	return n, nil
}

// Command is private: never serialize it into events, receipts or task descriptions.
type Command struct {
	Argv                             []string
	Cwd                              string
	ExitCode                         int
	Stdout, Stderr                   string
	StdoutTruncated, StderrTruncated bool
	Toolchain                        map[string]string
}

// Toolchain explicitly accepts version fields only. No environment is collected.
func Toolchain(values map[string]string) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"go_version", "node_version", "npm_version", "python_version", "rustc_version"} {
		if value, ok := values[key]; ok {
			result[key] = bounded(value, metadataLimit)
		}
	}
	return result
}
func bounded(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

var credential = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization)\s*[=:]\s*)("[^"\n]*"|'[^'\n]*'|[^\s,;]+)`)

// CommandSecrets identifies credential assignment values for redaction when a
// shell command echoes a value without its key. It never reads the environment.
func CommandSecrets(command string) []string {
	secrets := []string{command}
	for _, match := range credential.FindAllStringSubmatch(command, -1) {
		secrets = append(secrets, strings.Trim(match[2], "\"'"))
	}
	return secrets
}

// Public removes declared secrets before bounding, and common credential assignments.
// Adapters must supply secret values from their admission/configuration boundary;
// arbitrary secrets cannot be inferred reliably from natural language diagnostics.
func Public(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = credential.ReplaceAllString(value, "${1}[REDACTED]")
	return bounded(value, StreamLimit)
}

// PublicStream redacts complete retained lines on each side of the omitted
// middle. Drop cut lines before redaction: neither half of a split credential
// can safely be matched against the original secret.
func PublicStream(b *Buffer, secrets []string) string {
	if !b.Truncated {
		return Public(b.String(), secrets)
	}
	value := b.String()
	head, tail := value[:StreamLimit/2-len("[truncated]")], value[StreamLimit/2:]
	if end := strings.LastIndexByte(head, '\n'); end >= 0 {
		head = head[:end+1]
	} else {
		head = ""
	}
	if start := strings.IndexByte(tail, '\n'); start >= 0 {
		tail = tail[start+1:]
	} else {
		tail = ""
	}
	if end := strings.LastIndexByte(tail, '\n'); end >= 0 {
		tail = tail[:end+1]
	} else {
		tail = ""
	}
	return Public(head+"[truncated]"+tail, secrets)
}

// Write stores exact argv/cwd and bounded raw diagnostics in an owner-only file.
// Its content hash is an artifact identity, never a classification fingerprint.
func Write(projectDir string, command Command) (hash, path string, err error) {
	command.StdoutTruncated = command.StdoutTruncated || len(command.Stdout) > StreamLimit
	command.StderrTruncated = command.StderrTruncated || len(command.Stderr) > StreamLimit
	var stdout, stderr Buffer
	stdout.Write([]byte(command.Stdout))
	stderr.Write([]byte(command.Stderr))
	command.Stdout, command.Stderr = stdout.String(), stderr.String()
	command.Toolchain = Toolchain(command.Toolchain)
	raw, err := json.Marshal(command)
	if err != nil {
		return "", "", err
	}
	if len(raw) > maxEvidenceBytes {
		return "", "", fmt.Errorf("private evidence exceeds size limit")
	}
	sum := sha256.Sum256(raw)
	hash = hex.EncodeToString(sum[:])
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return "", "", err
	}
	defer root.Close()
	private, err := openDirectory(root, true)
	if err != nil {
		return "", "", err
	}
	defer private.Close()
	temp := ".capture-" + rand.Text()
	file, err := private.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", "", err
	}
	defer private.Remove(temp)
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", "", err
	}
	if err = private.Rename(temp, hash+".json"); err != nil {
		return "", "", err
	}
	return hash, Path(projectDir, hash), nil
}

// Read is an explicit private read for the executing owner, not a public API.
// Resolve only content-addressed regular files beneath this candidate's directory.
func Read(projectDir, hash string) (*Command, error) {
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != hash {
		return nil, fmt.Errorf("invalid evidence identity")
	}
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	private, err := openDirectory(root, false)
	if err != nil {
		return nil, err
	}
	defer private.Close()
	name := hash + ".json"
	info, err := private.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("private evidence file required")
	}
	file, err := private.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// argv is bounded by the OS exec boundary; reject oversized or corrupt artifacts.
	raw, err := io.ReadAll(io.LimitReader(file, maxEvidenceBytes+1))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, fmt.Errorf("evidence digest mismatch")
	}
	var command Command
	if err = json.Unmarshal(raw, &command); err != nil {
		return nil, err
	}
	return &command, nil
}

// Path is the sole supported reference location. Legacy root-level references
// are deliberately not migrated or read; their persisted ledger remains intact.
func Path(projectDir, hash string) string {
	return filepath.Join(projectDir, directory, hash+".json")
}

// Walk one component at a time so even symlinks within the project are refused.
// Existing state parents may be public; the evidence directory must be private.
func openDirectory(root *os.Root, create bool) (*os.Root, error) {
	current := root
	for _, component := range []string{".openexec", "data", "verification"} {
		next, err := openComponent(current, component, create)
		if current != root {
			current.Close()
		}
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func openComponent(parent *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := parent.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || (name == "verification" && info.Mode().Perm() != 0700) {
		return nil, fmt.Errorf("private evidence directory required")
	}
	return parent.OpenRoot(name)
}
