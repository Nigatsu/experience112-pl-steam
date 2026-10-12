package patch

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/arc"
	"github.com/Nigatsu/experience112-pl-steam/internal/installshield"
	"github.com/Nigatsu/experience112-pl-steam/internal/iso9660"
	"github.com/Nigatsu/experience112-pl-steam/internal/platform"
)

const polishArcName = "loc_plPL.arc"

// Source is where the Polish loc_plPL.arc comes from: an installer (ISO
// image, disc or copied disc folder) or an installed Polish game.
type Source struct {
	Desc    string
	cab     *installshield.Cabinet
	file    *installshield.File
	arcPath string
	closer  io.Closer
}

// Close releases the source (e.g. the ISO image file).
func (s *Source) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// dirFS opens installer files from a folder or a mounted disc.
type dirFS string

func (d dirFS) Open(name string) (io.ReadSeekCloser, error) {
	return os.Open(filepath.Join(string(d), name))
}

// FindSource returns the Polish source: explicit if given (.iso file, disc
// folder or installed game), otherwise a .iso next to the program, a disc in
// any drive, or a default installation of the Polish game.
func FindSource(explicit string, ui UI) (*Source, error) {
	if explicit != "" {
		if s := checkSource(explicit); s != nil {
			return s, nil
		}
		return nil, fmt.Errorf("%s: nie znaleziono polskiej wersji (obraz .iso, data1.hdr lub resources\\%s)", explicit, polishArcName)
	}
	for _, p := range nearbyISOs() {
		if s := checkISO(p); s != nil {
			return s, nil
		}
	}
	var cands []string
	for _, r := range platform.DriveRoots() {
		cands = append(cands, r)
		if platform.IsWine() { // macOS/Linux volumes appear under <drive>:\Volumes
			if m, _ := filepath.Glob(filepath.Join(r, "Volumes", "*")); m != nil {
				cands = append(cands, m...)
			}
		}
	}
	for _, pf := range []string{`C:\Program Files (x86)`, `C:\Program Files`} {
		cands = append(cands, filepath.Join(pf, "Techland", "eXperience112"))
	}
	for _, c := range cands {
		if s := checkSource(c); s != nil {
			return s, nil
		}
	}
	p := ui.Prompt("Nie znaleziono polskiej płyty. Podaj ścieżkę do obrazu .iso, płyty (folder z data1.hdr)\nlub do zainstalowanej polskiej wersji:")
	if s := checkSource(p); s != nil {
		return s, nil
	}
	return nil, errors.New("nie znaleziono polskiej wersji gry")
}

func checkSource(path string) *Source {
	if path == "" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !fi.IsDir() {
		switch {
		case strings.EqualFold(filepath.Ext(path), ".iso"):
			return checkISO(path)
		case strings.EqualFold(filepath.Base(path), polishArcName):
			return &Source{Desc: path, arcPath: path}
		}
		return nil
	}
	if exists(filepath.Join(path, "data1.hdr")) {
		if c, err := installshield.Open(dirFS(path)); err == nil {
			if f := c.Find(polishArcName); f != nil {
				return &Source{Desc: "płyta " + path, cab: c, file: f}
			}
		}
	}
	if p := filepath.Join(path, "resources", polishArcName); exists(p) {
		if a, err := arc.Open(p); err == nil {
			wide := a.Layout == arc.Wide
			a.Close()
			if wide {
				return &Source{Desc: "zainstalowana gra " + path, arcPath: p}
			}
		}
	}
	return nil
}

func checkISO(path string) *Source {
	img, err := iso9660.Open(path)
	if err != nil {
		return nil
	}
	if c, err := installshield.Open(img); err == nil {
		if f := c.Find(polishArcName); f != nil {
			return &Source{Desc: "obraz ISO " + path, cab: c, file: f, closer: img}
		}
	}
	img.Close()
	return nil
}

// nearbyISOs lists .iso files next to the program or the working directory,
// in their parent folders and one level of subfolders.
func nearbyISOs() []string {
	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		bases = append(bases, wd)
	}
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	for _, b := range bases {
		add(b)
	}
	for _, b := range bases {
		add(filepath.Dir(b))
	}
	for _, b := range bases {
		entries, _ := os.ReadDir(b)
		for _, e := range entries {
			if e.IsDir() {
				add(filepath.Join(b, e.Name()))
			}
		}
	}
	var out []string
	for _, d := range dirs {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".iso") {
				out = append(out, filepath.Join(d, e.Name()))
			}
		}
	}
	return out
}

// polishArchive returns a path to the Polish archive, extracting it from the
// installer into tmp if needed. cleanup removes the temporary file.
func (s *Source) polishArchive(tmp string, ui UI) (path string, cleanup func(), err error) {
	if s.cab == nil {
		return s.arcPath, func() {}, nil
	}
	f, err := os.Create(tmp)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.Remove(tmp) }
	w := bufio.NewWriterSize(f, 1<<20)
	last := -1
	err = s.cab.Extract(s.file, w, func(done uint64) {
		if pct := int(done * 100 / s.file.ExpandedSize); pct != last {
			last = pct
			ui.Printf("\r      %3d%%", pct)
		}
	})
	ui.Printf("\n")
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp, cleanup, nil
}
