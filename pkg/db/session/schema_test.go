package session

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema_ApplySuccessfully(t *testing.T) {
	// Create an in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Apply the schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Verify tables were created by querying sqlite_master
	tables := []string{"sessions", "messages", "tool_calls", "session_summaries"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s was not created: %v", table, err)
		}
	}
}

func TestSchema_Idempotent(t *testing.T) {
	// Create an in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Apply the schema twice - should not error due to IF NOT EXISTS
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema first time: %v", err)
	}

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema second time (not idempotent): %v", err)
	}
}

func TestSchema_SessionsTableColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Verify sessions table has expected columns
	rows, err := db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		t.Fatalf("Failed to query sessions table info: %v", err)
	}
	defer rows.Close()

	expectedColumns := map[string]bool{
		"id":                    false,
		"project_path":          false,
		"provider":              false,
		"model":                 false,
		"title":                 false,
		"parent_session_id":     false,
		"fork_point_message_id": false,
		"status":                false,
		"created_at":            false,
		"updated_at":            false,
	}

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		if _, ok := expectedColumns[name]; ok {
			expectedColumns[name] = true
		}
	}

	for col, found := range expectedColumns {
		if !found {
			t.Errorf("Expected column %s not found in sessions table", col)
		}
	}
}

func TestSchema_MessagesTableColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Verify messages table has expected columns
	rows, err := db.Query("PRAGMA table_info(messages)")
	if err != nil {
		t.Fatalf("Failed to query messages table info: %v", err)
	}
	defer rows.Close()

	expectedColumns := map[string]bool{
		"id":            false,
		"session_id":    false,
		"role":          false,
		"content":       false,
		"tokens_input":  false,
		"tokens_output": false,
		"cost_usd":      false,
		"created_at":    false,
	}

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		if _, ok := expectedColumns[name]; ok {
			expectedColumns[name] = true
		}
	}

	for col, found := range expectedColumns {
		if !found {
			t.Errorf("Expected column %s not found in messages table", col)
		}
	}
}

func TestSchema_ToolCallsTableColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Verify tool_calls table has expected columns
	rows, err := db.Query("PRAGMA table_info(tool_calls)")
	if err != nil {
		t.Fatalf("Failed to query tool_calls table info: %v", err)
	}
	defer rows.Close()

	expectedColumns := map[string]bool{
		"id":              false,
		"message_id":      false,
		"session_id":      false,
		"tool_name":       false,
		"tool_input":      false,
		"tool_output":     false,
		"status":          false,
		"approval_status": false,
		"approved_by":     false,
		"approved_at":     false,
		"started_at":      false,
		"completed_at":    false,
		"error":           false,
		"created_at":      false,
	}

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		if _, ok := expectedColumns[name]; ok {
			expectedColumns[name] = true
		}
	}

	for col, found := range expectedColumns {
		if !found {
			t.Errorf("Expected column %s not found in tool_calls table", col)
		}
	}
}

func TestSchema_ForeignKeyConstraints(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Create a session first
	_, err = db.Exec(`INSERT INTO sessions (id, project_path, provider, model, status)
		VALUES ('session-1', '/path', 'openai', 'gpt-4', 'active')`)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	// Insert a message for that session
	_, err = db.Exec(`INSERT INTO messages (id, session_id, role, content)
		VALUES ('msg-1', 'session-1', 'user', 'Hello')`)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	// Insert a tool call for that message
	_, err = db.Exec(`INSERT INTO tool_calls (id, message_id, session_id, tool_name, tool_input, status)
		VALUES ('tc-1', 'msg-1', 'session-1', 'read_file', '{}', 'pending')`)
	if err != nil {
		t.Fatalf("Failed to insert tool call: %v", err)
	}

	// Try to insert a message with non-existent session (should fail with foreign keys)
	_, err = db.Exec(`INSERT INTO messages (id, session_id, role, content)
		VALUES ('msg-2', 'non-existent', 'user', 'Hello')`)
	if err == nil {
		t.Error("Expected foreign key constraint violation, but insert succeeded")
	}
}

func TestSchema_CascadeDelete(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Create a session, message, and tool call
	_, err = db.Exec(`INSERT INTO sessions (id, project_path, provider, model, status)
		VALUES ('session-1', '/path', 'openai', 'gpt-4', 'active')`)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	_, err = db.Exec(`INSERT INTO messages (id, session_id, role, content)
		VALUES ('msg-1', 'session-1', 'user', 'Hello')`)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	_, err = db.Exec(`INSERT INTO tool_calls (id, message_id, session_id, tool_name, tool_input, status)
		VALUES ('tc-1', 'msg-1', 'session-1', 'read_file', '{}', 'pending')`)
	if err != nil {
		t.Fatalf("Failed to insert tool call: %v", err)
	}

	// Delete the session
	_, err = db.Exec(`DELETE FROM sessions WHERE id = 'session-1'`)
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Verify messages and tool_calls were cascade deleted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = 'session-1'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count messages: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 messages after cascade delete, got %d", count)
	}

	err = db.QueryRow("SELECT COUNT(*) FROM tool_calls WHERE session_id = 'session-1'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count tool_calls: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 tool_calls after cascade delete, got %d", count)
	}
}

func TestSchema_IndexesCreated(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Query for indexes
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'")
	if err != nil {
		t.Fatalf("Failed to query indexes: %v", err)
	}
	defer rows.Close()

	expectedIndexes := map[string]bool{
		"idx_sessions_project_path":        false,
		"idx_sessions_status":              false,
		"idx_sessions_created_at":          false,
		"idx_sessions_parent":              false,
		"idx_messages_session_id":          false,
		"idx_messages_created_at":          false,
		"idx_messages_role":                false,
		"idx_tool_calls_session_id":        false,
		"idx_tool_calls_message_id":        false,
		"idx_tool_calls_status":            false,
		"idx_tool_calls_tool_name":         false,
		"idx_session_summaries_session_id": false,
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan index name: %v", err)
		}
		if _, ok := expectedIndexes[name]; ok {
			expectedIndexes[name] = true
		}
	}

	for idx, found := range expectedIndexes {
		if !found {
			t.Errorf("Expected index %s not found", idx)
		}
	}
}
