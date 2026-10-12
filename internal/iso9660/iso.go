// Package iso9660 is a minimal read-only ISO 9660 reader. It lists the root
// directory of an image and opens files in it, which is all that is needed
// to read an installer straight from a .iso file without mounting it.
package iso9660

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
)

// SectorSize is the logical block size of ISO 9660 images.
const SectorSize = 2048

// ErrFormat is returned for files without a primary volume descriptor.
var ErrFormat = errors.New("iso9660: not an ISO 9660 image")

// Entry is one record of the root directory.
type Entry struct {
	Name  string // without the ";1" version suffix
	LBA   uint32
	Size  uint32
	IsDir bool
}

// Image is an opened ISO 9660 image.
type Image struct {
	r    io.ReaderAt
	c    io.Closer
	Root []Entry
}

// Open opens the image file at path.
func Open(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	img, err := NewImage(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	img.c = f
	return img, nil
}

// NewImage reads the volume descriptors and the root directory from r.
func NewImage(r io.ReaderAt) (*Image, error) {
	buf := make([]byte, SectorSize)
	for s := int64(16); s < 64; s++ {
		if _, err := r.ReadAt(buf, s*SectorSize); err != nil {
			return nil, ErrFormat
		}
		if string(buf[1:6]) != "CD001" {
			return nil, ErrFormat
		}
		switch buf[0] {
		case 1: // primary volume descriptor; root record at offset 156
			root := buf[156:]
			img := &Image{r: r}
			var err error
			img.Root, err = img.readDir(binary.LittleEndian.Uint32(root[2:]), binary.LittleEndian.Uint32(root[10:]))
			if err != nil {
				return nil, err
			}
			return img, nil
		case 255: // terminator
			return nil, ErrFormat
		}
	}
	return nil, ErrFormat
}

// Close closes the underlying file, if the image was opened with Open.
func (img *Image) Close() error {
	if img.c != nil {
		return img.c.Close()
	}
	return nil
}

func (img *Image) readDir(lba, size uint32) ([]Entry, error) {
	if size > 16<<20 {
		return nil, ErrFormat
	}
	b := make([]byte, size)
	if _, err := img.r.ReadAt(b, int64(lba)*SectorSize); err != nil {
		return nil, err
	}
	var out []Entry
	for p := 0; p < len(b); {
		n := int(b[p])
		if n == 0 { // records never cross sector boundaries
			p = (p/SectorSize + 1) * SectorSize
			continue
		}
		if p+33 > len(b) || p+n > len(b) {
			break
		}
		nameLen := int(b[p+32])
		if p+33+nameLen > len(b) {
			break
		}
		name := string(b[p+33 : p+33+nameLen])
		if name != "\x00" && name != "\x01" { // "." and ".."
			if i := strings.IndexByte(name, ';'); i >= 0 {
				name = name[:i]
			}
			out = append(out, Entry{
				Name:  strings.TrimSuffix(name, "."),
				LBA:   binary.LittleEndian.Uint32(b[p+2:]),
				Size:  binary.LittleEndian.Uint32(b[p+10:]),
				IsDir: b[p+25]&2 != 0,
			})
		}
		p += n
	}
	return out, nil
}

type file struct{ *io.SectionReader }

func (file) Close() error { return nil }

// Open opens a file of the root directory (case-insensitive). The returned
// reader stays valid while the image is open; closing it is a no-op.
func (img *Image) Open(name string) (io.ReadSeekCloser, error) {
	for _, e := range img.Root {
		if !e.IsDir && strings.EqualFold(e.Name, name) {
			return file{io.NewSectionReader(img.r, int64(e.LBA)*SectorSize, int64(e.Size))}, nil
		}
	}
	return nil, os.ErrNotExist
}
