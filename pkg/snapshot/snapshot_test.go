package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

type mockStateManager struct {
	state engine.State
}

func (m *mockStateManager) GetState(ctx context.Context) (engine.State, error) {
	return m.state, nil
}

func (m *mockStateManager) SaveState(ctx context.Context, state engine.State) error {
	m.state = state
	return nil
}

func (m *mockStateManager) BeginTransaction() engine.Transaction {
	return nil
}

func (m *mockStateManager) Lock(ctx context.Context) error {
	return nil
}

func (m *mockStateManager) Unlock(ctx context.Context) error {
	return nil
}

func TestManager_CreateSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	state := &mockStateManager{
		state: engine.State{
			Clusters: map[string]*api.Cluster{
				"cluster-1": {
					ID: "cluster-1",
					Metadata: api.ResourceMetadata{
						Name: "test-cluster",
					},
				},
			},
		},
	}

	manager, err := NewManager(tempDir, state)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	snapshot, err := manager.CreateSnapshot(ctx, "Test snapshot", TriggerManual)
	if err != nil {
		t.Errorf("CreateSnapshot() error = %v", err)
		return
	}

	if snapshot.ID == "" {
		t.Error("CreateSnapshot() snapshot ID is empty")
	}

	if snapshot.Metadata.ClusterCount != 1 {
		t.Errorf("CreateSnapshot() cluster count = %d, want 1", snapshot.Metadata.ClusterCount)
	}

	// Verify file was created
	snapshotPath := filepath.Join(tempDir, snapshot.ID+".json")
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Error("CreateSnapshot() snapshot file not created")
	}
}

func TestManager_RestoreSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	initialState := engine.State{
		Clusters: map[string]*api.Cluster{
			"cluster-1": {
				ID: "cluster-1",
				Metadata: api.ResourceMetadata{
					Name: "test-cluster",
				},
				Spec: api.ClusterSpec{
					ControlPlane: api.ControlPlaneSpec{
						Version: "1.28",
					},
				},
			},
		},
	}

	state := &mockStateManager{state: initialState}
	manager, err := NewManager(tempDir, state)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()

	// Create a snapshot
	snapshot, err := manager.CreateSnapshot(ctx, "Test snapshot", TriggerManual)
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}

	// Modify state
	state.state.Clusters["cluster-1"].Spec.ControlPlane.Version = "1.29"

	// Restore snapshot (dry run)
	result, err := manager.RestoreSnapshot(ctx, snapshot.ID, true)
	if err != nil {
		t.Errorf("RestoreSnapshot() error = %v", err)
		return
	}

	if !result.DryRun {
		t.Error("RestoreSnapshot() expected dry run")
	}

	if len(result.Changes) == 0 {
		t.Error("RestoreSnapshot() expected changes to be detected")
	}

	// Restore for real
	result, err = manager.RestoreSnapshot(ctx, snapshot.ID, false)
	if err != nil {
		t.Errorf("RestoreSnapshot() error = %v", err)
		return
	}

	if !result.Success {
		t.Error("RestoreSnapshot() expected success")
	}

	if result.BackupID == "" {
		t.Error("RestoreSnapshot() expected backup ID")
	}

	// Verify state was restored
	if state.state.Clusters["cluster-1"].Spec.ControlPlane.Version != "1.28" {
		t.Error("RestoreSnapshot() state not restored correctly")
	}
}

func TestManager_ListSnapshots(t *testing.T) {
	tempDir := t.TempDir()
	state := &mockStateManager{
		state: engine.State{
			Clusters: map[string]*api.Cluster{},
		},
	}

	manager, err := NewManager(tempDir, state)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()

	// Create multiple snapshots
	for i := 0; i < 3; i++ {
		_, err := manager.CreateSnapshot(ctx, "Snapshot", TriggerManual)
		if err != nil {
			t.Fatalf("CreateSnapshot() error = %v", err)
		}
		time.Sleep(time.Millisecond * 10) // Ensure different timestamps
	}

	snapshots, err := manager.ListSnapshots()
	if err != nil {
		t.Errorf("ListSnapshots() error = %v", err)
		return
	}

	if len(snapshots) != 3 {
		t.Errorf("ListSnapshots() got %d snapshots, want 3", len(snapshots))
	}

	// Verify sorted by creation time (newest first)
	for i := 0; i < len(snapshots)-1; i++ {
		if snapshots[i].CreatedAt.Before(snapshots[i+1].CreatedAt) {
			t.Error("ListSnapshots() not sorted correctly")
		}
	}
}

func TestManager_PruneSnapshots(t *testing.T) {
	tempDir := t.TempDir()
	state := &mockStateManager{
		state: engine.State{
			Clusters: map[string]*api.Cluster{},
		},
	}

	manager, err := NewManager(tempDir, state)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()

	// Create 5 snapshots
	for i := 0; i < 5; i++ {
		_, err := manager.CreateSnapshot(ctx, "Snapshot", TriggerManual)
		if err != nil {
			t.Fatalf("CreateSnapshot() error = %v", err)
		}
	}

	// Prune to keep only 3
	policy := RetentionPolicy{
		MaxCount: 3,
	}

	deleted, err := manager.PruneSnapshots(policy)
	if err != nil {
		t.Errorf("PruneSnapshots() error = %v", err)
		return
	}

	if len(deleted) != 2 {
		t.Errorf("PruneSnapshots() deleted %d snapshots, want 2", len(deleted))
	}

	// Verify only 3 remain
	snapshots, _ := manager.ListSnapshots()
	if len(snapshots) != 3 {
		t.Errorf("PruneSnapshots() left %d snapshots, want 3", len(snapshots))
	}
}
