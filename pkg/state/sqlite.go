// Package state provides state management implementations
package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

// SQLiteStateManager implements StateManager using SQLite
type SQLiteStateManager struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteStateManager creates a new SQLite state manager
func NewSQLiteStateManager(dbPath string) (*SQLiteStateManager, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sm := &SQLiteStateManager{
		db:     db,
		dbPath: dbPath,
	}

	if err := sm.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return sm, nil
}

func (s *SQLiteStateManager) initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS clusters (
		id TEXT PRIMARY KEY,
		metadata TEXT NOT NULL,
		spec TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS node_pools (
		id TEXT PRIMARY KEY,
		cluster_id TEXT NOT NULL,
		metadata TEXT NOT NULL,
		spec TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (cluster_id) REFERENCES clusters(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		type TEXT NOT NULL,
		resource_provider TEXT NOT NULL,
		resource_kind TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		resource_name TEXT NOT NULL,
		actor TEXT NOT NULL,
		payload TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_events_resource ON events(resource_provider, resource_kind, resource_id);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	`

	_, err := s.db.Exec(schema)
	return err
}

// GetState retrieves current state
func (s *SQLiteStateManager) GetState(ctx context.Context) (engine.State, error) {
	state := engine.State{
		Clusters:  make(map[string]*api.Cluster),
		NodePools: make(map[string]*api.NodePool),
		Networks:  make(map[string]interface{}),
		Metadata:  make(map[string]interface{}),
	}

	// Load clusters
	rows, err := s.db.QueryContext(ctx, "SELECT id, metadata, spec, status FROM clusters")
	if err != nil {
		return state, fmt.Errorf("failed to query clusters: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var metadataJSON, specJSON, statusJSON string

		if err := rows.Scan(&id, &metadataJSON, &specJSON, &statusJSON); err != nil {
			return state, fmt.Errorf("failed to scan cluster row: %w", err)
		}

		cluster := &api.Cluster{ID: id}
		if err := json.Unmarshal([]byte(metadataJSON), &cluster.Metadata); err != nil {
			return state, err
		}
		if err := json.Unmarshal([]byte(specJSON), &cluster.Spec); err != nil {
			return state, err
		}
		if err := json.Unmarshal([]byte(statusJSON), &cluster.Status); err != nil {
			return state, err
		}

		state.Clusters[id] = cluster
	}

	// Load node pools
	poolRows, err := s.db.QueryContext(ctx, "SELECT id, metadata, spec, status FROM node_pools")
	if err != nil {
		return state, fmt.Errorf("failed to query node pools: %w", err)
	}
	defer poolRows.Close()

	for poolRows.Next() {
		var id string
		var metadataJSON, specJSON, statusJSON string

		if err := poolRows.Scan(&id, &metadataJSON, &specJSON, &statusJSON); err != nil {
			return state, fmt.Errorf("failed to scan node pool row: %w", err)
		}

		pool := &api.NodePool{ID: id}
		if err := json.Unmarshal([]byte(metadataJSON), &pool.Metadata); err != nil {
			return state, err
		}
		if err := json.Unmarshal([]byte(specJSON), &pool.Spec); err != nil {
			return state, err
		}
		if err := json.Unmarshal([]byte(statusJSON), &pool.Status); err != nil {
			return state, err
		}

		state.NodePools[id] = pool
	}

	return state, nil
}

// SaveState persists state
func (s *SQLiteStateManager) SaveState(ctx context.Context, state engine.State) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Save clusters
	for _, cluster := range state.Clusters {
		metadataJSON, _ := json.Marshal(cluster.Metadata)
		specJSON, _ := json.Marshal(cluster.Spec)
		statusJSON, _ := json.Marshal(cluster.Status)

		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO clusters (id, metadata, spec, status, updated_at)
			 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			cluster.ID, metadataJSON, specJSON, statusJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to save cluster: %w", err)
		}
	}

	// Save node pools
	for _, pool := range state.NodePools {
		metadataJSON, _ := json.Marshal(pool.Metadata)
		specJSON, _ := json.Marshal(pool.Spec)
		statusJSON, _ := json.Marshal(pool.Status)

		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO node_pools (id, metadata, spec, status, updated_at)
			 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			pool.ID, metadataJSON, specJSON, statusJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to save node pool: %w", err)
		}
	}

	return tx.Commit()
}

// BeginTransaction starts a state transaction
func (s *SQLiteStateManager) BeginTransaction() engine.Transaction {
	return &sqliteTransaction{db: s.db}
}

// Lock acquires a lock on state
func (s *SQLiteStateManager) Lock(ctx context.Context) error {
	// SQLite handles locking automatically
	return nil
}

// Unlock releases the state lock
func (s *SQLiteStateManager) Unlock(ctx context.Context) error {
	// SQLite handles locking automatically
	return nil
}

// Close closes the database connection
func (s *SQLiteStateManager) Close() error {
	return s.db.Close()
}

type sqliteTransaction struct {
	db *sql.DB
	tx *sql.Tx
}

func (t *sqliteTransaction) Commit() error {
	if t.tx != nil {
		return t.tx.Commit()
	}
	return nil
}

func (t *sqliteTransaction) Rollback() error {
	if t.tx != nil {
		return t.tx.Rollback()
	}
	return nil
}
