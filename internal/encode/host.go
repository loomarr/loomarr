package encode

import (
	"os"
	"strings"
)

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
	// VideoToolboxDeinterlace: this ffmpeg build carries yadif_videotoolbox. Without it,
	// interlaced sources use declared CPU decode/deinterlace before the hardware encoder.
	VideoToolboxDeinterlace bool `json:"videoToolboxDeinterlace,omitempty"`
	// TonemapOpenCL: tonemap_opencl works. The first choice on Intel (zero-copy from VAAPI) and
	// NVIDIA (maintainer decision). There is no tonemap_vaapi: on the household Arc it outputs a
	// black picture at normal speed with no error (#1516), so it is never emitted. On VAAPI it also
	// means the VAAPI→OpenCL surface mapping works, which the 10-bit letterbox needs (#1673).
	TonemapOpenCL bool `json:"tonemapOpencl,omitempty"`
	// Libplacebo: libplacebo on its own Vulkan device works: the first choice for the curves only it
	// has (playout.ToneCurve), otherwise the second GPU choice, and on every GPU family the only
	// correct SDR→HDR10 conversion (4K HDR premium). Always through system memory.
	Libplacebo bool `json:"libplacebo,omitempty"`
	// CPUTonemap: the build has zscale + tonemap, the last-resort tone-map.
	CPUTonemap bool `json:"cpuTonemap,omitempty"`
	// Overlay: this host's GPU overlay passed the boot self-check (playout.WatermarkCheck), which
	// asserts the picture, not the exit code. False keeps every programme bug-free.
	Overlay bool `json:"overlay,omitempty"`
}

// GPUFilters is which optional GPU filters this ffmpeg BUILD carries (playout.GPUFiltersFor).
// Whether the host can run them is a runtime fact: tonemap_opencl needs an OpenCL ICD for the GPU
// (the image ships Intel's; NVIDIA's comes from the container runtime), libplacebo a Vulkan device.
type GPUFilters struct {
	TonemapOpenCL, Libplacebo bool
	VideoToolboxDeinterlace   bool
}

// Hardware decode sets per family: the codecs the certified hardware decodes. Anything else takes
// the decode fallback. AV1 needs Arc/Ampere or newer; older GPUs fault and the ladder's
// software-decode retry (IsHardwareDecodeFault) covers them.
var (
	VAAPIDecodes = []string{"h264", "hevc", "mpeg2video", "vp9", "av1"}
	CUDADecodes  = []string{"h264", "hevc", "mpeg2video", "vc1", "vp9", "av1"}
	VTDecodes    = []string{"h264", "hevc"}
	// AnyCodec lets ffmpeg decide: the generic family's hardware decode has no output format, so
	// ffmpeg falls back to the CPU on its own for a codec the GPU cannot decode.
	AnyCodec = []string{"*"}
)

// HostFor is the host profile for the chosen encoder on this build. A GPU tone-mapper the build
// carries but the host cannot run (no OpenCL ICD for this GPU, e.g. AMD or a missing runtime; no
// Vulkan device) fails at device creation, before any output; the live ladder then demotes it and
// retries with the next one, ending at the CPU tone-map (the packager's itemFaults, demoteTonemapper).
func HostFor(enc Encoder, cpuTonemap bool, gpu GPUFilters) HostProfile {
	h := HostProfile{Encoder: EngineOf(enc), CPUTonemap: cpuTonemap}
	switch h.Encoder {
	case EncoderVAAPI:
		h.Family, h.RenderNode, h.DecodeCodecs = FamilyVAAPI, RenderNode(), VAAPIDecodes
		h.TonemapOpenCL, h.Libplacebo = gpu.TonemapOpenCL, gpu.Libplacebo
	case EncoderNVENC:
		h.Family, h.DecodeCodecs = FamilyNVENC, CUDADecodes
		h.TonemapOpenCL, h.Libplacebo = gpu.TonemapOpenCL, gpu.Libplacebo
	case EncoderVideoToolbox:
		h.Family, h.DecodeCodecs = FamilyVideoToolbox, VTDecodes
		h.Libplacebo = gpu.Libplacebo
		h.VideoToolboxDeinterlace = gpu.VideoToolboxDeinterlace
	case EncoderSoftware:
		h.Family = FamilySoftware
	default:
		h.Family, h.DecodeCodecs = FamilyGeneric, AnyCodec
	}
	return h
}

// DecodeRefusal is why the GPU does NOT decode this source, or "" when it does. An unknown codec is
// treated as undecodable: with -hwaccel_output_format set, a source the GPU cannot decode arrives as
// CPU frames and the GPU graph fails, where the upload path always works.
func (h HostProfile) DecodeRefusal(codec, pixelFormat string, tenBit bool) string {
	codec = strings.ToLower(codec)
	switch {
	case codec == "":
		return "source codec unknown"
	case !contains(h.DecodeCodecs, codec):
		return "the GPU does not decode " + codec
	case !chroma420(pixelFormat):
		return "the GPU does not decode " + pixelFormat
	case codec == "h264" && tenBit:
		return "the GPU does not decode 10-bit H.264"
	}
	return ""
}

// chroma420 reports whether the pixel format is 4:2:0, the only chroma GPUs decode broadly. Unknown
// pixel formats count as 4:2:0: hardware decode then still depends on a known codec.
func chroma420(pixfmt string) bool {
	return pixfmt == "" || strings.Contains(pixfmt, "420") || pixfmt == "nv12" || pixfmt == "p010le"
}

// Env overrides for machine-specific paths. Deliberately NOT registry settings (§15): they
// describe this machine's filesystem, like the ffmpeg path, not app configuration that
// should round-trip through a database.
const (
	// RenderNodeEnv picks the DRM render node for VAAPI. A multi-GPU box has renderD129
	// and up, and the right one is not guessable.
	RenderNodeEnv = "PLAYOUT_RENDER_NODE"
)

// RenderNode is the DRM render node VAAPI and QSV open.
func RenderNode() string {
	if d := os.Getenv(RenderNodeEnv); d != "" {
		return d
	}
	return "/dev/dri/renderD128"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
