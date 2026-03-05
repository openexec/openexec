package audit

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Writer provides SQLite-based audit logging with optional encryption.
type Writer struct {
	db            *sql.DB
	mu            sync.Mutex
	encryptionKey []byte
	useEncryption bool
}

// NewWriter creates and initializes a new audit Writer.
func NewWriter(dbPath string) (*Writer, error) {
	return NewWriterWithEncryption(dbPath, nil)
}

// NewWriterWithEncryption creates and initializes a new audit Writer with encryption.
// If encryptionKey is nil, the writer will operate without encryption.
// The encryptionKey must be 16, 24, or 32 bytes for AES-128, AES-192, or AES-256.
func NewWriterWithEncryption(dbPath string, encryptionKey []byte) (*Writer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	w := &Writer{
		db:            db,
		useEncryption: encryptionKey != nil,
		encryptionKey: encryptionKey,
	}

	// Initialize schema
	if err := w.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return w, nil
}

// initSchema creates the audit tables if they don't exist.
func (w *Writer) initSchema() error {
	if _, err := w.db.Exec(Schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// Log writes an audit entry with the given event type, iteration, and data.
// If encryption is enabled, data is encrypted before storage.
func (w *Writer) Log(ctx context.Context, eventType string, iteration int, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Serialize data to JSON
	var dataStr string
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal data: %w", err)
		}
		dataStr = string(jsonData)
	}

	if w.useEncryption {
		return w.logEncrypted(ctx, eventType, iteration, dataStr)
	}

	query := `
		INSERT INTO audit_logs (timestamp, event_type, iteration, data)
		VALUES (?, ?, ?, ?)
	`

	_, err := w.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), eventType, iteration, dataStr)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}

// logEncrypted writes an encrypted audit entry (append-only).
func (w *Writer) logEncrypted(ctx context.Context, eventType string, iteration int, data string) error {
	// Encrypt the data
	encryptedData, iv, err := w.encryptData([]byte(data))
	if err != nil {
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Compute hash of original data for integrity verification
	hash := sha256.Sum256([]byte(data))
	hashStr := hex.EncodeToString(hash[:])

	query := `
		INSERT INTO encrypted_audit_logs (timestamp, event_type, iteration, encrypted_data, iv, hash)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = w.db.ExecContext(
		ctx,
		query,
		time.Now().UTC().Format(time.RFC3339),
		eventType,
		iteration,
		encryptedData,
		iv,
		hashStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert encrypted audit log: %w", err)
	}

	return nil
}

// encryptData encrypts data using AES-GCM.
func (w *Writer) encryptData(plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(w.encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// decryptData decrypts data using AES-GCM.
func (w *Writer) decryptData(ciphertext []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(w.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// Close closes the database connection.
func (w *Writer) Close() error {
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// GetDB returns the underlying sql.DB connection.
func (w *Writer) GetDB() *sql.DB {
	return w.db
}

// QueryLogs retrieves audit logs within a time range.
// For encrypted logs, this returns the encrypted data (caller must decrypt).
func (w *Writer) QueryLogs(ctx context.Context, since, until time.Time) ([]AuditLog, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	table := "audit_logs"
	if w.useEncryption {
		table = "encrypted_audit_logs"
	}

	query := `
		SELECT id, timestamp, event_type, iteration, data
		FROM ` + table + `
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY id DESC
	`

	rows, err := w.db.QueryContext(ctx, query, since.Format(time.RFC3339), until.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var data sql.NullString
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.EventType, &log.Iteration, &data); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if data.Valid {
			log.Data = data.String
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// QueryEncryptedLogs retrieves encrypted audit logs and decrypts them.
// Returns both encrypted and decrypted data for integrity verification.
func (w *Writer) QueryEncryptedLogs(ctx context.Context, since, until time.Time) ([]EncryptedAuditLog, error) {
	if !w.useEncryption {
		return nil, fmt.Errorf("encryption not enabled on this writer")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	query := `
		SELECT id, timestamp, event_type, iteration, encrypted_data, iv, hash
		FROM encrypted_audit_logs
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY id DESC
	`

	rows, err := w.db.QueryContext(ctx, query, since.Format(time.RFC3339), until.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query encrypted logs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var logs []EncryptedAuditLog
	for rows.Next() {
		var log EncryptedAuditLog
		var iv []byte
		var hash sql.NullString
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.EventType, &log.Iteration, &log.EncryptedData, &iv, &hash); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Decrypt the data
		decrypted, err := w.decryptData(log.EncryptedData, iv)
		if err != nil {
			log.DecryptionError = err.Error()
		} else {
			log.DecryptedData = string(decrypted)
		}

		if hash.Valid {
			log.Hash = hash.String
		}

		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// AuditLog represents a single audit log entry.
type AuditLog struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	Iteration int    `json:"iteration"`
	Data      string `json:"data,omitempty"`
}

// EncryptedAuditLog represents an encrypted audit log entry with integrity verification.
type EncryptedAuditLog struct {
	ID              int    `json:"id"`
	Timestamp       string `json:"timestamp"`
	EventType       string `json:"event_type"`
	Iteration       int    `json:"iteration"`
	EncryptedData   []byte `json:"encrypted_data,omitempty"`
	DecryptedData   string `json:"decrypted_data,omitempty"`
	Hash            string `json:"hash,omitempty"`
	DecryptionError string `json:"decryption_error,omitempty"`
}
