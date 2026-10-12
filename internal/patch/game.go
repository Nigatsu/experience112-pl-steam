package patch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/platform"
)

// IsGameDir reports whether dir looks like an eXperience112 Steam folder.
func IsGameDir(dir string) bool {
	return dir != "" && exists(filepath.Join(dir, "resources", "gamedata.arc")) &&
		(exists(filepath.Join(dir, "eXperience112.bin")) || exists(filepath.Join(dir, "eXperience112.exe")))
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// FindGame returns the game folder: explicit if given, otherwise the folder
// of the program, the working directory or a Steam library.
func FindGame(explicit string, ui UI) (string, error) {
	if explicit != "" {
		if !IsGameDir(explicit) {
			return "", fmt.Errorf("%s nie wygląda na folder gry eXperience112 (Steam)", explicit)
		}
		return explicit, nil
	}
	for _, c := range gameCandidates() {
		if IsGameDir(c) {
			return c, nil
		}
	}
	if p := ui.Prompt(`Nie znaleziono gry. Podaj folder gry (…\steamapps\common\Experience112):`); IsGameDir(p) {
		return p, nil
	}
	return "", errors.New("nie znaleziono wersji Steam gry eXperience112")
}

func gameCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, wd)
	}
	for _, root := range platform.SteamRoots() {
		libs := []string{root}
		if b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
				libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
			}
		}
		for _, l := range libs {
			out = append(out, filepath.Join(l, "steamapps", "common", "Experience112"))
		}
	}
	return out
}
