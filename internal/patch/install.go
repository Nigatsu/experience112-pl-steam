package patch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Options configures Install.
type Options struct {
	Game      string // Steam game folder
	Source    *Source
	NoTextFix bool // skip the descender-clipping fix
}

// Install builds the Polish archive and installs it into the Steam game,
// backing up every file it replaces.
func Install(opts Options, ui UI) error {
	res := filepath.Join(opts.Game, "resources")
	bak := filepath.Join(opts.Game, BackupDirName)
	enArc := filepath.Join(res, "loc_enUS.arc")
	if !exists(enArc) {
		enArc = filepath.Join(bak, "loc_enUS.arc") // re-run after a previous install
		if !exists(enArc) {
			return errors.New("nie znaleziono resources\\loc_enUS.arc (sprawdź spójność plików gry w Steam)")
		}
	}

	if opts.Source.cab != nil {
		ui.Printf("\n[1/4] Wypakowywanie %s...\n", polishArcName)
	} else {
		ui.Printf("\n[1/4] Używam zainstalowanego %s\n", polishArcName)
	}
	plArc, cleanup, err := opts.Source.polishArchive(filepath.Join(res, "_pl_source.tmp"), ui)
	if err != nil {
		return err
	}
	defer cleanup()

	ui.Printf("[2/4] Budowanie archiwum dla wersji Steam...\n")
	tmpOut := filepath.Join(res, polishArcName+".tmp")
	st, err := Build(plArc, enArc, tmpOut, ui)
	if err != nil {
		os.Remove(tmpOut)
		return err
	}
	ui.Printf("      polskich: %d (teksty: %d, czcionki: %d), angielskich zapasowych: %d, dopisanych napisów: %d\n",
		st.Polish, st.Text, st.Fonts, st.Fallback, st.MergedStrings)

	ui.Printf("[3/4] Instalacja...\n")
	if err := installFiles(opts.Game, tmpOut); err != nil {
		os.Remove(tmpOut)
		return err
	}
	ui.Printf("      %s zainstalowany, język ustawiony na plPL\n", polishArcName)
	ui.Printf("      kopia oryginałów: %s\n", bak)

	ui.Printf("[4/4] Poprawka ucinania dolnych części liter (g, j, y)...\n")
	if opts.NoTextFix {
		ui.Printf("      pominięto\n")
	} else if err := applyTextFix(opts.Game, ui); err != nil {
		ui.Printf("      UWAGA: %v\n", err)
	}
	return nil
}

// installFiles backs up loc_enUS.arc and the ini, moves the built archive
// into place and switches the game language.
func installFiles(game, built string) error {
	res := filepath.Join(game, "resources")
	bak := filepath.Join(game, BackupDirName)
	if err := os.MkdirAll(bak, 0o755); err != nil {
		return err
	}
	if p := filepath.Join(res, "loc_enUS.arc"); exists(p) {
		if err := moveFile(p, filepath.Join(bak, "loc_enUS.arc")); err != nil {
			return fmt.Errorf("kopia loc_enUS.arc: %w", err)
		}
	}
	ini := filepath.Join(game, "experience112.ini")
	if b := filepath.Join(bak, "experience112.ini"); !exists(b) && exists(ini) {
		if err := copyFile(ini, b); err != nil {
			return err
		}
	}
	final := filepath.Join(res, polishArcName)
	os.Remove(final)
	if err := os.Rename(built, final); err != nil {
		return err
	}
	if err := setIniRegion(ini, "plPL"); err != nil {
		return fmt.Errorf("experience112.ini: %w", err)
	}
	return nil
}

// Restore undoes Install using the backup folder.
func Restore(game string, ui UI) error {
	bak := filepath.Join(game, BackupDirName)
	if !exists(bak) {
		return errors.New("brak kopii zapasowej (" + BackupDirName + ") — nic do przywrócenia")
	}
	if !ui.Confirm("Przywrócić oryginalne pliki gry?", true) {
		return ErrAborted
	}
	res := filepath.Join(game, "resources")
	if b := filepath.Join(bak, "loc_enUS.arc"); exists(b) {
		if err := moveFile(b, filepath.Join(res, "loc_enUS.arc")); err != nil {
			return err
		}
	}
	os.Remove(filepath.Join(res, polishArcName))
	for _, f := range []string{"experience112.ini", "engine.dll"} {
		if b := filepath.Join(bak, f); exists(b) {
			if err := copyFile(b, filepath.Join(game, f)); err != nil {
				return err
			}
			os.Remove(b)
		}
	}
	if err := restoreMoved(game); err != nil {
		return err
	}
	revertTextFix(bak)
	os.Remove(filepath.Join(bak, stateFile))
	os.Remove(bak)
	ui.Printf("Przywrócono oryginalne pliki gry.\n")
	return nil
}
