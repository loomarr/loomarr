# Set up hardware encoding

**For:** household admins with an Intel, AMD or NVIDIA GPU on a Linux host.
**You'll get:** the GPU passed to Loomarr, so it can play more channels at once, including 4K HDR.

Hardware passthrough is a Linux-host capability. Docker Desktop on macOS does not expose the Mac
GPU to this Linux container; use software playout there and size channel capacity accordingly.

This only applies when Loomarr does the streaming (the default). On the Tunarr backend, Tunarr's
own transcode settings apply.

It's optional. Without a GPU, Loomarr encodes in software — everything works, you just run fewer
channels at once.

## How Loomarr picks an encoder

At boot it trial-encodes with every encoder your ffmpeg reports and keeps the ones that actually
produce output. That result sets both the encoder and how many channels can stream at once.

Leave `PLAYOUT_ENCODER` empty so this measurement stands. Checking for a device file isn't
enough: on a box where `/dev/dri/renderD128` belongs to an NVIDIA card, that check picks a
VAAPI encoder that fails at tune time.

The boot log names every encoder family that passed.

## Intel and AMD

VAAPI, QSV and Vulkan all reach the GPU through `/dev/dri`:

```bash
PLAYOUT_RENDER_DEVICE=/dev/dri docker compose -f docker/compose.yaml --profile sqlite up -d
```

Leaving it unset is fine — the container starts normally on a host with no GPU.

Driver libraries ship in the image. QSV is amd64-only, because `intel-media-va-driver` has no
arm64 build. VAAPI and Vulkan work on both.

### HDR tone mapping on Intel

An HDR film on an SDR channel is tone-mapped to SDR. On Intel that runs on the GPU through OpenCL,
using Intel's compute runtime, which the amd64 image ships. Which GPUs get it depends on the
generation:

| Intel graphics | Examples | HDR tone mapping |
| --- | --- | --- |
| Gen12 and newer | Tiger Lake, Alder Lake (incl. N100/N305), Raptor Lake, Arc A-series and B-series, Meteor Lake and later | On the GPU (OpenCL) |
| Gen8 to Gen11 | Broadwell, Skylake, Kaby Lake, Coffee Lake, Gemini Lake, Ice Lake, Elkhart Lake | On the CPU, after the GPU scales the picture down |
| AMD (VAAPI) | Radeon | Tries libplacebo (Vulkan) through system memory, then the CPU (not yet measured on AMD) |

The CPU path produces the same curve and a correct picture, but it costs more CPU for each HDR
stream. Gen8 to Gen11 need Intel's separate legacy runtime, which adds about 550 MB to the image,
so it is not included. Decoding, scaling and encoding stay on the GPU on every generation above.

Loomarr never uses the VAAPI tone-mapper (`tonemap_vaapi`): on Arc it produces a black picture.

The first HDR stream after a container start takes about a second longer while Intel's runtime
compiles its kernels. When the GPU tone-mapper can't start, the log line
`HDR tone-map produced nothing — retrying with the next tone-mapper` names the fallback it took.

### Picking the right GPU on a multi-GPU host

Loomarr probes the render node `/dev/dri/renderD128` by default. On a box with **more than one
GPU** — a discrete card such as an **Intel Arc** alongside the CPU's integrated graphics, or an Arc
next to an NVIDIA card — the one you want is often `renderD129` (or higher), and which node is which
is not guessable. If the encoder you expect never passes the boot probe on such a host, point Loomarr
at the right node:

```bash
PLAYOUT_RENDER_NODE=/dev/dri/renderD129 \
PLAYOUT_RENDER_DEVICE=/dev/dri \
  docker compose -f docker/compose.yaml --profile sqlite up -d
```

To find which node is your card, list them by device path or ask VAAPI directly:

```bash
ls -l /dev/dri/by-path/                       # maps PCI addresses to renderD12x
vainfo --display drm --device /dev/dri/renderD129   # should list H264/HEVC encode entrypoints
```

The node that reports encode entrypoints (`VAEntrypointEncSlice…`) is the one to set. A single-GPU
host needs none of this — the default `renderD128` is correct.

## NVIDIA

NVENC needs nothing from the image — the NVIDIA container toolkit provides the driver:

```bash
docker compose -f docker/compose.yaml -f docker/compose.nvidia.yaml --profile sqlite up -d
```

The overlay requests `capabilities: [gpu, video]`. **Both are needed** — with only `gpu`, the
container sees the card but every NVENC trial fails.

HDR tone mapping on NVIDIA runs through OpenCL. The overlay's `compute` driver capability brings
NVIDIA's OpenCL library into the container. The image ships the small `nvidia.icd` file that points
the OpenCL loader at it; the NVIDIA container runtime does not provide that file. The libplacebo
curves (see below) need NVIDIA's Vulkan driver, which the container runtime does not currently
provide, so on NVIDIA they fall back to the CPU tone-mapper.

## HDR tone curve

`playout.tone_curve` (Settings → Playback) picks the curve every HDR film is tone-mapped with. A
change applies to streams that start afterwards.

| Curve | Intel | NVIDIA | No GPU tone-mapper |
| --- | --- | --- | --- |
| Hable (default), Mobius, Reinhard | GPU, no copy (about 0.1 cores per HDR stream) | GPU (OpenCL) | CPU, same curve |
| BT.2390, BT.2446 Method A, Spline | libplacebo, through system memory (about 0.25 cores per HDR stream) | CPU, Mobius curve | CPU, Mobius curve |

The CPU tone-mapper has no BT.2390, BT.2446 Method A or Spline, so it uses Mobius, which, like those
three, keeps the normal range linear and only rolls off the highlights. The log says so when it
happens. Every path produces a picture: when one tone-mapper can't start, the next one takes over.

## Concurrent channels

`PLAYOUT_MAX_CHANNELS` defaults to `0` (automatic), so Loomarr uses the per-encoder capacity its
trial measured inside the container. A channel needing a full transcode counts against that budget;
one that can be copied through does not.

Set a positive value only to lower the measured budget when real, complex content needs more
headroom. A configured value can never raise capacity above what the trial proved.

## Checking what happened

```bash
docker logs loomarr 2>&1 | grep -i 'encoder\|capability'
```

`scripts/playout-diag.sh` gives a fuller read-only snapshot: ffmpeg processes per channel, GPU
state, and whether each airing is direct-playing or transcoding.
