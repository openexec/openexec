package audit

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema_ValidSQL(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "audit_schema_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Execute the legacy Schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Errorf("Schema SQL execution failed: %v", err)
	}

	// Verify legacy tables were created
	tables := []string{"audit_logs", "encrypted_audit_logs"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s was not created: %v", table, err)
		}
	}
}

func TestEntrySchema_ValidSQL(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "audit_entry_schema_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Execute the EntrySchema
	_, err = db.Exec(EntrySchema)
	if err != nil {
		t.Errorf("EntrySchema SQL execution failed: %v", err)
	}

	// Verify tables were created
	tables := []string{"audit_entries", "encrypted_audit_entries"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s was not created: %v", table, err)
		}
	}

	// Verify audit_entries columns
	expectedColumns := []string{
		"id", "timestamp", "event_type", "severity", "session_id",
		"message_id", "tool_call_id", "actor_id", "actor_type",
		"project_path", "provider", "model", "tokens_input",
		"tokens_output", "cost_usd", "duration_ms", "success",
		"error_message", "metadata", "iteration", "ip_address",
		"user_agent", "created_at",
	}

	rows, err := db.Query("PRAGMA table_info(audit_entries)")
	if err != nil {
		t.Fatalf("Failed to get table info: %v", err)
	}
	defer rows.Close()

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, dfltValue, pk interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		foundColumns[name] = true
	}

	for _, col := range expectedColumns {
		if !foundColumns[col] {
			t.Errorf("Column %s not found in audit_entries table", col)
		}
	}
}

func TestEntrySchema_InsertAndQuery(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "audit_entry_insert_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Execute the EntrySchema
	_, err = db.Exec(EntrySchema)
	if err != nil {
		t.Fatalf("EntrySchema SQL execution failed: %v", err)
	}

	// Create an audit entry using the builder
	builder, err := NewEntry(EventSessionCreated, "user-123", "user")
	if err != nil {
		t.Fatalf("NewEntry() error: %v", err)
	}

	entry, err := builder.
		WithSession("session-456").
		WithProject("/path/to/project").
		WithProvider("openai", "gpt-4").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Insert the entry
	_, err = db.Exec(`
		INSERT INTO audit_entries (
			id, timestamp, event_type, severity, session_id, actor_id, actor_type,
			project_path, provider, model, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ID,
		entry.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		entry.EventType.String(),
		entry.Severity.String(),
		entry.SessionID.String,
		entry.ActorID,
		entry.ActorType,
		entry.ProjectPath.String,
		entry.Provider.String,
		entry.Model.String,
		entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	)
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Query the entry back
	var (
		id, eventType, severity, sessionID, actorID, actorType string
		projectPath, provider, model                           string
	)
	err = db.QueryRow(`
		SELECT id, event_type, severity, session_id, actor_id, actor_type,
			   project_path, provider, model
		FROM audit_entries WHERE id = ?
	`, entry.ID).Scan(
		&id, &eventType, &severity, &sessionID, &actorID, &actorType,
		&projectPath, &provider, &model,
	)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}

	// Verify values
	if id != entry.ID {
		t.Errorf("id = %v, want %v", id, entry.ID)
	}
	if eventType != "session.created" {
		t.Errorf("event_type = %v, want session.created", eventType)
	}
	if severity != "info" {
		t.Errorf("severity = %v, want info", severity)
	}
	if sessionID != "session-456" {
		t.Errorf("session_id = %v, want session-456", sessionID)
	}
	if actorID != "user-123" {
		t.Errorf("actor_id = %v, want user-123", actorID)
	}
	if actorType != "user" {
		t.Errorf("actor_type = %v, want user", actorType)
	}
	if projectPath != "/path/to/project" {
		t.Errorf("project_path = %v, want /path/to/project", projectPath)
	}
	if provider != "openai" {
		t.Errorf("provider = %v, want openai", provider)
	}
	if model != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", model)
	}
}

func TestMigrationSQL_ValidSQL(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "audit_migration_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Execute both legacy Schema and EntrySchema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema SQL execution failed: %v", err)
	}

	_, err = db.Exec(EntrySchema)
	if err != nil {
		t.Fatalf("EntrySchema SQL execution failed: %v", err)
	}

	// Insert some legacy data
	_, err = db.Exec(`
		INSERT INTO audit_logs (timestamp, event_type, iteration, data)
		VALUES ('2024-01-15T10:30:00Z', 'test.event', 1, '{"key": "value"}')
	`)
	if err != nil {
		t.Fatalf("INSERT into audit_logs failed: %v", err)
	}

	// Execute migration
	_, err = db.Exec(MigrationSQL)
	if err != nil {
		t.Fatalf("MigrationSQL execution failed: %v", err)
	}

	// Verify data was migrated
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM audit_entries WHERE event_type = 'test.event'`).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated entry, got %d", count)
	}

	// Verify migrated entry has correct ID prefix
	var id string
	err = db.QueryRow(`SELECT id FROM audit_entries WHERE event_type = 'test.event'`).Scan(&id)
	if err != nil {
		t.Fatalf("SELECT id failed: %v", err)
	}
	if id != "legacy-1" {
		t.Errorf("Expected id 'legacy-1', got %s", id)
	}
}

func TestSchema_IndexesCreated(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "audit_index_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Execute the EntrySchema
	_, err = db.Exec(EntrySchema)
	if err != nil {
		t.Fatalf("EntrySchema SQL execution failed: %v", err)
	}

	// Verify indexes were created
	expectedIndexes := []string{
		"idx_audit_entries_timestamp",
		"idx_audit_entries_event_type",
		"idx_audit_entries_severity",
		"idx_audit_entries_session_id",
		"idx_audit_entries_actor_id",
		"idx_audit_entries_project_path",
		"idx_audit_entries_provider",
		"idx_audit_entries_event_time",
		"idx_audit_entries_session_time",
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index'")
	if err != nil {
		t.Fatalf("Failed to query indexes: %v", err)
	}
	defer rows.Close()

	foundIndexes := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan index name: %v", err)
		}
		foundIndexes[name] = true
	}

	for _, idx := range expectedIndexes {
		if !foundIndexes[idx] {
			t.Errorf("Index %s not found", idx)
		}
	}
}
