# Hardware acceleration

Only relevant on the default (`internal`) playout backend, where Loomarr does the encoding.
On the Tunarr backend, Tunarr's own transcode settings apply instead.

**It is optional.** With no GPU passed through, Loomarr encodes in software and everything
works — you simply run fewer channels at once. Nothing fails to start.

## How Loomarr picks an encoder

It **measures**, rather than detecting. At boot it enumerates the encoders your ffmpeg reports
and *trial-encodes with each one*, keeping only those that actually produce output. The result
sets both the chosen encoder and how many channels may stream concurrently.

This matters because the obvious shortcut is wrong: the presence of `/dev/dri/renderD128` does
not mean VAAPI works. On a box where that node belongs to an NVIDIA card, a file-existence check
picks a broken encoder and every channel fails at tune time.

So **leave `PLAYOUT_ENCODER` empty.** Setting it replaces a measurement with a guess. The boot
log names every family that passed, which is how you answer "did my GPU get picked up?"

## Intel and AMD — VAAPI, QSV, Vulkan

All reach the GPU through `/dev/dri`. Pass it through:

```bash
PLAYOUT_RENDER_DEVICE=/dev/dri docker compose -f docker/compose.yaml --profile sqlite up -d
```

The compose file guards this behind an env default, so leaving it unset starts fine on a host
with no GPU rather than failing the whole stack.

The driver libraries ship in the image. One arch caveat: `intel-media-va-driver` has **no arm64
build**, so QSV is amd64-only. VAAPI and Vulkan work on both.

## NVIDIA — NVENC

NVENC needs **nothing from the image** — the NVIDIA container toolkit injects the driver and
devices. Use the overlay:

```bash
docker compose -f docker/compose.yaml -f docker/compose.nvidia.yaml --profile sqlite up -d
```

> ⚠ **The `video` capability is not optional.** The overlay requests
> `capabilities: [gpu, video]`. Omitting `video` is the reason a GPU that `nvidia-smi` can see
> still fails every `h264_nvenc` trial — the container gets compute access but not the encoder.

## How many channels

`PLAYOUT_MAX_CHANNELS` (default 4) caps concurrent streams. Loomarr also computes a per-encoder
capacity at boot from what the trials measured, and admission is cost-aware — a channel that
needs a full transcode counts for more than one that can be copied through directly.

Raise the cap only as far as your hardware sustained in the probe. Setting it high does not
create capacity; it creates stuttering.

## Checking what happened

```bash
docker logs loomarr 2>&1 | grep -i 'encoder\|capability'
```

`scripts/playout-diag.sh` gives a fuller read-only snapshot of live playout — the ffmpeg
processes per channel, GPU state, and whether each airing is direct-playing or transcoding.
