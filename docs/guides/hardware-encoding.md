# Set up hardware encoding

**For:** household admins with an Intel, AMD or NVIDIA GPU on a Linux host.
**You'll get:** the GPU passed to Loomarr, so it can play more channels at once, including 4K HDR.

It's optional. Without a GPU, Loomarr encodes in software: everything works, you just run fewer
channels at once. This only applies when Loomarr does the streaming (the default); on the Tunarr
backend, Tunarr's own transcode settings apply. Docker Desktop on macOS can't pass the Mac GPU
through, so there Loomarr always encodes in software.

Which GPUs do what, including HDR tone mapping, is in the
[hardware reference](https://mantonx.github.io/loomarr/reference/hardware/).

## Intel and AMD

VAAPI, QSV and Vulkan all reach the GPU through `/dev/dri`. Pass it in:

```bash
PLAYOUT_RENDER_DEVICE=/dev/dri docker compose -f docker/compose.yaml --profile sqlite up -d
```

Leaving it unset is fine: the container starts normally on a host with no GPU.

### More than one GPU

Loomarr probes the render node `/dev/dri/renderD128` by default. With more than one GPU, such as an
Intel Arc beside the CPU's integrated graphics, or an Arc beside an NVIDIA card, the one you want is
often `renderD129` or higher. A single-GPU host can skip this section.

1. Find the node that can encode. It reports `VAEntrypointEncSlice…` entrypoints:

   ```bash
   ls -l /dev/dri/by-path/
   ```

   ```bash
   vainfo --display drm --device /dev/dri/renderD129
   ```

2. Point Loomarr at it:

   ```bash
   PLAYOUT_RENDER_NODE=/dev/dri/renderD129 \
   PLAYOUT_RENDER_DEVICE=/dev/dri \
     docker compose -f docker/compose.yaml --profile sqlite up -d
   ```

## NVIDIA

Install the NVIDIA container toolkit on the host, then add the NVIDIA overlay:

```bash
docker compose -f docker/compose.yaml -f docker/compose.nvidia.yaml --profile sqlite up -d
```

The overlay asks for `capabilities: [gpu, video]`. Keep both: with only `gpu`, the container sees
the card but every NVENC trial fails.

## Check it worked

At boot, Loomarr trial-encodes with every encoder it finds and keeps the ones that produce output.
Leave `PLAYOUT_ENCODER` empty so that measurement stands. The boot log names every encoder family
that passed:

```bash
docker logs loomarr 2>&1 | grep -i 'encoder\|capability'
```

If the encoder you expect is missing, check the device or overlay above; on a multi-GPU host, the
render node. `scripts/playout-diag.sh` gives a fuller read-only snapshot: ffmpeg processes per
channel, GPU state, and whether each airing is direct-playing or transcoding.

## Choose an HDR tone curve

An HDR film on an SDR channel is tone-mapped. The default curve, Hable, is the cheapest on every
GPU. To change it, set **HDR tone curve** in **Settings → Playback**; streams that start afterwards use
the new curve. The reference lists what each curve costs on each GPU.

## Channels at once

Loomarr sets how many channels can transcode at once from what its boot trial measured. Leave
`PLAYOUT_MAX_CHANNELS` at `0` (automatic). Set a lower number only if complex content needs more
headroom; it can never raise the limit above what the trial proved.
