package patch

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/arc"
)

// The Steam languages of eXperience112 all use the same 8-bit engine but are
// packaged differently:
//
//	English: English base archives + loc_enUS.arc   (the patch target)
//	Spanish: English base + loose override files in resources\{interface,strings,movies}
//	French:  French base (text in gamedata.arc, voices in sound.arc)
//	German:  French base + loc_deDE.arc
//
// The Polish archive is built against the English base, so French and German
// must be switched to English in Steam first. Loose overrides and other
// languages' loc_*.arc files would mix with Polish and are moved aside.

// looseDirs are the folders the Spanish release uses for loose overrides.
var looseDirs = []string{"interface", "strings", "movies"}

// edition summarises the installed Steam language edition.
type edition struct {
	frenchBase bool     // gamedata.arc carries the French texts
	hasEnglish bool     // loc_enUS.arc present (or already backed up)
	loose      []string // loose override folders present in resources
	otherLoc   []string // loc_xxXX.arc files other than enUS/plPL
}

func detectEdition(game string) (*edition, error) {
	res := filepath.Join(game, "resources")
	g, err := arc.Open(filepath.Join(res, "gamedata.arc"))
	if err != nil {
		return nil, err
	}
	ed := &edition{frenchBase: g.Find(`strings\frfr.loc`) != nil}
	g.Close()
	ed.hasEnglish = exists(filepath.Join(res, "loc_enUS.arc")) || exists(filepath.Join(game, BackupDirName, "loc_enUS.arc"))
	for _, d := range looseDirs {
		if fi, err := os.Stat(filepath.Join(res, d)); err == nil && fi.IsDir() {
			ed.loose = append(ed.loose, d)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(res, "loc_*.arc"))
	for _, p := range matches {
		if b := strings.ToLower(filepath.Base(p)); b != "loc_enus.arc" && b != "loc_plpl.arc" {
			ed.otherLoc = append(ed.otherLoc, filepath.Base(p))
		}
	}
	return ed, nil
}

// SwitchToEnglishHelp tells the user how to switch the Steam edition; show
// it together with an *EditionError.
const SwitchToEnglishHelp = "Przełącz język gry w Steam na angielski:\n" +
	"  Biblioteka → Experience 112 → Właściwości → Język → English,\n" +
	"poczekaj na pobranie plików i uruchom ten program ponownie.\n" +
	"(Jeśli język to już English: Właściwości → Pliki lokalne → Sprawdź spójność plików gry.)"

// EditionError reports an installed Steam language edition that cannot be
// patched.
type EditionError struct {
	Edition string // "francuska", "niemiecka", or "" when loc_enUS.arc is missing
}

func (e *EditionError) Error() string {
	if e.Edition == "" {
		return "brak resources\\loc_enUS.arc — spolszczenie wymaga angielskiej wersji Steam"
	}
	return "zainstalowana jest " + e.Edition + " wersja Steam — spolszczenie wymaga wersji angielskiej"
}

// CheckEdition fails for editions that cannot be patched and, after asking,
// moves files that would conflict with the Polish archive into the backup.
func CheckEdition(game string, ui UI) error {
	ed, err := detectEdition(game)
	if err != nil {
		return err
	}
	switch {
	case ed.frenchBase && len(ed.otherLoc) > 0:
		return &EditionError{Edition: "niemiecka"}
	case ed.frenchBase:
		return &EditionError{Edition: "francuska"}
	case !ed.hasEnglish:
		return &EditionError{}
	}
	if len(ed.loose) == 0 && len(ed.otherLoc) == 0 {
		return nil
	}

	ui.Printf("\n")
	if len(ed.loose) > 0 {
		ui.Printf("Znaleziono luźne pliki nadpisujące teksty gry (np. hiszpańska wersja Steam):\n")
		for _, d := range ed.loose {
			ui.Printf("   resources\\%s\\\n", d)
		}
	}
	for _, f := range ed.otherLoc {
		ui.Printf("Znaleziono archiwum innego języka: resources\\%s\n", f)
	}
	ui.Printf("Zostałyby wczytane razem z polską wersją (mieszanka języków).\n")
	if !ui.Confirm("Przenieść je do kopii zapasowej "+BackupDirName+"?", true) {
		return errors.New("przerwano — usuń te pliki lub przełącz język gry w Steam na angielski")
	}
	res := filepath.Join(game, "resources")
	bak := filepath.Join(game, BackupDirName)
	for _, d := range ed.loose {
		if err := moveTree(filepath.Join(res, d), filepath.Join(bak, "loose", d)); err != nil {
			return err
		}
	}
	for _, f := range ed.otherLoc {
		if err := moveFile(filepath.Join(res, f), filepath.Join(bak, "otherloc", f)); err != nil {
			return err
		}
	}
	ui.Printf("Przeniesiono.\n")
	return nil
}

// restoreMoved puts back loose overrides and other languages' archives.
func restoreMoved(game string) error {
	res := filepath.Join(game, "resources")
	bak := filepath.Join(game, BackupDirName)
	for _, d := range looseDirs {
		if p := filepath.Join(bak, "loose", d); exists(p) {
			if err := moveTree(p, filepath.Join(res, d)); err != nil {
				return err
			}
		}
	}
	os.Remove(filepath.Join(bak, "loose"))
	matches, _ := filepath.Glob(filepath.Join(bak, "otherloc", "*.arc"))
	for _, p := range matches {
		if err := moveFile(p, filepath.Join(res, filepath.Base(p))); err != nil {
			return err
		}
	}
	os.Remove(filepath.Join(bak, "otherloc"))
	return nil
}

var iniRegion = regexp.MustCompile(`(?m)^(regionStrings|regionVoices)=[^\r\n]*`)

// setIniRegion sets regionStrings/regionVoices in experience112.ini,
// adding a [game] section if the keys are missing.
func setIniRegion(path, region string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		return errors.New("plik w formacie UTF-16 — ustaw ręcznie regionStrings/regionVoices")
	}
	var out []byte
	if iniRegion.Match(b) {
		out = iniRegion.ReplaceAll(b, []byte("${1}="+region))
	} else {
		nl := "\n"
		if strings.Contains(string(b), "\r\n") {
			nl = "\r\n"
		}
		out = append(b, []byte(nl+"[game]"+nl+"regionStrings="+region+nl+"regionVoices="+region+nl)...)
	}
	return os.WriteFile(path, out, 0o644)
}
