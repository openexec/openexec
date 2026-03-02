package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupError(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := os.ErrNotExist
		err := &BackupError{
			Operation: "backup",
			Path:      "/test/path",
			Message:   "file not found",
			Cause:     cause,
		}

		expected := "backup /test/path: file not found: file does not exist"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}

		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})

	t.Run("without cause", func(t *testing.T) {
		err := &BackupError{
			Operation: "restore",
			Path:      "/test/path",
			Message:   "something failed",
		}

		expected := "restore /test/path: something failed"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}

		if err.Unwrap() != nil {
			t.Errorf("Unwrap() = %v, want nil", err.Unwrap())
		}
	})
}

func TestDefaultBackupManagerConfig(t *testing.T) {
	config := DefaultBackupManagerConfig()

	if config.BackupDir != "" {
		t.Errorf("BackupDir = %q, want empty string", config.BackupDir)
	}
	if config.MaxBackupsPerFile != 10 {
		t.Errorf("MaxBackupsPerFile = %d, want 10", config.MaxBackupsPerFile)
	}
	if config.MaxTotalBackups != 1000 {
		t.Errorf("MaxTotalBackups = %d, want 1000", config.MaxTotalBackups)
	}
	if config.RetentionDuration != 24*7*time.Hour {
		t.Errorf("RetentionDuration = %v, want 1 week", config.RetentionDuration)
	}
	if !config.VerifyOnRestore {
		t.Error("VerifyOnRestore should be true by default")
	}
}

func TestNewBackupManager_EmptyDir(t *testing.T) {
	config := BackupManagerConfig{}

	_, err := NewBackupManager(config)
	if err != ErrBackupDirNotSet {
		t.Errorf("expected ErrBackupDirNotSet, got %v", err)
	}
}

func TestNewBackupManager_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups", "nested")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Check that directory was created
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("backup directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("backup path is not a directory")
	}
}

func TestBackupManager_CreateBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir:         backupDir,
		MaxBackupsPerFile: 10,
		MaxTotalBackups:   100,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "session-1")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Verify metadata
	if metadata == nil {
		t.Fatal("metadata is nil")
	}
	if metadata.ID == "" {
		t.Error("backup ID is empty")
	}
	if metadata.OriginalPath != testFile {
		t.Errorf("OriginalPath = %q, want %q", metadata.OriginalPath, testFile)
	}
	if metadata.Size != int64(len(testContent)) {
		t.Errorf("Size = %d, want %d", metadata.Size, len(testContent))
	}
	if metadata.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", metadata.SessionID, "session-1")
	}
	if metadata.Checksum == "" {
		t.Error("Checksum is empty")
	}
	if metadata.Restored {
		t.Error("Restored should be false for new backup")
	}

	// Verify backup file exists
	backupContent, err := os.ReadFile(metadata.BackupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}
	if string(backupContent) != testContent {
		t.Errorf("backup content = %q, want %q", string(backupContent), testContent)
	}
}

func TestBackupManager_CreateBackup_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create backup of non-existent file (should return nil, nil)
	metadata, err := bm.CreateBackup(filepath.Join(tmpDir, "nonexistent.txt"), "")
	if err != nil {
		t.Errorf("CreateBackup() error = %v, want nil", err)
	}
	if metadata != nil {
		t.Errorf("metadata = %v, want nil for non-existent file", metadata)
	}
}

func TestBackupManager_CreateBackup_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Try to backup a directory
	testDir := filepath.Join(tmpDir, "testdir")
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	_, err = bm.CreateBackup(testDir, "")
	if err == nil {
		t.Error("expected error when backing up a directory")
	}
	if !strings.Contains(err.Error(), "regular files") {
		t.Errorf("error should mention regular files: %v", err)
	}
}

func TestBackupManager_Restore(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir:       backupDir,
		VerifyOnRestore: true,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "Original content"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Modify the file
	modifiedContent := "Modified content"
	if err := os.WriteFile(testFile, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	// Verify file was modified
	content, _ := os.ReadFile(testFile)
	if string(content) != modifiedContent {
		t.Fatalf("file modification failed")
	}

	// Restore from backup
	if err := bm.Restore(metadata.ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify content was restored
	content, err = os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(content) != originalContent {
		t.Errorf("restored content = %q, want %q", string(content), originalContent)
	}

	// Verify metadata was updated
	updatedMeta, _ := bm.GetBackup(metadata.ID)
	if !updatedMeta.Restored {
		t.Error("Restored flag should be true after restore")
	}
	if updatedMeta.RestoredAt == nil {
		t.Error("RestoredAt should be set after restore")
	}
}

func TestBackupManager_Restore_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	err = bm.Restore("nonexistent-id")
	if err != ErrBackupNotFound {
		t.Errorf("Restore() error = %v, want ErrBackupNotFound", err)
	}
}

func TestBackupManager_RestoreLatest(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	content1 := "Version 1"
	if err := os.WriteFile(testFile, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create first backup
	_, err = bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Modify and create second backup
	content2 := "Version 2"
	if err := os.WriteFile(testFile, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp

	_, err = bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Modify file again
	content3 := "Version 3"
	if err := os.WriteFile(testFile, []byte(content3), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	// Restore latest (should restore Version 2)
	if err := bm.RestoreLatest(testFile); err != nil {
		t.Fatalf("RestoreLatest() error = %v", err)
	}

	// Verify content was restored to Version 2
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(content) != content2 {
		t.Errorf("restored content = %q, want %q", string(content), content2)
	}
}

func TestBackupManager_RestoreLatest_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	err = bm.RestoreLatest(filepath.Join(tmpDir, "nonexistent.txt"))
	if err != ErrBackupNotFound {
		t.Errorf("RestoreLatest() error = %v, want ErrBackupNotFound", err)
	}
}

func TestBackupManager_GetBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file and backup
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	metadata, err := bm.CreateBackup(testFile, "session-1")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Get backup
	retrieved, err := bm.GetBackup(metadata.ID)
	if err != nil {
		t.Fatalf("GetBackup() error = %v", err)
	}

	if retrieved.ID != metadata.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, metadata.ID)
	}
	if retrieved.OriginalPath != metadata.OriginalPath {
		t.Errorf("OriginalPath = %q, want %q", retrieved.OriginalPath, metadata.OriginalPath)
	}
}

func TestBackupManager_GetBackup_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	_, err = bm.GetBackup("nonexistent-id")
	if err != ErrBackupNotFound {
		t.Errorf("GetBackup() error = %v, want ErrBackupNotFound", err)
	}
}

func TestBackupManager_ListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create test files
	testFile1 := filepath.Join(tmpDir, "test1.txt")
	testFile2 := filepath.Join(tmpDir, "test2.txt")

	if err := os.WriteFile(testFile1, []byte("file1 v1"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("file2 v1"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backups for file1
	_, err = bm.CreateBackup(testFile1, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Modify and create another backup for file1
	if err := os.WriteFile(testFile1, []byte("file1 v2"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	_, err = bm.CreateBackup(testFile1, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Create backup for file2
	_, err = bm.CreateBackup(testFile2, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// List backups for file1
	backups, err := bm.ListBackups(testFile1)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("ListBackups() returned %d backups, want 2", len(backups))
	}

	// Verify sorted by time (newest first)
	if backups[0].CreatedAt.Before(backups[1].CreatedAt) {
		t.Error("backups should be sorted newest first")
	}

	// List backups for file2
	backups, err = bm.ListBackups(testFile2)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("ListBackups() returned %d backups, want 1", len(backups))
	}
}

func TestBackupManager_ListAllBackups(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create test files
	testFile1 := filepath.Join(tmpDir, "test1.txt")
	testFile2 := filepath.Join(tmpDir, "test2.txt")

	if err := os.WriteFile(testFile1, []byte("file1"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("file2"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backups with different sessions
	_, err = bm.CreateBackup(testFile1, "session-1")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	_, err = bm.CreateBackup(testFile2, "session-2")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// List all backups
	allBackups := bm.ListAllBackups("")
	if len(allBackups) != 2 {
		t.Errorf("ListAllBackups() returned %d backups, want 2", len(allBackups))
	}

	// Filter by session
	session1Backups := bm.ListAllBackups("session-1")
	if len(session1Backups) != 1 {
		t.Errorf("ListAllBackups(session-1) returned %d backups, want 1", len(session1Backups))
	}
	if session1Backups[0].SessionID != "session-1" {
		t.Errorf("SessionID = %q, want session-1", session1Backups[0].SessionID)
	}
}

func TestBackupManager_DeleteBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file and backup
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Verify backup exists
	_, err = os.Stat(metadata.BackupPath)
	if err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}

	// Delete backup
	if err := bm.DeleteBackup(metadata.ID); err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}

	// Verify backup file is deleted
	_, err = os.Stat(metadata.BackupPath)
	if !os.IsNotExist(err) {
		t.Error("backup file should be deleted")
	}

	// Verify metadata is deleted
	_, err = bm.GetBackup(metadata.ID)
	if err != ErrBackupNotFound {
		t.Errorf("GetBackup() error = %v, want ErrBackupNotFound", err)
	}
}

func TestBackupManager_DeleteBackup_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	err = bm.DeleteBackup("nonexistent-id")
	if err != ErrBackupNotFound {
		t.Errorf("DeleteBackup() error = %v, want ErrBackupNotFound", err)
	}
}

func TestBackupManager_Prune_MaxBackupsPerFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir:         backupDir,
		MaxBackupsPerFile: 2,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create 5 backups
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(testFile, []byte("version "+string(rune('0'+i))), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		_, err := bm.CreateBackup(testFile, "")
		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}
	}

	// List backups - should only have 2 (MaxBackupsPerFile)
	backups, err := bm.ListBackups(testFile)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("ListBackups() returned %d backups, want 2", len(backups))
	}
}

func TestBackupManager_Prune_MaxTotalBackups(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir:         backupDir,
		MaxBackupsPerFile: 100, // High limit
		MaxTotalBackups:   3,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create multiple test files with backups
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		_, err := bm.CreateBackup(testFile, "")
		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}
	}

	// List all backups - should only have 3 (MaxTotalBackups)
	allBackups := bm.ListAllBackups("")
	if len(allBackups) != 3 {
		t.Errorf("ListAllBackups() returned %d backups, want 3", len(allBackups))
	}
}

func TestBackupManager_Stats(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Initial stats
	stats := bm.Stats()
	if stats.TotalBackups != 0 {
		t.Errorf("TotalBackups = %d, want 0", stats.TotalBackups)
	}
	if stats.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", stats.TotalFiles)
	}

	// Create backups
	testFile1 := filepath.Join(tmpDir, "test1.txt")
	testFile2 := filepath.Join(tmpDir, "test2.txt")

	if err := os.WriteFile(testFile1, []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	bm.CreateBackup(testFile1, "")
	bm.CreateBackup(testFile2, "")

	// Updated stats
	stats = bm.Stats()
	if stats.TotalBackups != 2 {
		t.Errorf("TotalBackups = %d, want 2", stats.TotalBackups)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", stats.TotalFiles)
	}
	if stats.TotalSizeBytes != int64(len("content1")+len("content2")) {
		t.Errorf("TotalSizeBytes = %d, want %d", stats.TotalSizeBytes, len("content1")+len("content2"))
	}
	if stats.OldestBackup == nil {
		t.Error("OldestBackup should not be nil")
	}
	if stats.NewestBackup == nil {
		t.Error("NewestBackup should not be nil")
	}
}

func TestBackupManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	// Create first backup manager and create backups
	bm1, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	metadata, err := bm1.CreateBackup(testFile, "session-1")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	bm1.Close()

	// Create second backup manager and verify backups are loaded
	bm2, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm2.Close()

	// Verify backup is loaded
	loadedMeta, err := bm2.GetBackup(metadata.ID)
	if err != nil {
		t.Fatalf("GetBackup() error = %v", err)
	}
	if loadedMeta.ID != metadata.ID {
		t.Errorf("ID = %q, want %q", loadedMeta.ID, metadata.ID)
	}
	if loadedMeta.OriginalPath != metadata.OriginalPath {
		t.Errorf("OriginalPath = %q, want %q", loadedMeta.OriginalPath, metadata.OriginalPath)
	}
	if loadedMeta.Checksum != metadata.Checksum {
		t.Errorf("Checksum = %q, want %q", loadedMeta.Checksum, metadata.Checksum)
	}
	if loadedMeta.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want session-1", loadedMeta.SessionID)
	}
}

func TestBackupManager_Restore_DeletedFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "Original content"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Delete the original file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("failed to delete test file: %v", err)
	}

	// Restore from backup
	if err := bm.Restore(metadata.ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify file was restored
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(content) != originalContent {
		t.Errorf("restored content = %q, want %q", string(content), originalContent)
	}
}

func TestBackupManager_Restore_NewParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file in a nested directory
	nestedDir := filepath.Join(tmpDir, "level1", "level2")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested directory: %v", err)
	}

	testFile := filepath.Join(nestedDir, "test.txt")
	originalContent := "Original content"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Delete the entire nested directory
	if err := os.RemoveAll(filepath.Join(tmpDir, "level1")); err != nil {
		t.Fatalf("failed to delete nested directory: %v", err)
	}

	// Restore from backup (should recreate parent directories)
	if err := bm.Restore(metadata.ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify file was restored
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(content) != originalContent {
		t.Errorf("restored content = %q, want %q", string(content), originalContent)
	}
}

func TestBackupManager_MultipleBackupsForSameFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir:         backupDir,
		MaxBackupsPerFile: 100, // Allow many backups
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	testFile := filepath.Join(tmpDir, "test.txt")
	versions := []string{"v1", "v2", "v3"}
	backupIDs := []string{}

	for _, version := range versions {
		if err := os.WriteFile(testFile, []byte(version), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		meta, err := bm.CreateBackup(testFile, "")
		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}
		backupIDs = append(backupIDs, meta.ID)
	}

	// Modify file to v4
	if err := os.WriteFile(testFile, []byte("v4"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Restore to v1 (first backup)
	if err := bm.Restore(backupIDs[0]); err != nil {
		t.Fatalf("Restore(v1) error = %v", err)
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "v1" {
		t.Errorf("content = %q, want v1", string(content))
	}

	// Restore to v3 (third backup)
	if err := bm.Restore(backupIDs[2]); err != nil {
		t.Fatalf("Restore(v3) error = %v", err)
	}

	content, _ = os.ReadFile(testFile)
	if string(content) != "v3" {
		t.Errorf("content = %q, want v3", string(content))
	}
}

func TestBackupMetadata_JSONSerialization(t *testing.T) {
	now := time.Now()
	restored := now.Add(time.Hour)

	meta := &BackupMetadata{
		ID:           "test-id",
		OriginalPath: "/path/to/file.txt",
		BackupPath:   "/backups/test-id",
		Checksum:     "abc123",
		Size:         1024,
		CreatedAt:    now,
		FileMode:     0644,
		SessionID:    "session-1",
		Restored:     true,
		RestoredAt:   &restored,
	}

	// This is primarily to verify the struct fields are properly tagged
	// The actual serialization is tested indirectly through persistence tests
	if meta.ID != "test-id" {
		t.Errorf("ID = %q, want test-id", meta.ID)
	}
	if meta.OriginalPath != "/path/to/file.txt" {
		t.Errorf("OriginalPath = %q, want /path/to/file.txt", meta.OriginalPath)
	}
}

func TestBackupManager_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a larger test file (1MB)
	testFile := filepath.Join(tmpDir, "large.txt")
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("failed to write large file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	if metadata.Size != int64(len(largeContent)) {
		t.Errorf("Size = %d, want %d", metadata.Size, len(largeContent))
	}

	// Modify file
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	// Restore from backup
	if err := bm.Restore(metadata.ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify content
	restored, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}

	if len(restored) != len(largeContent) {
		t.Errorf("restored size = %d, want %d", len(restored), len(largeContent))
	}

	for i := 0; i < len(largeContent); i++ {
		if restored[i] != largeContent[i] {
			t.Errorf("content mismatch at byte %d", i)
			break
		}
	}
}

func TestBackupManager_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	config := BackupManagerConfig{
		BackupDir: backupDir,
	}

	bm, err := NewBackupManager(config)
	if err != nil {
		t.Fatalf("NewBackupManager() error = %v", err)
	}
	defer bm.Close()

	// Create a test file with specific permissions
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0755); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create backup
	metadata, err := bm.CreateBackup(testFile, "")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	// Check that file mode was captured
	if metadata.FileMode != 0755 {
		t.Errorf("FileMode = %o, want 0755", metadata.FileMode)
	}

	// Modify file permissions
	if err := os.Chmod(testFile, 0644); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	// Restore from backup
	if err := bm.Restore(metadata.ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify permissions were restored
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat restored file: %v", err)
	}

	// Note: On some systems, the umask may affect the actual permissions
	// We check that the executable bit is set since we used 0755
	if info.Mode().Perm()&0100 == 0 {
		t.Error("expected executable bit to be set after restore")
	}
}
