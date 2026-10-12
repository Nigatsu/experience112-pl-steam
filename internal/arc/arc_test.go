package arc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKey(t *testing.T) {
	// Reference values from an independent implementation that was checked
	// against keys stored in the game's archives.
	tests := []struct {
		data []byte
		want uint32
	}{
		{nil, 0xe9415965},
		{[]byte("a"), 0xf30c72a5},
		{[]byte("abc"), 0xa28fa03b},
		{[]byte("The quick brown fox jumps over the lazy dog"), 0xc282ee1f},
		{bytes.Repeat([]byte("x"), 55), 0xc2ee6321},
		{bytes.Repeat([]byte("x"), 56), 0xabaaa252},
		{bytes.Repeat([]byte("x"), 63), 0x18ea514c},
		{bytes.Repeat([]byte("x"), 64), 0xafdd6121},
		{bytes.Repeat([]byte("x"), 65), 0x90c3312c},
		{pattern(5000), 0xa5d3354e},
	}
	for _, tt := range tests {
		if got := Key(tt.data); got != tt.want {
			t.Errorf("Key(%d bytes) = %#08x, want %#08x", len(tt.data), got, tt.want)
		}
	}
}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*37 + 11)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	files := map[string][]byte{
		`strings\plpl.loc`:         []byte("StringTable = {}"),
		`fonts\tahoma.ttf`:         pattern(1001), // odd size: plain tail bytes
		`sounds\voices\plpl\a.ogg`: pattern(4096),
		`empty.txt`:                nil,
	}
	order := []string{`strings\plpl.loc`, `fonts\tahoma.ttf`, `sounds\voices\plpl\a.ogg`, `empty.txt`}
	for _, l := range []Layout{ANSI, Wide} {
		t.Run(l.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.arc")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			w, err := NewWriter(f, l)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range order {
				if err := w.Add(p, files[p]); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()

			a, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if a.Layout != l {
				t.Fatalf("layout = %v, want %v", a.Layout, l)
			}
			if len(a.Entries) != len(order) {
				t.Fatalf("got %d entries, want %d", len(a.Entries), len(order))
			}
			for i, p := range order {
				e := &a.Entries[i]
				if e.Path != p || e.Name != p[strings.LastIndex(p, `\`)+1:] {
					t.Errorf("entry %d = %q/%q", i, e.Path, e.Name)
				}
				got, err := a.ReadFile(e)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, files[p]) {
					t.Errorf("%s: content mismatch", p)
				}
				if e.Key != Key(got) {
					t.Errorf("%s: stored key does not match content", p)
				}
			}
			if a.Find(`FONTS\TAHOMA.TTF`) == nil || a.Index()[`empty.txt`] == nil {
				t.Error("lookup failed")
			}
		})
	}
}

func TestNotArchive(t *testing.T) {
	data := []byte("definitely not an archive at all")
	if _, err := NewReader(bytes.NewReader(data), int64(len(data))); err != ErrFormat {
		t.Fatalf("err = %v, want ErrFormat", err)
	}
}

func TestANSIRejectsNonASCII(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "x.arc"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, _ := NewWriter(f, ANSI)
	if err := w.Add(`łódź.txt`, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("expected error for non-ASCII path")
	}
}
