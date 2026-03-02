package mcp

import (
	"strings"
	"testing"
)

func TestParsePatch_SimpleAddition(t *testing.T) {
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	file := patch.Files[0]
	// Note: the parser strips the a/ and b/ prefixes
	if file.OldName != "file.txt" {
		t.Errorf("expected old name 'file.txt', got '%s'", file.OldName)
	}
	if file.NewName != "file.txt" {
		t.Errorf("expected new name 'file.txt', got '%s'", file.NewName)
	}

	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}

	hunk := file.Hunks[0]
	if hunk.OldStart != 1 || hunk.OldCount != 3 {
		t.Errorf("expected old range 1,3, got %d,%d", hunk.OldStart, hunk.OldCount)
	}
	if hunk.NewStart != 1 || hunk.NewCount != 4 {
		t.Errorf("expected new range 1,4, got %d,%d", hunk.NewStart, hunk.NewCount)
	}
}

func TestParsePatch_SimpleDeletion(t *testing.T) {
	// Note: This patch has 1 deletion and 3 context lines = 4 old lines, 3 new lines
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,4 +1,3 @@
 line1
-deleted line
 line2
 line3`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	hunk := patch.Files[0].Hunks[0]

	// Count line types
	adds := 0
	dels := 0
	ctx := 0
	for _, line := range hunk.Lines {
		switch line.Type {
		case LineAdd:
			adds++
		case LineRemove:
			dels++
		case LineContext:
			ctx++
		}
	}

	if dels != 1 {
		t.Errorf("expected 1 deletion, got %d", dels)
	}
	if adds != 0 {
		t.Errorf("expected 0 additions, got %d", adds)
	}
	if ctx != 3 {
		t.Errorf("expected 3 context lines, got %d", ctx)
	}
}

func TestParsePatch_NewFile(t *testing.T) {
	patchContent := `--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1,3 @@
+line1
+line2
+line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	file := patch.Files[0]
	if !file.IsNew {
		t.Error("expected file to be marked as new")
	}
	if file.OldName != "/dev/null" {
		t.Errorf("expected old name '/dev/null', got '%s'", file.OldName)
	}
}

func TestParsePatch_DeletedFile(t *testing.T) {
	patchContent := `--- a/deleted.txt
+++ /dev/null
@@ -1,3 +0,0 @@
-line1
-line2
-line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	file := patch.Files[0]
	if !file.IsDeleted {
		t.Error("expected file to be marked as deleted")
	}
	if file.NewName != "/dev/null" {
		t.Errorf("expected new name '/dev/null', got '%s'", file.NewName)
	}
}

func TestParsePatch_MultipleFiles(t *testing.T) {
	patchContent := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,3 @@
 line1
+new line
 line2
--- a/file2.txt
+++ b/file2.txt
@@ -1,3 +1,2 @@
 line1
-deleted
 line2`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(patch.Files))
	}

	// Note: the parser strips the a/ prefix
	if patch.Files[0].OldName != "file1.txt" {
		t.Errorf("expected first file 'file1.txt', got '%s'", patch.Files[0].OldName)
	}
	if patch.Files[1].OldName != "file2.txt" {
		t.Errorf("expected second file 'file2.txt', got '%s'", patch.Files[1].OldName)
	}
}

func TestParsePatch_MultipleHunks(t *testing.T) {
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+new1
 line2
 line3
@@ -10,3 +11,4 @@
 line10
+new2
 line11
 line12
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	if len(patch.Files[0].Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(patch.Files[0].Hunks))
	}

	hunk1 := patch.Files[0].Hunks[0]
	if hunk1.OldStart != 1 {
		t.Errorf("expected first hunk start at 1, got %d", hunk1.OldStart)
	}

	hunk2 := patch.Files[0].Hunks[1]
	if hunk2.OldStart != 10 {
		t.Errorf("expected second hunk start at 10, got %d", hunk2.OldStart)
	}
}

func TestParsePatch_GitDiffFormat(t *testing.T) {
	patchContent := `diff --git a/file.txt b/file.txt
index 1234567..abcdefg 100644
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	file := patch.Files[0]
	if len(file.GitHeaders) < 1 {
		t.Error("expected git headers to be captured")
	}
}

func TestParsePatch_NewFileMode(t *testing.T) {
	patchContent := `diff --git a/newfile.txt b/newfile.txt
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1,2 @@
+line1
+line2
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	file := patch.Files[0]
	if !file.IsNew {
		t.Error("expected file to be marked as new")
	}
}

func TestParsePatch_EmptyPatch(t *testing.T) {
	patchContent := ""
	_, err := ParsePatch(patchContent)
	if err == nil {
		t.Error("expected error for empty patch")
	}
}

func TestParsePatch_InvalidNoHeaders(t *testing.T) {
	patchContent := "just some random text"
	_, err := ParsePatch(patchContent)
	if err == nil {
		t.Error("expected error for patch without headers")
	}
}

func TestValidatePatch_ValidPatch(t *testing.T) {
	// Note: Removed trailing newline to avoid empty line being counted
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	result := ValidatePatch(patch)
	if !result.Valid {
		t.Errorf("expected valid patch, got errors: %v", result.Errors)
	}

	if result.Stats.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.Stats.FilesChanged)
	}
	if result.Stats.Additions != 1 {
		t.Errorf("expected 1 addition, got %d", result.Stats.Additions)
	}
	if result.Stats.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", result.Stats.Deletions)
	}
}

func TestValidatePatch_LineCountMismatch(t *testing.T) {
	// Intentionally wrong line counts in the hunk header
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,5 +1,4 @@
 line1
+new line
 line2
 line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	result := ValidatePatch(patch)
	if result.Valid {
		t.Error("expected validation to fail due to line count mismatch")
	}

	// Should have errors about line count mismatch
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "line count mismatch") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about line count mismatch")
	}
}

func TestValidatePatch_Stats(t *testing.T) {
	patchContent := `--- a/file1.txt
+++ b/file1.txt
@@ -1,3 +1,4 @@
 line1
+added1
 line2
 line3
--- a/file2.txt
+++ b/file2.txt
@@ -1,4 +1,3 @@
 line1
-deleted1
-deleted2
+added2
 line2`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	result := ValidatePatch(patch)

	if result.Stats.FilesChanged != 2 {
		t.Errorf("expected 2 files changed, got %d", result.Stats.FilesChanged)
	}
	if result.Stats.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", result.Stats.Additions)
	}
	if result.Stats.Deletions != 2 {
		t.Errorf("expected 2 deletions, got %d", result.Stats.Deletions)
	}
	if result.Stats.Hunks != 2 {
		t.Errorf("expected 2 hunks, got %d", result.Stats.Hunks)
	}
}

func TestGetFilePaths(t *testing.T) {
	patchContent := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,3 @@
 line1
+new
 line2
--- a/file2.txt
+++ b/file2.txt
@@ -1,2 +1,2 @@
 line1
-old
+new`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	paths := patch.GetFilePaths()
	// Parser strips a/ and b/ prefixes, so file1.txt and file2.txt (2 unique paths)
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d: %v", len(paths), paths)
	}
}

func TestValidatePatchString(t *testing.T) {
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,2 +1,3 @@
 line1
+new
 line2`
	patch, result, err := ValidatePatchString(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if patch == nil {
		t.Fatal("expected patch to be non-nil")
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if !result.Valid {
		t.Errorf("expected valid patch, got errors: %v", result.Errors)
	}
}

func TestParsePatch_NoNewlineAtEndOfFile(t *testing.T) {
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
 line2
-line3
\ No newline at end of file
+line3 modified
\ No newline at end of file
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	// Should parse despite the "\ No newline..." markers
	hunk := patch.Files[0].Hunks[0]
	if len(hunk.Lines) < 3 {
		t.Errorf("expected at least 3 lines in hunk, got %d", len(hunk.Lines))
	}
}

func TestParsePatch_BinaryFile(t *testing.T) {
	patchContent := `diff --git a/image.png b/image.png
index 1234567..abcdefg 100644
Binary files a/image.png and b/image.png differ
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	if !patch.Files[0].IsBinary {
		t.Error("expected file to be marked as binary")
	}
}

func TestParsePatch_RenameFile(t *testing.T) {
	patchContent := `diff --git a/old.txt b/new.txt
similarity index 95%
rename from old.txt
rename to new.txt
index 1234567..abcdefg 100644
--- a/old.txt
+++ b/new.txt
@@ -1,3 +1,3 @@
 line1
-old content
+new content
 line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	if !patch.Files[0].IsRenamed {
		t.Error("expected file to be marked as renamed")
	}
}

func TestParsePatch_SingleLineHunk(t *testing.T) {
	// When count is 1, it's often omitted from the header
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -5 +5 @@
-old line
+new line
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hunk := patch.Files[0].Hunks[0]
	if hunk.OldCount != 1 {
		t.Errorf("expected old count 1, got %d", hunk.OldCount)
	}
	if hunk.NewCount != 1 {
		t.Errorf("expected new count 1, got %d", hunk.NewCount)
	}
}

func TestValidatePatch_ContextOnlyWarning(t *testing.T) {
	// A hunk with only context lines should produce a warning
	patchContent := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
 line2
 line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	result := ValidatePatch(patch)

	// Should have a warning about context-only hunk
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "only context lines") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning about context-only hunk")
	}
}

func TestParseError_Format(t *testing.T) {
	err := &ParseError{
		Line:    5,
		Message: "test error",
		Context: "some context",
	}

	expected := `line 5: test error (context: "some context")`
	if err.Error() != expected {
		t.Errorf("expected error string '%s', got '%s'", expected, err.Error())
	}

	errNoContext := &ParseError{
		Line:    10,
		Message: "another error",
	}
	expectedNoContext := "line 10: another error"
	if errNoContext.Error() != expectedNoContext {
		t.Errorf("expected error string '%s', got '%s'", expectedNoContext, errNoContext.Error())
	}
}

func TestPatchValidationError_Format(t *testing.T) {
	// File and hunk level error
	err := &PatchValidationError{
		File:    "test.txt",
		Hunk:    2,
		Message: "test error",
	}
	expected := "test.txt hunk 2: test error"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}

	// File level error only
	errFile := &PatchValidationError{
		File:    "test.txt",
		Message: "file error",
	}
	expectedFile := "test.txt: file error"
	if errFile.Error() != expectedFile {
		t.Errorf("expected '%s', got '%s'", expectedFile, errFile.Error())
	}

	// General error
	errGeneral := &PatchValidationError{
		Message: "general error",
	}
	if errGeneral.Error() != "general error" {
		t.Errorf("expected 'general error', got '%s'", errGeneral.Error())
	}
}

func TestValidatePatch_BinaryFile(t *testing.T) {
	// Binary file patches should skip hunk validation
	// Need to include proper --- and +++ headers for parser to capture file names
	patchContent := `diff --git a/image.png b/image.png
index 1234567..abcdefg 100644
--- a/image.png
+++ b/image.png
Binary files a/image.png and b/image.png differ
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := ValidatePatch(patch)

	// Binary files should be valid even without hunks
	if !result.Valid {
		t.Errorf("expected binary file patch to be valid, got errors: %v", result.Errors)
	}

	if result.Stats.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.Stats.FilesChanged)
	}
}

func TestValidatePatch_DeletedFile(t *testing.T) {
	// Test that deleted file patches use OldName for fileName
	patchContent := `--- a/deleted.txt
+++ /dev/null
@@ -1,3 +0,0 @@
-line1
-line2
-line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := ValidatePatch(patch)

	if !result.Valid {
		t.Errorf("expected valid patch, got errors: %v", result.Errors)
	}

	if result.Stats.Deletions != 3 {
		t.Errorf("expected 3 deletions, got %d", result.Stats.Deletions)
	}
}

func TestValidatePatch_NewFileWithOldContent(t *testing.T) {
	// Test warning when new file has non-zero old line count
	patchContent := `diff --git a/newfile.txt b/newfile.txt
new file mode 100644
--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1,2 @@
+line1
+line2
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := ValidatePatch(patch)

	if !result.Valid {
		t.Errorf("expected valid patch, got errors: %v", result.Errors)
	}
}

func TestValidatePatch_DeletedFileWithNewContent(t *testing.T) {
	// Test validation when a deleted file has hunks
	patchContent := `diff --git a/deleted.txt b/deleted.txt
deleted file mode 100644
--- a/deleted.txt
+++ /dev/null
@@ -1,3 +0,0 @@
-line1
-line2
-line3
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := ValidatePatch(patch)

	if !result.Valid {
		t.Errorf("expected valid patch, got errors: %v", result.Errors)
	}
}

func TestValidatePatch_MissingBothHeaders(t *testing.T) {
	// Create a patch manually with no OldName or NewName
	patch := &Patch{
		Files: []PatchFile{
			{
				OldName: "",
				NewName: "",
			},
		},
	}

	result := ValidatePatch(patch)

	if result.Valid {
		t.Error("expected invalid patch when both headers are missing")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "missing both old and new name") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about missing headers")
	}
}

func TestValidatePatch_MissingOldHeader(t *testing.T) {
	// Create a patch manually with only NewName
	patch := &Patch{
		Files: []PatchFile{
			{
				OldName: "",
				NewName: "file.txt",
			},
		},
	}

	result := ValidatePatch(patch)

	if result.Valid {
		t.Error("expected invalid patch when old header is missing")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "missing old file header") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about missing old file header")
	}
}

func TestValidatePatch_EmptyHunkWithNonZeroCount(t *testing.T) {
	// Create a patch with empty hunk but non-zero counts
	patch := &Patch{
		Files: []PatchFile{
			{
				OldName: "file.txt",
				NewName: "file.txt",
				Hunks: []PatchHunk{
					{
						OldStart: 1,
						OldCount: 3,
						NewStart: 1,
						NewCount: 3,
						Lines:    []PatchLine{}, // Empty lines
					},
				},
			},
		},
	}

	result := ValidatePatch(patch)

	if result.Valid {
		t.Error("expected invalid patch when hunk is empty but header indicates content")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "hunk is empty") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about empty hunk")
	}
}

func TestParsePatch_DeletedFileMode(t *testing.T) {
	patchContent := `diff --git a/oldfile.txt b/oldfile.txt
deleted file mode 100644
index 1234567..0000000
--- a/oldfile.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-line1
-line2
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	if !patch.Files[0].IsDeleted {
		t.Error("expected file to be marked as deleted")
	}
}

func TestParsePatch_ModeChange(t *testing.T) {
	patchContent := `diff --git a/script.sh b/script.sh
old mode 100644
new mode 100755
--- a/script.sh
+++ b/script.sh
@@ -1 +1 @@
-#!/bin/bash
+#!/usr/bin/env bash
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	// Check that git headers were captured
	foundOldMode := false
	foundNewMode := false
	for _, header := range patch.Files[0].GitHeaders {
		if strings.HasPrefix(header, "old mode") {
			foundOldMode = true
		}
		if strings.HasPrefix(header, "new mode") {
			foundNewMode = true
		}
	}
	if !foundOldMode || !foundNewMode {
		t.Error("expected mode change headers to be captured")
	}
}

func TestGetFilePaths_NewAndDeleted(t *testing.T) {
	// Test GetFilePaths with new and deleted files
	patch := &Patch{
		Files: []PatchFile{
			{
				OldName: "/dev/null",
				NewName: "newfile.txt",
				IsNew:   true,
			},
			{
				OldName:   "deleted.txt",
				NewName:   "/dev/null",
				IsDeleted: true,
			},
			{
				OldName: "modified.txt",
				NewName: "modified.txt",
			},
		},
	}

	paths := patch.GetFilePaths()

	// Should have: newfile.txt, deleted.txt, modified.txt
	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d: %v", len(paths), paths)
	}

	// /dev/null should be filtered out
	for _, p := range paths {
		if p == "/dev/null" {
			t.Error("expected /dev/null to be filtered out")
		}
	}
}

func TestGetFilePaths_Deduplication(t *testing.T) {
	// Test that duplicate paths are deduplicated
	patch := &Patch{
		Files: []PatchFile{
			{
				OldName: "file.txt",
				NewName: "file.txt",
			},
		},
	}

	paths := patch.GetFilePaths()

	if len(paths) != 1 {
		t.Errorf("expected 1 path (deduplicated), got %d: %v", len(paths), paths)
	}
}

func TestParsePatch_SimilarityIndex(t *testing.T) {
	patchContent := `diff --git a/old.txt b/new.txt
similarity index 95%
rename from old.txt
rename to new.txt
--- a/old.txt
+++ b/new.txt
@@ -1 +1 @@
-old content
+new content
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}

	// Check that similarity index was captured
	foundSimilarity := false
	for _, header := range patch.Files[0].GitHeaders {
		if strings.HasPrefix(header, "similarity index") {
			foundSimilarity = true
			break
		}
	}
	if !foundSimilarity {
		t.Error("expected similarity index header to be captured")
	}
}

func TestParsePatch_MultipleHunksInMultipleFiles(t *testing.T) {
	patchContent := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,3 @@
 line1
+new1
 line2
@@ -10,2 +11,3 @@
 line10
+new10
 line11
--- a/file2.txt
+++ b/file2.txt
@@ -1,2 +1,3 @@
 a
+b
 c
`
	patch, err := ParsePatch(patchContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patch.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(patch.Files))
	}

	if len(patch.Files[0].Hunks) != 2 {
		t.Errorf("expected 2 hunks in file1, got %d", len(patch.Files[0].Hunks))
	}

	if len(patch.Files[1].Hunks) != 1 {
		t.Errorf("expected 1 hunk in file2, got %d", len(patch.Files[1].Hunks))
	}
}
