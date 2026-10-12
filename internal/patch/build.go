package patch

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/arc"
	"github.com/Nigatsu/experience112-pl-steam/internal/textconv"
	"github.com/Nigatsu/experience112-pl-steam/internal/ttf"
)

// keepSteam lists Steam files that are kept even when a Polish one exists
// (the Steam logo/startup sequence).
var keepSteam = map[string]bool{`scripts\start.lua`: true}

// textExts are the text resource types converted from UTF-16 to cp1250.
// .lps files are the lip-sync tracks.
var textExts = []string{".xml", ".loc", ".lua", ".cci", ".txt", ".lps"}

// polishGlyphs are the characters Polish text needs. Only their cp1250 byte
// codes are remapped in the fonts: the engine sizes glyph cells by the
// tallest glyph in 0x20..0xFE, so pulling in other (taller) Central-European
// glyphs would clip descenders.
const polishGlyphs = "ĄąĆćĘęŁłŃńŚśŹźŻż„”’–…"

var fontRemap = func() map[uint16]rune {
	m := map[uint16]rune{}
	for _, r := range polishGlyphs {
		if b, ok := textconv.CP1250Byte(r); ok {
			m[uint16(b)] = r
		}
	}
	return m
}()

// Stats summarises a build.
type Stats struct {
	Polish, Text, Fonts, Fallback, MergedStrings int
}

// Build writes outPath: Steam's English archive (enPath) with every file
// replaced by its Polish counterpart from plPath, converted for the ANSI
// engine. Files Polish lacks stay English.
func Build(plPath, enPath, outPath string, ui UI) (Stats, error) {
	var st Stats
	pl, err := arc.Open(plPath)
	if err != nil {
		return st, err
	}
	defer pl.Close()
	if pl.Layout != arc.Wide {
		return st, errors.New("źródłowe loc_plPL.arc nie pochodzi z polskiej wersji pudełkowej")
	}
	en, err := arc.Open(enPath)
	if err != nil {
		return st, err
	}
	defer en.Close()
	if en.Layout != arc.ANSI {
		return st, errors.New("loc_enUS.arc nie pochodzi z wersji Steam")
	}

	f, err := os.Create(outPath)
	if err != nil {
		return st, err
	}
	defer f.Close()
	w, err := arc.NewWriter(f, arc.ANSI)
	if err != nil {
		return st, err
	}
	plByPath := pl.Index()
	for i := range en.Entries {
		e := &en.Entries[i]
		path := textconv.PolishPath(e.Path)
		data, err := en.ReadFile(e)
		if err != nil {
			return st, err
		}
		if pe := plByPath[strings.ToLower(path)]; pe == nil {
			st.Fallback++
		} else if !keepSteam[strings.ToLower(e.Path)] {
			if data, err = convertFile(pl, pe, path, data, &st); err != nil {
				return st, err
			}
		}
		if err := w.Add(path, data); err != nil {
			return st, err
		}
		if i%200 == 0 || i == len(en.Entries)-1 {
			ui.Printf("\r      %d/%d plików", i+1, len(en.Entries))
		}
	}
	ui.Printf("\n")
	if err := w.Close(); err != nil {
		return st, err
	}
	return st, f.Close()
}

// convertFile returns the Polish version of one file; steam is the
// corresponding Steam file (used for XML headers and missing strings).
func convertFile(pl *arc.File, pe *arc.Entry, path string, steam []byte, st *Stats) ([]byte, error) {
	data, err := pl.ReadFile(pe)
	if err != nil {
		return nil, err
	}
	st.Polish++
	low := strings.ToLower(path)
	switch {
	case strings.HasSuffix(low, ".ttf"):
		out, _, err := ttf.Remap(data, fontRemap)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		st.Fonts++
		return out, nil
	case hasSuffix(low, textExts):
		st.Text++
		out := textconv.Convert(data, steam)
		switch {
		case strings.HasSuffix(low, ".lps"): // bare CR in Polish; ANSI parser wants CRLF
			out = textconv.ToCRLF(out)
		case strings.HasSuffix(low, ".loc"):
			var n int
			out, n = textconv.MergeLoc(out, steam)
			st.MergedStrings += n
		}
		return out, nil
	}
	return data, nil
}

func hasSuffix(s string, suffixes []string) bool {
	for _, x := range suffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}
