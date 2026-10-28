package api

import (
	"encoding/json"
	"ravenseed/internal/models"
	"strings"
	"testing"
)

func TestArrSettingsAPIHidesKeys(t *testing.T) {
	f := newFixture(t, false)
	v := models.ArrSettings{Instances: []models.ArrInstance{{Name: "Sonarr", Kind: "sonarr", URL: "http://sonarr:8989", APIKey: "sonarr-secret", Enabled: true}}}
	code, body := f.do("PUT", "/api/v1/settings/arr", v, "")
	if code != 200 || strings.Contains(string(body), "sonarr-secret") {
		t.Fatalf("save: %d %s", code, body)
	}
	code, body = f.do("GET", "/api/v1/settings/arr", nil, "")
	var saved models.ArrSettings
	if code != 200 || json.Unmarshal(body, &saved) != nil || len(saved.Instances) != 1 || !saved.Instances[0].HasAPIKey || saved.Instances[0].ID == "" || strings.Contains(string(body), "sonarr-secret") {
		t.Fatalf("read: %d %s", code, body)
	}
	if code, _ = f.do("PUT", "/api/v1/settings/arr", models.ArrSettings{Instances: []models.ArrInstance{{Name: "x", Kind: "lidarr", URL: "http://a"}}}, ""); code != 400 {
		t.Fatalf("invalid kind accepted: %d", code)
	}
	if code, _ = f.do("POST", "/api/v1/settings/arr/test", models.ArrInstance{Name: "x", Kind: "radarr", URL: "http://127.0.0.1:1"}, ""); code != 400 {
		t.Fatalf("keyless test accepted: %d", code)
	}
}
