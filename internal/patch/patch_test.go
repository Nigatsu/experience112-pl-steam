package patch

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/Nigatsu/experience112-pl-steam/internal/arc"
)

// testUI accepts every default and records the output.
type testUI struct{ out bytes.Buffer }

func (u *testUI) Printf(format string, args ...any)      { fmt.Fprintf(&u.out, format, args...) }
func (u *testUI) Confirm(question string, def bool) bool { return def }
func (u *testUI) Prompt(question string) string          { return "" }

type file struct {
	path string
	data []byte
}

func writeArc(t *testing.T, path string, l arc.Layout, files []file) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, err := arc.NewWriter(f, l)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range files {
		if err := w.Add(x.path, x.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func readArc(t *testing.T, path string) (arc.Layout, map[string][]byte) {
	t.Helper()
	a, err := arc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	m := map[string][]byte{}
	for i := range a.Entries {
		b, err := a.ReadFile(&a.Entries[i])
		if err != nil {
			t.Fatal(err)
		}
		m[a.Entries[i].Path] = b
	}
	return a.Layout, m
}

func u16(s string) []byte {
	b := []byte{0xff, 0xfe}
	for _, c := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const steamLoc = "StringTable =\r\n{\r\n    [\"#a\"] = { text=\"Hello\" },\r\n    [\"#new\"] = { text=\"Steam only\" },\r\n}\r\n"

// makeGame creates a synthetic English Steam install.
func makeGame(t *testing.T, gamedataExtra ...file) string {
	t.Helper()
	g := t.TempDir()
	writeFile(t, filepath.Join(g, "eXperience112.exe"), []byte("MZ stub"))
	writeFile(t, filepath.Join(g, "engine.dll"), []byte("not the real engine"))
	writeFile(t, filepath.Join(g, "experience112.ini"), []byte("[game]\nregionStrings=enUS\nregionVoices=enUS\n"))
	writeArc(t, filepath.Join(g, "resources", "gamedata.arc"), arc.ANSI,
		append([]file{{`zones\test.bsp`, []byte("geometry")}}, gamedataExtra...))
	writeArc(t, filepath.Join(g, "resources", "loc_enUS.arc"), arc.ANSI, []file{
		{`strings\enus.loc`, []byte(steamLoc)},
		{`interface\localization\loc_enus.xml`, []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<T>Quit</T>")},
		{`sounds\voices\enus\a\line.lps`, []byte(";path\r\n0\r\n0.0 0.1 mbpx\r\n")},
		{`sounds\voices\enus\a\line.ogg`, []byte("OggS english")},
		{`scripts\start.lua`, []byte("-- steam logos")},
		{`movies\steam_only.ogg`, []byte("OggS steam movie")},
	})
	return g
}

// makePolishInstall creates a synthetic installed Polish game.
func makePolishInstall(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	writeArc(t, filepath.Join(d, "resources", polishArcName), arc.Wide, []file{
		{`strings\plpl.loc`, u16("StringTable =\r\n{\r\n    [\"#a\"] = { text=\"Cześć\" },\r\n}\r\n")},
		{`interface\localization\loc_plpl.xml`, u16("<?xml version=\"1.0\" encoding=\"UTF-16\"?>\n<T>Wyjście</T>")},
		{`sounds\voices\plpl\a\line.lps`, u16(";path\r0\r0.0 0.2 AA\r")},
		{`sounds\voices\plpl\a\line.ogg`, []byte("OggS polski")},
		{`scripts\start.lua`, u16("-- techland logo")},
	})
	return d
}

func snapshot(t *testing.T, root string) map[string][16]byte {
	t.Helper()
	m := map[string][16]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		m[rel] = md5.Sum(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallAndRestore(t *testing.T) {
	game := makeGame(t)
	before := snapshot(t, game)
	ui := &testUI{}

	src := checkSource(makePolishInstall(t))
	if src == nil {
		t.Fatal("Polish source not recognised")
	}
	if err := Install(Options{Game: game, Source: src, NoTextFix: true}, ui); err != nil {
		t.Fatalf("Install: %v\n%s", err, ui.out.String())
	}

	layout, files := readArc(t, filepath.Join(game, "resources", polishArcName))
	if layout != arc.ANSI {
		t.Fatalf("installed archive layout = %v", layout)
	}
	want := map[string]string{
		`strings\plpl.loc`: "StringTable =\r\n{\r\n    [\"#a\"] = { text=\"Cze\x9c\xe6\" },\r\n" +
			"    [\"#new\"] = { text=\"Steam only\" },\r\n}\r\n",
		`interface\localization\loc_plpl.xml`: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<T>Wyj\x9ccie</T>",
		`sounds\voices\plpl\a\line.lps`:       ";path\r\n0\r\n0.0 0.2 AA\r\n",
		`sounds\voices\plpl\a\line.ogg`:       "OggS polski",
		`scripts\start.lua`:                   "-- steam logos",
		`movies\steam_only.ogg`:               "OggS steam movie",
	}
	if len(files) != len(want) {
		t.Errorf("archive has %d files, want %d", len(files), len(want))
	}
	for p, w := range want {
		if got := string(files[p]); got != w {
			t.Errorf("%s = %q, want %q", p, got, w)
		}
	}
	ini, _ := os.ReadFile(filepath.Join(game, "experience112.ini"))
	if !strings.Contains(string(ini), "regionStrings=plPL\nregionVoices=plPL") {
		t.Errorf("ini not switched: %q", ini)
	}
	if exists(filepath.Join(game, "resources", "loc_enUS.arc")) {
		t.Error("loc_enUS.arc must be moved to the backup")
	}

	// A second run must work from the backed-up English archive.
	if err := Install(Options{Game: game, Source: src, NoTextFix: true}, ui); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	if err := Restore(game, ui); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, game); !equalSnapshots(before, after) {
		t.Errorf("restore differs:\nbefore %v\nafter  %v", before, after)
	}
}

func equalSnapshots(a, b map[string][16]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestCheckEditionRejectsFrenchBase(t *testing.T) {
	game := makeGame(t, file{`strings\frfr.loc`, []byte("StringTable = {}")})
	var ee *EditionError
	if err := CheckEdition(game, &testUI{}); !errors.As(err, &ee) || ee.Edition != "francuska" {
		t.Fatalf("err = %v, want French EditionError", err)
	}
	writeArc(t, filepath.Join(game, "resources", "loc_deDE.arc"), arc.ANSI, nil)
	if err := CheckEdition(game, &testUI{}); !errors.As(err, &ee) || ee.Edition != "niemiecka" {
		t.Fatalf("err = %v, want German EditionError", err)
	}
	os.Remove(filepath.Join(game, "resources", "loc_enUS.arc"))
	os.Remove(filepath.Join(game, "resources", "gamedata.arc"))
	writeArc(t, filepath.Join(game, "resources", "gamedata.arc"), arc.ANSI, nil)
	os.Remove(filepath.Join(game, "resources", "loc_deDE.arc"))
	if err := CheckEdition(game, &testUI{}); !errors.As(err, &ee) || ee.Edition != "" {
		t.Fatalf("err = %v, want missing-English EditionError", err)
	}
}

func TestCheckEditionMovesOverrides(t *testing.T) {
	game := makeGame(t)
	writeFile(t, filepath.Join(game, "resources", "strings", "enus.loc"), []byte("spanish"))
	writeFile(t, filepath.Join(game, "resources", "interface", "documents", "a.xml"), []byte("<es/>"))
	writeArc(t, filepath.Join(game, "resources", "loc_deDE.arc"), arc.ANSI, nil)
	before := snapshot(t, game)

	if err := CheckEdition(game, &testUI{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"strings", "interface", "loc_deDE.arc"} {
		if exists(filepath.Join(game, "resources", p)) {
			t.Errorf("resources\\%s was not moved", p)
		}
	}
	if err := restoreMoved(game); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, game); !equalSnapshots(before, after) {
		t.Errorf("overrides not restored:\nbefore %v\nafter  %v", before, after)
	}
}

func TestSetIniRegion(t *testing.T) {
	tests := []struct{ in, want string }{
		{"[game]\r\nregionStrings=enUS\r\nregionVoices=enUS\r\n", "[game]\r\nregionStrings=plPL\r\nregionVoices=plPL\r\n"},
		{"[display]\nwidth=1024\n", "[display]\nwidth=1024\n\n[game]\nregionStrings=plPL\nregionVoices=plPL\n"},
	}
	for _, tt := range tests {
		p := filepath.Join(t.TempDir(), "experience112.ini")
		writeFile(t, p, []byte(tt.in))
		if err := setIniRegion(p, "plPL"); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(p); string(got) != tt.want {
			t.Errorf("setIniRegion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPatchEngine(t *testing.T) {
	img := make([]byte, engineCheckOff+64)
	copy(img[engineCheckOff:], engineOriginal)

	if already, err := patchEngine(img); err != nil || already {
		t.Fatalf("first patch: already=%v err=%v", already, err)
	}
	if !bytes.Equal(img[engineCheckOff:engineCheckOff+len(enginePatched)], enginePatched) {
		t.Fatal("bytes not patched")
	}
	if already, err := patchEngine(img); err != nil || !already {
		t.Fatalf("second patch: already=%v err=%v", already, err)
	}
	if _, err := patchEngine(make([]byte, engineCheckOff+64)); err != errUnknownEngine {
		t.Fatalf("unknown engine: err = %v", err)
	}
}

func TestFontRemapCoversPolish(t *testing.T) {
	if len(fontRemap) != len([]rune(polishGlyphs)) {
		t.Fatalf("fontRemap has %d codes for %d Polish characters", len(fontRemap), len([]rune(polishGlyphs)))
	}
	if fontRemap[0xB3] != 'ł' || fontRemap[0xE5] != 0 {
		t.Error("unexpected remap table")
	}
}
