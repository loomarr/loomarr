package testkit

import "encoding/binary"

// PGSTrack builds a Blu-ray PGS (.sup) stream the way an authoring tool lays it out, on a
// 1920x1080 canvas, for tests that decode a real track through the image worker.
type PGSTrack struct{ b []byte }

// Show adds a display set at ms: palette 0 (index 1 opaque white) and one 2x2 white object placed
// at x,y.
func (p *PGSTrack) Show(ms, x, y int) {
	pcs := []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, 0x80, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(pcs[15:17], uint16(x)) //nolint:gosec // fixture coordinates are small
	binary.BigEndian.PutUint16(pcs[17:19], uint16(y)) //nolint:gosec // fixture coordinates are small
	p.segment(ms, 0x16, pcs)
	p.segment(ms, 0x17, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	p.segment(ms, 0x14, []byte{0, 0, 1, 235, 128, 128, 255})
	// A 2x2 object: each row is two pixels of index 1, then the end-of-line code.
	ods := []byte{0, 1, 0, 0xC0, 0, 0, 0, 0, 2, 0, 2}
	p.segment(ms, 0x15, append(ods, 1, 1, 0, 0, 1, 1, 0, 0))
	p.segment(ms, 0x80, nil)
}

// Clear adds a display set at ms that empties the screen.
func (p *PGSTrack) Clear(ms int) {
	p.segment(ms, 0x16, []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 2, 0, 0, 0, 0})
	p.segment(ms, 0x17, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	p.segment(ms, 0x80, nil)
}

// Bytes is the stream so far.
func (p *PGSTrack) Bytes() []byte { return p.b }

func (p *PGSTrack) segment(ms int, kind byte, data []byte) {
	h := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
	binary.BigEndian.PutUint32(h[2:6], uint32(ms*90))       //nolint:gosec // fixture times are small
	binary.BigEndian.PutUint16(h[11:13], uint16(len(data))) //nolint:gosec // segments are small
	p.b = append(append(p.b, h...), data...)
}
