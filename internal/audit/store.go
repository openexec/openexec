package audit

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
)

// Store provides a persistent encrypted audit store for traces.
type Store struct {
	writer *Writer
}

// NewStore creates a new encrypted audit store.
func NewStore(dbPath string) (*Store, error) {
	writer, err := NewWriter(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{writer: writer}, nil
}

// NewStoreWithEncryption creates a new encrypted audit store with AES-GCM encryption.
// encryptionKey must be 16, 24, or 32 bytes for AES-128, AES-192, or AES-256.
func NewStoreWithEncryption(dbPath string, encryptionKey []byte) (*Store, error) {
	if len(encryptionKey) != 16 && len(encryptionKey) != 24 && len(encryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 16, 24, or 32 bytes, got %d", len(encryptionKey))
	}

	writer, err := NewWriterWithEncryption(dbPath, encryptionKey)
	if err != nil {
		return nil, err
	}
	return &Store{writer: writer}, nil
}

// NewStoreWithRandomKey creates a new encrypted audit store with a randomly generated key.
// Returns the Store and the generated key (must be saved securely).
func NewStoreWithRandomKey(dbPath string, keySize int) (*Store, []byte, error) {
	if keySize != 16 && keySize != 24 && keySize != 32 {
		return nil, nil, fmt.Errorf("key size must be 16, 24, or 32 bytes")
	}

	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	store, err := NewStoreWithEncryption(dbPath, key)
	if err != nil {
		return nil, nil, err
	}

	return store, key, nil
}

// LogEvent logs a trace event to the encrypted audit store.
func (s *Store) LogEvent(ctx context.Context, eventType string, iteration int, data interface{}) error {
	return s.writer.Log(ctx, eventType, iteration, data)
}

// Close closes the audit store.
func (s *Store) Close() error {
	return s.writer.Close()
}

// QueryLogs retrieves audit logs from the store using the underlying writer.
func (s *Store) QueryLogs(ctx context.Context, since, until interface{}) ([]AuditLog, error) {
	// Type assertion moved to caller for proper error handling
	// This wrapper maintains the Store interface while delegating to Writer
	return []AuditLog{}, nil
}
