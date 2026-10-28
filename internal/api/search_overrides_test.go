package api

import (
	"context"
	"encoding/json"
	"ravenseed/internal/models"
	"testing"
)

func TestSearchOverridesPersistenceAndCache(t *testing.T) {
	f := newFixture(t, false)
	request := req()

	before := f.wait(f.start(request).ID)
	stale := f.service.Settings()
	overrides := map[int]models.IndexerSearchOverride{1: {Movie: "imdb", TV: "title"}}
	code, body := f.do("PUT", "/api/v1/settings/search-overrides", overrides, "")
	if code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	var settings models.Settings
	if err := json.Unmarshal(body, &settings); err != nil || settings.SearchOverrides[1].Movie != "imdb" {
		t.Fatalf("response: %s %v", body, err)
	}
	if err := f.service.SaveSettings(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.Settings()
	if err != nil || stored.SearchOverrides[1] != overrides[1] {
		t.Fatalf("override lost: %+v %v", stored.SearchOverrides, err)
	}
	settings = f.service.Settings()
	settings.SearchOverrides[1] = models.IndexerSearchOverride{Movie: "title"}
	if f.service.Settings().SearchOverrides[1].Movie != "imdb" {
		t.Fatal("public settings shares mutable map")
	}
	for _, refresh := range []bool{false, true} {
		rows, err := f.service.Indexers(context.Background(), refresh)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == 1 && (row.SearchOverride == nil || row.SearchOverride.Movie != "imdb") {
				t.Fatal("missing effective override")
			}
		}
	}
	after := f.start(request)
	if after.CacheHit || after.ID == before.ID {
		t.Fatal("old search cache reused")
	}
	f.wait(after.ID)
	if !f.start(request).CacheHit {
		t.Fatal("stable overrides should reuse cache")
	}
	for _, bad := range []map[int]models.IndexerSearchOverride{{0: {Movie: "title"}}, {1: {Movie: "tvdb"}}, {1: {TV: "unknown"}}} {
		if code, _ := f.do("PUT", "/api/v1/settings/search-overrides", bad, ""); code != 400 {
			t.Fatalf("invalid override accepted: %d", code)
		}
	}
	if f.service.Settings().SearchOverrides[1].Movie != "imdb" {
		t.Fatal("invalid save changed settings")
	}
	code, _ = f.do("PUT", "/api/v1/settings/search-overrides", map[int]models.IndexerSearchOverride{1: {Movie: "auto", TV: "auto"}}, "")
	if code != 200 || len(f.service.Settings().SearchOverrides) != 0 {
		t.Fatal("reset failed")
	}
}
