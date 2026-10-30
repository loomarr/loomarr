// Package encode is the hardware-encode vocabulary every ffmpeg caller shares (#2121): which video
// encoders exist, which hardware engine and pipeline family each belongs to, what this host's GPU
// decodes, the device and decode arguments that must precede `-i`, and each family's encoder flags
// and rate control.
//
// It was carved out of internal/playout so the filler pipeline can encode on the same GPU path as
// the live channels without importing the streaming engine. What stayed behind is what is specific
// to a channel's output: the per-family filter graphs that fit, pad, tone-map and watermark every
// item to one uniform picture, and the live MPEG-TS and fMP4 outputs. A caller that preserves the
// source geometry (a filler derivative) needs none of that, and builds its own small graph from the
// pieces here.
//
// Everything here is data in, arguments out. Nothing probes the host: which encoder this machine
// actually runs is playout's trial encode (playout.DetectObserved), and the result is passed in.
package encode
