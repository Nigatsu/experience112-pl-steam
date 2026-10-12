package iso9660

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"
)

func dirRecord(name string, lba, size uint32, dir bool) []byte {
	n := 33 + len(name)
	if n%2 == 1 {
		n++
	}
	r := make([]byte, n)
	r[0] = byte(n)
	binary.LittleEndian.PutUint32(r[2:], lba)
	binary.BigEndian.PutUint32(r[6:], lba)
	binary.LittleEndian.PutUint32(r[10:], size)
	binary.BigEndian.PutUint32(r[14:], size)
	if dir {
		r[25] = 2
	}
	r[32] = byte(len(name))
	copy(r[33:], name)
	return r
}

// buildImage makes a tiny image: PVD (16), terminator (17), root dir (18),
// file data (19).
func buildImage(content []byte) []byte {
	img := make([]byte, 20*SectorSize)
	pvd := img[16*SectorSize:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	copy(pvd[156:], dirRecord("\x00", 18, SectorSize, true))
	term := img[17*SectorSize:]
	term[0] = 255
	copy(term[1:], "CD001")
	var root []byte
	root = append(root, dirRecord("\x00", 18, SectorSize, true)...)
	root = append(root, dirRecord("\x01", 18, SectorSize, true)...)
	root = append(root, dirRecord("SUPPORT", 0, 0, true)...)
	root = append(root, dirRecord("DATA1.HDR;1", 19, uint32(len(content)), false)...)
	copy(img[18*SectorSize:], root)
	copy(img[19*SectorSize:], content)
	return img
}

func TestOpenFile(t *testing.T) {
	content := []byte("ISc( header bytes")
	img, err := NewImage(bytes.NewReader(buildImage(content)))
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Root) != 2 {
		t.Fatalf("root has %d entries, want 2: %+v", len(img.Root), img.Root)
	}
	f, err := img.Open("data1.hdr")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q", got)
	}
	if _, err := img.Open("support"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("opening a directory: err = %v", err)
	}
	if _, err := img.Open("missing.cab"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: err = %v", err)
	}
}

func TestNotISO(t *testing.T) {
	if _, err := NewImage(bytes.NewReader(make([]byte, 40*SectorSize))); err != ErrFormat {
		t.Fatalf("err = %v, want ErrFormat", err)
	}
	if _, err := NewImage(bytes.NewReader([]byte("short"))); err != ErrFormat {
		t.Fatalf("short input: err = %v, want ErrFormat", err)
	}
}
