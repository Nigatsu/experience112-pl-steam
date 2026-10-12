package textconv

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func utf16le(s string, bom bool) []byte {
	var b []byte
	if bom {
		b = append(b, 0xff, 0xfe)
	}
	for _, c := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

func TestEncodeCP1250(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Zażółć gęślą jaźń", "Za\xbf\xf3\xb3\xe6 g\xea\x9cl\xb9 ja\x9f\xf1"},
		{"ŁĄCZĘ ŚŹŻ", "\xa3\xa5CZ\xca \x8c\x8f\xaf"},
		{"„cytat” – koniec…", "\x84cytat\x94 \x96 koniec\x85"},
		{"crème à la", "creme a la"},
		{"Щука", "Shchuka"},
		{"日本", "??"},
	}
	for _, tt := range tests {
		if got := string(EncodeCP1250(tt.in)); got != tt.want {
			t.Errorf("EncodeCP1250(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDecode(t *testing.T) {
	for _, in := range [][]byte{utf16le("Łódź", true), utf16le("Łódź", false), {0xa3, 0xf3, 'd', 0x9f}} {
		if got := Decode(in); got != "Łódź" {
			t.Errorf("Decode(% x) = %q", in, got)
		}
	}
}

func TestConvertXMLDeclaration(t *testing.T) {
	polish := utf16le("<?xml version=\"1.0\" encoding=\"UTF-16\"?>\n<Doc>Ćma</Doc>", true)

	steamWithDecl := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Doc>Moth</Doc>")
	if got, want := string(Convert(polish, steamWithDecl)), "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Doc>\xc6ma</Doc>"; got != want {
		t.Errorf("with Steam declaration: %q, want %q", got, want)
	}
	if got, want := string(Convert(polish, []byte("<Doc>Moth</Doc>"))), "<Doc>\xc6ma</Doc>"; got != want {
		t.Errorf("without Steam declaration: %q, want %q", got, want)
	}
}

func TestMergeLoc(t *testing.T) {
	polish := []byte("StringTable =\r\n{\r\n    [\"#a\"] = { text=\"A\" },\r\n}\r\n")
	steam := []byte("StringTable =\r\n{\r\n    [\"#a\"] = { text=\"x\" },\r\n    [\"#b\"] = { text=\"B\" },\r\n}\r\n")
	got, n := MergeLoc(polish, steam)
	want := "StringTable =\r\n{\r\n    [\"#a\"] = { text=\"A\" },\r\n    [\"#b\"] = { text=\"B\" },\r\n}\r\n"
	if n != 1 || string(got) != want {
		t.Errorf("MergeLoc = %d, %q", n, got)
	}
	if _, n := MergeLoc(steam, polish); n != 0 {
		t.Errorf("nothing to merge, got %d", n)
	}
}

func TestToCRLF(t *testing.T) {
	if got := string(ToCRLF([]byte("a\rb\nc\r\nd"))); got != "a\r\nb\r\nc\r\nd" {
		t.Errorf("ToCRLF = %q", got)
	}
}

func TestPolishPath(t *testing.T) {
	for in, want := range map[string]string{
		`strings\enus.loc`:                        `strings\plpl.loc`,
		`sounds\voices\enUS\ud01\UD01_Lea_20.ogg`: `sounds\voices\plPL\ud01\UD01_Lea_20.ogg`,
		`interface\localization\loc_ENUS.xml`:     `interface\localization\loc_PLPL.xml`,
		`fonts\tahoma.ttf`:                        `fonts\tahoma.ttf`,
	} {
		if got := PolishPath(in); got != want {
			t.Errorf("PolishPath(%q) = %q, want %q", in, got, want)
		}
	}
}
