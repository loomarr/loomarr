# Hardware acceleration

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

## NVIDIA

NVENC needs nothing from the image — the NVIDIA container toolkit provides the driver:

```bash
docker compose -f docker/compose.yaml -f docker/compose.nvidia.yaml --profile sqlite up -d
```

The overlay requests `capabilities: [gpu, video]`. **Both are needed** — with only `gpu`, the
container sees the card but every NVENC trial fails.

## Concurrent channels

`PLAYOUT_MAX_CHANNELS` (default 4) caps how many stream at once. Loomarr also computes a
per-encoder capacity from the boot trials, and a channel needing a full transcode counts for
more than one that can be copied through.

Raise the cap only as far as your hardware measured. Setting it higher causes stuttering, not
capacity.

## Checking what happened

```bash
docker logs loomarr 2>&1 | grep -i 'encoder\|capability'
```

`scripts/playout-diag.sh` gives a fuller read-only snapshot: ffmpeg processes per channel, GPU
state, and whether each airing is direct-playing or transcoding.
