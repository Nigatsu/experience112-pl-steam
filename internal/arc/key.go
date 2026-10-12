package arc

import (
	"encoding/binary"
	"math"
)

// Key returns the per-file XOR key the engine stores for (and verifies on)
// every archived file.
//
// It is MD5 with one deviation found in the engine: ROTATE_LEFT is
// implemented with a signed right shift, so the bits rotated in from the
// top are smeared with the sign bit. The four state words of the digest are
// folded together with XOR.
func Key(data []byte) uint32 {
	st := [4]uint32{0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476}
	n := len(data) &^ 63
	for i := 0; i < n; i += 64 {
		transform(&st, data[i:i+64])
	}
	tail := make([]byte, 0, 128)
	tail = append(tail, data[n:]...)
	tail = append(tail, 0x80)
	for len(tail)%64 != 56 {
		tail = append(tail, 0)
	}
	tail = binary.LittleEndian.AppendUint64(tail, uint64(len(data))*8)
	for i := 0; i < len(tail); i += 64 {
		transform(&st, tail[i:i+64])
	}
	return st[0] ^ st[1] ^ st[2] ^ st[3]
}

var shifts = [64]uint{
	7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
	5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
	4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
	6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
}

var sines = func() (k [64]uint32) {
	for i := range k {
		k[i] = uint32(math.Abs(math.Sin(float64(i+1))) * (1 << 32))
	}
	return
}()

// rotl is the engine's rotate: x<<c | (int32(x) >> (32-c)).
func rotl(x uint32, c uint) uint32 {
	return x<<c | uint32(int32(x)>>(32-c))
}

func transform(st *[4]uint32, block []byte) {
	var x [16]uint32
	for i := range x {
		x[i] = binary.LittleEndian.Uint32(block[i*4:])
	}
	a, b, c, d := st[0], st[1], st[2], st[3]
	for i := 0; i < 64; i++ {
		var f uint32
		var g int
		switch {
		case i < 16:
			f, g = b&c|^b&d, i
		case i < 32:
			f, g = d&b|^d&c, (5*i+1)%16
		case i < 48:
			f, g = b^c^d, (3*i+5)%16
		default:
			f, g = c^(b|^d), (7*i)%16
		}
		t := a + f + sines[i] + x[g]
		a, d, c, b = d, c, b, b+rotl(t, shifts[i])
	}
	st[0] += a
	st[1] += b
	st[2] += c
	st[3] += d
}
