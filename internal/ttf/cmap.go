// Package ttf edits the character map (cmap) of TrueType fonts.
package ttf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// ErrNoUnicodeCmap is returned for fonts without a Unicode format 4 cmap.
var ErrNoUnicodeCmap = errors.New("ttf: no Unicode format 4 cmap")

// Remap makes each code point in m (key) render the glyph that the font
// assigns to the target character (value), in every Unicode format 4 cmap
// subtable. Codes whose target has no glyph are skipped. It returns the
// new font and the number of codes remapped per subtable.
//
// The rebuilt cmap table is appended to the font and the table directory
// and checksums are updated; all other tables are left untouched.
func Remap(font []byte, m map[uint16]rune) ([]byte, int, error) {
	if len(font) < 12 {
		return nil, 0, errors.New("ttf: font too short")
	}
	numTables := int(binary.BigEndian.Uint16(font[4:]))
	if 12+numTables*16 > len(font) {
		return nil, 0, errors.New("ttf: truncated table directory")
	}
	cmapRec, headRec := -1, -1
	for i := 0; i < numTables; i++ {
		r := 12 + i*16
		switch string(font[r : r+4]) {
		case "cmap":
			cmapRec = r
		case "head":
			headRec = r
		}
	}
	if cmapRec < 0 || headRec < 0 {
		return nil, 0, errors.New("ttf: missing cmap or head table")
	}
	cmap, err := table(font, cmapRec)
	if err != nil {
		return nil, 0, err
	}
	subs, err := parseCmap(cmap)
	if err != nil {
		return nil, 0, err
	}

	var uni map[uint16]uint16
	for _, s := range subs {
		if s.unicodeFormat4() {
			uni = decodeFormat4(s.data)
			break
		}
	}
	if uni == nil {
		return nil, 0, ErrNoUnicodeCmap
	}
	add := map[uint16]uint16{}
	for code, target := range m {
		if target > 0xffff {
			continue
		}
		if g := uni[uint16(target)]; g != 0 {
			add[code] = g
		}
	}
	for i := range subs {
		if !subs[i].unicodeFormat4() {
			continue
		}
		mp := decodeFormat4(subs[i].data)
		for c, g := range add {
			mp[c] = g
		}
		lang := binary.BigEndian.Uint16(subs[i].data[4:])
		if subs[i].data, err = encodeFormat4(mp, lang); err != nil {
			return nil, 0, err
		}
	}
	newCmap := buildCmap(subs)

	out := pad4(append([]byte(nil), font...))
	off := len(out)
	out = pad4(append(out, newCmap...))
	binary.BigEndian.PutUint32(out[cmapRec+4:], checksum(newCmap))
	binary.BigEndian.PutUint32(out[cmapRec+8:], uint32(off))
	binary.BigEndian.PutUint32(out[cmapRec+12:], uint32(len(newCmap)))
	headOff := int(binary.BigEndian.Uint32(out[headRec+8:]))
	if headOff+12 > len(out) {
		return nil, 0, errors.New("ttf: truncated head table")
	}
	binary.BigEndian.PutUint32(out[headOff+8:], 0) // checkSumAdjustment
	binary.BigEndian.PutUint32(out[headOff+8:], 0xB1B0AFBA-checksum(out))
	return out, len(add), nil
}

type subtable struct {
	platform, encoding uint16
	data               []byte
}

func (s subtable) unicodeFormat4() bool {
	unicode := s.platform == 0 || s.platform == 3 && (s.encoding == 1 || s.encoding == 10)
	return unicode && len(s.data) >= 14 && binary.BigEndian.Uint16(s.data) == 4
}

func table(font []byte, rec int) ([]byte, error) {
	off := int(binary.BigEndian.Uint32(font[rec+8:]))
	n := int(binary.BigEndian.Uint32(font[rec+12:]))
	if off < 0 || n < 0 || off+n > len(font) {
		return nil, fmt.Errorf("ttf: table %q out of range", font[rec:rec+4])
	}
	return font[off : off+n], nil
}

func parseCmap(cmap []byte) ([]subtable, error) {
	if len(cmap) < 4 {
		return nil, errors.New("ttf: truncated cmap")
	}
	n := int(binary.BigEndian.Uint16(cmap[2:]))
	if 4+n*8 > len(cmap) {
		return nil, errors.New("ttf: truncated cmap")
	}
	subs := make([]subtable, n)
	for i := range subs {
		r := 4 + i*8
		off := int(binary.BigEndian.Uint32(cmap[r+4:]))
		if off+8 > len(cmap) {
			return nil, errors.New("ttf: cmap subtable out of range")
		}
		var size int
		switch binary.BigEndian.Uint16(cmap[off:]) {
		case 0, 2, 4, 6:
			size = int(binary.BigEndian.Uint16(cmap[off+2:]))
		default: // 8, 10, 12, 13, 14: u32 length
			size = int(binary.BigEndian.Uint32(cmap[off+4:]))
		}
		if off+size > len(cmap) {
			return nil, errors.New("ttf: cmap subtable out of range")
		}
		subs[i] = subtable{
			platform: binary.BigEndian.Uint16(cmap[r:]),
			encoding: binary.BigEndian.Uint16(cmap[r+2:]),
			data:     cmap[off : off+size],
		}
	}
	return subs, nil
}

func buildCmap(subs []subtable) []byte {
	var b bytes.Buffer
	w16 := func(v uint16) { b.Write(binary.BigEndian.AppendUint16(nil, v)) }
	w16(0)
	w16(uint16(len(subs)))
	off := 4 + 8*len(subs)
	for _, s := range subs {
		w16(s.platform)
		w16(s.encoding)
		b.Write(binary.BigEndian.AppendUint32(nil, uint32(off)))
		off += len(s.data)
	}
	for _, s := range subs {
		b.Write(s.data)
	}
	return b.Bytes()
}

func decodeFormat4(b []byte) map[uint16]uint16 {
	segX2 := int(binary.BigEndian.Uint16(b[6:]))
	ends, starts := 14, 14+segX2+2
	deltas, ranges := starts+segX2, starts+2*segX2
	m := map[uint16]uint16{}
	if ranges+segX2 > len(b) {
		return m
	}
	for i := 0; i < segX2/2; i++ {
		end := int(binary.BigEndian.Uint16(b[ends+2*i:]))
		start := int(binary.BigEndian.Uint16(b[starts+2*i:]))
		delta := binary.BigEndian.Uint16(b[deltas+2*i:])
		ro := int(binary.BigEndian.Uint16(b[ranges+2*i:]))
		for c := start; c <= end && c != 0xffff; c++ {
			var g uint16
			if ro == 0 {
				g = uint16(c) + delta
			} else {
				p := ranges + 2*i + ro + 2*(c-start)
				if p+2 > len(b) {
					continue
				}
				if g = binary.BigEndian.Uint16(b[p:]); g != 0 {
					g += delta
				}
			}
			if g != 0 {
				m[uint16(c)] = g
			}
		}
	}
	return m
}

// encodeFormat4 writes one segment per run of consecutive codes, using
// idDelta when the run maps with a constant delta and glyphIdArray otherwise.
func encodeFormat4(m map[uint16]uint16, lang uint16) ([]byte, error) {
	codes := make([]int, 0, len(m))
	for c := range m {
		if c != 0xffff {
			codes = append(codes, int(c))
		}
	}
	sort.Ints(codes)
	type segment struct{ start, end int }
	var segs []segment
	for i := 0; i < len(codes); {
		j := i
		for j+1 < len(codes) && codes[j+1] == codes[j]+1 {
			j++
		}
		segs = append(segs, segment{codes[i], codes[j]})
		i = j + 1
	}
	segs = append(segs, segment{0xffff, 0xffff})

	n := len(segs)
	ends := make([]uint16, n)
	starts := make([]uint16, n)
	deltas := make([]uint16, n)
	ranges := make([]uint16, n)
	var glyphs []uint16
	for i, s := range segs {
		ends[i], starts[i] = uint16(s.end), uint16(s.start)
		if s.start == 0xffff {
			deltas[i] = 1
			continue
		}
		d := m[uint16(s.start)] - uint16(s.start)
		uniform := true
		for c := s.start; c <= s.end && uniform; c++ {
			uniform = m[uint16(c)]-uint16(c) == d
		}
		if uniform {
			deltas[i] = d
			continue
		}
		ranges[i] = uint16(2*(n-i) + 2*len(glyphs))
		for c := s.start; c <= s.end; c++ {
			glyphs = append(glyphs, m[uint16(c)])
		}
	}
	search, selector := 1, 0
	for search*2 <= n {
		search *= 2
		selector++
	}
	b := make([]byte, 0, 16+8*n+2*len(glyphs))
	w := func(vs ...uint16) {
		for _, v := range vs {
			b = binary.BigEndian.AppendUint16(b, v)
		}
	}
	w(4, 0, lang, uint16(2*n), uint16(2*search), uint16(selector), uint16(2*n-2*search))
	w(ends...)
	w(0)
	w(starts...)
	w(deltas...)
	w(ranges...)
	w(glyphs...)
	if len(b) > 0xffff {
		return nil, errors.New("ttf: format 4 subtable too large")
	}
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	return b, nil
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}
