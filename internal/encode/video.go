package encode

import "strconv"

// Video is one encode's codec and rate control in the vocabulary every family maps to its own
// flags (VideoArgs).
type Video struct {
	// HEVC encodes with the family's HEVC sibling (HEVCVariant) instead of its H.264 encoder.
	HEVC bool
	// HDR is an HDR10 output: HEVC Main10.
	HDR bool
	// Quality is the QVBR quality target; TargetKbps and MaxKbps its average and cap.
	Quality, TargetKbps, MaxKbps int
	// GOPFrames is the closed-GOP length in frames: an IDR at every keyframe.
	GOPFrames int
}

// VideoArgs maps one encode onto the family's encoder. Rate control, per family:
//
//	VAAPI         QVBR: -rc_mode QVBR -global_quality Q, target and cap (exact).
//	NVENC         VBR with a constant-quality target: -rc vbr -cq Q, target and cap.
//	software      capped CRF: -crf Q with the cap (x264 has no average target alongside CRF).
//	VideoToolbox  average bitrate with the cap (-q:v would drop the bitrate limits).
//	generic       average bitrate with the cap.
//
// Every family: H.264 High (HEVC Main), no B-frames, one closed GOP per segment, IDR at every
// keyframe, no scene-cut keyframes. HEVC is tagged hvc1: Apple's HLS authoring spec requires it
// for fMP4 (ffmpeg's mp4 muxer writes hev1 otherwise); an MPEG-TS output ignores the tag.
func VideoArgs(host HostProfile, v Video) []string {
	args := videoArgs(host, v)
	if v.HEVC {
		args = append(args, "-tag:v", "hvc1")
	}
	return args
}

// File is one offline derivative's video encode (#2121): constant quality, the source's own
// geometry and cadence, and a keyframe at frame 0 and every KeyframeSeconds.
type File struct {
	// Quality is the family's constant-quality level (FileLevel): NVENC -cq, VAAPI -qp, VideoToolbox
	// -q:v, x264 -crf. The scales differ per family, so a level is only meaningful with its family.
	Quality int
	// KeyframeSeconds forces a keyframe at frame 0 and every N seconds, so a cut or a seek lands
	// instantly rather than decoding from the previous GOP.
	KeyframeSeconds int
	// Preset is the x264 preset; empty is the measured `veryfast`. The hardware families ignore it.
	Preset string
}

// FileLevel is the measured constant-quality level for an offline derivative on the family: the
// highest level whose per-clip VMAF against the source stays at or above 95 for 90% of a real filler
// corpus (cmd/filler-encode-bench, docs/design/filler-pipeline.md). ok is false for a family not
// yet measured on its hardware.
func FileLevel(f Family) (level int, ok bool) {
	switch f {
	case FamilyNVENC:
		return 25, true // measured on a GeForce RTX 3080 Ti, 2026-10-30
	case FamilySoftware:
		return 16, true // libx264 veryfast, 2026-10-30
	}
	return 0, false
}

// FileFamily is the family an offline encode runs on: the host's own when it has a constant-quality
// file mode and a measured level (FileLevel), else software. The generic encoders (QSV, AMF, Vulkan,
// RKMPP, V4L2M2M) each spell constant quality differently and none has been measured, so a
// derivative is never their first test; a family with a mode but no measurement is not guessed.
func FileFamily(host HostProfile) Family {
	switch host.Family {
	case FamilyNVENC, FamilyVAAPI, FamilyVideoToolbox:
		if _, ok := FileLevel(host.Family); ok {
			return host.Family
		}
	}
	return FamilySoftware
}

// FileVideoArgs is the encoder, constant-quality rate control and keyframe cadence for an offline
// derivative on family (a caller passes FileFamily(host); the bench passes the family it measures).
// Unlike the live VideoArgs there is no bitrate cap and no low-latency tuning: nobody is waiting on
// the bytes, and B-frames and lookahead buy size. A family without a file mode encodes in software.
func FileVideoArgs(family Family, f File) []string {
	q := strconv.Itoa(f.Quality)
	var args []string
	switch family {
	case FamilyNVENC:
		args = []string{"-c:v", string(EncoderNVENC), "-preset", "p5", "-tune", "hq", "-profile:v", "high",
			"-rc", "vbr", "-cq", q, "-b:v", "0", "-forced-idr", "1"}
	case FamilyVAAPI:
		// CQP, not ICQ: ICQ is Intel-only, and VAAPI is also AMD's family.
		args = []string{"-c:v", string(EncoderVAAPI), "-profile:v", "high", "-rc_mode", "CQP", "-qp", q}
	case FamilyVideoToolbox:
		args = []string{"-c:v", string(EncoderVideoToolbox), "-profile:v", "high", "-allow_sw", "0", "-q:v", q}
	default:
		preset := f.Preset
		if preset == "" {
			preset = "veryfast"
		}
		args = []string{"-c:v", string(EncoderSoftware), "-preset", preset, "-profile:v", "high", "-crf", q}
	}
	if f.KeyframeSeconds > 0 {
		// `expr:gte(t,n_forced*N)` is the form that also guarantees the FIRST frame is an IDR — a
		// clip whose first keyframe arrives late is the black-screen-on-start class §9.1 records.
		args = append(args, "-force_key_frames", "expr:gte(t,n_forced*"+strconv.Itoa(f.KeyframeSeconds)+")")
	}
	return args
}

// videoArgs is one family's encoder and rate control (VideoArgs).
func videoArgs(host HostProfile, v Video) []string {
	g := strconv.Itoa(v.GOPFrames)
	target, maxrate := strconv.Itoa(v.TargetKbps)+"k", strconv.Itoa(v.MaxKbps)+"k"
	q := strconv.Itoa(v.Quality)
	profile := "high"
	switch {
	case v.HDR:
		profile = "main10"
	case v.HEVC:
		profile = "main"
	}
	enc := func(h264 Encoder) string {
		if v.HEVC {
			return string(HEVCVariant(h264))
		}
		return string(h264)
	}
	switch host.Family {
	case FamilyVAAPI:
		return []string{"-c:v", enc(EncoderVAAPI), "-profile:v", profile,
			"-rc_mode", "QVBR", "-global_quality", q, "-b:v", target, "-maxrate", maxrate,
			"-g", g, "-bf", "0", "-sei", "0"}
	case FamilyNVENC:
		return []string{"-c:v", enc(EncoderNVENC), "-preset", "p4", "-tune", "ll", "-profile:v", profile,
			"-rc", "vbr", "-cq", q, "-b:v", target, "-maxrate", maxrate, "-bufsize", maxrate,
			"-g", g, "-bf", "0", "-forced-idr", "1", "-strict_gop", "1", "-no-scenecut", "1"}
	case FamilySoftware:
		if v.HEVC {
			return []string{"-c:v", enc(EncoderSoftware), "-preset", "veryfast", "-profile:v", profile,
				"-crf", q, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-keyint_min", g, "-bf", "0",
				"-x265-params", "open-gop=0:scenecut=0:repeat-headers=1"}
		}
		return []string{"-c:v", enc(EncoderSoftware), "-preset", "veryfast", "-profile:v", profile,
			"-crf", q, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-keyint_min", g, "-sc_threshold", "0",
			"-bf", "0", "-x264-params", "open-gop=0"}
	case FamilyVideoToolbox:
		// The packager paces delivery. The camera-capture realtime hint limits unpaced
		// throughput on Apple Silicon, reducing the lead available at item boundaries.
		return []string{"-c:v", enc(EncoderVideoToolbox), "-profile:v", profile, "-allow_sw", "0", "-realtime", "0",
			"-b:v", target, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-bf", "0"}
	default:
		return []string{"-c:v", enc(EngineOf(host.Encoder)), "-b:v", target, "-maxrate", maxrate, "-bufsize", maxrate,
			"-g", g, "-bf", "0"}
	}
}
