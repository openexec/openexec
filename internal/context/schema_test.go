package context

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema_SQLValidity(t *testing.T) {
	// Create a temporary database to test schema validity
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute the schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Verify tables were created
	tables := []string{
		"context_items",
		"gatherer_configs",
		"context_budgets",
		"gatherer_executions",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s was not created: %v", table, err)
		}
	}
}

func TestSchema_IndexCreation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute the schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Check that indexes were created
	expectedIndexes := []string{
		"idx_context_items_session_id",
		"idx_context_items_type",
		"idx_context_items_priority",
		"idx_context_items_gathered_at",
		"idx_context_items_is_stale",
		"idx_context_items_content_hash",
		"idx_context_items_session_priority",
		"idx_context_items_session_type",
		"idx_gatherer_configs_project_path",
		"idx_gatherer_configs_type",
		"idx_gatherer_configs_is_enabled",
		"idx_context_budgets_project_path",
		"idx_context_budgets_is_default",
		"idx_gatherer_executions_gatherer_id",
		"idx_gatherer_executions_session_id",
		"idx_gatherer_executions_status",
		"idx_gatherer_executions_started_at",
		"idx_gatherer_executions_gatherer_time",
	}

	for _, idx := range expectedIndexes {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&name)
		if err != nil {
			t.Errorf("Index %s was not created: %v", idx, err)
		}
	}
}

func TestSeedSQL_Validity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute the schema first
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Execute the seed SQL
	_, err = db.Exec(SeedSQL)
	if err != nil {
		t.Fatalf("SeedSQL execution failed: %v", err)
	}

	// Verify gatherer configs were seeded
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM gatherer_configs`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count gatherer_configs: %v", err)
	}
	if count == 0 {
		t.Error("SeedSQL should have created gatherer configs")
	}

	// Verify default budget was seeded
	err = db.QueryRow(`SELECT COUNT(*) FROM context_budgets WHERE is_default = 1`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count default budgets: %v", err)
	}
	if count != 1 {
		t.Errorf("SeedSQL should have created exactly 1 default budget, got %d", count)
	}

	// Verify specific gatherers were created
	expectedGatherers := []string{
		"default-project-instructions",
		"default-git-status",
		"default-environment",
		"default-package-info",
		"default-directory-structure",
		"default-recent-files",
		"default-git-diff",
		"default-git-log",
	}

	for _, id := range expectedGatherers {
		var foundID string
		err := db.QueryRow(`SELECT id FROM gatherer_configs WHERE id = ?`, id).Scan(&foundID)
		if err != nil {
			t.Errorf("Gatherer %s was not created: %v", id, err)
		}
	}
}

func TestSeedSQL_Idempotent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema and seed
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}
	_, err = db.Exec(SeedSQL)
	if err != nil {
		t.Fatalf("First SeedSQL execution failed: %v", err)
	}

	// Get initial counts
	var initialGatherers, initialBudgets int
	db.QueryRow(`SELECT COUNT(*) FROM gatherer_configs`).Scan(&initialGatherers)
	db.QueryRow(`SELECT COUNT(*) FROM context_budgets`).Scan(&initialBudgets)

	// Execute seed again
	_, err = db.Exec(SeedSQL)
	if err != nil {
		t.Fatalf("Second SeedSQL execution failed: %v", err)
	}

	// Verify counts haven't changed
	var finalGatherers, finalBudgets int
	db.QueryRow(`SELECT COUNT(*) FROM gatherer_configs`).Scan(&finalGatherers)
	db.QueryRow(`SELECT COUNT(*) FROM context_budgets`).Scan(&finalBudgets)

	if initialGatherers != finalGatherers {
		t.Errorf("SeedSQL is not idempotent for gatherer_configs: %d -> %d", initialGatherers, finalGatherers)
	}
	if initialBudgets != finalBudgets {
		t.Errorf("SeedSQL is not idempotent for context_budgets: %d -> %d", initialBudgets, finalBudgets)
	}
}

func TestCleanupSQL_Validity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Execute cleanup SQL (should not fail even on empty database)
	_, err = db.Exec(CleanupSQL)
	if err != nil {
		t.Fatalf("CleanupSQL execution failed: %v", err)
	}
}

func TestSchema_ContextItemsTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Insert a context item
	_, err = db.Exec(`
		INSERT INTO context_items (id, type, source, content, content_hash, token_count, priority, gathered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, "test-id", "git_status", "git status", "content here", "abc123", 100, 75)
	if err != nil {
		t.Fatalf("Failed to insert context_item: %v", err)
	}

	// Verify the insert
	var id, contextType, source string
	var tokenCount, priority int
	err = db.QueryRow(`SELECT id, type, source, token_count, priority FROM context_items WHERE id = ?`, "test-id").Scan(&id, &contextType, &source, &tokenCount, &priority)
	if err != nil {
		t.Fatalf("Failed to query context_item: %v", err)
	}

	if id != "test-id" {
		t.Errorf("context_item id = %s, want test-id", id)
	}
	if contextType != "git_status" {
		t.Errorf("context_item type = %s, want git_status", contextType)
	}
	if tokenCount != 100 {
		t.Errorf("context_item token_count = %d, want 100", tokenCount)
	}
	if priority != 75 {
		t.Errorf("context_item priority = %d, want 75", priority)
	}
}

func TestSchema_GathererConfigsTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Insert a gatherer config
	_, err = db.Exec(`
		INSERT INTO gatherer_configs (id, type, name, priority, max_tokens, refresh_interval_seconds, is_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "test-gatherer", "git_status", "Test Git Status", 75, 2000, 30, 1)
	if err != nil {
		t.Fatalf("Failed to insert gatherer_config: %v", err)
	}

	// Verify the insert
	var name string
	var maxTokens, refreshInterval int
	var isEnabled bool
	err = db.QueryRow(`SELECT name, max_tokens, refresh_interval_seconds, is_enabled FROM gatherer_configs WHERE id = ?`, "test-gatherer").Scan(&name, &maxTokens, &refreshInterval, &isEnabled)
	if err != nil {
		t.Fatalf("Failed to query gatherer_config: %v", err)
	}

	if name != "Test Git Status" {
		t.Errorf("gatherer_config name = %s, want 'Test Git Status'", name)
	}
	if maxTokens != 2000 {
		t.Errorf("gatherer_config max_tokens = %d, want 2000", maxTokens)
	}
	if refreshInterval != 30 {
		t.Errorf("gatherer_config refresh_interval_seconds = %d, want 30", refreshInterval)
	}
	if !isEnabled {
		t.Error("gatherer_config is_enabled should be true")
	}
}

func TestSchema_ContextBudgetsTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Insert a budget
	_, err = db.Exec(`
		INSERT INTO context_budgets (id, total_token_budget, reserved_for_system_prompt, reserved_for_conversation, min_priority_to_include, is_default)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "test-budget", 128000, 2000, 32000, 10, 1)
	if err != nil {
		t.Fatalf("Failed to insert context_budget: %v", err)
	}

	// Verify the insert
	var totalBudget, systemReserved, convReserved, minPriority int
	var isDefault bool
	err = db.QueryRow(`SELECT total_token_budget, reserved_for_system_prompt, reserved_for_conversation, min_priority_to_include, is_default FROM context_budgets WHERE id = ?`, "test-budget").Scan(&totalBudget, &systemReserved, &convReserved, &minPriority, &isDefault)
	if err != nil {
		t.Fatalf("Failed to query context_budget: %v", err)
	}

	if totalBudget != 128000 {
		t.Errorf("context_budget total_token_budget = %d, want 128000", totalBudget)
	}
	if systemReserved != 2000 {
		t.Errorf("context_budget reserved_for_system_prompt = %d, want 2000", systemReserved)
	}
	if convReserved != 32000 {
		t.Errorf("context_budget reserved_for_conversation = %d, want 32000", convReserved)
	}
	if !isDefault {
		t.Error("context_budget is_default should be true")
	}
}

func TestSchema_GathererExecutionsTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context_schema_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute schema
	_, err = db.Exec(Schema)
	if err != nil {
		t.Fatalf("Schema execution failed: %v", err)
	}

	// Insert a gatherer config first (for foreign key)
	_, err = db.Exec(`
		INSERT INTO gatherer_configs (id, type, name, priority, max_tokens, refresh_interval_seconds, is_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "test-gatherer", "git_status", "Test", 75, 2000, 30, 1)
	if err != nil {
		t.Fatalf("Failed to insert gatherer_config: %v", err)
	}

	// Insert an execution
	_, err = db.Exec(`
		INSERT INTO gatherer_executions (id, gatherer_id, status, tokens_gathered, duration_ms, started_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
	`, "test-exec", "test-gatherer", "completed", 1500, 250)
	if err != nil {
		t.Fatalf("Failed to insert gatherer_execution: %v", err)
	}

	// Verify the insert
	var status string
	var tokensGathered, durationMs int
	err = db.QueryRow(`SELECT status, tokens_gathered, duration_ms FROM gatherer_executions WHERE id = ?`, "test-exec").Scan(&status, &tokensGathered, &durationMs)
	if err != nil {
		t.Fatalf("Failed to query gatherer_execution: %v", err)
	}

	if status != "completed" {
		t.Errorf("gatherer_execution status = %s, want completed", status)
	}
	if tokensGathered != 1500 {
		t.Errorf("gatherer_execution tokens_gathered = %d, want 1500", tokensGathered)
	}
	if durationMs != 250 {
		t.Errorf("gatherer_execution duration_ms = %d, want 250", durationMs)
	}
}

func TestSchema_ContainsExpectedStatements(t *testing.T) {
	// Verify Schema contains key SQL statements
	requiredPhrases := []string{
		"CREATE TABLE IF NOT EXISTS context_items",
		"CREATE TABLE IF NOT EXISTS gatherer_configs",
		"CREATE TABLE IF NOT EXISTS context_budgets",
		"CREATE TABLE IF NOT EXISTS gatherer_executions",
		"CREATE INDEX IF NOT EXISTS",
		"FOREIGN KEY",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(Schema, phrase) {
			t.Errorf("Schema should contain '%s'", phrase)
		}
	}
}

func TestSeedSQL_ContainsExpectedGatherers(t *testing.T) {
	// Verify SeedSQL contains expected gatherer types
	expectedTypes := []string{
		"project_instructions",
		"git_status",
		"environment",
		"package_info",
		"directory_structure",
		"recent_files",
		"git_diff",
		"git_log",
	}

	for _, contextType := range expectedTypes {
		if !strings.Contains(SeedSQL, contextType) {
			t.Errorf("SeedSQL should contain gatherer for '%s'", contextType)
		}
	}
}

func TestCleanupSQL_ContainsExpectedOperations(t *testing.T) {
	// Verify CleanupSQL contains expected cleanup operations
	expectedOperations := []string{
		"UPDATE context_items",
		"DELETE FROM context_items",
		"DELETE FROM gatherer_executions",
		"is_stale",
		"expires_at",
	}

	for _, op := range expectedOperations {
		if !strings.Contains(CleanupSQL, op) {
			t.Errorf("CleanupSQL should contain '%s'", op)
		}
	}
}
