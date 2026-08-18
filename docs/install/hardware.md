# Hardware acceleration

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
