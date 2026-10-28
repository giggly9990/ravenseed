package api

import (
	"encoding/json"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"testing"
)

func TestIndexerPolicyAPIStoresVersionsAndExposesPresets(t *testing.T) {
	f := newFixture(t, false)
	code, body := f.do("GET", "/api/v1/indexer-policies/3", nil, "")
	if code != 200 {
		t.Fatalf("default policy: %d %s", code, body)
	}
	var v models.IndexerPolicy
	if err := json.Unmarshal(body, &v); err != nil || v.Enabled || v.IndexerID != 3 {
		t.Fatalf("wrong disabled default: %+v %v", v, err)
	}
	v = policy.Default(3)
	v.Enabled = true
	code, body = f.do("PUT", "/api/v1/indexer-policies/3", v, "")
	if code != 200 {
		t.Fatalf("save policy: %d %s", code, body)
	}
	if err := json.Unmarshal(body, &v); err != nil || v.SnapshotID == "" {
		t.Fatalf("snapshot not saved: %+v %v", v, err)
	}
	if code, _ := f.do("GET", "/api/v1/indexer-policies", nil, ""); code != 200 {
		t.Fatal("policy list failed", code)
	}
	code, body = f.do("GET", "/api/v1/policy-presets", nil, "")
	var presets []models.PolicyPreset
	if code != 200 || json.Unmarshal(body, &presets) != nil || len(presets) != 4 {
		t.Fatalf("presets: %d %s", code, body)
	}
	if code, _ := f.do("POST", "/api/v1/indexer-policies/3/recheck", map[string]any{}, ""); code != 202 {
		t.Fatal("recheck endpoint failed", code)
	}
	if code, _ := f.do("DELETE", "/api/v1/indexer-policies/3", nil, ""); code != 200 {
		t.Fatal("delete endpoint failed", code)
	}
	if _, err := f.store.PolicySnapshot(v.SnapshotID); err != nil {
		t.Fatal("historical snapshot deleted", err)
	}
}

func TestPolicySnapshotChangesSearchCacheAndClassifiesVariants(t *testing.T) {
	f := newFixture(t, false)
	before := f.start(req())
	f.wait(before.ID)
	v := policy.Default(3)
	v.Enabled = true
	saved, err := f.store.SaveIndexerPolicy(v)
	if err != nil {
		t.Fatal(err)
	}
	after := f.start(req())
	if after.CacheHit || after.ID == before.ID || after.PolicySnapshotID != saved.SnapshotID {
		t.Fatalf("policy change reused old analysis: %+v", after)
	}
	f.wait(after.ID)
	families, err := f.store.Families(after.ID)
	if err != nil || len(families) == 0 || len(families[0].Variants) == 0 {
		t.Fatalf("missing families: %v %v", families, err)
	}
	vary := families[0].Variants[0]
	if vary.Policy == nil || vary.Policy.Classification != models.PolicyMissingSlot || vary.Policy.PolicySnapshotID != saved.SnapshotID {
		t.Fatalf("variant policy analysis missing: %+v", vary)
	}
}
