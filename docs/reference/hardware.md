# Hardware

**For:** household admins choosing hardware, or checking what theirs can do.
**You'll get:** which encoders and GPUs Loomarr uses, where HDR tone mapping runs, and what each
path costs. To set a GPU up, follow [Set up hardware encoding](../guides/hardware-encoding.md).

Everything here applies when Loomarr does the streaming (the default). On the Tunarr backend,
Tunarr's own transcode settings apply.

## Platforms

| Host | Hardware encoding |
| --- | --- |
| Linux, Intel or AMD GPU | VAAPI and Vulkan on amd64 and arm64; QSV on amd64 only (Intel's `intel-media-va-driver` has no arm64 build) |
| Linux, NVIDIA GPU | NVENC, through the NVIDIA container toolkit |
| Docker Desktop on macOS | None: the Mac GPU isn't passed to the Linux container. Playout runs in software |
| No GPU | Software encoding. Everything works; fewer channels play at once |

The image ships the Intel and AMD driver libraries. NVIDIA's driver comes from the container
toolkit, not the image.

## How the encoder is chosen

At boot, Loomarr trial-encodes with every encoder its ffmpeg reports and keeps the ones that
produce output. That result sets both the encoder and how many channels can stream at once. The
boot log names every encoder family that passed.

A device file alone proves nothing. On a host where `/dev/dri/renderD128` belongs to an NVIDIA
card, a device check would pick a VAAPI encoder that then fails at tune time. That's why
`PLAYOUT_ENCODER` should stay empty.

`PLAYOUT_MAX_CHANNELS` defaults to `0`, which uses the capacity the trial measured. A channel
that needs a full transcode counts against it; one that's copied through doesn't. A positive value
can only lower the budget, never raise it above what the trial proved.

## HDR tone mapping by GPU

An HDR film on an SDR channel is tone-mapped to SDR. Decoding, scaling and encoding stay on the GPU
on every row; only the tone mapping moves.

| GPU | Examples | Tone mapping runs on |
| --- | --- | --- |
| Intel Gen12 and newer | Tiger Lake, Alder Lake (incl. N100/N305), Raptor Lake, Arc A-series and B-series, Meteor Lake and later | The GPU, through OpenCL (Intel's compute runtime ships in the amd64 image) |
| Intel Gen8 to Gen11 | Broadwell, Skylake, Kaby Lake, Coffee Lake, Gemini Lake, Ice Lake, Elkhart Lake | The CPU, after the GPU scales the picture down. Their legacy runtime would add about 550 MB to the image, so it isn't included |
| AMD (VAAPI) | Radeon | libplacebo (Vulkan) through system memory, then the CPU. Not yet measured on AMD |
| NVIDIA | Any NVENC card | The GPU, through OpenCL. The overlay's `compute` capability brings in NVIDIA's OpenCL library; the image ships the `nvidia.icd` file that points to it |

The CPU path draws the same curve and a correct picture, at more CPU per HDR stream.

Loomarr never uses the VAAPI tone-mapper (`tonemap_vaapi`): on Arc it produces a black picture.

The first HDR stream after a container start takes about a second longer while Intel's runtime
compiles its kernels. When a tone-mapper can't start, the log line
`HDR tone-map produced nothing — retrying with the next tone-mapper` names the fallback it took.

## HDR tone curves

`playout.tone_curve` (Settings → Playback) picks the curve. A change applies to streams that start
afterwards.

| Curve | Intel | NVIDIA | No GPU tone-mapper |
| --- | --- | --- | --- |
| Hable (default), Mobius, Reinhard | GPU, no copy (about 0.1 cores per HDR stream) | GPU (OpenCL) | CPU, same curve |
| BT.2390, BT.2446 Method A, Spline | libplacebo, through system memory (about 0.25 cores per HDR stream) | CPU, Mobius curve | CPU, Mobius curve |

The libplacebo curves need a Vulkan driver. NVIDIA's container runtime doesn't provide one today,
so on NVIDIA they fall back to the CPU. The CPU tone-mapper has no BT.2390, BT.2446 Method A or
Spline, so it uses Mobius, which, like those three, keeps the normal range linear and only rolls
off the highlights. The log says when that happens. Every path produces a picture: when one
tone-mapper can't start, the next takes over.
