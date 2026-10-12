// Package installshield extracts files from InstallShield cabinets of version
// 6 and later (data1.hdr + dataN.cab), as used by the eXperience112 retail
// installer. The on-disk format follows the documentation of the unshield
// project. Only what is needed to stream one file out is implemented.
package installshield

import (
	"bytes"
	"compress/flate"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// File flags.
const (
	flagSplit      = 1
	flagObfuscated = 2
	flagCompressed = 4
	flagInvalid    = 8
)

const descriptorSize = 0x57

// FS opens cabinet files (data1.hdr, data2.cab, ...) by name.
type FS interface {
	Open(name string) (io.ReadSeekCloser, error)
}

// File describes one file stored in the cabinet set.
type File struct {
	Index          int
	Name           string
	Dir            string
	ExpandedSize   uint64
	compressedSize uint64
	dataOffset     uint64
	md5            [md5.Size]byte
	flags          uint16
	volume         uint16
}

// Cabinet is an opened InstallShield cabinet set.
type Cabinet struct {
	fs    FS
	Files []File
}

// ErrFormat is returned for headers that are not InstallShield v6+.
var ErrFormat = errors.New("installshield: unsupported or invalid cabinet")

func le16(b []byte, o int) uint16 { return binary.LittleEndian.Uint16(b[o:]) }
func le32(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
func le64(b []byte, o int) uint64 { return binary.LittleEndian.Uint64(b[o:]) }

// majorVersion decodes the version field of the common header.
func majorVersion(v uint32) int {
	switch v >> 24 {
	case 1:
		return int(v>>12) & 0xf
	case 2, 4:
		return int(v&0xffff) / 100
	}
	return 0
}

// Open reads data1.hdr from fs.
func Open(fs FS) (*Cabinet, error) {
	f, err := fs.Open("data1.hdr")
	if err != nil {
		return nil, err
	}
	hdr, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	return parseHeader(fs, hdr)
}

func parseHeader(fs FS, hdr []byte) (c *Cabinet, err error) {
	defer func() { // malformed headers must not panic
		if r := recover(); r != nil {
			c, err = nil, ErrFormat
		}
	}()
	if len(hdr) < 0x200 || string(hdr[:4]) != "ISc(" || majorVersion(le32(hdr, 4)) < 6 {
		return nil, ErrFormat
	}
	cdo := int(le32(hdr, 0x0c))
	cd := hdr[cdo:]
	tableOff := int(le32(cd, 0x0c))
	dirCount := int(le32(cd, 0x1c))
	fileCount := int(le32(cd, 0x28))
	tableOff2 := int(le32(cd, 0x2c))
	table := cdo + tableOff

	dirs := make([]string, dirCount)
	for i := range dirs {
		dirs[i] = cstring(hdr, table+int(le32(hdr, table+i*4)))
	}
	c = &Cabinet{fs: fs, Files: make([]File, fileCount)}
	for i := range c.Files {
		p := table + tableOff2 + i*descriptorSize
		f := &c.Files[i]
		f.Index = i
		f.flags = le16(hdr, p)
		f.ExpandedSize = le64(hdr, p+0x02)
		f.compressedSize = le64(hdr, p+0x0a)
		f.dataOffset = le64(hdr, p+0x12)
		copy(f.md5[:], hdr[p+0x1a:p+0x2a])
		f.Name = cstring(hdr, table+int(le32(hdr, p+0x3a)))
		if d := int(le16(hdr, p+0x3e)); d < len(dirs) {
			f.Dir = dirs[d]
		}
		f.volume = le16(hdr, p+0x55)
	}
	return c, nil
}

func cstring(b []byte, o int) string {
	if o < 0 || o >= len(b) {
		return ""
	}
	if i := bytes.IndexByte(b[o:], 0); i >= 0 {
		return string(b[o : o+i])
	}
	return string(b[o:])
}

// Find returns the first valid file with the given name (case-insensitive).
func (c *Cabinet) Find(name string) *File {
	for i := range c.Files {
		if f := &c.Files[i]; f.flags&flagInvalid == 0 && strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

// Extract writes the expanded contents of f to w and verifies size and MD5.
// progress, if not nil, is called with the number of bytes written so far.
func (c *Cabinet) Extract(f *File, w io.Writer, progress func(done uint64)) error {
	src, err := c.openStored(f)
	if err != nil {
		return err
	}
	defer src.close()

	h := md5.New()
	out := io.MultiWriter(w, h)
	var done uint64
	if f.flags&flagCompressed == 0 {
		n, err := io.Copy(out, src)
		if err != nil {
			return err
		}
		done = uint64(n)
	} else {
		// Compressed data is a sequence of independent raw-deflate chunks,
		// each preceded by its u16 length.
		var lenBuf [2]byte
		chunk := make([]byte, 0x10001)
		for done < f.ExpandedSize {
			if _, err := io.ReadFull(src, lenBuf[:]); err != nil {
				return fmt.Errorf("installshield: %s: chunk header: %w", f.Name, err)
			}
			n := int(binary.LittleEndian.Uint16(lenBuf[:]))
			if _, err := io.ReadFull(src, chunk[:n]); err != nil {
				return fmt.Errorf("installshield: %s: chunk: %w", f.Name, err)
			}
			chunk[n] = 0 // zlib expects a trailing byte
			zr := flate.NewReader(bytes.NewReader(chunk[:n+1]))
			m, err := io.Copy(out, zr)
			zr.Close()
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("installshield: %s: inflate: %w", f.Name, err)
			}
			done += uint64(m)
			if progress != nil {
				progress(done)
			}
		}
	}
	if done != f.ExpandedSize {
		return fmt.Errorf("installshield: %s: got %d bytes, want %d", f.Name, done, f.ExpandedSize)
	}
	if !bytes.Equal(h.Sum(nil), f.md5[:]) {
		return fmt.Errorf("installshield: %s: MD5 mismatch (damaged disc or image?)", f.Name)
	}
	return nil
}

// volume is one opened dataN.cab with its header fields.
type volume struct {
	f                                    io.ReadSeekCloser
	firstIndex, lastIndex                uint32
	firstOffset, firstExpanded, firstCmp uint64
	lastOffset, lastExpanded, lastCmp    uint64
}

func (c *Cabinet) openVolume(n int) (*volume, error) {
	f, err := c.fs.Open(fmt.Sprintf("data%d.cab", n))
	if err != nil {
		return nil, err
	}
	var h [0x14 + 0x40]byte
	if _, err := io.ReadFull(f, h[:]); err != nil || string(h[:4]) != "ISc(" {
		f.Close()
		return nil, fmt.Errorf("installshield: data%d.cab: %w", n, ErrFormat)
	}
	v := h[0x14:]
	u64 := func(o int) uint64 { return uint64(le32(v, o)) | uint64(le32(v, o+4))<<32 }
	return &volume{
		f:          f,
		firstIndex: le32(v, 8), lastIndex: le32(v, 12),
		firstOffset: u64(16), firstExpanded: u64(24), firstCmp: u64(32),
		lastOffset: u64(40), lastExpanded: u64(48), lastCmp: u64(56),
	}, nil
}

// storedReader yields the stored bytes of a file, following it across
// volumes when it is split and undoing obfuscation.
type storedReader struct {
	c       *Cabinet
	file    *File
	vol     *volume
	volNum  int
	left    uint64 // bytes left in the current volume
	total   uint64 // bytes left overall
	obfSeed uint32
}

func (c *Cabinet) openStored(f *File) (*storedReader, error) {
	total := f.ExpandedSize
	if f.flags&flagCompressed != 0 {
		total = f.compressedSize
	}
	r := &storedReader{c: c, file: f, volNum: int(f.volume), total: total}
	if err := r.enterVolume(true); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *storedReader) enterVolume(first bool) error {
	r.close()
	v, err := r.c.openVolume(r.volNum)
	if err != nil {
		return err
	}
	r.vol = v
	compressed := r.file.flags&flagCompressed != 0
	idx := uint32(r.file.Index)
	off, size := r.file.dataOffset, r.total
	switch {
	case first && r.file.flags&flagSplit == 0:
	case v.lastIndex == idx && (first || v.firstIndex != idx):
		off, size = v.lastOffset, v.lastCmp
		if !compressed {
			size = v.lastExpanded
		}
	case v.firstIndex == idx:
		off, size = v.firstOffset, v.firstCmp
		if !compressed {
			size = v.firstExpanded
		}
	}
	r.left = min(size, r.total)
	_, err = v.f.Seek(int64(off), io.SeekStart)
	return err
}

func (r *storedReader) Read(p []byte) (int, error) {
	for r.left == 0 {
		if r.total == 0 {
			return 0, io.EOF
		}
		r.volNum++
		if err := r.enterVolume(false); err != nil {
			return 0, err
		}
	}
	if uint64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.vol.f.Read(p)
	if r.file.flags&flagObfuscated != 0 {
		for i := range p[:n] {
			x := p[i] ^ 0xd5
			p[i] = (x>>2 | x<<6) - byte(r.obfSeed%0x47)
			r.obfSeed++
		}
	}
	r.left -= uint64(n)
	r.total -= uint64(n)
	if err == io.EOF && r.total > 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (r *storedReader) close() {
	if r.vol != nil {
		r.vol.f.Close()
		r.vol = nil
	}
}
