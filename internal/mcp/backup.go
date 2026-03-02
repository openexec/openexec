// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements a backup manager for file operations.
package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// BackupError represents an error that occurred during backup operations.
type BackupError struct {
	Operation string
	Path      string
	Message   string
	Cause     error
}

func (e *BackupError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s %s: %s: %v", e.Operation, e.Path, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s %s: %s", e.Operation, e.Path, e.Message)
}

func (e *BackupError) Unwrap() error {
	return e.Cause
}

// Common backup errors.
var (
	ErrBackupNotFound     = errors.New("backup not found")
	ErrBackupDirNotSet    = errors.New("backup directory not configured")
	ErrBackupExists       = errors.New("backup already exists")
	ErrOriginalNotFound   = errors.New("original file not found")
	ErrBackupCorrupted    = errors.New("backup file is corrupted (checksum mismatch)")
	ErrMaxBackupsExceeded = errors.New("maximum number of backups exceeded")
)

// BackupMetadata contains metadata about a backup file.
type BackupMetadata struct {
	// ID is a unique identifier for this backup.
	ID string `json:"id"`

	// OriginalPath is the absolute path of the original file.
	OriginalPath string `json:"original_path"`

	// BackupPath is the path where the backup is stored.
	BackupPath string `json:"backup_path"`

	// Checksum is the SHA256 hash of the original file content.
	Checksum string `json:"checksum"`

	// Size is the size of the original file in bytes.
	Size int64 `json:"size"`

	// CreatedAt is when the backup was created.
	CreatedAt time.Time `json:"created_at"`

	// FileMode is the original file's permissions.
	FileMode os.FileMode `json:"file_mode"`

	// SessionID is the optional session identifier for grouping backups.
	SessionID string `json:"session_id,omitempty"`

	// Restored indicates whether this backup has been restored.
	Restored bool `json:"restored"`

	// RestoredAt is when the backup was restored (if applicable).
	RestoredAt *time.Time `json:"restored_at,omitempty"`
}

// BackupManagerConfig holds configuration for the BackupManager.
type BackupManagerConfig struct {
	// BackupDir is the directory where backups are stored.
	BackupDir string

	// MaxBackupsPerFile is the maximum number of backups to keep per file.
	// Older backups are automatically pruned. 0 means unlimited.
	MaxBackupsPerFile int

	// MaxTotalBackups is the maximum total number of backups to keep.
	// Oldest backups across all files are pruned. 0 means unlimited.
	MaxTotalBackups int

	// RetentionDuration is how long to keep backups.
	// Backups older than this are automatically pruned. 0 means forever.
	RetentionDuration time.Duration

	// VerifyOnRestore determines whether to verify checksum before restoring.
	VerifyOnRestore bool
}

// DefaultBackupManagerConfig returns a default configuration with sensible defaults.
func DefaultBackupManagerConfig() BackupManagerConfig {
	return BackupManagerConfig{
		BackupDir:         "",                // Must be set by caller
		MaxBackupsPerFile: 10,                // Keep 10 backups per file
		MaxTotalBackups:   1000,              // Keep up to 1000 total backups
		RetentionDuration: 24 * 7 * time.Hour, // Keep backups for 1 week
		VerifyOnRestore:   true,
	}
}

// BackupManager handles file backup operations for write_file tool.
type BackupManager struct {
	config BackupManagerConfig
	mu     sync.RWMutex

	// metadataFile is the path to the metadata index file.
	metadataFile string

	// backups is an in-memory cache of backup metadata indexed by backup ID.
	backups map[string]*BackupMetadata

	// fileBackups maps original file paths to their backup IDs (sorted by time, newest first).
	fileBackups map[string][]string
}

// NewBackupManager creates a new BackupManager with the given configuration.
func NewBackupManager(config BackupManagerConfig) (*BackupManager, error) {
	if config.BackupDir == "" {
		return nil, ErrBackupDirNotSet
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(config.BackupDir, 0755); err != nil {
		return nil, &BackupError{
			Operation: "init",
			Path:      config.BackupDir,
			Message:   "failed to create backup directory",
			Cause:     err,
		}
	}

	bm := &BackupManager{
		config:       config,
		metadataFile: filepath.Join(config.BackupDir, "backups.json"),
		backups:      make(map[string]*BackupMetadata),
		fileBackups:  make(map[string][]string),
	}

	// Load existing metadata
	if err := bm.loadMetadata(); err != nil {
		// If metadata doesn't exist, that's fine - start fresh
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return bm, nil
}

// CreateBackup creates a backup of the specified file.
// Returns the backup metadata or an error if the backup failed.
func (bm *BackupManager) CreateBackup(filePath string, sessionID string) (*BackupMetadata, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Validate the file path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, &BackupError{
			Operation: "backup",
			Path:      filePath,
			Message:   "failed to resolve absolute path",
			Cause:     err,
		}
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - no backup needed (new file)
			return nil, nil
		}
		return nil, &BackupError{
			Operation: "backup",
			Path:      absPath,
			Message:   "failed to stat file",
			Cause:     err,
		}
	}

	// Only backup regular files
	if !info.Mode().IsRegular() {
		return nil, &BackupError{
			Operation: "backup",
			Path:      absPath,
			Message:   "can only backup regular files",
		}
	}

	// Generate backup ID
	backupID := bm.generateBackupID(absPath)

	// Create backup path
	backupPath := filepath.Join(bm.config.BackupDir, backupID)

	// Open source file
	srcFile, err := os.Open(absPath)
	if err != nil {
		return nil, &BackupError{
			Operation: "backup",
			Path:      absPath,
			Message:   "failed to open source file",
			Cause:     err,
		}
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode())
	if err != nil {
		return nil, &BackupError{
			Operation: "backup",
			Path:      backupPath,
			Message:   "failed to create backup file",
			Cause:     err,
		}
	}
	defer dstFile.Close()

	// Copy content and calculate checksum
	hash := sha256.New()
	reader := io.TeeReader(srcFile, hash)

	n, err := io.Copy(dstFile, reader)
	if err != nil {
		// Clean up failed backup
		os.Remove(backupPath)
		return nil, &BackupError{
			Operation: "backup",
			Path:      absPath,
			Message:   "failed to copy file content",
			Cause:     err,
		}
	}

	// Ensure data is written to disk
	if err := dstFile.Sync(); err != nil {
		os.Remove(backupPath)
		return nil, &BackupError{
			Operation: "backup",
			Path:      backupPath,
			Message:   "failed to sync backup file",
			Cause:     err,
		}
	}

	// Create metadata
	now := time.Now()
	metadata := &BackupMetadata{
		ID:           backupID,
		OriginalPath: absPath,
		BackupPath:   backupPath,
		Checksum:     hex.EncodeToString(hash.Sum(nil)),
		Size:         n,
		CreatedAt:    now,
		FileMode:     info.Mode(),
		SessionID:    sessionID,
		Restored:     false,
	}

	// Store metadata
	bm.backups[backupID] = metadata

	// Update file backups index
	if bm.fileBackups[absPath] == nil {
		bm.fileBackups[absPath] = []string{}
	}
	bm.fileBackups[absPath] = append([]string{backupID}, bm.fileBackups[absPath]...)

	// Prune if necessary
	if err := bm.pruneBackupsLocked(); err != nil {
		// Log but don't fail the backup
		// In production, this would be logged properly
	}

	// Save metadata
	if err := bm.saveMetadata(); err != nil {
		return nil, err
	}

	return metadata, nil
}

// Restore restores a file from a backup.
func (bm *BackupManager) Restore(backupID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	metadata, ok := bm.backups[backupID]
	if !ok {
		return ErrBackupNotFound
	}

	// Verify backup file exists
	backupInfo, err := os.Stat(metadata.BackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BackupError{
				Operation: "restore",
				Path:      metadata.BackupPath,
				Message:   "backup file missing",
				Cause:     ErrBackupNotFound,
			}
		}
		return &BackupError{
			Operation: "restore",
			Path:      metadata.BackupPath,
			Message:   "failed to stat backup file",
			Cause:     err,
		}
	}

	// Verify checksum if configured
	if bm.config.VerifyOnRestore {
		if err := bm.verifyBackup(metadata); err != nil {
			return err
		}
	}

	// Create parent directory if needed
	parentDir := filepath.Dir(metadata.OriginalPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return &BackupError{
			Operation: "restore",
			Path:      parentDir,
			Message:   "failed to create parent directory",
			Cause:     err,
		}
	}

	// Open backup file
	srcFile, err := os.Open(metadata.BackupPath)
	if err != nil {
		return &BackupError{
			Operation: "restore",
			Path:      metadata.BackupPath,
			Message:   "failed to open backup file",
			Cause:     err,
		}
	}
	defer srcFile.Close()

	// Create temporary file in same directory for atomic write
	tmpPath := metadata.OriginalPath + ".restore.tmp"
	dstFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, backupInfo.Mode())
	if err != nil {
		return &BackupError{
			Operation: "restore",
			Path:      tmpPath,
			Message:   "failed to create temporary restore file",
			Cause:     err,
		}
	}

	// Copy content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		dstFile.Close()
		os.Remove(tmpPath)
		return &BackupError{
			Operation: "restore",
			Path:      metadata.OriginalPath,
			Message:   "failed to copy backup content",
			Cause:     err,
		}
	}

	// Sync and close
	if err := dstFile.Sync(); err != nil {
		dstFile.Close()
		os.Remove(tmpPath)
		return &BackupError{
			Operation: "restore",
			Path:      tmpPath,
			Message:   "failed to sync restore file",
			Cause:     err,
		}
	}
	dstFile.Close()

	// Atomically rename
	if err := os.Rename(tmpPath, metadata.OriginalPath); err != nil {
		os.Remove(tmpPath)
		return &BackupError{
			Operation: "restore",
			Path:      metadata.OriginalPath,
			Message:   "failed to rename restore file",
			Cause:     err,
		}
	}

	// Restore original permissions
	if err := os.Chmod(metadata.OriginalPath, metadata.FileMode); err != nil {
		// Non-fatal, but note it
		// In production, this would be logged
	}

	// Update metadata
	now := time.Now()
	metadata.Restored = true
	metadata.RestoredAt = &now

	// Save metadata
	return bm.saveMetadata()
}

// RestoreLatest restores the most recent backup for a file.
func (bm *BackupManager) RestoreLatest(filePath string) error {
	bm.mu.RLock()
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		bm.mu.RUnlock()
		return &BackupError{
			Operation: "restore",
			Path:      filePath,
			Message:   "failed to resolve absolute path",
			Cause:     err,
		}
	}

	backupIDs := bm.fileBackups[absPath]
	if len(backupIDs) == 0 {
		bm.mu.RUnlock()
		return ErrBackupNotFound
	}

	// Get the most recent backup ID (first in the slice)
	backupID := backupIDs[0]
	bm.mu.RUnlock()

	return bm.Restore(backupID)
}

// GetBackup retrieves metadata for a specific backup.
func (bm *BackupManager) GetBackup(backupID string) (*BackupMetadata, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	metadata, ok := bm.backups[backupID]
	if !ok {
		return nil, ErrBackupNotFound
	}

	// Return a copy to prevent external modification
	copyMeta := *metadata
	return &copyMeta, nil
}

// ListBackups returns all backups for a specific file, sorted by creation time (newest first).
func (bm *BackupManager) ListBackups(filePath string) ([]*BackupMetadata, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, &BackupError{
			Operation: "list",
			Path:      filePath,
			Message:   "failed to resolve absolute path",
			Cause:     err,
		}
	}

	backupIDs := bm.fileBackups[absPath]
	result := make([]*BackupMetadata, 0, len(backupIDs))

	for _, id := range backupIDs {
		if meta, ok := bm.backups[id]; ok {
			copyMeta := *meta
			result = append(result, &copyMeta)
		}
	}

	return result, nil
}

// ListAllBackups returns all backups, optionally filtered by session ID.
func (bm *BackupManager) ListAllBackups(sessionID string) []*BackupMetadata {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make([]*BackupMetadata, 0, len(bm.backups))

	for _, meta := range bm.backups {
		if sessionID == "" || meta.SessionID == sessionID {
			copyMeta := *meta
			result = append(result, &copyMeta)
		}
	}

	// Sort by creation time, newest first
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result
}

// DeleteBackup removes a backup and its metadata.
func (bm *BackupManager) DeleteBackup(backupID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	metadata, ok := bm.backups[backupID]
	if !ok {
		return ErrBackupNotFound
	}

	// Remove backup file
	if err := os.Remove(metadata.BackupPath); err != nil && !os.IsNotExist(err) {
		return &BackupError{
			Operation: "delete",
			Path:      metadata.BackupPath,
			Message:   "failed to remove backup file",
			Cause:     err,
		}
	}

	// Remove from metadata
	delete(bm.backups, backupID)

	// Remove from file index
	if ids := bm.fileBackups[metadata.OriginalPath]; ids != nil {
		newIDs := make([]string, 0, len(ids)-1)
		for _, id := range ids {
			if id != backupID {
				newIDs = append(newIDs, id)
			}
		}
		if len(newIDs) == 0 {
			delete(bm.fileBackups, metadata.OriginalPath)
		} else {
			bm.fileBackups[metadata.OriginalPath] = newIDs
		}
	}

	return bm.saveMetadata()
}

// Prune removes old backups according to the configured retention policy.
func (bm *BackupManager) Prune() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.pruneBackupsLocked()
}

// pruneBackupsLocked performs backup pruning. Caller must hold the write lock.
func (bm *BackupManager) pruneBackupsLocked() error {
	now := time.Now()
	toDelete := []string{}

	// Prune by retention duration
	if bm.config.RetentionDuration > 0 {
		cutoff := now.Add(-bm.config.RetentionDuration)
		for id, meta := range bm.backups {
			if meta.CreatedAt.Before(cutoff) {
				toDelete = append(toDelete, id)
			}
		}
	}

	// Prune by max backups per file
	if bm.config.MaxBackupsPerFile > 0 {
		for _, ids := range bm.fileBackups {
			if len(ids) > bm.config.MaxBackupsPerFile {
				// ids are sorted newest first, so remove oldest
				excess := ids[bm.config.MaxBackupsPerFile:]
				toDelete = append(toDelete, excess...)
			}
		}
	}

	// Prune by max total backups
	if bm.config.MaxTotalBackups > 0 && len(bm.backups) > bm.config.MaxTotalBackups {
		// Get all backups sorted by age (oldest first)
		allBackups := make([]*BackupMetadata, 0, len(bm.backups))
		for _, meta := range bm.backups {
			allBackups = append(allBackups, meta)
		}
		sort.Slice(allBackups, func(i, j int) bool {
			return allBackups[i].CreatedAt.Before(allBackups[j].CreatedAt)
		})

		// Remove oldest until we're under the limit
		excess := len(bm.backups) - bm.config.MaxTotalBackups
		for i := 0; i < excess && i < len(allBackups); i++ {
			toDelete = append(toDelete, allBackups[i].ID)
		}
	}

	// Remove duplicates
	deleteSet := make(map[string]bool)
	for _, id := range toDelete {
		deleteSet[id] = true
	}

	// Delete backups
	for id := range deleteSet {
		if meta, ok := bm.backups[id]; ok {
			// Remove file
			os.Remove(meta.BackupPath)

			// Remove from metadata
			delete(bm.backups, id)

			// Remove from file index
			if ids := bm.fileBackups[meta.OriginalPath]; ids != nil {
				newIDs := make([]string, 0, len(ids)-1)
				for _, backupID := range ids {
					if backupID != id {
						newIDs = append(newIDs, backupID)
					}
				}
				if len(newIDs) == 0 {
					delete(bm.fileBackups, meta.OriginalPath)
				} else {
					bm.fileBackups[meta.OriginalPath] = newIDs
				}
			}
		}
	}

	return bm.saveMetadata()
}

// verifyBackup verifies a backup file's integrity by checking its checksum.
func (bm *BackupManager) verifyBackup(metadata *BackupMetadata) error {
	file, err := os.Open(metadata.BackupPath)
	if err != nil {
		return &BackupError{
			Operation: "verify",
			Path:      metadata.BackupPath,
			Message:   "failed to open backup file",
			Cause:     err,
		}
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return &BackupError{
			Operation: "verify",
			Path:      metadata.BackupPath,
			Message:   "failed to compute checksum",
			Cause:     err,
		}
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	if checksum != metadata.Checksum {
		return &BackupError{
			Operation: "verify",
			Path:      metadata.BackupPath,
			Message:   "checksum mismatch",
			Cause:     ErrBackupCorrupted,
		}
	}

	return nil
}

// generateBackupID generates a unique backup ID based on file path and timestamp.
func (bm *BackupManager) generateBackupID(filePath string) string {
	timestamp := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", filePath, timestamp)))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes for a reasonable ID length
}

// loadMetadata loads backup metadata from disk.
func (bm *BackupManager) loadMetadata() error {
	data, err := os.ReadFile(bm.metadataFile)
	if err != nil {
		return err
	}

	var backups []*BackupMetadata
	if err := json.Unmarshal(data, &backups); err != nil {
		return &BackupError{
			Operation: "load",
			Path:      bm.metadataFile,
			Message:   "failed to parse metadata",
			Cause:     err,
		}
	}

	// Rebuild indexes
	bm.backups = make(map[string]*BackupMetadata)
	bm.fileBackups = make(map[string][]string)

	for _, meta := range backups {
		bm.backups[meta.ID] = meta
		if bm.fileBackups[meta.OriginalPath] == nil {
			bm.fileBackups[meta.OriginalPath] = []string{}
		}
		bm.fileBackups[meta.OriginalPath] = append(bm.fileBackups[meta.OriginalPath], meta.ID)
	}

	// Sort file backups by creation time (newest first)
	for path := range bm.fileBackups {
		ids := bm.fileBackups[path]
		sort.Slice(ids, func(i, j int) bool {
			mi := bm.backups[ids[i]]
			mj := bm.backups[ids[j]]
			return mi.CreatedAt.After(mj.CreatedAt)
		})
	}

	return nil
}

// saveMetadata saves backup metadata to disk.
func (bm *BackupManager) saveMetadata() error {
	// Collect all metadata
	backups := make([]*BackupMetadata, 0, len(bm.backups))
	for _, meta := range bm.backups {
		backups = append(backups, meta)
	}

	// Sort by creation time for deterministic output
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.Before(backups[j].CreatedAt)
	})

	data, err := json.MarshalIndent(backups, "", "  ")
	if err != nil {
		return &BackupError{
			Operation: "save",
			Path:      bm.metadataFile,
			Message:   "failed to serialize metadata",
			Cause:     err,
		}
	}

	// Write to temporary file first
	tmpPath := bm.metadataFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return &BackupError{
			Operation: "save",
			Path:      tmpPath,
			Message:   "failed to write metadata",
			Cause:     err,
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, bm.metadataFile); err != nil {
		os.Remove(tmpPath)
		return &BackupError{
			Operation: "save",
			Path:      bm.metadataFile,
			Message:   "failed to rename metadata file",
			Cause:     err,
		}
	}

	return nil
}

// Close closes the backup manager and releases resources.
func (bm *BackupManager) Close() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.saveMetadata()
}

// Stats returns statistics about the backup manager.
type BackupStats struct {
	TotalBackups   int   `json:"total_backups"`
	TotalFiles     int   `json:"total_files"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
	OldestBackup   *time.Time `json:"oldest_backup,omitempty"`
	NewestBackup   *time.Time `json:"newest_backup,omitempty"`
}

// Stats returns current backup statistics.
func (bm *BackupManager) Stats() BackupStats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	stats := BackupStats{
		TotalBackups: len(bm.backups),
		TotalFiles:   len(bm.fileBackups),
	}

	var oldest, newest time.Time

	for _, meta := range bm.backups {
		stats.TotalSizeBytes += meta.Size

		if oldest.IsZero() || meta.CreatedAt.Before(oldest) {
			oldest = meta.CreatedAt
		}
		if newest.IsZero() || meta.CreatedAt.After(newest) {
			newest = meta.CreatedAt
		}
	}

	if !oldest.IsZero() {
		stats.OldestBackup = &oldest
	}
	if !newest.IsZero() {
		stats.NewestBackup = &newest
	}

	return stats
}
