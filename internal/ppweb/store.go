package ppweb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/vakaka1/pp/internal/config"
)

const sqliteDriverName = "sqlite3"

type Store struct {
	db *sql.DB
}

func OpenStore(databasePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open(sqliteDriverName, databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			admin_id INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			FOREIGN KEY(admin_id) REFERENCES admins(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS connections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			tag TEXT NOT NULL UNIQUE,
			protocol TEXT NOT NULL,
			listen TEXT NOT NULL,
			tls_json TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			settings_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_connections_protocol ON connections(protocol);`,
		`CREATE TABLE IF NOT EXISTS clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			connection_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			psk TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(connection_id) REFERENCES connections(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_clients_connection_id ON clients(connection_id);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to run migration: %w", err)
		}
	}

	// Add missing columns to existing tables
	s.ensureColumn(ctx, "connections", "tls_json", "TEXT")
	s.ensureColumn(ctx, "clients", "psk", "TEXT NOT NULL DEFAULT ''")

	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, columnType string) {
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType)
	_, _ = s.db.ExecContext(ctx, query)
}

func (s *Store) HasSetup(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM admins`).Scan(&count); err != nil {
		return false, fmt.Errorf("failed to check setup state: %w", err)
	}
	return count > 0, nil
}

func (s *Store) CreateInitialAdmin(ctx context.Context, appName, username, passwordHash, coreConfigPath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin setup transaction: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM admins`).Scan(&count); err != nil {
		return fmt.Errorf("failed to inspect admins: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("setup has already been completed")
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO admins(username, password_hash, created_at) VALUES(?, ?, ?)`,
		username,
		passwordHash,
		now.Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("failed to create initial admin: %w", err)
	}

	settings := map[string]string{
		"app_name":         appName,
		"core_config_path": coreConfigPath,
		"initialized_at":   now.Format(time.RFC3339),
	}

	for key, value := range settings {
		if err := upsertSettingTx(ctx, tx, key, value); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit setup transaction: %w", err)
	}

	return nil
}

func (s *Store) FindAdminByUsername(ctx context.Context, username string) (*Admin, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, created_at FROM admins WHERE lower(username) = lower(?) LIMIT 1`,
		username,
	)

	var (
		admin     Admin
		createdAt string
	)
	if err := row.Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}

	admin.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &admin, nil
}

func (s *Store) CreateSession(ctx context.Context, adminID int64, ttl time.Duration) (*Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &Session{
		ID:        token,
		AdminID:   adminID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions(id, admin_id, created_at, expires_at) VALUES(?, ?, ?, ?)`,
		session.ID,
		session.AdminID,
		session.CreatedAt.Format(time.RFC3339),
		session.ExpiresAt.Format(time.RFC3339),
	); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func (s *Store) CleanupExpiredSessions(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}
	return nil
}

func (s *Store) FindAdminBySession(ctx context.Context, sessionID string) (*Admin, error) {
	if sessionID == "" {
		return nil, nil
	}

	if err := s.CleanupExpiredSessions(ctx); err != nil {
		return nil, err
	}

	row := s.db.QueryRowContext(
		ctx,
		`SELECT a.id, a.username, a.password_hash, a.created_at
		 FROM sessions sess
		 JOIN admins a ON a.id = sess.admin_id
		 WHERE sess.id = ? AND sess.expires_at > ?
		 LIMIT 1`,
		sessionID,
		time.Now().UTC().Format(time.RFC3339),
	)

	var (
		admin     Admin
		createdAt string
	)
	if err := row.Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session admin: %w", err)
	}

	admin.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &admin, nil
}

func (s *Store) GetAppSettings(ctx context.Context, fallbackCoreConfigPath string) (*AppSettings, error) {
	settingsMap, err := s.getSettingsMap(ctx)
	if err != nil {
		return nil, err
	}

	settings := &AppSettings{
		AppName:        firstNonEmpty(settingsMap["app_name"], "PP Web"),
		CoreConfigPath: firstNonEmpty(settingsMap["core_config_path"], fallbackCoreConfigPath),
		LastSyncError:  settingsMap["last_sync_error"],
	}

	if settingsMap["initialized_at"] != "" {
		settings.InitializedAt, _ = time.Parse(time.RFC3339, settingsMap["initialized_at"])
	}
	if settingsMap["last_sync_at"] != "" {
		settings.LastSyncAt, _ = time.Parse(time.RFC3339, settingsMap["last_sync_at"])
	}

	return settings, nil
}

func (s *Store) RecordSyncResult(ctx context.Context, syncTime time.Time, syncErr string) error {
	settings := map[string]string{
		"last_sync_at":    syncTime.UTC().Format(time.RFC3339),
		"last_sync_error": syncErr,
	}

	for key, value := range settings {
		if err := s.UpsertSetting(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO settings(key, value, updated_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key,
		value,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to save setting %q: %w", key, err)
	}
	return nil
}

func (s *Store) ListConnections(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, tag, protocol, listen, tls_json, enabled, settings_json, created_at, updated_at
		 FROM connections
		 ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	var connections []Connection
	for rows.Next() {
		connection, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, *connection)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate connections: %w", err)
	}

	return connections, nil
}

func (s *Store) GetConnection(ctx context.Context, id int64) (*Connection, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, tag, protocol, listen, tls_json, enabled, settings_json, created_at, updated_at
		 FROM connections WHERE id = ? LIMIT 1`,
		id,
	)

	connection, err := scanConnection(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return connection, nil
}

func (s *Store) SaveConnection(ctx context.Context, id int64, input ConnectionInput) (*Connection, error) {
	if id != 0 && input.TLS == nil {
		existing, err := s.GetConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, fmt.Errorf("connection %d not found", id)
		}
		input.TLS = existing.TLS
	}

	settingsJSON, err := json.Marshal(input.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to encode connection settings: %w", err)
	}

	var tlsJSON sql.NullString
	if input.TLS != nil {
		raw, _ := json.Marshal(input.TLS)
		tlsJSON = sql.NullString{String: string(raw), Valid: true}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if id == 0 {
		result, err := s.db.ExecContext(
			ctx,
			`INSERT INTO connections(name, tag, protocol, listen, tls_json, enabled, settings_json, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			input.Name,
			input.Tag,
			input.Protocol,
			input.Listen,
			tlsJSON,
			boolToInt(input.Enabled),
			string(settingsJSON),
			now,
			now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection: %w", err)
		}
		insertedID, _ := result.LastInsertId()
		return s.GetConnection(ctx, insertedID)
	}

	result, err := s.db.ExecContext(
		ctx,
		`UPDATE connections
		 SET name = ?, tag = ?, protocol = ?, listen = ?, tls_json = ?, enabled = ?, settings_json = ?, updated_at = ?
		 WHERE id = ?`,
		input.Name,
		input.Tag,
		input.Protocol,
		input.Listen,
		tlsJSON,
		boolToInt(input.Enabled),
		string(settingsJSON),
		now,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update connection: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("connection %d not found", id)
	}

	return s.GetConnection(ctx, id)
}

func (s *Store) DeleteConnection(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	return nil
}

func (s *Store) ListClientsByConnection(ctx context.Context, connectionID int64) ([]Client, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, connection_id, name, psk, created_at FROM clients WHERE connection_id = ? ORDER BY id DESC`,
		connectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
	}
	defer rows.Close()

	var clients []Client
	for rows.Next() {
		var (
			c         Client
			createdAt string
		)
		if err := rows.Scan(&c.ID, &c.ConnectionID, &c.Name, &c.PSK, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		clients = append(clients, c)
	}
	return clients, nil
}

func (s *Store) CreateClient(ctx context.Context, connectionID int64, name, psk string) (*Client, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO clients(connection_id, name, psk, created_at) VALUES(?, ?, ?, ?)`,
		connectionID,
		name,
		psk,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Client{
		ID:           id,
		ConnectionID: connectionID,
		Name:         name,
		PSK:          psk,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// GetClient returns a single client by ID, including its PSK.
func (s *Store) GetClient(ctx context.Context, id int64) (*Client, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, connection_id, name, psk, created_at FROM clients WHERE id = ? LIMIT 1`,
		id,
	)
	var (
		c         Client
		createdAt string
	)
	if err := row.Scan(&c.ID, &c.ConnectionID, &c.Name, &c.PSK, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &c, nil
}

// ListAllClientPSKsByConnection returns all PSKs for clients of a given connection.
// Used when building the server-side core config.
func (s *Store) ListAllClientPSKsByConnection(ctx context.Context, connectionID int64) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT psk FROM clients WHERE connection_id = ? AND psk != '' ORDER BY id`,
		connectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list client PSKs: %w", err)
	}
	defer rows.Close()
	var psks []string
	for rows.Next() {
		var psk string
		if err := rows.Scan(&psk); err != nil {
			return nil, err
		}
		psks = append(psks, psk)
	}
	return psks, nil
}

func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}
	return nil
}

func (s *Store) getSettingsMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("failed to query settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate settings: %w", err)
	}
	return settings, nil
}

func upsertSettingTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO settings(key, value, updated_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key,
		value,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("failed to save setting %q: %w", key, err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConnection(src scanner) (*Connection, error) {
	var (
		connection   Connection
		enabled      int
		tlsJSON      sql.NullString
		settingsJSON string
		createdAt    string
		updatedAt    string
	)

	if err := src.Scan(
		&connection.ID,
		&connection.Name,
		&connection.Tag,
		&connection.Protocol,
		&connection.Listen,
		&tlsJSON,
		&enabled,
		&settingsJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	connection.Enabled = enabled == 1
	connection.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	connection.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	connection.Settings = map[string]any{}
	if settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &connection.Settings); err != nil {
			return nil, fmt.Errorf("failed to decode connection settings: %w", err)
		}
	}
	if tlsJSON.Valid && tlsJSON.String != "" {
		connection.TLS = &config.TLSConfig{}
		if err := json.Unmarshal([]byte(tlsJSON.String), connection.TLS); err != nil {
			return nil, fmt.Errorf("failed to decode connection tls: %w", err)
		}
	}

	return &connection, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
