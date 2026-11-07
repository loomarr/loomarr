// Package header reads what a media file's container header says about it — duration, its
// streams and their codecs, geometry, colour, languages, chapters, and the keyframe index — without
// decoding a frame (decision 0046, #2151 step 3).
//
// It is the only code in Loomarr that parses Matroska/WebM (EBML) and MP4/MOV (ISO BMFF) bytes.
// A cold file on a network share costs one disk seek per read, so every reader here works from as
// few reads as the container allows: the Matroska SeekHead, Info and Tracks sit in the first
// 64 KiB of nearly every file, and an MP4's moov is found by skipping box headers, never by reading
// mdat. Anything else (AVI, MPEG-TS, a header that does not parse, an MP4 chapter track) reports
// ok=false and the caller falls back to ffprobe (Probe does this).
//
// Values are spelled the way ffprobe spells them (codec names, "matroska,webm", "24000/1001", pixel
// formats), because the inventory already stores ffprobe's observations and its consumers compare
// those strings. The parity test holds the two to the same answers.
package header
