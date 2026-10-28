package uploads

import (
	"github.com/google/shlex"
	"ravenseed/internal/models"
	"reflect"
	"testing"
)

func TestFreeformPresetArguments(t *testing.T) {
	for _, value := range []string{"--anon --modq", `--description "A user's release" --custom-future-flag`, `--screens 6 --imghost imgbox`} {
		normalized, err := NormalizeArgs(value)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := shlex.Split(value)
		after, _ := shlex.Split(normalized)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("quoted arguments changed: %s", normalized)
		}
	}
	for _, value := range []string{`--description "unfinished`, "--anon\n--modq", "upload.py --anon", "--trackers OTHER", "-tk OTHER", "--track=OTHER", "--tmdb movie/1", "--unattended_confirm", "--"} {
		if _, err := NormalizeArgs(value); err == nil {
			t.Fatalf("invalid arguments accepted: %q", value)
		}
	}
}

func TestPresetValidation(t *testing.T) {
	preset := models.UploadFlagPreset{ID: "anon", Name: " Anonymous moderation ", Args: "--anon --modq"}
	value, err := Validate(models.UploadSettings{FlagPresets: []models.UploadFlagPreset{preset}, DefaultFlagPresetID: "anon"})
	if err != nil || value.FlagPresets[0].Name != "Anonymous moderation" {
		t.Fatal("valid preset rejected", err)
	}
	for _, v := range []models.UploadSettings{
		{DefaultFlagPresetID: "missing"},
		{FlagPresets: []models.UploadFlagPreset{preset, preset}},
		{FlagPresets: []models.UploadFlagPreset{{ID: "bad", Name: "", Args: "--anon"}}},
	} {
		if _, err := Validate(v); err == nil {
			t.Fatal("invalid preset accepted")
		}
	}
}
