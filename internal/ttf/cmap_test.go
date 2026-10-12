package ttf

import (
	"encoding/binary"
	"math/rand"
	"reflect"
	"testing"
)

func TestFormat4RoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	m := map[uint16]uint16{}
	for c := uint16(0x20); c < 0x7f; c++ { // constant-delta run
		m[c] = c - 29
	}
	for i := 0; i < 500; i++ { // scattered codes
		m[uint16(rng.Intn(0xfff0))] = uint16(rng.Intn(3000) + 1)
	}
	b, err := encodeFormat4(m, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeFormat4(b); !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip lost mappings: got %d, want %d", len(got), len(m))
	}
}

// buildFont makes a minimal sfnt with a head table and a (3,1) format 4 cmap.
func buildFont(t *testing.T, m map[uint16]uint16) []byte {
	t.Helper()
	sub, err := encodeFormat4(m, 0)
	if err != nil {
		t.Fatal(err)
	}
	cmap := buildCmap([]subtable{{platform: 3, encoding: 1, data: sub}})
	head := make([]byte, 54)
	const dirLen = 12 + 2*16
	font := make([]byte, dirLen)
	binary.BigEndian.PutUint32(font, 0x00010000)
	binary.BigEndian.PutUint16(font[4:], 2)
	put := func(rec int, tag string, data []byte) {
		font = pad4(font)
		copy(font[rec:], tag)
		binary.BigEndian.PutUint32(font[rec+4:], checksum(data))
		binary.BigEndian.PutUint32(font[rec+8:], uint32(len(font)))
		binary.BigEndian.PutUint32(font[rec+12:], uint32(len(data)))
		font = append(font, data...)
	}
	put(12, "cmap", cmap)
	put(28, "head", head)
	return pad4(font)
}

func TestRemap(t *testing.T) {
	const (
		glyphLStroke = 200 // glyph of 'ł' (U+0142)
		glyphSup3    = 99  // glyph of '³' (U+00B3)
	)
	font := buildFont(t, map[uint16]uint16{'A': 36, 0xB3: glyphSup3, 0x0142: glyphLStroke})

	out, n, err := Remap(font, map[uint16]rune{0xB3: 'ł', 0xB9: 'ą'}) // 'ą' has no glyph
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("remapped %d codes, want 1", n)
	}
	subs, err := parseCmap(mustTable(t, out, "cmap"))
	if err != nil {
		t.Fatal(err)
	}
	got := decodeFormat4(subs[0].data)
	if got[0xB3] != glyphLStroke || got['A'] != 36 || got[0x0142] != glyphLStroke {
		t.Errorf("cmap after remap: %v", got)
	}
	if _, ok := got[0xB9]; ok {
		t.Error("code without target glyph must stay unmapped")
	}
	if sum := checksum(out); sum != 0xB1B0AFBA {
		t.Errorf("font checksum = %#x, want 0xB1B0AFBA", sum)
	}
}

func mustTable(t *testing.T, font []byte, tag string) []byte {
	t.Helper()
	for i := 0; i < int(binary.BigEndian.Uint16(font[4:])); i++ {
		r := 12 + i*16
		if string(font[r:r+4]) == tag {
			b, err := table(font, r)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatalf("no %s table", tag)
	return nil
}

func TestRemapRejectsGarbage(t *testing.T) {
	if _, _, err := Remap([]byte("not a font at all"), nil); err == nil {
		t.Fatal("expected error")
	}
}
