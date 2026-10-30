package encode

import "strings"

// DeviceInitArgs returns args that must appear BEFORE the input — hardware device setup is
// a global option, and placing it after `-i` silently applies to nothing.
func DeviceInitArgs(enc Encoder) []string {
	switch EngineOf(enc) {
	case EncoderVAAPI:
		return []string{"-vaapi_device", RenderNode()}
	case EncoderQSV:
		// QSV needs an EXPLICIT device when a filter uploads frames to it. Found by the
		// probe on this box: without it the failure is not a QSV error at all but
		// "[hwupload] A hardware device reference is required to upload frames to" —
		// which reads like a filter bug and sends you looking in the wrong place.
		return []string{"-init_hw_device", "qsv=hw:" + RenderNode(), "-filter_hw_device", "hw"}
	case EncoderVulkan:
		return []string{"-init_hw_device", "vulkan=vk:0", "-filter_hw_device", "vk"}
	default:
		// nvenc, amf, videotoolbox, rkmpp and v4l2m2m initialise their own context.
		return nil
	}
}

// UploadFilter returns the filter that moves frames into GPU memory, which some
// hardware encoders require and which is the most common reason a "supported" encoder
// fails at init.
//
// Kept minimal deliberately: the generic family encodes a NORMALIZED picture, so there is no
// scaling to do here. That sidesteps the per-encoder scaler differences that dominate viewra's
// and ErsatzTV's filter code — no `pad_qsv` exists, `scale_qsv` ignores
// force_original_aspect_ratio, and `scale_cuda` needs a conditional hwupload when the
// source was software-decoded.
func UploadFilter(enc Encoder) string {
	switch EngineOf(enc) {
	case EncoderVAAPI:
		// nv12 first: hwupload will not accept the yuv420p a lavfi source produces.
		return "format=nv12,hwupload"
	case EncoderQSV:
		// extra_hw_frames gives the encoder a pool deep enough for its lookahead; the
		// default is too small and shows up as intermittent frame-allocation failures.
		return "format=nv12,hwupload=extra_hw_frames=64"
	case EncoderVulkan:
		return "format=nv12,hwupload"
	default:
		// nvenc, amf, videotoolbox, rkmpp and v4l2m2m accept CPU frames directly;
		// software needs nothing.
		return ""
	}
}

// DecodeArgs returns the `-hwaccel` args that move DECODING onto the GPU, for a caller whose
// filters run on the CPU (the generic family, the capability trial).
//
// Decoding, not encoding, is where the CPU actually goes on high-resolution sources. Measured
// on a 4K 10-bit HEVC film with an RTX 3080 Ti:
//
//	CPU decode + CPU scale + CPU encode (libx264)   260% CPU
//	CPU decode + CPU scale + GPU encode (nvenc)     341% CPU   ← encode moved, cost went UP
//	GPU decode + CPU scale + GPU encode (nvenc)     ~0% CPU    ← this
//
// The middle row is the instructive one: moving only the encode made CPU *rise*, because the
// decode was always the real cost and it had simply been throttled by waiting on a slow
// software encoder. "GPU encoding" without GPU decoding buys very little on 4K.
//
// ⚠ NO `-hwaccel_output_format`, deliberately. Setting it keeps decoded frames in GPU memory,
// which is faster still — and turns any unsupported input into a HARD FAILURE instead of a
// silent, correct fallback to software decode. ffmpeg cannot hardware-decode every codec
// (VC-1, some VP9 profiles, anything the GPU generation predates), and a channel must not die
// because one film in its lineup is an odd codec. Without the flag ffmpeg downloads frames to
// CPU memory after decoding, which costs a copy and keeps the CPU filters working. The certified
// families keep frames on the GPU instead, and decide per source with HostProfile.DecodeRefusal.
func DecodeArgs(enc Encoder) []string {
	switch EngineOf(enc) {
	case EncoderNVENC:
		return []string{"-hwaccel", "cuda"}
	case EncoderVAAPI, EncoderQSV:
		// Both decode through VAAPI on Linux. The device comes from DeviceInitArgs, which
		// already ran — `-hwaccel vaapi` reuses it rather than opening a second one.
		return []string{"-hwaccel", "vaapi"}
	case EncoderVideoToolbox:
		return []string{"-hwaccel", "videotoolbox"}
	case EncoderVulkan:
		return []string{"-hwaccel", "vulkan"}
	default:
		// Software, AMF, RKMPP and V4L2M2M: no hardware decode offered here.
		//
		// AMF is an ENCODE-only interface — its decode path is a separate AMD stack that this
		// ffmpeg build does not wire up. RKMPP and V4L2M2M are SBC encoders whose decoders are
		// unreliable enough across kernels that asking for them costs more failures than it
		// saves cycles. And for libx264 a GPU decode would mean decoding on the GPU only to
		// download every frame for a CPU encode — strictly slower than decoding on the CPU.
		return nil
	}
}

// IsHardwareDecodeFault reports whether an ffmpeg stderr line says the GPU DECODER failed on its
// input, e.g. "[AVHWFramesContext @ 0x…] Failed to sync surface 0xc: 23 (internal decoding error)"
// from iHD/VAAPI. That is a fault of decoding this source, distinct from encoder capacity or VRAM
// contention: retrying the same `-hwaccel` path fails identically, so recovery must decode in
// software (the packager's itemFaults retry) while the encode stays on hardware.
func IsHardwareDecodeFault(line string) bool {
	return strings.Contains(line, "Failed to sync surface") || strings.Contains(line, "internal decoding error")
}
