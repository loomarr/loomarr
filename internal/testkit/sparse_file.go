package testkit

import "bytes"

// SparseIdentityCollision returns two equal-sized byte streams with identical first and last
// 64 KiB but different middles. Loomarr's bounded ClipID deliberately treats them as one catalog
// identity; ownership-boundary tests use them to prove full-byte equality is checked separately.
func SparseIdentityCollision() ([]byte, []byte) {
	const window = 64 << 10
	head := bytes.Repeat([]byte{'h'}, window)
	tail := bytes.Repeat([]byte{'t'}, window)
	left := make([]byte, 0, window*3)
	left = append(left, head...)
	left = append(left, bytes.Repeat([]byte{'a'}, window)...)
	left = append(left, tail...)
	right := make([]byte, 0, window*3)
	right = append(right, head...)
	right = append(right, bytes.Repeat([]byte{'b'}, window)...)
	right = append(right, tail...)
	return left, right
}
