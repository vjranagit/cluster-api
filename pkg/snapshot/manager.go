// Package snapshot provides state snapshot and rollback capabilities
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

// Manager handles state snapshots and rollbacks
type Manager struct {
	snapshotDir string
	state       engine.StateManager
}

// NewManager creates a new snapshot manager
func NewManager(snapshotDir string, state engine.StateManager) (*Manager, error) {
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	return &Manager{
		snapshotDir: snapshotDir,
		state:       state,
	}, nil
}

// Snapshot represents a point-in-time state snapshot
type Snapshot struct {
	ID          string
	CreatedAt   time.Time
	Description string
	State       engine.State
	Metadata    SnapshotMetadata
	Checksum    string
}

// SnapshotMetadata contains snapshot metadata
type SnapshotMetadata struct {
	Version       string
	CreatedBy     string
	TriggerReason TriggerReason
	ClusterCount  int
	NodePoolCount int
	Tags          map[string]string
}

// TriggerReason describes why snapshot was created
type TriggerReason string

const (
	TriggerManual         TriggerReason = "manual"
	TriggerPreUpgrade     TriggerReason = "pre_upgrade"
	TriggerPreDelete      TriggerReason = "pre_delete"
	TriggerScheduled      TriggerReason = "scheduled"
	TriggerPreApply       TriggerReason = "pre_apply"
	TriggerDriftRemediate TriggerReason = "drift_remediate"
)

// CreateSnapshot creates a new snapshot of current state
func (m *Manager) CreateSnapshot(ctx context.Context, description string, reason TriggerReason) (*Snapshot, error) {
	// Get current state
	currentState, err := m.state.GetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}

	snapshot := &Snapshot{
		ID:          generateSnapshotID(),
		CreatedAt:   time.Now(),
		Description: description,
		State:       currentState,
		Metadata: SnapshotMetadata{
			Version:       "1.0",
			CreatedBy:     "provctl",
			TriggerReason: reason,
			ClusterCount:  len(currentState.Clusters),
			NodePoolCount: len(currentState.NodePools),
			Tags:          make(map[string]string),
		},
	}

	// Calculate checksum for integrity verification
	snapshot.Checksum = calculateChecksum(snapshot.State)

	// Persist snapshot
	if err := m.saveSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("failed to save snapshot: %w", err)
	}

	return snapshot, nil
}

// RestoreSnapshot restores state from a snapshot
func (m *Manager) RestoreSnapshot(ctx context.Context, snapshotID string, dryRun bool) (*RestoreResult, error) {
	snapshot, err := m.LoadSnapshot(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Verify checksum
	if calculateChecksum(snapshot.State) != snapshot.Checksum {
		return nil, fmt.Errorf("snapshot checksum mismatch - data may be corrupted")
	}

	result := &RestoreResult{
		SnapshotID:  snapshotID,
		RestoredAt:  time.Now(),
		DryRun:      dryRun,
		Changes:     []RestoreChange{},
	}

	// Get current state
	currentState, err := m.state.GetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}

	// Determine changes needed
	result.Changes = m.calculateRestoreChanges(snapshot.State, currentState)

	if !dryRun {
		// Create a backup of current state before restoring
		backup, err := m.CreateSnapshot(ctx, "Pre-restore backup", TriggerManual)
		if err != nil {
			return nil, fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupID = backup.ID

		// Apply restore
		if err := m.state.SaveState(ctx, snapshot.State); err != nil {
			return nil, fmt.Errorf("failed to restore state: %w", err)
		}

		result.Success = true
	} else {
		result.Success = false // Dry run doesn't actually restore
	}

	return result, nil
}

// RestoreResult contains the results of a restore operation
type RestoreResult struct {
	SnapshotID string
	BackupID   string
	RestoredAt time.Time
	DryRun     bool
	Success    bool
	Changes    []RestoreChange
}

// RestoreChange represents a change that will be made during restore
type RestoreChange struct {
	Action   ChangeAction
	Resource api.ResourceID
	Before   interface{}
	After    interface{}
}

// ChangeAction represents the type of change
type ChangeAction string

const (
	ActionAdd    ChangeAction = "add"
	ActionModify ChangeAction = "modify"
	ActionRemove ChangeAction = "remove"
)

func (m *Manager) calculateRestoreChanges(snapshot, current engine.State) []RestoreChange {
	var changes []RestoreChange

	// Check for clusters to add or modify
	for id, snapshotCluster := range snapshot.Clusters {
		if currentCluster, exists := current.Clusters[id]; exists {
			// Cluster exists, check if modified
			if !clustersEqual(snapshotCluster, currentCluster) {
				changes = append(changes, RestoreChange{
					Action: ActionModify,
					Resource: api.ResourceID{
						Kind: "Cluster",
						ID:   id,
						Name: snapshotCluster.Metadata.Name,
					},
					Before: currentCluster.Spec,
					After:  snapshotCluster.Spec,
				})
			}
		} else {
			// Cluster doesn't exist, will be added
			changes = append(changes, RestoreChange{
				Action: ActionAdd,
				Resource: api.ResourceID{
					Kind: "Cluster",
					ID:   id,
					Name: snapshotCluster.Metadata.Name,
				},
				After: snapshotCluster.Spec,
			})
		}
	}

	// Check for clusters to remove
	for id, currentCluster := range current.Clusters {
		if _, exists := snapshot.Clusters[id]; !exists {
			changes = append(changes, RestoreChange{
				Action: ActionRemove,
				Resource: api.ResourceID{
					Kind: "Cluster",
					ID:   id,
					Name: currentCluster.Metadata.Name,
				},
				Before: currentCluster.Spec,
			})
		}
	}

	// Similar logic for node pools
	for id, snapshotPool := range snapshot.NodePools {
		if currentPool, exists := current.NodePools[id]; exists {
			if !nodePoolsEqual(snapshotPool, currentPool) {
				changes = append(changes, RestoreChange{
					Action: ActionModify,
					Resource: api.ResourceID{
						Kind: "NodePool",
						ID:   id,
						Name: snapshotPool.Metadata.Name,
					},
					Before: currentPool.Spec,
					After:  snapshotPool.Spec,
				})
			}
		} else {
			changes = append(changes, RestoreChange{
				Action: ActionAdd,
				Resource: api.ResourceID{
					Kind: "NodePool",
					ID:   id,
					Name: snapshotPool.Metadata.Name,
				},
				After: snapshotPool.Spec,
			})
		}
	}

	for id, currentPool := range current.NodePools {
		if _, exists := snapshot.NodePools[id]; !exists {
			changes = append(changes, RestoreChange{
				Action: ActionRemove,
				Resource: api.ResourceID{
					Kind: "NodePool",
					ID:   id,
					Name: currentPool.Metadata.Name,
				},
				Before: currentPool.Spec,
			})
		}
	}

	return changes
}

// ListSnapshots returns all snapshots sorted by creation time
func (m *Manager) ListSnapshots() ([]SnapshotInfo, error) {
	files, err := os.ReadDir(m.snapshotDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot directory: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		snapshotID := file.Name()[:len(file.Name())-5] // Remove .json
		snapshot, err := m.LoadSnapshot(snapshotID)
		if err != nil {
			continue // Skip invalid snapshots
		}

		info := SnapshotInfo{
			ID:            snapshot.ID,
			CreatedAt:     snapshot.CreatedAt,
			Description:   snapshot.Description,
			TriggerReason: snapshot.Metadata.TriggerReason,
			ClusterCount:  snapshot.Metadata.ClusterCount,
			NodePoolCount: snapshot.Metadata.NodePoolCount,
		}

		fileInfo, _ := file.Info()
		info.SizeBytes = fileInfo.Size()

		snapshots = append(snapshots, info)
	}

	// Sort by creation time (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

// SnapshotInfo contains snapshot summary information
type SnapshotInfo struct {
	ID            string
	CreatedAt     time.Time
	Description   string
	TriggerReason TriggerReason
	ClusterCount  int
	NodePoolCount int
	SizeBytes     int64
}

// LoadSnapshot loads a snapshot by ID
func (m *Manager) LoadSnapshot(snapshotID string) (*Snapshot, error) {
	path := filepath.Join(m.snapshotDir, snapshotID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// DeleteSnapshot deletes a snapshot
func (m *Manager) DeleteSnapshot(snapshotID string) error {
	path := filepath.Join(m.snapshotDir, snapshotID+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	return nil
}

// PruneSnapshots removes old snapshots based on retention policy
func (m *Manager) PruneSnapshots(policy RetentionPolicy) ([]string, error) {
	snapshots, err := m.ListSnapshots()
	if err != nil {
		return nil, err
	}

	var deleted []string
	now := time.Now()

	for _, snapshot := range snapshots {
		shouldDelete := false

		// Age-based retention
		if policy.MaxAge > 0 && now.Sub(snapshot.CreatedAt) > policy.MaxAge {
			shouldDelete = true
		}

		// Count-based retention (keep only N most recent)
		if policy.MaxCount > 0 && len(snapshots)-len(deleted) > policy.MaxCount {
			shouldDelete = true
		}

		if shouldDelete {
			if err := m.DeleteSnapshot(snapshot.ID); err != nil {
				return deleted, err
			}
			deleted = append(deleted, snapshot.ID)
		}
	}

	return deleted, nil
}

// RetentionPolicy defines snapshot retention rules
type RetentionPolicy struct {
	MaxAge   time.Duration
	MaxCount int
}

func (m *Manager) saveSnapshot(snapshot *Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	path := filepath.Join(m.snapshotDir, snapshot.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot file: %w", err)
	}

	return nil
}

func generateSnapshotID() string {
	return fmt.Sprintf("snapshot-%s", time.Now().Format("20060102-150405"))
}

func calculateChecksum(state engine.State) string {
	// Simple checksum - in production, use proper hashing
	data, _ := json.Marshal(state)
	return fmt.Sprintf("%x", len(data))
}

func clustersEqual(a, b *api.Cluster) bool {
	// Deep comparison - simplified version
	aJSON, _ := json.Marshal(a.Spec)
	bJSON, _ := json.Marshal(b.Spec)
	return string(aJSON) == string(bJSON)
}

func nodePoolsEqual(a, b *api.NodePool) bool {
	// Deep comparison - simplified version
	aJSON, _ := json.Marshal(a.Spec)
	bJSON, _ := json.Marshal(b.Spec)
	return string(aJSON) == string(bJSON)
}

// FormatRestoreResult generates a human-readable restore result
func FormatRestoreResult(result *RestoreResult) string {
	output := fmt.Sprintf("📸 Snapshot Restore %s\n\n", result.SnapshotID)

	if result.DryRun {
		output += "⚠ DRY RUN - No changes were applied\n\n"
	} else if result.Success {
		output += "✓ Restore completed successfully\n"
		output += fmt.Sprintf("Backup created: %s\n\n", result.BackupID)
	} else {
		output += "✗ Restore failed\n\n"
	}

	if len(result.Changes) == 0 {
		output += "No changes needed - state matches snapshot\n"
		return output
	}

	addCount := 0
	modifyCount := 0
	removeCount := 0

	for _, change := range result.Changes {
		switch change.Action {
		case ActionAdd:
			addCount++
		case ActionModify:
			modifyCount++
		case ActionRemove:
			removeCount++
		}
	}

	output += fmt.Sprintf("Changes: %d to add, %d to modify, %d to remove\n\n",
		addCount, modifyCount, removeCount)

	output += "Detailed Changes:\n"
	for _, change := range result.Changes {
		icon := ""
		switch change.Action {
		case ActionAdd:
			icon = "+"
		case ActionModify:
			icon = "~"
		case ActionRemove:
			icon = "-"
		}

		output += fmt.Sprintf("  %s %s/%s\n", icon, change.Resource.Kind, change.Resource.Name)
	}

	return output
}
