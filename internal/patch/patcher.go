package patch

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

// ApplyUnifiedDiff applies a minimal subset of unified diff to files under root.
// It rejects paths outside root and refuses to create directories above root.
func ApplyUnifiedDiff(root string, r io.Reader, dryRun bool) error {
    if root == "" { root = "." }
    absRoot, err := filepath.Abs(root)
    if err != nil { return err }

    scanner := bufio.NewScanner(r)
    var current string
    var sb strings.Builder
    writeFile := func() error {
        if current == "" { return nil }
        target := filepath.Join(absRoot, current)
        absTarget, err := filepath.Abs(target)
        if err != nil { return err }
        if !strings.HasPrefix(absTarget, absRoot+string(os.PathSeparator)) && absTarget != absRoot {
            return fmt.Errorf("path escapes workspace root: %s", current)
        }
        if dryRun { return nil }
        if err := os.MkdirAll(filepath.Dir(absTarget), 0750); err != nil { return err }
        return os.WriteFile(absTarget, []byte(sb.String()), 0644)
    }

    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "+++ ") {
            // ignore
            continue
        }
        if strings.HasPrefix(line, "--- ") {
            // start of a file block; flush previous
            if err := writeFile(); err != nil { return err }
            sb.Reset()
            path := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
            // unified diff path often starts with a/ or b/
            path = strings.TrimPrefix(path, "a/")
            path = strings.TrimPrefix(path, "b/")
            current = path
            continue
        }
        if strings.HasPrefix(line, "@@") {
            // We don't try to apply hunks precisely; this is a minimal patcher
            // expecting full-file replacements provided as diffs.
            continue
        }
        if len(line) > 0 && (line[0] == '+' || line[0] == ' ') {
            sb.WriteString(line[1:])
            sb.WriteByte('\n')
        }
        // Lines starting with '-' are removed in full replacement mode.
    }
    if err := scanner.Err(); err != nil { return err }
    if sb.Len() == 0 && current == "" {
        return errors.New("empty diff")
    }
    return writeFile()
}

