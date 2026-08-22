package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrExternalConnectionNotFound = errors.New("external connection not found")
	ErrExternalConnectionDisabled = errors.New("external connection is disabled")
)

type ExternalConnection struct {
	ID                   string                      `json:"id"`
	Name                 string                      `json:"name"`
	Provider             string                      `json:"provider"`
	ServerURL            string                      `json:"server_url"`
	CredentialRef        string                      `json:"credential_ref"`
	CredentialCiphertext []byte                      `json:"-"`
	Status               string                      `json:"status"`
	Identity             json.RawMessage             `json:"identity"`
	CatalogDigest        string                      `json:"catalog_digest"`
	ToolCount            int                         `json:"tool_count"`
	ProtocolVersion      string                      `json:"protocol_version"`
	ServerName           string                      `json:"server_name"`
	ServerVersion        string                      `json:"server_version"`
	LastHealthError      string                      `json:"last_health_error,omitempty"`
	LastCheckedAt        *time.Time                  `json:"last_checked_at,omitempty"`
	CreatedAt            time.Time                   `json:"created_at"`
	UpdatedAt            time.Time                   `json:"updated_at"`
	Bindings             []ExternalConnectionBinding `json:"bindings,omitempty"`
}

type ExternalConnectionBinding struct {
	ConnectionID   string    `json:"connection_id"`
	ProjectRef     string    `json:"project_ref"`
	AllowedEffects []string  `json:"allowed_effects"`
	AllowedTools   []string  `json:"allowed_tools"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ExternalCatalogSnapshot struct {
	ID              string          `json:"id"`
	ConnectionID    string          `json:"connection_id"`
	Digest          string          `json:"digest"`
	ProtocolVersion string          `json:"protocol_version"`
	ServerName      string          `json:"server_name"`
	ServerVersion   string          `json:"server_version"`
	Tools           json.RawMessage `json:"tools"`
	CreatedAt       time.Time       `json:"created_at"`
}

type ExternalInvocation struct {
	ID            string     `json:"id"`
	ConnectionID  string     `json:"connection_id"`
	ProjectRef    string     `json:"project_ref"`
	ToolName      string     `json:"tool_name"`
	CatalogDigest string     `json:"catalog_digest"`
	Effect        string     `json:"effect"`
	Status        string     `json:"status"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

func (s *Store) CreateExternalConnection(ctx context.Context, connection ExternalConnection, binding ExternalConnectionBinding) error {
	if connection.ID == "" || binding.ConnectionID != connection.ID || binding.ProjectRef == "" {
		return errors.New("external connection id and project binding are required")
	}
	identity := connection.Identity
	if len(identity) == 0 {
		identity = json.RawMessage(`{}`)
	}
	effects, err := json.Marshal(binding.AllowedEffects)
	if err != nil {
		return fmt.Errorf("encode allowed effects: %w", err)
	}
	tools, err := json.Marshal(binding.AllowedTools)
	if err != nil {
		return fmt.Errorf("encode allowed tools: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO external_connections
		(id, name, provider, server_url, credential_ref, credential_ciphertext, status, identity_json,
		 catalog_digest, tool_count, protocol_version, server_name, server_version, last_health_error,
		 last_checked_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		connection.ID, connection.Name, connection.Provider, connection.ServerURL, connection.CredentialRef,
		connection.CredentialCiphertext, connection.Status, string(identity), connection.CatalogDigest,
		connection.ToolCount, connection.ProtocolVersion, connection.ServerName, connection.ServerVersion,
		connection.LastHealthError, timeValue(connection.LastCheckedAt), connection.CreatedAt.Format(time.RFC3339Nano),
		connection.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create external connection: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO external_connection_bindings
		(connection_id, project_ref, allowed_effects, allowed_tools, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, connection.ID, binding.ProjectRef, string(effects), string(tools),
		binding.CreatedAt.Format(time.RFC3339Nano), binding.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create external connection binding: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ListExternalConnections(ctx context.Context, projectRef string) ([]ExternalConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.provider, c.server_url, c.credential_ref,
		c.status, c.identity_json, c.catalog_digest, c.tool_count, c.protocol_version, c.server_name,
		c.server_version, c.last_health_error, c.last_checked_at, c.created_at, c.updated_at,
		b.connection_id, b.project_ref, b.allowed_effects, b.allowed_tools, b.created_at, b.updated_at
		FROM external_connections c JOIN external_connection_bindings b ON b.connection_id = c.id
		WHERE b.project_ref = ? ORDER BY c.created_at ASC`, projectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExternalConnection
	for rows.Next() {
		var connection ExternalConnection
		var identityJSON, createdAt, updatedAt string
		var checkedAt sql.NullString
		var binding ExternalConnectionBinding
		var effectsJSON, toolsJSON, bindingCreatedAt, bindingUpdatedAt string
		err := rows.Scan(&connection.ID, &connection.Name, &connection.Provider, &connection.ServerURL,
			&connection.CredentialRef, &connection.Status, &identityJSON, &connection.CatalogDigest,
			&connection.ToolCount, &connection.ProtocolVersion, &connection.ServerName, &connection.ServerVersion,
			&connection.LastHealthError, &checkedAt, &createdAt, &updatedAt, &binding.ConnectionID,
			&binding.ProjectRef, &effectsJSON, &toolsJSON, &bindingCreatedAt, &bindingUpdatedAt)
		if err != nil {
			return nil, err
		}
		connection.Identity = json.RawMessage(identityJSON)
		if connection.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		if connection.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, err
		}
		if checkedAt.Valid {
			parsed, parseErr := time.Parse(time.RFC3339Nano, checkedAt.String)
			if parseErr != nil {
				return nil, parseErr
			}
			connection.LastCheckedAt = &parsed
		}
		if err = json.Unmarshal([]byte(effectsJSON), &binding.AllowedEffects); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(toolsJSON), &binding.AllowedTools); err != nil {
			return nil, err
		}
		if binding.CreatedAt, err = time.Parse(time.RFC3339Nano, bindingCreatedAt); err != nil {
			return nil, err
		}
		if binding.UpdatedAt, err = time.Parse(time.RFC3339Nano, bindingUpdatedAt); err != nil {
			return nil, err
		}
		connection.Bindings = []ExternalConnectionBinding{binding}
		result = append(result, connection)
	}
	return result, rows.Err()
}

func (s *Store) GetExternalConnection(ctx context.Context, id string) (ExternalConnection, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, provider, server_url, credential_ref, status,
		identity_json, catalog_digest, tool_count, protocol_version, server_name, server_version,
		last_health_error, last_checked_at, created_at, updated_at FROM external_connections WHERE id = ?`, id)
	connection, err := scanExternalConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ExternalConnection{}, ErrExternalConnectionNotFound
	}
	return connection, err
}

func (s *Store) GetExternalConnectionCredential(ctx context.Context, id string) ([]byte, error) {
	var ciphertext []byte
	err := s.db.QueryRowContext(ctx, `SELECT credential_ciphertext FROM external_connections WHERE id = ?`, id).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExternalConnectionNotFound
	}
	return ciphertext, err
}

func (s *Store) GetExternalConnectionBinding(ctx context.Context, connectionID, projectRef string) (ExternalConnectionBinding, error) {
	var binding ExternalConnectionBinding
	var effectsJSON, toolsJSON, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT connection_id, project_ref, allowed_effects, allowed_tools,
		created_at, updated_at FROM external_connection_bindings WHERE connection_id = ? AND project_ref = ?`,
		connectionID, projectRef).Scan(&binding.ConnectionID, &binding.ProjectRef, &effectsJSON, &toolsJSON, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExternalConnectionBinding{}, ErrExternalConnectionNotFound
	}
	if err != nil {
		return ExternalConnectionBinding{}, err
	}
	if err := json.Unmarshal([]byte(effectsJSON), &binding.AllowedEffects); err != nil {
		return ExternalConnectionBinding{}, err
	}
	if err := json.Unmarshal([]byte(toolsJSON), &binding.AllowedTools); err != nil {
		return ExternalConnectionBinding{}, err
	}
	binding.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ExternalConnectionBinding{}, err
	}
	binding.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return binding, err
}

func (s *Store) UpdateExternalConnectionCredential(ctx context.Context, id string, ciphertext []byte, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE external_connections SET credential_ciphertext = ?, updated_at = ? WHERE id = ?`,
		ciphertext, now.Format(time.RFC3339Nano), id)
	return externalUpdateResult(result, err)
}

func (s *Store) SetExternalConnectionStatus(ctx context.Context, id, status, healthError string, checkedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE external_connections SET status = ?, last_health_error = ?,
		last_checked_at = ?, updated_at = ? WHERE id = ?`, status, healthError, checkedAt.Format(time.RFC3339Nano),
		checkedAt.Format(time.RFC3339Nano), id)
	return externalUpdateResult(result, err)
}

func (s *Store) RecordExternalCatalog(ctx context.Context, connectionID string, snapshot ExternalCatalogSnapshot, identity json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO external_catalog_snapshots
		(id, connection_id, digest, protocol_version, server_name, server_version, tools_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, connectionID, snapshot.Digest, snapshot.ProtocolVersion,
		snapshot.ServerName, snapshot.ServerVersion, string(snapshot.Tools), snapshot.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE external_connections SET status = 'healthy', identity_json = ?,
		catalog_digest = ?, tool_count = ?, protocol_version = ?, server_name = ?, server_version = ?,
		last_health_error = '', last_checked_at = ?, updated_at = ? WHERE id = ? AND status <> 'disabled'`, string(identity), snapshot.Digest,
		toolCount(snapshot.Tools), snapshot.ProtocolVersion, snapshot.ServerName, snapshot.ServerVersion,
		snapshot.CreatedAt.Format(time.RFC3339Nano), snapshot.CreatedAt.Format(time.RFC3339Nano), connectionID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrExternalConnectionDisabled
	}
	return tx.Commit()
}

func (s *Store) RecordExternalInvocation(ctx context.Context, invocation ExternalInvocation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO external_invocations
		(id, connection_id, project_ref, tool_name, catalog_digest, effect, status, error_message, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, invocation.ID, invocation.ConnectionID, invocation.ProjectRef,
		invocation.ToolName, invocation.CatalogDigest, invocation.Effect, invocation.Status, invocation.ErrorMessage,
		invocation.StartedAt.Format(time.RFC3339Nano), timeValue(invocation.CompletedAt))
	return err
}

func scanExternalConnection(scanner interface{ Scan(...any) error }) (ExternalConnection, error) {
	var connection ExternalConnection
	var identityJSON, createdAt, updatedAt string
	var checkedAt sql.NullString
	err := scanner.Scan(&connection.ID, &connection.Name, &connection.Provider, &connection.ServerURL,
		&connection.CredentialRef, &connection.Status, &identityJSON, &connection.CatalogDigest, &connection.ToolCount,
		&connection.ProtocolVersion, &connection.ServerName, &connection.ServerVersion, &connection.LastHealthError,
		&checkedAt, &createdAt, &updatedAt)
	if err != nil {
		return ExternalConnection{}, err
	}
	connection.Identity = json.RawMessage(identityJSON)
	connection.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ExternalConnection{}, err
	}
	connection.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ExternalConnection{}, err
	}
	if checkedAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, checkedAt.String)
		if parseErr != nil {
			return ExternalConnection{}, parseErr
		}
		connection.LastCheckedAt = &parsed
	}
	return connection, nil
}

func toolCount(raw json.RawMessage) int {
	var tools []json.RawMessage
	if json.Unmarshal(raw, &tools) != nil {
		return 0
	}
	return len(tools)
}

func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}

func externalUpdateResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrExternalConnectionNotFound
	}
	return nil
}
