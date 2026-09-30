// Package evidence retains private command evidence separately from repair classification.
package evidence

import (
	"bytes"
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

// Buffer drains the entire stream while retaining a bounded prefix.
// Use one buffer per stream; exec.Cmd serializes writes to each writer.
type Buffer struct {
	buffer    bytes.Buffer
	Truncated bool
}

func (b *Buffer) Len() int       { return b.buffer.Len() }
func (b *Buffer) String() string { return b.buffer.String() }
func (b *Buffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := StreamLimit - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.Truncated = true
	}
	_, _ = b.buffer.Write(p)
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

// PublicStream avoids exposing a partial secret at the capture boundary by
// dropping the final possibly incomplete line of a truncated stream.
func PublicStream(b *Buffer, secrets []string) string {
	value := b.String()
	if b.Truncated {
		if end := strings.LastIndexByte(value, '\n'); end >= 0 {
			value = value[:end+1]
		} else {
			value = ""
		}
		value += "[truncated]"
	}
	return Public(value, secrets)
}

// Write stores exact argv/cwd and bounded raw diagnostics in an owner-only file.
// Its content hash is an artifact identity, never a classification fingerprint.
func Write(projectDir string, command Command) (hash, path string, err error) {
	command.StdoutTruncated = command.StdoutTruncated || len(command.Stdout) > StreamLimit
	command.StderrTruncated = command.StderrTruncated || len(command.Stderr) > StreamLimit
	command.Stdout, command.Stderr = bounded(command.Stdout, StreamLimit), bounded(command.Stderr, StreamLimit)
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
