// Package textconv converts text resources of the Polish (UNICODE) build of
// eXperience112 into the form expected by the 8-bit (ANSI) Steam build.
package textconv

import (
	"bytes"
	"encoding/binary"
	"regexp"
	"strings"
	"unicode/utf16"
)

// cp1250 maps bytes 0x80..0xFF of Windows-1250 to Unicode (0 = undefined).
var cp1250 = [128]rune{
	0x20AC, 0, 0x201A, 0, 0x201E, 0x2026, 0x2020, 0x2021, 0, 0x2030, 0x0160, 0x2039, 0x015A, 0x0164, 0x017D, 0x0179,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0, 0x2122, 0x0161, 0x203A, 0x015B, 0x0165, 0x017E, 0x017A,
	0x00A0, 0x02C7, 0x02D8, 0x0141, 0x00A4, 0x0104, 0x00A6, 0x00A7, 0x00A8, 0x00A9, 0x015E, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x017B,
	0x00B0, 0x00B1, 0x02DB, 0x0142, 0x00B4, 0x00B5, 0x00B6, 0x00B7, 0x00B8, 0x0105, 0x015F, 0x00BB, 0x013D, 0x02DD, 0x013E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7, 0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7, 0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7, 0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7, 0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}

var fromUnicode = func() map[rune]byte {
	m := make(map[rune]byte, len(cp1250))
	for i, r := range cp1250 {
		if r != 0 {
			m[r] = byte(0x80 + i)
		}
	}
	return m
}()

// translit holds ASCII fallbacks for characters Windows-1250 cannot encode
// that occur in the game's texts (French accents, a little Cyrillic).
var translit = func() map[rune]string {
	m := map[rune]string{}
	for chars, repl := range map[string]string{
		"àâãå": "a", "ÀÂÃÅ": "A", "èê": "e", "ÈÊ": "E", "ìï": "i", "ÌÏ": "I",
		"òõ": "o", "ÒÕ": "O", "ùû": "u", "ÙÛ": "U", "ñ": "n", "Ñ": "N", "ÿ": "y",
	} {
		for _, r := range chars {
			m[r] = repl
		}
	}
	lower := []rune("абвгдеёжзийклмнопрстуфхцчшщъыьэюя")
	upper := []rune("АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ")
	latin := []string{"a", "b", "v", "g", "d", "e", "e", "zh", "z", "i", "j", "k", "l", "m", "n", "o", "p",
		"r", "s", "t", "u", "f", "h", "c", "ch", "sh", "shch", "", "y", "", "e", "yu", "ya"}
	for i, l := range latin {
		m[lower[i]] = l
		if l != "" {
			m[upper[i]] = strings.ToUpper(l[:1]) + l[1:]
		} else {
			m[upper[i]] = ""
		}
	}
	return m
}()

// CP1250Byte returns the Windows-1250 byte for r, if it has one.
func CP1250Byte(r rune) (byte, bool) {
	if r < 0x80 {
		return byte(r), true
	}
	b, ok := fromUnicode[r]
	return b, ok
}

// EncodeCP1250 encodes s as Windows-1250, transliterating characters the
// code page lacks and replacing anything else with '?'.
func EncodeCP1250(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if b, ok := CP1250Byte(r); ok {
			out = append(out, b)
		} else if t, ok := translit[r]; ok {
			out = append(out, t...)
		} else {
			out = append(out, '?')
		}
	}
	return out
}

// Decode returns the text of b, which is UTF-16LE (with or without BOM) as
// written by the Polish build, or Windows-1250 otherwise.
func Decode(b []byte) string {
	if IsUTF16(b) {
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[i*2:])
		}
		return strings.TrimPrefix(string(utf16.Decode(u)), "\uFEFF")
	}
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case cp1250[c-0x80] != 0:
			sb.WriteRune(cp1250[c-0x80])
		default:
			sb.WriteRune('\uFFFD')
		}
	}
	return sb.String()
}

// IsUTF16 reports whether b looks like UTF-16LE text: it starts with a
// byte order mark, or contains NUL bytes (which 8-bit text never does).
func IsUTF16(b []byte) bool {
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		return true
	}
	return bytes.IndexByte(b[:min(len(b), 512)], 0) >= 0
}

var xmlDecl = regexp.MustCompile(`^<\?xml[^>]*\?>\r?\n?`)

// Convert turns a Polish text resource into Windows-1250 bytes. An XML
// declaration (which names UTF-16) is replaced by the declaration of the
// Steam counterpart, or dropped if that has none.
func Convert(polish, steam []byte) []byte {
	text := Decode(polish)
	if loc := xmlDecl.FindStringIndex(text); loc != nil {
		text = string(xmlDecl.Find(steam)) + text[loc[1]:]
	}
	return EncodeCP1250(text)
}

var locID = regexp.MustCompile(`\["(#[^"]+)"\]`)

// MergeLoc appends to a Polish string table (strings\*.loc) every line of
// the Steam table whose ID the Polish one lacks, so no subtitle lookup
// comes back empty. It returns the merged table and the number of lines
// added.
func MergeLoc(polish, steam []byte) ([]byte, int) {
	have := map[string]bool{}
	for _, m := range locID.FindAllSubmatch(polish, -1) {
		have[string(m[1])] = true
	}
	var extra []byte
	n := 0
	for _, line := range splitLines(steam) {
		if m := locID.FindSubmatch(line); m != nil && !have[string(m[1])] {
			extra = append(extra, line...)
			n++
		}
	}
	if n == 0 {
		return polish, 0
	}
	end := bytes.LastIndexByte(bytes.TrimRight(polish, " \t\r\n\v\f"), '}')
	if end < 0 {
		return polish, 0
	}
	out := make([]byte, 0, len(polish)+len(extra))
	out = append(out, polish[:end]...)
	out = append(out, extra...)
	return append(out, polish[end:]...), n
}

// splitLines splits after \n, \r or \r\n, keeping the terminators.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		i := bytes.IndexAny(b, "\r\n")
		if i < 0 {
			out = append(out, b)
			break
		}
		j := i + 1
		if b[i] == '\r' && j < len(b) && b[j] == '\n' {
			j++
		}
		out = append(out, b[:j])
		b = b[j:]
	}
	return out
}

var anyEOL = regexp.MustCompile(`\r\n|\r|\n`)

// ToCRLF normalises line endings to CRLF. The Polish lip-sync (.lps) files
// use bare CR, which the ANSI build does not parse.
func ToCRLF(b []byte) []byte { return anyEOL.ReplaceAll(b, []byte("\r\n")) }

var enUS = regexp.MustCompile(`(?i)enus`)

// PolishPath maps an English archive path to its Polish equivalent,
// preserving the case style ("enUS" -> "plPL", "enus" -> "plpl").
func PolishPath(p string) string {
	return enUS.ReplaceAllStringFunc(p, func(s string) string {
		switch s {
		case "enUS":
			return "plPL"
		case "ENUS":
			return "PLPL"
		}
		return "plpl"
	})
}
