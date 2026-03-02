package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if store.writer == nil {
		t.Error("NewStore() writer should not be nil")
	}
}

func TestNewStore_InvalidPath(t *testing.T) {
	// Try to create a store with an invalid path
	_, err := NewStore("/nonexistent/path/store.db")
	if err == nil {
		t.Error("NewStore() should fail with invalid path")
	}
}

func TestNewStoreWithEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_encrypted_store.db")

	// Test valid key sizes
	keySizes := []int{16, 24, 32}
	for _, keySize := range keySizes {
		key := make([]byte, keySize)
		for i := range key {
			key[i] = byte(i)
		}

		store, err := NewStoreWithEncryption(dbPath, key)
		if err != nil {
			t.Errorf("NewStoreWithEncryption(%d bytes) error = %v", keySize, err)
			continue
		}
		store.Close()

		// Clean up for next iteration
		os.Remove(dbPath)
	}
}

func TestNewStoreWithEncryption_InvalidKeySize(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_encrypted_store.db")

	invalidKeySizes := []int{0, 8, 15, 17, 20, 31, 33, 64}
	for _, keySize := range invalidKeySizes {
		key := make([]byte, keySize)

		_, err := NewStoreWithEncryption(dbPath, key)
		if err == nil {
			t.Errorf("NewStoreWithEncryption(%d bytes) should fail with invalid key size", keySize)
		}
	}
}

func TestNewStoreWithRandomKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_random_key_store.db")

	// Test valid key sizes
	keySizes := []int{16, 24, 32}
	for _, keySize := range keySizes {
		store, key, err := NewStoreWithRandomKey(dbPath, keySize)
		if err != nil {
			t.Errorf("NewStoreWithRandomKey(%d) error = %v", keySize, err)
			continue
		}

		if len(key) != keySize {
			t.Errorf("NewStoreWithRandomKey(%d) key length = %d, want %d", keySize, len(key), keySize)
		}

		// Verify key is not all zeros (random)
		allZero := true
		for _, b := range key {
			if b != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Errorf("NewStoreWithRandomKey(%d) generated all-zero key", keySize)
		}

		store.Close()

		// Clean up for next iteration
		os.Remove(dbPath)
	}
}

func TestNewStoreWithRandomKey_InvalidKeySize(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_random_key_store.db")

	invalidKeySizes := []int{0, 8, 15, 17, 20, 31, 33, 64}
	for _, keySize := range invalidKeySizes {
		_, _, err := NewStoreWithRandomKey(dbPath, keySize)
		if err == nil {
			t.Errorf("NewStoreWithRandomKey(%d) should fail with invalid key size", keySize)
		}
	}
}

func TestStore_LogEvent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Log a simple event
	err = store.LogEvent(ctx, "test.event", 1, map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("LogEvent() error = %v", err)
	}
}

func TestStore_LogEvent_WithNilData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Log event with nil data
	err = store.LogEvent(ctx, "test.event", 1, nil)
	if err != nil {
		t.Errorf("LogEvent() with nil data error = %v", err)
	}
}

func TestStore_LogEvent_WithComplexData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Complex data structure
	data := map[string]interface{}{
		"string":  "hello",
		"number":  42,
		"float":   3.14,
		"boolean": true,
		"nested": map[string]interface{}{
			"array": []int{1, 2, 3},
		},
	}

	err = store.LogEvent(ctx, "test.complex", 5, data)
	if err != nil {
		t.Errorf("LogEvent() with complex data error = %v", err)
	}
}

func TestStore_LogEvent_Encrypted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_encrypted_store.db")

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	store, err := NewStoreWithEncryption(dbPath, key)
	if err != nil {
		t.Fatalf("NewStoreWithEncryption() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Log event with encryption
	data := map[string]string{
		"secret": "sensitive-data",
	}
	err = store.LogEvent(ctx, "test.encrypted", 1, data)
	if err != nil {
		t.Errorf("LogEvent() encrypted error = %v", err)
	}
}

func TestStore_LogEvent_MultipleIterations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Log multiple events with different iterations
	for i := 1; i <= 10; i++ {
		err = store.LogEvent(ctx, "test.iteration", i, map[string]int{"iteration": i})
		if err != nil {
			t.Errorf("LogEvent() iteration %d error = %v", i, err)
		}
	}
}

func TestStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	// Close the store
	err = store.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStore_QueryLogs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// QueryLogs returns empty result for now (as per implementation)
	logs, err := store.QueryLogs(ctx, nil, nil)
	if err != nil {
		t.Errorf("QueryLogs() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("QueryLogs() returned %d logs, want 0", len(logs))
	}
}

func TestStore_DatabasePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	// Create store and log an event
	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	ctx := context.Background()
	err = store1.LogEvent(ctx, "test.persistence", 1, map[string]string{"data": "test"})
	if err != nil {
		t.Fatalf("LogEvent() error = %v", err)
	}
	store1.Close()

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Reopen store
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() second open error = %v", err)
	}
	defer store2.Close()
}

func TestStore_EncryptedPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_encrypted_store.db")

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// Create encrypted store and log an event
	store1, err := NewStoreWithEncryption(dbPath, key)
	if err != nil {
		t.Fatalf("NewStoreWithEncryption() error = %v", err)
	}

	ctx := context.Background()
	err = store1.LogEvent(ctx, "test.encrypted.persistence", 1, map[string]string{"secret": "value"})
	if err != nil {
		t.Fatalf("LogEvent() error = %v", err)
	}
	store1.Close()

	// Reopen with same key
	store2, err := NewStoreWithEncryption(dbPath, key)
	if err != nil {
		t.Fatalf("NewStoreWithEncryption() second open error = %v", err)
	}
	defer store2.Close()
}

func TestStore_ConcurrentLogEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_store.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Run concurrent log events
	done := make(chan bool)
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func(iteration int) {
			for j := 0; j < 10; j++ {
				if err := store.LogEvent(ctx, "test.concurrent", iteration, map[string]int{"i": iteration, "j": j}); err != nil {
					errCh <- err
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	close(errCh)
	for err := range errCh {
		t.Errorf("Concurrent LogEvent error: %v", err)
	}
}
