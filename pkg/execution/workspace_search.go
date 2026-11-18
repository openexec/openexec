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
	"unicode/utf8"
)

const (
	maxSearchEntries   = 4096
	maxSearchBytes     = 16 << 20
	maxSearchLineBytes = 1024
	searchIncomplete   = "[incomplete search: limits, truncated lines or unreadable entries; narrow the path or text]\n"
)

// governingInstructionFiles are the files agents are told to obey. A search
// that matched inside one carries instructions, so its result must stay in the
// prompt. This is decided by the file the scan itself matched in, never by
// guessing at result text.
var governingInstructionFiles = map[string]bool{
	"agents.md": true, "agents.local.md": true, "claude.md": true, "gemini.md": true,
	".cursorrules": true, "copilot-instructions.md": true,
}

func isGoverningInstructionFile(path string) bool {
	return governingInstructionFiles[strings.ToLower(filepath.Base(path))]
}

// searchFiles reuses the workspace read boundary; it cannot execute commands,
// follow links during traversal, or turn a readable root into a writable one.
func (e *WorkspaceToolExecutor) searchFiles(ctx context.Context, request ToolRequest) (string, error) {
	output, _, err := e.searchFilesWithProvenance(ctx, request)
	return output, err
}

// searchFilesWithProvenance reports completeness from the scan itself, never
// by interpreting a potentially adversarial result string.
func (e *WorkspaceToolExecutor) searchFilesWithProvenance(ctx context.Context, request ToolRequest) (string, bool, error) {
	var input struct {
		Path string `json:"path"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return "", false, fmt.Errorf("invalid input: %w", err)
	}
	if input.Text == "" || strings.ContainsAny(input.Text, "\r\n") {
		return "", false, errors.New("text must be a non-empty single-line literal")
	}
	complete := false
	output, err := e.withinRoots(input.Path, request.WorkingDir, readableRoots(request), "read", func(root *os.Root, relative string) (string, error) {
		var out strings.Builder
		entries, remaining := 0, maxSearchBytes
		incomplete := false
		governed := false
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
				if path == filepath.ToSlash(relative) {
					return walkErr
				}
				incomplete = true
				return nil
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
				if path == filepath.ToSlash(relative) {
					return err
				}
				incomplete = true
				return nil
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
				// Results obey the same WorkingDir anchoring as read_file, even
				// when withinRoots selected a parent or sibling readable root.
				absolute, err := filepath.Abs(filepath.Join(root.Name(), filepath.FromSlash(path)))
				if err != nil {
					return err
				}
				displayPath, err := filepath.Rel(request.WorkingDir, absolute)
				if err != nil {
					return err
				}
				if len(text) > maxSearchLineBytes {
					end := maxSearchLineBytes
					for end > 0 && !utf8.RuneStart(text[end]) {
						end--
					}
					text = text[:end] + " [line truncated]"
					incomplete = true
				}
				match := fmt.Sprintf("%s:%d:%s\n", filepath.ToSlash(displayPath), line+1, text)
				if out.Len()+len(match)+len(searchIncomplete) > maxReadFileBytes {
					incomplete = true
					return fs.SkipAll
				}
				out.WriteString(match)
				if isGoverningInstructionFile(path) {
					governed = true
				}
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
		// Complete means safe to omit from a prompt: the scan finished, matched
		// in no governing instruction file, and is faithfully recallable as text.
		complete = !incomplete && !governed && utf8.ValidString(out.String())
		return out.String(), nil
	})
	return output, complete && err == nil, err
}
