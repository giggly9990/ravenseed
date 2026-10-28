package uploads

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/shlex"
	"ravenseed/internal/models"
)

var presetID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var plainArgument = regexp.MustCompile(`^[a-zA-Z0-9_./:=,+@%-]+$`)

func NormalizeArgs(value string) (string, error) {
	if len(value) > 4096 || strings.ContainsFunc(value, unicode.IsControl) {
		return "", errors.New("preset flags must be a single line of up to 4096 characters")
	}
	args, err := shlex.Split(value)
	if err != nil {
		return "", errors.New("preset flags contain an unfinished quote or escape")
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return "", errors.New("enter UA flags, not a command or download path")
	}
	for i, arg := range args {
		flag, _, _ := strings.Cut(arg, "=")
		if flag == "--" {
			return "", errors.New("end-of-options markers are not supported in presets")
		}
		for _, reserved := range []string{"--trackers", "-tk", "--tmdb", "-tmdb", "--imdb", "-imdb", "--unattended", "-ua", "--unattended_confirm", "-uac"} {
			// argparse accepts unique prefixes of long and multi-letter
			// single-dash options, such as --tm or -tmd for -tmdb.
			if flag == reserved || (strings.HasPrefix(flag, "--") && strings.HasPrefix(reserved, flag)) || (!strings.HasPrefix(flag, "--") && !strings.HasPrefix(reserved, "--") && len(flag) > 2 && strings.HasPrefix(reserved, flag)) {
				return "", errors.New("RavenSeed supplies the target tracker, media IDs and unattended mode")
			}
		}
		if !plainArgument.MatchString(arg) {
			args[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(args, " "), nil
}

func validatePresets(v *models.UploadSettings) error {
	if len(v.FlagPresets) > 100 {
		return errors.New("up to 100 UA flag presets are supported")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for i := range v.FlagPresets {
		p := &v.FlagPresets[i]
		p.Name = strings.TrimSpace(p.Name)
		name := strings.ToLower(p.Name)
		if !presetID.MatchString(p.ID) || ids[p.ID] || name == "" || len(p.Name) > 80 || names[name] || strings.ContainsFunc(p.Name, unicode.IsControl) {
			return errors.New("use unique preset IDs and names of 1 to 80 characters")
		}
		args, err := NormalizeArgs(p.Args)
		if err != nil {
			return fmt.Errorf("preset %s: %w", p.Name, err)
		}
		p.Args = args
		ids[p.ID], names[name] = true, true
	}
	if v.DefaultFlagPresetID != "" && !ids[v.DefaultFlagPresetID] {
		return errors.New("choose an existing default UA preset")
	}
	return nil
}
