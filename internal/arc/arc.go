// Package arc reads and writes the ARC0 archives used by the Lexis engine
// (eXperience112).
//
// Format:
//
//	header : "ARC0" u32 version(=1) u32 count u32 tocOffset
//	data   : each file XOR'd dword-wise with its own key (tail <4 bytes plain)
//	toc    : per entry name[N] path[P] u32 offset u32 size u32 fileKey,
//	         all XOR'd dword-wise with the archive key
//	trailer: u32 archive key = XOR of all file keys
//
// Two layouts exist: [Wide] (UNICODE builds, UTF-16LE names, N=0x100
// P=0x200) and [ANSI] (8-bit builds, N=0x80 P=0x100).
package arc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// Layout describes the name/path field sizes of the table of contents.
type Layout struct {
	nameLen, pathLen int
	wide             bool
}

var (
	// Wide is the layout of UNICODE builds (e.g. the Polish retail release).
	Wide = Layout{nameLen: 0x100, pathLen: 0x200, wide: true}
	// ANSI is the layout of 8-bit builds (e.g. the Steam release).
	ANSI = Layout{nameLen: 0x80, pathLen: 0x100}
)

func (l Layout) entrySize() int { return l.nameLen + l.pathLen + 12 }

// String returns "wide" or "ansi".
func (l Layout) String() string {
	if l.wide {
		return "wide"
	}
	return "ansi"
}

// Entry is one file in an archive.
type Entry struct {
	Name   string // file name
	Path   string // full path inside the archive, backslash separated
	Offset uint32
	Size   uint32
	Key    uint32 // XOR key; equals Key(plaintext)
}

const magic = "ARC0"

// ErrFormat is returned for files that are not ARC0 archives.
var ErrFormat = errors.New("arc: not an ARC0 archive")

// Reader gives random access to the files of an archive.
type Reader struct {
	r       io.ReaderAt
	Layout  Layout
	Key     uint32
	Entries []Entry
}

// File is a Reader backed by an open file.
type File struct {
	*Reader
	f *os.File
}

// Open opens the archive at path.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r, err := NewReader(f, st.Size())
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &File{Reader: r, f: f}, nil
}

// Close closes the underlying file.
func (f *File) Close() error { return f.f.Close() }

// NewReader parses the table of contents of an archive of the given size.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	var h [16]byte
	if _, err := r.ReadAt(h[:], 0); err != nil {
		return nil, err
	}
	if string(h[:4]) != magic || binary.LittleEndian.Uint32(h[4:]) != 1 {
		return nil, ErrFormat
	}
	count := int(binary.LittleEndian.Uint32(h[8:]))
	toc := int64(binary.LittleEndian.Uint32(h[12:]))
	if toc < 16 || toc > size-4 {
		return nil, ErrFormat
	}
	raw := make([]byte, size-toc)
	if _, err := r.ReadAt(raw, toc); err != nil {
		return nil, err
	}
	var l Layout
	switch len(raw) {
	case count*Wide.entrySize() + 4:
		l = Wide
	case count*ANSI.entrySize() + 4:
		l = ANSI
	default:
		return nil, fmt.Errorf("arc: unknown table layout (%d entries, %d bytes)", count, len(raw))
	}
	key := binary.LittleEndian.Uint32(raw[len(raw)-4:])
	a := &Reader{r: r, Layout: l, Key: key, Entries: make([]Entry, count)}
	n := l.entrySize()
	rec := make([]byte, n)
	for i := range a.Entries {
		copy(rec, raw[i*n:])
		xorDwords(rec, key)
		a.Entries[i] = Entry{
			Name:   l.decodeString(rec[:l.nameLen]),
			Path:   l.decodeString(rec[l.nameLen : l.nameLen+l.pathLen]),
			Offset: binary.LittleEndian.Uint32(rec[n-12:]),
			Size:   binary.LittleEndian.Uint32(rec[n-8:]),
			Key:    binary.LittleEndian.Uint32(rec[n-4:]),
		}
	}
	return a, nil
}

// ReadFile returns the decrypted contents of e.
func (a *Reader) ReadFile(e *Entry) ([]byte, error) {
	b := make([]byte, e.Size)
	if _, err := a.r.ReadAt(b, int64(e.Offset)); err != nil {
		return nil, fmt.Errorf("arc: read %s: %w", e.Path, err)
	}
	xorDwords(b, e.Key)
	return b, nil
}

// Index maps lower-cased paths to entries.
func (a *Reader) Index() map[string]*Entry {
	m := make(map[string]*Entry, len(a.Entries))
	for i := range a.Entries {
		m[strings.ToLower(a.Entries[i].Path)] = &a.Entries[i]
	}
	return m
}

// Find returns the entry with the given path (case-insensitive) or nil.
func (a *Reader) Find(path string) *Entry {
	for i := range a.Entries {
		if strings.EqualFold(a.Entries[i].Path, path) {
			return &a.Entries[i]
		}
	}
	return nil
}

// Writer streams a new archive.
type Writer struct {
	w       io.WriteSeeker
	layout  Layout
	off     uint32
	key     uint32
	entries []Entry
}

// NewWriter starts an archive with the given layout on w.
func NewWriter(w io.WriteSeeker, l Layout) (*Writer, error) {
	if _, err := w.Write(make([]byte, 16)); err != nil {
		return nil, err
	}
	return &Writer{w: w, layout: l, off: 16}, nil
}

// Add appends a file stored under path (backslash separated).
func (w *Writer) Add(path string, data []byte) error {
	if uint64(w.off)+uint64(len(data)) > 1<<32-1 {
		return errors.New("arc: archive exceeds 4 GiB")
	}
	k := Key(data)
	enc := append([]byte(nil), data...)
	xorDwords(enc, k)
	if _, err := w.w.Write(enc); err != nil {
		return err
	}
	name := path[strings.LastIndex(path, `\`)+1:]
	w.entries = append(w.entries, Entry{name, path, w.off, uint32(len(data)), k})
	w.off += uint32(len(data))
	w.key ^= k
	return nil
}

// Close writes the table of contents and header. It does not close the
// underlying writer.
func (w *Writer) Close() error {
	l := w.layout
	n := l.entrySize()
	for _, e := range w.entries {
		rec := make([]byte, n)
		if err := l.encodeString(rec[:l.nameLen], e.Name); err != nil {
			return err
		}
		if err := l.encodeString(rec[l.nameLen:l.nameLen+l.pathLen], e.Path); err != nil {
			return err
		}
		binary.LittleEndian.PutUint32(rec[n-12:], e.Offset)
		binary.LittleEndian.PutUint32(rec[n-8:], e.Size)
		binary.LittleEndian.PutUint32(rec[n-4:], e.Key)
		xorDwords(rec, w.key)
		if _, err := w.w.Write(rec); err != nil {
			return err
		}
	}
	var h [16]byte
	binary.LittleEndian.PutUint32(h[:4], w.key)
	if _, err := w.w.Write(h[:4]); err != nil {
		return err
	}
	copy(h[:], magic)
	binary.LittleEndian.PutUint32(h[4:], 1)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(w.entries)))
	binary.LittleEndian.PutUint32(h[12:], w.off)
	if _, err := w.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := w.w.Write(h[:])
	return err
}

// xorDwords XORs every complete little-endian dword of b with key.
func xorDwords(b []byte, key uint32) {
	for i := 0; i+4 <= len(b); i += 4 {
		binary.LittleEndian.PutUint32(b[i:], binary.LittleEndian.Uint32(b[i:])^key)
	}
}

func (l Layout) decodeString(b []byte) string {
	if !l.wide {
		for i, c := range b {
			if c == 0 {
				return string(b[:i])
			}
		}
		return string(b)
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func (l Layout) encodeString(dst []byte, s string) error {
	var b []byte
	if l.wide {
		for _, c := range utf16.Encode([]rune(s)) {
			b = binary.LittleEndian.AppendUint16(b, c)
		}
	} else {
		for _, r := range s {
			if r > 0x7f {
				return fmt.Errorf("arc: non-ASCII path %q in ANSI archive", s)
			}
		}
		b = []byte(s)
	}
	if len(b) >= len(dst) {
		return fmt.Errorf("arc: path too long: %q", s)
	}
	copy(dst, b)
	return nil
}
