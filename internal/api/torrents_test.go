package api

import (
	"encoding/json"
	"ravenseed/internal/models"
	"testing"
)

func TestTorrentDefaultsAndCategoryRestrictions(t *testing.T) {
	f := newFixture(t, false)
	v := models.TorrentSettings{DefaultInstanceID: 7, Categories: []models.InstanceCategories{{InstanceID: 7, Enabled: []string{"Movies", "Movies"}}, {InstanceID: 8, Enabled: []string{}}}}
	code, body := f.do("PUT", "/api/v1/settings/torrents", v, "")
	if code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	stored, err := f.store.TorrentSettings()
	if err != nil || stored.DefaultInstanceID != 7 || len(stored.Categories[0].Enabled) != 1 {
		t.Fatal("settings not persisted", err)
	}
	if !stored.AllowsCategory(7, "Movies") || stored.AllowsCategory(7, "TV") || stored.AllowsCategory(8, "Movies") || !stored.AllowsCategory(9, "TV") || !stored.AllowsCategory(8, "") {
		t.Fatal("category scope incorrect")
	}
	code, body = f.do("GET", "/api/v1/settings/torrents", nil, "")
	if code != 200 || json.Unmarshal(body, &v) != nil || v.DefaultInstanceID != 7 {
		t.Fatal("settings read failed")
	}
	job := f.start(req())
	f.wait(job.ID)
	rows, _ := f.store.Results(job.ID)
	endpoint := "/api/v1/searches/" + job.ID + "/results/" + rows[0].ID + "/add"
	if code, _ := f.do("POST", endpoint, models.AddTorrentRequest{Category: "TV"}, ""); code != 400 {
		t.Fatal("disabled category was accepted")
	}
	if len(f.adds) != 0 {
		t.Fatal("disabled category contacted qui")
	}
	if code, body := f.do("POST", endpoint, models.AddTorrentRequest{Category: "Movies"}, ""); code != 200 {
		t.Fatalf("default destination: %d %s", code, body)
	}
	if (<-f.adds)["category"] != "Movies" {
		t.Fatal("enabled category lost")
	}
}

func TestTorrentDefaultsValidationAndAuth(t *testing.T) {
	f := newFixture(t, true)
	for _, method := range []string{"GET", "PUT"} {
		if code, _ := f.do(method, "/api/v1/settings/torrents", models.TorrentSettings{}, ""); code != 401 {
			t.Fatal("settings not authenticated")
		}
	}
	plain := newFixture(t, false)
	for _, v := range []models.TorrentSettings{
		{DefaultInstanceID: -1},
		{Categories: []models.InstanceCategories{{InstanceID: 0}}},
		{Categories: []models.InstanceCategories{{InstanceID: 7}, {InstanceID: 7}}},
		{Categories: []models.InstanceCategories{{InstanceID: 7, Enabled: []string{"bad\nname"}}}},
	} {
		if code, _ := plain.do("PUT", "/api/v1/settings/torrents", v, ""); code != 400 {
			t.Fatal("invalid settings accepted")
		}
	}
}
