// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements a unified diff patch parser and validator.
package mcp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PatchFile represents a single file modification in a unified diff patch.
type PatchFile struct {
	OldName    string      // Original file path (from --- line)
	NewName    string      // New file path (from +++ line)
	Hunks      []PatchHunk // List of hunks in this file
	IsBinary   bool        // True if this is a binary file
	IsNew      bool        // True if this is a new file
	IsDeleted  bool        // True if this is a deleted file
	IsRenamed  bool        // True if this is a rename operation
	GitHeaders []string    // Git-specific headers (index, mode, etc.)
}

// PatchHunk represents a single hunk in a unified diff.
type PatchHunk struct {
	OldStart int         // Starting line in old file
	OldCount int         // Number of lines in old file
	NewStart int         // Starting line in new file
	NewCount int         // Number of lines in new file
	Header   string      // The full @@ line
	Lines    []PatchLine // Lines in this hunk
}

// PatchLine represents a single line in a hunk.
type PatchLine struct {
	Type    LineType // Type of line (context, add, remove)
	Content string   // Line content (without the prefix character)
}

// LineType indicates the type of a patch line.
type LineType int

const (
	LineContext LineType = iota // Context line (space prefix)
	LineAdd                     // Added line (+ prefix)
	LineRemove                  // Removed line (- prefix)
)

// Patch represents a complete unified diff patch.
type Patch struct {
	Files      []PatchFile // Files modified by this patch
	RawContent string      // Original patch content
}

// PatchStats provides statistics about a patch.
type PatchStats struct {
	FilesChanged int // Number of files changed
	Additions    int // Number of lines added
	Deletions    int // Number of lines deleted
	Hunks        int // Total number of hunks
}

// ParseError represents an error encountered while parsing a patch.
type ParseError struct {
	Line    int    // Line number in the patch (1-indexed)
	Message string // Error description
	Context string // Surrounding context for debugging
}

func (e *ParseError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("line %d: %s (context: %q)", e.Line, e.Message, e.Context)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// PatchValidationError represents a validation error in a parsed patch.
type PatchValidationError struct {
	File    string // File path affected
	Hunk    int    // Hunk number (1-indexed), 0 if file-level error
	Message string // Error description
}

func (e *PatchValidationError) Error() string {
	if e.Hunk > 0 {
		return fmt.Sprintf("%s hunk %d: %s", e.File, e.Hunk, e.Message)
	}
	if e.File != "" {
		return fmt.Sprintf("%s: %s", e.File, e.Message)
	}
	return e.Message
}

// PatchValidationResult contains the result of patch validation.
type PatchValidationResult struct {
	Valid    bool                   // True if patch is valid
	Errors   []PatchValidationError // List of validation errors
	Warnings []PatchValidationError // List of validation warnings
	Stats    PatchStats             // Patch statistics
}

// Regular expressions for parsing
var (
	// Matches: diff --git a/path b/path
	gitDiffHeaderRe = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)

	// Matches: --- a/path or --- /dev/null
	oldFileRe = regexp.MustCompile(`^--- (?:a/)?(.+)$`)

	// Matches: +++ b/path or +++ /dev/null
	newFileRe = regexp.MustCompile(`^\+\+\+ (?:b/)?(.+)$`)

	// Matches: @@ -old_start,old_count +new_start,new_count @@ optional context
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

	// Matches binary file markers
	binaryFileRe = regexp.MustCompile(`^Binary files .+ and .+ differ$`)

	// Matches: new file mode XXX
	newFileModeRe = regexp.MustCompile(`^new file mode \d+$`)

	// Matches: deleted file mode XXX
	deletedFileModeRe = regexp.MustCompile(`^deleted file mode \d+$`)

	// Matches: rename from/to
	renameFromRe = regexp.MustCompile(`^rename from (.+)$`)
	renameToRe   = regexp.MustCompile(`^rename to (.+)$`)
)

// ParsePatch parses a unified diff patch string into a structured Patch object.
func ParsePatch(content string) (*Patch, error) {
	lines := strings.Split(content, "\n")
	patch := &Patch{
		RawContent: content,
		Files:      make([]PatchFile, 0),
	}

	var currentFile *PatchFile
	var currentHunk *PatchHunk
	lineNum := 0

	for lineNum < len(lines) {
		line := lines[lineNum]
		lineNum++

		// Skip empty lines at the start
		if currentFile == nil && strings.TrimSpace(line) == "" {
			continue
		}

		// Check for git diff header
		if matches := gitDiffHeaderRe.FindStringSubmatch(line); matches != nil {
			// Save previous file if any
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				patch.Files = append(patch.Files, *currentFile)
			}

			currentFile = &PatchFile{
				GitHeaders: []string{line},
			}
			continue
		}

		// Check for git-specific headers
		if currentFile != nil && currentHunk == nil {
			if newFileModeRe.MatchString(line) {
				currentFile.IsNew = true
				currentFile.GitHeaders = append(currentFile.GitHeaders, line)
				continue
			}
			if deletedFileModeRe.MatchString(line) {
				currentFile.IsDeleted = true
				currentFile.GitHeaders = append(currentFile.GitHeaders, line)
				continue
			}
			if renameFromRe.MatchString(line) || renameToRe.MatchString(line) {
				currentFile.IsRenamed = true
				currentFile.GitHeaders = append(currentFile.GitHeaders, line)
				continue
			}
			if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "old mode") ||
				strings.HasPrefix(line, "new mode") || strings.HasPrefix(line, "similarity index") {
				currentFile.GitHeaders = append(currentFile.GitHeaders, line)
				continue
			}
		}

		// Check for binary file marker
		if binaryFileRe.MatchString(line) {
			if currentFile != nil {
				currentFile.IsBinary = true
			}
			continue
		}

		// Check for old file header (--- line)
		if matches := oldFileRe.FindStringSubmatch(line); matches != nil {
			// If we already have a file with content (hunks or pending hunk), this is a new file
			hasContent := currentFile != nil && (len(currentFile.Hunks) > 0 || currentHunk != nil)
			if hasContent {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				patch.Files = append(patch.Files, *currentFile)
				currentFile = &PatchFile{}
			} else if currentFile == nil {
				currentFile = &PatchFile{}
			}
			currentFile.OldName = matches[1]
			continue
		}

		// Check for new file header (+++ line)
		if matches := newFileRe.FindStringSubmatch(line); matches != nil {
			if currentFile == nil {
				return nil, &ParseError{
					Line:    lineNum,
					Message: "found +++ line without corresponding --- line",
					Context: line,
				}
			}
			currentFile.NewName = matches[1]

			// Detect new/deleted file based on /dev/null
			if currentFile.OldName == "/dev/null" {
				currentFile.IsNew = true
			}
			if currentFile.NewName == "/dev/null" {
				currentFile.IsDeleted = true
			}
			continue
		}

		// Check for hunk header
		if matches := hunkHeaderRe.FindStringSubmatch(line); matches != nil {
			// Save previous hunk if any
			if currentHunk != nil {
				if currentFile == nil {
					return nil, &ParseError{
						Line:    lineNum,
						Message: "hunk found without file header",
						Context: line,
					}
				}
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			currentHunk = &PatchHunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Header:   line,
				Lines:    make([]PatchLine, 0),
			}
			continue
		}

		// Parse hunk lines
		if currentHunk != nil && len(line) > 0 {
			prefix := line[0]
			var content string
			if len(line) > 1 {
				content = line[1:]
			}

			switch prefix {
			case ' ':
				currentHunk.Lines = append(currentHunk.Lines, PatchLine{
					Type:    LineContext,
					Content: content,
				})
			case '+':
				currentHunk.Lines = append(currentHunk.Lines, PatchLine{
					Type:    LineAdd,
					Content: content,
				})
			case '-':
				currentHunk.Lines = append(currentHunk.Lines, PatchLine{
					Type:    LineRemove,
					Content: content,
				})
			case '\\':
				// "\ No newline at end of file" - ignore but continue
				continue
			default:
				// Unrecognized line in hunk context - this might indicate
				// we've moved past the hunk (e.g., reached another diff header)
				// Re-process this line
				lineNum--
				if currentFile != nil && currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				}
				currentHunk = nil
			}
		} else if currentHunk != nil && len(line) == 0 {
			// Empty line: check if the hunk is complete based on line counts
			oldLines := 0
			newLines := 0
			for _, l := range currentHunk.Lines {
				switch l.Type {
				case LineContext:
					oldLines++
					newLines++
				case LineRemove:
					oldLines++
				case LineAdd:
					newLines++
				}
			}
			// Only add empty line as context if hunk is not yet complete
			if oldLines < currentHunk.OldCount || newLines < currentHunk.NewCount {
				currentHunk.Lines = append(currentHunk.Lines, PatchLine{
					Type:    LineContext,
					Content: "",
				})
			}
		}
	}

	// Save final file and hunk
	if currentHunk != nil && currentFile != nil {
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
	}
	if currentFile != nil {
		patch.Files = append(patch.Files, *currentFile)
	}

	// Basic validation: must have at least one file
	if len(patch.Files) == 0 {
		return nil, &ParseError{
			Line:    1,
			Message: "no files found in patch",
		}
	}

	return patch, nil
}

// ValidatePatch validates a parsed patch and returns detailed validation results.
func ValidatePatch(patch *Patch) *PatchValidationResult {
	result := &PatchValidationResult{
		Valid:    true,
		Errors:   make([]PatchValidationError, 0),
		Warnings: make([]PatchValidationError, 0),
	}

	for _, file := range patch.Files {
		result.Stats.FilesChanged++

		// Validate file headers - must have both old and new file headers for a valid diff
		if file.OldName == "" && file.NewName == "" {
			result.Valid = false
			result.Errors = append(result.Errors, PatchValidationError{
				File:    "(unknown)",
				Message: "file is missing both old and new name headers",
			})
			continue
		}

		// A valid unified diff needs both --- and +++ lines
		if file.OldName == "" {
			result.Valid = false
			result.Errors = append(result.Errors, PatchValidationError{
				File:    file.NewName,
				Message: "missing old file header (--- line)",
			})
			continue
		}
		if file.NewName == "" {
			result.Valid = false
			result.Errors = append(result.Errors, PatchValidationError{
				File:    file.OldName,
				Message: "missing new file header (+++ line)",
			})
			continue
		}

		fileName := file.NewName
		if fileName == "" || fileName == "/dev/null" {
			fileName = file.OldName
		}

		// Skip binary files - they don't have hunks
		if file.IsBinary {
			continue
		}

		// Validate hunks
		for hunkIdx, hunk := range file.Hunks {
			result.Stats.Hunks++
			hunkNum := hunkIdx + 1

			// Count lines in hunk
			oldLines := 0
			newLines := 0

			for _, line := range hunk.Lines {
				switch line.Type {
				case LineContext:
					oldLines++
					newLines++
				case LineRemove:
					oldLines++
					result.Stats.Deletions++
				case LineAdd:
					newLines++
					result.Stats.Additions++
				}
			}

			// Validate line counts match header
			if oldLines != hunk.OldCount {
				result.Valid = false
				result.Errors = append(result.Errors, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: fmt.Sprintf("old line count mismatch: header says %d but found %d", hunk.OldCount, oldLines),
				})
			}

			if newLines != hunk.NewCount {
				result.Valid = false
				result.Errors = append(result.Errors, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: fmt.Sprintf("new line count mismatch: header says %d but found %d", hunk.NewCount, newLines),
				})
			}

			// Validate hunk has content (unless it's 0,0 which means empty)
			if len(hunk.Lines) == 0 && (hunk.OldCount > 0 || hunk.NewCount > 0) {
				result.Valid = false
				result.Errors = append(result.Errors, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: "hunk is empty but header indicates content",
				})
			}

			// Validate line numbers are positive
			if hunk.OldStart < 0 {
				result.Valid = false
				result.Errors = append(result.Errors, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: "old file start line cannot be negative",
				})
			}
			if hunk.NewStart < 0 {
				result.Valid = false
				result.Errors = append(result.Errors, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: "new file start line cannot be negative",
				})
			}

			// Warning: hunk with only additions or only deletions
			hasAdditions := false
			hasDeletions := false
			for _, line := range hunk.Lines {
				if line.Type == LineAdd {
					hasAdditions = true
				}
				if line.Type == LineRemove {
					hasDeletions = true
				}
			}

			if !hasAdditions && !hasDeletions && len(hunk.Lines) > 0 {
				result.Warnings = append(result.Warnings, PatchValidationError{
					File:    fileName,
					Hunk:    hunkNum,
					Message: "hunk contains only context lines (no changes)",
				})
			}
		}

		// Validate new files have no old content
		if file.IsNew && !file.IsBinary {
			for hunkIdx, hunk := range file.Hunks {
				if hunk.OldCount > 0 {
					result.Warnings = append(result.Warnings, PatchValidationError{
						File:    fileName,
						Hunk:    hunkIdx + 1,
						Message: "new file has non-zero old line count",
					})
				}
			}
		}

		// Validate deleted files have no new content
		if file.IsDeleted && !file.IsBinary {
			for hunkIdx, hunk := range file.Hunks {
				if hunk.NewCount > 0 {
					result.Warnings = append(result.Warnings, PatchValidationError{
						File:    fileName,
						Hunk:    hunkIdx + 1,
						Message: "deleted file has non-zero new line count",
					})
				}
			}
		}
	}

	return result
}

// Stats returns statistics about the patch.
func (p *Patch) Stats() PatchStats {
	result := ValidatePatch(p)
	return result.Stats
}

// GetFilePaths returns a list of all file paths affected by the patch.
func (p *Patch) GetFilePaths() []string {
	paths := make([]string, 0, len(p.Files))
	seen := make(map[string]bool)

	for _, file := range p.Files {
		if file.OldName != "" && file.OldName != "/dev/null" && !seen[file.OldName] {
			paths = append(paths, file.OldName)
			seen[file.OldName] = true
		}
		if file.NewName != "" && file.NewName != "/dev/null" && !seen[file.NewName] {
			paths = append(paths, file.NewName)
			seen[file.NewName] = true
		}
	}

	return paths
}

// ValidatePatchString parses and validates a patch string, returning detailed results.
func ValidatePatchString(content string) (*Patch, *PatchValidationResult, error) {
	patch, err := ParsePatch(content)
	if err != nil {
		return nil, nil, err
	}

	result := ValidatePatch(patch)
	return patch, result, nil
}
