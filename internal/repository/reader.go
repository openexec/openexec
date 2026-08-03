package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
)

var (
	ErrOutsideRoot   = errors.New("source path is outside repository root")
	ErrStalePointer  = errors.New("source pointer is stale")
	ErrRangeTooLarge = errors.New("source range exceeds configured limit")
)

type RootedReader struct {
	Root         string
	RepositoryID string
	WorktreeID   string
	MaxBytes     int
}

func NewRootedReader(root, repositoryID, worktreeID string, maxBytes int) (*RootedReader, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = evaluated
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	return &RootedReader{Root: filepath.Clean(abs), RepositoryID: repositoryID, WorktreeID: worktreeID, MaxBytes: maxBytes}, nil
}

func (r *RootedReader) ReadRange(ctx context.Context, request knowledge.SourceReadRequest) (knowledge.SourceReadResult, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.SourceReadResult{}, err
	}
	if request.RepositoryID != r.RepositoryID || request.WorktreeID != r.WorktreeID {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: repository or worktree identity mismatch", ErrOutsideRoot)
	}
	if request.FilePath == "" || filepath.IsAbs(request.FilePath) {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: invalid relative path", ErrOutsideRoot)
	}
	clean := filepath.Clean(filepath.FromSlash(request.FilePath))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: traversal", ErrOutsideRoot)
	}
	joined := filepath.Join(r.Root, clean)
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return knowledge.SourceReadResult{}, fmt.Errorf("resolve source path: %w", err)
	}
	rel, err := filepath.Rel(r.Root, resolved)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: symlink escape", ErrOutsideRoot)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return knowledge.SourceReadResult{}, err
	}
	if request.StartByte < 0 || request.EndByte < request.StartByte || request.EndByte > len(data) {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: byte range no longer exists", ErrStalePointer)
	}
	if request.EndByte-request.StartByte > r.MaxBytes {
		return knowledge.SourceReadResult{}, ErrRangeTooLarge
	}
	fileHash := digest(data)
	rangeData := data[request.StartByte:request.EndByte]
	rangeHash := digest(rangeData)
	if request.FileHash == "" || request.RangeHash == "" || fileHash != request.FileHash || rangeHash != request.RangeHash {
		return knowledge.SourceReadResult{}, fmt.Errorf("%w: expected file=%s range=%s, current file=%s range=%s", ErrStalePointer, request.FileHash, request.RangeHash, fileHash, rangeHash)
	}
	return knowledge.SourceReadResult{FilePath: filepath.ToSlash(clean), StartByte: request.StartByte, EndByte: request.EndByte, Content: string(rangeData), FileHash: fileHash, RangeHash: rangeHash}, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
