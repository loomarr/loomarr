package playout

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The transcode pipeline builder (#1512 phase 1a).
//
// Build is a PURE function from three pieces of data to the ffmpeg arguments for one item:
//
//   - HostProfile: what this host's hardware and ffmpeg build can do. Data, never a probe, so every
//     family is testable on any machine. The composition root fills it from the chosen encoder and
//     the build's filter list (HostFor).
//   - MediaFormat: the source's stream facts, from Loomarr's inventory measurement.
//   - OutputProfile: the channel's one uniform output.
//
// Each hardware family keeps frames on the GPU from decode to encode. A stage leaves the GPU only
// through a DECLARED fallback, recorded in Pipeline.Fallbacks so the caller can log it:
//
//   - decode: the GPU cannot decode this codec/pixel format, or it faulted on this source
//     (IsHardwareDecodeFault). The CPU decodes, and the frames are uploaded once, before the scale.
//   - tonemap: the GPU has no usable tone-mapper. The picture is scaled on the GPU first, then
//     tone-mapped on the CPU at output size, then uploaded again.
//
// Measured on the household Arc (spike #1513): the full-GPU H.264 1080p graph runs at ~0.04 cores
// per stream and 21x, against ~0.16 cores and 11x for the old CPU-filter chain.

// Family is one hardware pipeline implementation.
type Family string

const (
	// FamilyVAAPI is Intel (iGPU/Arc) and AMD on Linux. Certified on the household Arc.
	FamilyVAAPI Family = "vaapi"
	// FamilyNVENC is NVIDIA: CUDA decode, CUDA filters, NVENC. Certified on the dev GeForce.
	FamilyNVENC Family = "nvenc"
	// FamilySoftware is libx264 on the CPU. Certified on the household host with the GPU hidden.
	FamilySoftware Family = "software"
	// FamilyVideoToolbox is Apple Silicon. UNVERIFIED: no real Mac has run it yet.
	FamilyVideoToolbox Family = "videotoolbox"
	// FamilyGeneric is every other encoder (QSV, Vulkan, AMF, RKMPP, V4L2M2M): CPU filters, then the
	// encoder's own upload. UNVERIFIED; it is the designed fallback for hosts outside the matrix.
	FamilyGeneric Family = "generic"
)

// Verified reports whether the family has been certified on real hardware.
func (f Family) Verified() bool {
	return f == FamilyVAAPI || f == FamilyNVENC || f == FamilySoftware
}

// HostProfile is what one host can do, as data.
type HostProfile struct {
	Family Family `json:"family"`
	// Encoder is the H.264 encoder of the family (the HEVC variant is derived). Only FamilyGeneric
	// needs it to pick between its encoders; the others know theirs.
	Encoder Encoder `json:"encoder"`
	// RenderNode is the DRM render node VAAPI opens.
	RenderNode string `json:"renderNode,omitempty"`
	// DecodeCodecs are the source codecs the GPU decodes, lowercased as ffprobe names them. A codec
	// outside the set takes the decode fallback. Empty means the GPU decodes nothing.
	DecodeCodecs []string `json:"decodeCodecs,omitempty"`
	// TonemapVAAPI: tonemap_vaapi works (Intel; AMD's driver lacks the HDR VPP).
	TonemapVAAPI bool `json:"tonemapVaapi,omitempty"`
	// TonemapOpenCL: tonemap_opencl works (NVIDIA's first choice, maintainer decision).
	TonemapOpenCL bool `json:"tonemapOpencl,omitempty"`
	// Libplacebo: libplacebo on its own Vulkan device works (NVIDIA's second choice).
	Libplacebo bool `json:"libplacebo,omitempty"`
	// CPUTonemap: the build has zscale + tonemap, the last-resort tone-map.
	CPUTonemap bool `json:"cpuTonemap,omitempty"`
	// SoftwareHDR: this host's CPU keeps up with a 4K HDR source tone-mapped at 720p. Measured false
	// on 4 CPUs (1.04x), so a software host refuses HDR unless a measurement says otherwise.
	SoftwareHDR bool `json:"softwareHdr,omitempty"`
}

// GPUFilters is which GPU tone-mappers this ffmpeg BUILD carries (GPUFiltersFor). Whether the
// driver can run them is the host profile's business: tonemap_vaapi is compiled in everywhere
// but only Intel's driver implements it.
type GPUFilters struct {
	TonemapVAAPI, TonemapOpenCL, Libplacebo bool
}

// Hardware decode sets per family: the codecs the certified hardware decodes. Anything else takes
// the decode fallback. AV1 needs Arc/Ampere or newer; older GPUs fault and the ladder's
// software-decode retry (IsHardwareDecodeFault) covers them.
var (
	vaapiDecodes = []string{"h264", "hevc", "mpeg2video", "vp9", "av1"}
	cudaDecodes  = []string{"h264", "hevc", "mpeg2video", "vc1", "vp9", "av1"}
	vtDecodes    = []string{"h264", "hevc"}
	// anyCodec lets ffmpeg decide: the generic family's hardware decode has no output format, so
	// ffmpeg falls back to the CPU on its own for a codec the GPU cannot decode.
	anyCodec = []string{"*"}
)

// HostFor is the host profile for the chosen encoder on this build. A GPU tone-mapper the build
// carries but the driver cannot run (tonemap_vaapi on AMD, OpenCL without an ICD) fails the first
// start; the live ladder then demotes it and retries (ProgramSpec.DemoteTonemap).
func HostFor(enc Encoder, cpuTonemap bool, gpu GPUFilters) HostProfile {
	h := HostProfile{Encoder: engineOf(enc), CPUTonemap: cpuTonemap}
	switch h.Encoder {
	case EncoderVAAPI:
		h.Family, h.RenderNode, h.DecodeCodecs = FamilyVAAPI, renderNode(), vaapiDecodes
		h.TonemapVAAPI = gpu.TonemapVAAPI
	case EncoderNVENC:
		h.Family, h.DecodeCodecs = FamilyNVENC, cudaDecodes
		h.TonemapOpenCL, h.Libplacebo = gpu.TonemapOpenCL, gpu.Libplacebo
	case EncoderVideoToolbox:
		h.Family, h.DecodeCodecs = FamilyVideoToolbox, vtDecodes
	case EncoderSoftware:
		h.Family = FamilySoftware
	default:
		h.Family, h.DecodeCodecs = FamilyGeneric, anyCodec
	}
	return h
}

// OutputProfile is the channel's uniform output (maintainer decisions, #1512).
type OutputProfile struct {
	Width, Height, FPS int
	// HEVC keeps today's HEVC-plan sessions uniformly HEVC (§9.1 V49). The beta.8 output is H.264.
	HEVC bool
	// Quality is the QVBR quality target; TargetKbps and MaxKbps its average and cap. Each family maps
	// them to its closest equivalent (see videoEncoder).
	Quality, TargetKbps, MaxKbps int
	// GOPSeconds is the closed-GOP length, equal to the segment length: an IDR at every segment cut.
	GOPSeconds int
	AudioKbps  int
}

// Maintainer's output picture setting (#1512, 2026-09-26). The target and cap are the 1080p budget;
// lower rungs scale them by pixel count (outputRate).
const (
	outputQuality    = 22
	outputTargetKbps = 8000
	outputMaxKbps    = 12000
	outputGOPSeconds = 1
	// The floor keeps a small rung's cap above what q22 needs on busy motion.
	outputFloorTargetKbps = 1000
	outputFloorMaxKbps    = 1500
)

// ChannelOutput is the uniform output for a channel profile: its geometry, cadence and audio
// bitrate, with the maintainer's rate control. The profile's rung bitrate no longer drives video.
func ChannelOutput(p Profile) OutputProfile {
	return OutputProfile{
		Width: p.Width, Height: p.Height, FPS: p.Framerate,
		HEVC:       engineOf(p.Encoder) != p.Encoder,
		Quality:    outputQuality,
		TargetKbps: outputRate(outputTargetKbps, outputFloorTargetKbps, p.Width, p.Height),
		MaxKbps:    outputRate(outputMaxKbps, outputFloorMaxKbps, p.Width, p.Height),
		GOPSeconds: outputGOPSeconds, AudioKbps: p.AudioBitrate,
	}
}

// outputRate scales a 1080p budget by the rung's share of 1080p's pixels, to the nearest
// 100 kbit/s and never below floor or above the 1080p budget (720p: 8000 -> 3600).
func outputRate(at1080p, floor, width, height int) int {
	share := min(float64(width*height)/(1920*1080), 1)
	return max(int(math.Round(float64(at1080p)*share/100))*100, floor)
}

func (o OutputProfile) gop() int {
	s := o.GOPSeconds
	if s <= 0 {
		s = outputGOPSeconds
	}
	return o.FPS * s
}

// ErrRefused means the host cannot produce this source in real time; the caller shows the slate.
var ErrRefused = errors.New("playout: host cannot transcode this source in real time")

// Pipeline is the built command for one item, in the pieces the live chain splices around its own
// input, seek, map and output options.
type Pipeline struct {
	Family Family
	// PreInput is hardware device setup, hardware decode and probing: global/input options that
	// must precede -i.
	PreInput []string
	// VideoFilter is the -vf graph.
	VideoFilter string
	// VideoEncode is the encoder and its rate control and GOP.
	VideoEncode []string
	// AudioEncode is AAC-LC stereo 48 kHz.
	AudioEncode []string
	// Fallbacks names each stage that left the GPU and why ("decode: …", "tonemap: …").
	Fallbacks []string
	// MissingFacts lists the stream facts Loomarr did not supply; non-empty means ffmpeg probes the
	// source itself (today's behaviour) instead of the minimal probe.
	MissingFacts []string
}

// minimalProbe is the spike's cold-start win (p95 448 → 348 ms), valid only when Loomarr supplies
// the stream facts ffmpeg would otherwise probe for.
var minimalProbe = []string{"-analyzeduration", "0", "-probesize", "32768", "-fpsprobesize", "0"}

// conformColour labels every output frame BT.709 limited-range, left-sited, in the graph. Without it
// the SPS VUI mirrors each source's colour description and the SPS differs between items. Output
// -color_* flags must not be used instead: on ffmpeg 8 they join format negotiation and insert a
// software auto_scale that fails the VAAPI HDR graph (spike #1513).
const conformColour = "setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"

// Build returns the pipeline for one source on one host, or ErrRefused.
func Build(host HostProfile, src MediaFormat, out OutputProfile) (Pipeline, error) {
	b := builder{host: host, src: src, out: out, hdr: src.HDR()}
	b.p.Family = host.Family
	b.p.AudioEncode = []string{"-c:a", "aac", "-profile:a", "aac_low", "-b:a", strconv.Itoa(out.AudioKbps) + "k", "-ac", "2", "-ar", "48000"}
	var err error
	switch host.Family {
	case FamilyVAAPI:
		err = b.vaapi()
	case FamilyNVENC:
		err = b.nvenc()
	case FamilyVideoToolbox:
		err = b.videotoolbox()
	case FamilySoftware:
		err = b.software()
	default:
		err = b.generic()
	}
	if err != nil {
		return Pipeline{}, err
	}
	b.p.MissingFacts = missingFacts(src)
	if len(b.p.MissingFacts) == 0 {
		b.p.PreInput = append(b.p.PreInput, minimalProbe...)
	}
	b.p.VideoEncode = videoEncoder(host, out)
	return b.p, nil
}

// ItemArgs is the complete standalone command for one item: seek, the exact frame count over-asked
// by a tenth of a second (ffmpeg's -frames is not exact; the consumer trims), first video plus the
// chosen audio track, MPEG-TS to stdout.
func (p Pipeline) ItemArgs(input string, seek time.Duration, frames, fps, audioTrack int) []string {
	args := []string{"-hide_banner", "-nostdin", "-nostats", "-loglevel", "error"}
	args = append(args, p.PreInput...)
	if seek > 0 {
		args = append(args, "-ss", seconds(seek))
	}
	args = append(args, "-i", input, "-map", "0:v:0", "-map", "0:a:"+strconv.Itoa(audioTrack))
	if frames > 0 {
		args = append(args, "-frames:v", strconv.Itoa(frames+(fps+9)/10))
	}
	args = append(args, "-vf", p.VideoFilter)
	args = append(args, p.VideoEncode...)
	args = append(args, p.AudioEncode...)
	return append(args, "-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "pipe:1")
}

type builder struct {
	host HostProfile
	src  MediaFormat
	out  OutputProfile
	hdr  bool
	p    Pipeline
}

func (b *builder) fallback(stage, why string) { b.p.Fallbacks = append(b.p.Fallbacks, stage+": "+why) }

// hardwareDecodes reports whether the GPU decodes this source, recording the decode fallback when not.
// An unknown codec is treated as undecodable: with -hwaccel_output_format set, a source the GPU
// cannot decode arrives as CPU frames and the GPU graph fails, where the upload path always works.
func (b *builder) hardwareDecodes() bool {
	codec := strings.ToLower(b.src.VideoCodec)
	switch {
	case codec == "":
		b.fallback("decode", "source codec unknown")
		return false
	case !contains(b.host.DecodeCodecs, codec):
		b.fallback("decode", "the GPU does not decode "+codec)
		return false
	case !chroma420(b.src.PixelFormat):
		b.fallback("decode", "the GPU does not decode "+b.src.PixelFormat)
		return false
	case codec == "h264" && b.src.TenBit():
		b.fallback("decode", "the GPU does not decode 10-bit H.264")
		return false
	}
	return true
}

// chroma420 reports whether the pixel format is 4:2:0, the only chroma GPUs decode broadly. Unknown
// pixel formats count as 4:2:0: hardware decode then still depends on a known codec.
func chroma420(pixfmt string) bool {
	return pixfmt == "" || strings.Contains(pixfmt, "420") || pixfmt == "nv12" || pixfmt == "p010le"
}

// cpuPixelFormat is what a CPU-decoded frame is converted to before it is uploaded: 10-bit stays
// 10-bit for an HDR source so the tone-map sees the full range.
func (b *builder) cpuPixelFormat() string {
	if b.hdr {
		return "p010le"
	}
	return "nv12"
}

func (b *builder) fit() string {
	return fmt.Sprintf("w=%d:h=%d:force_original_aspect_ratio=decrease:force_divisible_by=2", b.out.Width, b.out.Height)
}

func (b *builder) tail() string {
	return fmt.Sprintf("fps=%d,%s", b.out.FPS, conformColour)
}

// cpuTonemap is the declared tone-map fallback for a GPU family: download the already-downscaled
// 10-bit frame, tone-map it on the CPU, convert to 8-bit. The caller re-uploads.
func (b *builder) cpuTonemap(why string) (string, error) {
	if !b.host.CPUTonemap {
		return "", fmt.Errorf("%w: HDR source and no tone-mapper (%s)", ErrRefused, why)
	}
	b.fallback("tonemap", why)
	return "hwdownload,format=p010le," + hdrToSDRChain + ",format=nv12", nil
}

func (b *builder) vaapi() error {
	node := b.host.RenderNode
	if node == "" {
		node = renderNode()
	}
	b.p.PreInput = []string{"-init_hw_device", "vaapi=va:" + node, "-filter_hw_device", "va"}
	var f []string
	if b.hardwareDecodes() {
		b.p.PreInput = append(b.p.PreInput, "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi")
		if b.src.Interlaced {
			f = append(f, "deinterlace_vaapi")
		}
	} else {
		if b.src.Interlaced {
			f = append(f, "bwdif=mode=send_frame")
		}
		f = append(f, "format="+b.cpuPixelFormat(), "hwupload")
	}
	switch {
	case !b.hdr:
		f = append(f, "scale_vaapi="+b.fit()+":format=nv12")
	case b.host.TonemapVAAPI:
		// Scale BEFORE the tone-map: tone-mapping at 4K runs 0.7x on the A380, at 1080p 2.6x.
		f = append(f, "scale_vaapi="+b.fit()+":format=p010", "tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709")
	default:
		tm, err := b.cpuTonemap("no tonemap_vaapi on this GPU")
		if err != nil {
			return err
		}
		f = append(f, "scale_vaapi="+b.fit()+":format=p010", tm, "hwupload")
	}
	f = append(f, fmt.Sprintf("pad_vaapi=w=%d:h=%d", b.out.Width, b.out.Height), b.tail())
	b.p.VideoFilter = strings.Join(f, ",")
	return nil
}

func (b *builder) nvenc() error {
	b.p.PreInput = []string{"-init_hw_device", "cuda=cu:0"}
	filterDevice := "cu"
	hw := b.hardwareDecodes()
	var f []string
	if hw {
		if b.src.Interlaced {
			f = append(f, "bwdif_cuda=mode=send_frame")
		}
	} else {
		if b.src.Interlaced {
			f = append(f, "bwdif=mode=send_frame")
		}
		f = append(f, "format="+b.cpuPixelFormat(), "hwupload_cuda")
	}
	if !b.hdr {
		f = append(f, "scale_cuda="+b.fit()+":format=nv12")
	} else {
		// Every HDR path scales in CUDA first and tone-maps the 1080p 10-bit frame.
		f = append(f, "scale_cuda="+b.fit()+":format=p010le")
		switch {
		case b.host.TonemapOpenCL:
			// ffmpeg cannot map CUDA frames to OpenCL, so the downscaled frame hops through system
			// memory both ways. Measured on the dev GeForce: see the PR's numbers.
			b.fallback("tonemap", "tonemap_opencl: the 1080p frame is copied through system memory")
			filterDevice = "ocl"
			b.p.PreInput = append(b.p.PreInput, "-init_hw_device", "opencl=ocl")
			f = append(f, "hwdownload", "format=p010le", "hwupload",
				"tonemap_opencl=tonemap=hable:desat=0:t=bt709:m=bt709:p=bt709:r=tv:format=nv12",
				"hwdownload", "format=nv12", "hwupload_cuda")
		case b.host.Libplacebo:
			// libplacebo on its own Vulkan device: ffmpeg's Vulkan hwaccel device fails to init
			// with it on the dev GeForce, and the per-spawn device costs ~2 s of start (spike §7).
			b.fallback("tonemap", "libplacebo: the 1080p frame is copied through system memory")
			f = append(f, "hwdownload", "format=p010le",
				"libplacebo=format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping=bt.2390",
				"hwupload_cuda")
		default:
			tm, err := b.cpuTonemap("neither tonemap_opencl nor libplacebo works on this host")
			if err != nil {
				return err
			}
			f = append(f, tm, "hwupload_cuda")
		}
	}
	b.p.PreInput = append(b.p.PreInput, "-filter_hw_device", filterDevice)
	if hw {
		b.p.PreInput = append(b.p.PreInput, "-hwaccel", "cuda", "-hwaccel_device", "cu", "-hwaccel_output_format", "cuda")
	}
	f = append(f, fmt.Sprintf("pad_cuda=w=%d:h=%d:x=-1:y=-1", b.out.Width, b.out.Height), b.tail())
	b.p.VideoFilter = strings.Join(f, ",")
	return nil
}

// videotoolbox: VT decode and scale_vt on the GPU. VideoToolbox has no pad and no tone-map filter,
// so the scaled frame is downloaded and padded (and, for HDR, tone-mapped) on the CPU at output
// size; h264_videotoolbox uploads it. scale_vt has no aspect-fit option, so the fitted size is
// computed here from the source facts. UNVERIFIED on a real Mac.
func (b *builder) videotoolbox() error {
	fw, fh, sized := fitSize(b.src.Width, b.src.Height, b.out.Width, b.out.Height)
	var f []string
	if sized && b.hardwareDecodes() {
		b.p.PreInput = []string{"-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox_vld"}
		if b.src.Interlaced {
			f = append(f, "yadif_videotoolbox")
		}
		f = append(f, fmt.Sprintf("scale_vt=w=%d:h=%d", fw, fh), "hwdownload")
	} else {
		if !sized {
			b.fallback("decode", "source geometry unknown")
		}
		if b.src.Interlaced {
			f = append(f, "bwdif=mode=send_frame")
		}
		f = append(f, "scale="+b.fit())
	}
	if b.hdr {
		if !b.host.CPUTonemap {
			return fmt.Errorf("%w: HDR source and no tone-mapper", ErrRefused)
		}
		b.fallback("tonemap", "VideoToolbox has no tone-map filter")
		f = append(f, "format=p010le", hdrToSDRChain)
	}
	f = append(f, "format=nv12", fmt.Sprintf("pad=%d:%d:-1:-1", b.out.Width, b.out.Height), b.tail())
	b.p.VideoFilter = strings.Join(f, ",")
	return nil
}

// software: libx264 on the CPU. HDR is downscaled first and tone-mapped at no more than 720 lines
// (maintainer decision), and refused when the host profile says the CPU cannot keep up.
func (b *builder) software() error {
	var f []string
	if b.src.Interlaced {
		f = append(f, "bwdif=mode=send_frame")
	}
	if b.hdr {
		if !b.host.SoftwareHDR || !b.host.CPUTonemap {
			return fmt.Errorf("%w: 4K HDR tone-mapping needs more CPU than this host has", ErrRefused)
		}
		w, h := b.out.Width, b.out.Height
		if h > softwareTonemapLines {
			w, h = even(w*softwareTonemapLines/h), softwareTonemapLines
		}
		f = append(f, fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=decrease:force_divisible_by=2", w, h), hdrToSDRChain)
	}
	f = append(f, "scale="+b.fit(), "format=yuv420p",
		fmt.Sprintf("pad=%d:%d:-1:-1", b.out.Width, b.out.Height), b.tail())
	b.p.VideoFilter = strings.Join(f, ",")
	return nil
}

const softwareTonemapLines = 720

// generic: the designed fallback for encoders outside the certified matrix. Hardware decode without
// an output format (ffmpeg downloads the frames, and falls back to the CPU on its own for anything the
// GPU cannot decode), CPU filters, then the encoder's own upload step.
func (b *builder) generic() error {
	enc := b.host.Encoder
	b.p.PreInput = deviceInitArgs(enc)
	if len(b.host.DecodeCodecs) > 0 {
		b.p.PreInput = append(b.p.PreInput, hardwareDecodeArgs(enc)...)
	}
	b.fallback("filters", "no GPU graph for "+string(enc))
	var f []string
	if b.src.Interlaced {
		f = append(f, "bwdif=mode=send_frame")
	}
	f = append(f, "scale="+b.fit(), fmt.Sprintf("pad=%d:%d:-1:-1", b.out.Width, b.out.Height))
	if b.hdr {
		if !b.host.CPUTonemap {
			return fmt.Errorf("%w: HDR source and no tone-mapper", ErrRefused)
		}
		f = append(f, hdrToSDRChain)
	}
	f = append(f, b.tail())
	if up := hardwareUploadFilter(enc); up != "" {
		f = append(f, up)
	} else {
		f = append(f, "format=yuv420p")
	}
	b.p.VideoFilter = strings.Join(f, ",")
	return nil
}

// videoEncoder maps the uniform output onto each family's encoder. Rate control, per family:
//
//	VAAPI         QVBR: -rc_mode QVBR -global_quality Q, target and cap (exact).
//	NVENC         VBR with a constant-quality target: -rc vbr -cq Q, target and cap.
//	software      capped CRF: -crf Q with the cap (x264 has no average target alongside CRF).
//	VideoToolbox  average bitrate with the cap (-q:v would drop the bitrate limits).
//	generic       average bitrate with the cap.
//
// Every family: H.264 High (HEVC Main), no B-frames, one closed GOP per segment, IDR at every
// keyframe, no scene-cut keyframes.
func videoEncoder(host HostProfile, out OutputProfile) []string {
	g := strconv.Itoa(out.gop())
	target, maxrate := strconv.Itoa(out.TargetKbps)+"k", strconv.Itoa(out.MaxKbps)+"k"
	q := strconv.Itoa(out.Quality)
	profile := "high"
	if out.HEVC {
		profile = "main"
	}
	enc := func(h264 Encoder) string {
		if out.HEVC {
			return string(hevcVariant(h264))
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
		if out.HEVC {
			return []string{"-c:v", enc(EncoderSoftware), "-preset", "veryfast", "-profile:v", profile,
				"-crf", q, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-keyint_min", g, "-bf", "0",
				"-x265-params", "open-gop=0:scenecut=0:repeat-headers=1"}
		}
		return []string{"-c:v", enc(EncoderSoftware), "-preset", "veryfast", "-profile:v", profile,
			"-crf", q, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-keyint_min", g, "-sc_threshold", "0",
			"-bf", "0", "-x264-params", "open-gop=0"}
	case FamilyVideoToolbox:
		return []string{"-c:v", enc(EncoderVideoToolbox), "-profile:v", profile, "-allow_sw", "0", "-realtime", "1",
			"-b:v", target, "-maxrate", maxrate, "-bufsize", maxrate, "-g", g, "-bf", "0"}
	default:
		return []string{"-c:v", enc(engineOf(host.Encoder)), "-b:v", target, "-maxrate", maxrate, "-bufsize", maxrate,
			"-g", g, "-bf", "0"}
	}
}

// missingFacts lists the stream facts minimal probing depends on that the source does not supply.
// The container must declare its streams in a header (Matroska, MP4/MOV): MPEG-TS, AVI and other
// packet-discovered containers need ffmpeg's probe to find their streams at all.
func missingFacts(f MediaFormat) []string {
	var missing []string
	add := func(ok bool, name string) {
		if !ok {
			missing = append(missing, name)
		}
	}
	add(f.VideoCodec != "", "video codec")
	add(f.Width > 0 && f.Height > 0, "geometry")
	add(f.FrameRate > 0, "frame rate")
	add(f.PixelFormat != "", "pixel format")
	add(f.AudioCodec != "", "audio codec")
	add(f.AudioChannels > 0, "audio layout")
	add(f.AudioSampleRate > 0, "audio sample rate")
	add(headerContainer(f.Container), "header-declared container")
	return missing
}

func headerContainer(c string) bool {
	return strings.Contains(c, "matroska") || strings.Contains(c, "mp4") || strings.Contains(c, "mov")
}

// fitSize is force_original_aspect_ratio=decrease:force_divisible_by=2 computed in Go, for scalers
// without the option. ok is false when the source geometry is unknown.
func fitSize(srcW, srcH, w, h int) (int, int, bool) {
	if srcW <= 0 || srcH <= 0 {
		return 0, 0, false
	}
	if srcW*h > srcH*w { // wider than the output: width-bound
		return w, even(srcH * w / srcW), true
	}
	return even(srcW * h / srcH), h, true
}

func even(n int) int { return n &^ 1 }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
