package installshield

import (
	"bytes"
	"compress/flate"
	"crypto/md5"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"testing"
)

type memFS map[string][]byte

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func (m memFS) Open(name string) (io.ReadSeekCloser, error) {
	b, ok := m[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return nopCloser{bytes.NewReader(b)}, nil
}

func deflateChunk(t *testing.T, p []byte) []byte {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(p)
	w.Close()
	out := binary.LittleEndian.AppendUint16(nil, uint16(buf.Len()))
	return append(out, buf.Bytes()...)
}

// buildCabinet creates a minimal v12 cabinet set holding one compressed file.
func buildCabinet(t *testing.T, name string, content []byte) memFS {
	t.Helper()
	half := len(content) / 2
	stored := append(deflateChunk(t, content[:half]), deflateChunk(t, content[half:])...)

	const (
		cdo      = 0x200 // cab descriptor
		tableRel = 0x100 // file table, relative to cdo
		table    = cdo + tableRel
		descRel  = 0x40 // descriptors, relative to table
		dataOff  = 0x14 + 0x40
	)
	hdr := make([]byte, 0x400)
	copy(hdr, "ISc(")
	binary.LittleEndian.PutUint32(hdr[4:], 0x020004b0) // version 12
	binary.LittleEndian.PutUint32(hdr[0x0c:], cdo)
	binary.LittleEndian.PutUint32(hdr[cdo+0x0c:], tableRel)
	binary.LittleEndian.PutUint32(hdr[cdo+0x1c:], 1) // directories
	binary.LittleEndian.PutUint32(hdr[cdo+0x28:], 1) // files
	binary.LittleEndian.PutUint32(hdr[cdo+0x2c:], descRel)
	binary.LittleEndian.PutUint32(hdr[table:], 0x20) // dir 0 name
	copy(hdr[table+0x20:], "resources\x00")
	copy(hdr[table+0x30:], name+"\x00")

	d := hdr[table+descRel:]
	binary.LittleEndian.PutUint16(d[0x00:], flagCompressed)
	binary.LittleEndian.PutUint64(d[0x02:], uint64(len(content)))
	binary.LittleEndian.PutUint64(d[0x0a:], uint64(len(stored)))
	binary.LittleEndian.PutUint64(d[0x12:], dataOff)
	sum := md5.Sum(content)
	copy(d[0x1a:], sum[:])
	binary.LittleEndian.PutUint32(d[0x3a:], 0x30)
	binary.LittleEndian.PutUint16(d[0x55:], 1) // volume

	vol := make([]byte, dataOff)
	copy(vol, "ISc(")
	vol = append(vol, stored...)
	return memFS{"data1.hdr": hdr, "data1.cab": vol}
}

func TestExtract(t *testing.T) {
	content := []byte(strings.Repeat("Polish voices and subtitles. ", 5000))
	fs := buildCabinet(t, "loc_plPL.arc", content)

	c, err := Open(fs)
	if err != nil {
		t.Fatal(err)
	}
	f := c.Find("LOC_PLPL.ARC")
	if f == nil {
		t.Fatal("file not found")
	}
	if f.Dir != "resources" {
		t.Errorf("Dir = %q", f.Dir)
	}
	var out bytes.Buffer
	var last uint64
	if err := c.Extract(f, &out, func(done uint64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), content) {
		t.Fatal("content mismatch")
	}
	if last != uint64(len(content)) {
		t.Errorf("last progress = %d", last)
	}
}

func TestExtractDetectsCorruption(t *testing.T) {
	content := []byte(strings.Repeat("abc", 1000))
	fs := buildCabinet(t, "x.bin", content)
	hdr := fs["data1.hdr"]
	hdr[0x300+0x40+0x1a] ^= 0xff // break the stored MD5
	c, err := Open(fs)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Extract(c.Find("x.bin"), io.Discard, nil); err == nil {
		t.Fatal("expected MD5 error")
	}
}

func TestOpenRejectsGarbage(t *testing.T) {
	if _, err := Open(memFS{"data1.hdr": bytes.Repeat([]byte{0xff}, 0x400)}); err != ErrFormat {
		t.Fatalf("err = %v, want ErrFormat", err)
	}
}

func TestMajorVersion(t *testing.T) {
	for v, want := range map[uint32]int{0x020004b0: 12, 0x01009000: 9, 0x04000258: 6, 0: 0} {
		if got := majorVersion(v); got != want {
			t.Errorf("majorVersion(%#x) = %d, want %d", v, got, want)
		}
	}
}
