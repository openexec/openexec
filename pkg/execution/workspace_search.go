package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxSearchEntries = 4096
	maxSearchBytes   = 16 << 20
	searchIncomplete = "[incomplete search: limit reached; narrow the path or text]\n"
)

// searchFiles reuses the workspace read boundary; it cannot execute commands,
// follow links during traversal, or turn a readable root into a writable one.
func (e *WorkspaceToolExecutor) searchFiles(ctx context.Context, request ToolRequest) (string, error) {
	var input struct {
		Path string `json:"path"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if input.Text == "" || strings.ContainsAny(input.Text, "\r\n") {
		return "", errors.New("text must be a non-empty single-line literal")
	}
	return withinRoots(input.Path, request.WorkingDir, readableRoots(request), "read", func(root *os.Root, relative string) (string, error) {
		var out strings.Builder
		entries, remaining := 0, maxSearchBytes
		incomplete := false
		err := fs.WalkDir(root.FS(), filepath.ToSlash(relative), func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > maxSearchEntries || remaining == 0 {
				incomplete = true
				return fs.SkipAll
			}
			if walkErr != nil {
				return walkErr
			}
			if _, denied := deniedComponent(path); denied {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			file, err := root.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			if err := refuseDeniedOpenedTarget(root, file, path, input.Path); err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(file, int64(remaining)+1))
			if err != nil {
				return err
			}
			if len(data) > remaining {
				data = data[:remaining]
				incomplete = true
			}
			remaining -= len(data)
			if bytes.IndexByte(data, 0) >= 0 {
				return nil
			}
			for line, text := range strings.Split(string(data), "\n") {
				if !strings.Contains(text, input.Text) {
					continue
				}
				match := fmt.Sprintf("%s:%d:%s\n", path, line+1, text)
				if out.Len()+len(match)+len(searchIncomplete) > maxReadFileBytes {
					incomplete = true
					return fs.SkipAll
				}
				out.WriteString(match)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("search files: %w", err)
		}
		if incomplete {
			out.WriteString(searchIncomplete)
		} else if out.Len() == 0 {
			out.WriteString("No matches in regular text files (symbolic links and private configuration directories skipped).\n")
		}
		return out.String(), nil
	})
}
