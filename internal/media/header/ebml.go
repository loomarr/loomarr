package header

import (
	"encoding/binary"
	"math"
)

// readElementHeader decodes an EBML element ID and data size. length is -1 for unknown size.
// n is the header size in bytes, 0 when the buffer does not hold a whole header.
func readElementHeader(b []byte) (id uint64, length int64, n int) {
	if len(b) == 0 {
		return 0, 0, 0
	}
	idLen := 1
	for mask := byte(0x80); idLen <= 4 && b[0]&mask == 0; mask >>= 1 {
		idLen++
	}
	if idLen > 4 || len(b) < idLen+1 {
		return 0, 0, 0
	}
	for _, c := range b[:idLen] {
		id = id<<8 | uint64(c)
	}
	sizeLen := 1
	for mask := byte(0x80); sizeLen <= 8 && b[idLen]&mask == 0; mask >>= 1 {
		sizeLen++
	}
	if sizeLen > 8 || len(b) < idLen+sizeLen {
		return 0, 0, 0
	}
	value := uint64(b[idLen] & (0xFF >> sizeLen))
	allOnes := value == uint64(0xFF>>sizeLen)
	for _, c := range b[idLen+1 : idLen+sizeLen] {
		value = value<<8 | uint64(c)
		allOnes = allOnes && c == 0xFF
	}
	if allOnes {
		return id, -1, idLen + sizeLen
	}
	if value > math.MaxInt64 {
		return 0, 0, 0
	}
	return id, int64(value), idLen + sizeLen
}

// forEach calls fn for each child element in buf, stopping at the first truncated one.
func forEach(buf []byte, fn func(id uint64, body []byte)) {
	for len(buf) > 0 {
		id, l, n := readElementHeader(buf)
		if n == 0 || l < 0 || l > int64(len(buf)-n) {
			return
		}
		fn(id, buf[n:int64(n)+l])
		buf = buf[int64(n)+l:]
	}
}

// beUint reads an EBML unsigned integer (big-endian, 0–8 bytes).
func beUint(b []byte) uint64 {
	var buf [8]byte
	if len(b) > 8 {
		b = b[len(b)-8:]
	}
	copy(buf[8-len(b):], b)
	return binary.BigEndian.Uint64(buf[:])
}

// beFloat reads an EBML float (4 or 8 bytes); anything else is 0.
func beFloat(b []byte) float64 {
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b))
	}
	return 0
}
