package database

import (
	"path/filepath"
	"ravenseed/internal/policy"
	"testing"
)

func TestPolicySnapshotsRemainReadableAfterChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "policies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	v := policy.Default(8)
	v.Enabled = true
	first, err := store.SaveIndexerPolicy(v)
	if err != nil {
		t.Fatal(err)
	}
	v.Corrections = nil
	second, err := store.SaveIndexerPolicy(v)
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotID == second.SnapshotID {
		t.Fatal("policy version was reused")
	}
	current, err := store.IndexerPolicy(8)
	if err != nil || current.SnapshotID != second.SnapshotID {
		t.Fatalf("current policy: %+v %v", current, err)
	}
	prior, err := store.PolicySnapshot(first.SnapshotID)
	if err != nil || len(prior.Corrections) == 0 {
		t.Fatalf("historical rules changed: %+v %v", prior, err)
	}
	if _, err := store.DeleteIndexerPolicy(8); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PolicySnapshot(first.SnapshotID); err != nil {
		t.Fatal("delete removed historical snapshot", err)
	}
}
