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
