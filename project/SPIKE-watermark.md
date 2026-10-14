# Spike: channel watermark samples (#1512, phase 1d)

Design-sample spike. No production code. The scripts in `spike/watermark/` are throwaway. This note records
what the maintainer's sample sheets were made with, what the GPU overlay needs from the pipeline, what it
costs, and the settings fields a watermark would add. The sheets hold film frames and stay local: they are
never committed. Items marked **[maintainer]** need a look or product decision.

Maintainer rules this spike works within (fixed): the bug is burned into the channel stream on programmes
only. Image source order is custom upload > the network's TMDB logo, automatic when ≥ 60% of the lineup's TV
runtime is one network, drawn as a white semi-transparent silhouette from the logo's alpha > a generated
typographic bug (the LLM picks a callsign and a template; a deterministic renderer draws it) > never posters.
On by default, with per-channel enabled/corner/opacity/size/margin. GPU overlay only, per family; a family
that cannot overlay on the GPU does not get a watermark.

## Conditions

- RTX 3080 Ti (driver 615.71.09), native ffmpeg n9.0.2, every encode under the shared GPU lock. The
  production-image checks ran in throwaway `ghcr.io/loomarr/loomarr:0.2.0-beta.7` containers (ffmpeg
  n8.1.2-50-g1a748fe2cd), `--gpus all`, with NVIDIA's OpenCL ICD mounted for the HDR path.
- The graph is the real one: `playout.Build(HostFor(EncoderNVENC, …, GPUFilters{TonemapOpenCL, Libplacebo}),
  src, 1080p24 q22 8M/12M)`, printed for an SDR H.264 source and a 4K PQ HEVC source at base `498d3f77`,
  with the overlay spliced in after `pad_cuda`.
- Sources, genre words only: bright = an animated feature (pale sky filling the top-right, 1.66:1); dark =
  80s horror at night; busy = a 16:9 sitcom episode in a nightclub, chosen as the highest top-right Canny
  edge density among 12 random 1080p episodes; 4K HDR = an HDR10 epic, 2.39:1 letterboxed inside 3840x2160.
  Frames are decoded from the encoded H.264 at t = 2 s. That is what a viewer receives.

## The look proposal

- **Placement:** top-right, margin 5% of width and height (96 / 54 px at 1080p; EBU R95 graphics-safe).
- **Size rule, equal visual weight:** every bug covers the same area, 2·H², with H = 6% of frame height
  (65 px), capped at H tall. A wide wordmark comes out shorter and a square mark reaches H. At 1080p:
  HBO 143x59, NBC 185x45, monogram 65x65, stacked words 130x65. A 6.9:1 wordmark is 238x35; **[maintainer]**
  whether to cap width (for example at 4H).
- **Opacity:** 65% default, baked into the PNG's alpha once at render time. The GPU does a plain blend.
- **Colour:** white everywhere, with shape only in alpha. White has neutral chroma, so 4:2:0 cannot fringe it.
- **Alternative shown:** 4.5% / 85% (smaller, more solid).
- **Soft shadow (proposal, [maintainer]):** a white bug at 65% nearly vanishes on a pale sky (8-bit luma 202
  under the bug against 245 on it). A black shadow at 35% of the bug's opacity, offset H/40 and blurred
  H/30, gives the only dark edge (179 → 238). It is baked into the PNG and costs nothing at runtime. It is
  a deviation from "white silhouette", so it needs approval.

## Image sources

**TMDB network logos.** `GET /3/network/{id}` gives `logo_path` (the primary). `/3/network/{id}/images` lists
the variants, each served as an RGBA PNG. The silhouette is the logo's alpha, trimmed to its bounding box.
Multicolour logos work (NBC's peacock survives because the gaps between feathers are transparent).

- **Thin primary logos fail.** TMDB's primary Nickelodeon logo is the 2023 splat: 1–2 px strokes and an
  11 px wordmark at the default size. It breaks up on the bright frame and disappears on the busy one.
  TMDB's alternative 6.9:1 wordmark for the same network is legible on all four frames. **Picking the logo
  variant matters more than size or opacity.** Proposal: score each variant's alpha after downscaling to
  the target size (fill ratio, or minimum stroke width via erosion) and take the best, not `logo_path`.
- The TMDB key is a v4 bearer token (`Authorization: Bearer`), as `internal/tmdb` already sends it.

**Typographic templates.** Deterministic: drawn at 8x, reduced with Lanczos, no system fonts, no randomness.

| Template | Example | Face |
| --- | --- | --- |
| Plate: callsign knocked out of a rounded plate | RETRO | Geist Bold |
| Monogram: one or two letters in a ring | N | Geist Bold |
| Stacked: two words justified to one width | MIDNIGHT / HORROR | Barlow Condensed ExtraBold |
| Tag: channel number knocked out of a square, then the callsign | 42 KIDS | Barlow Condensed ExtraBold |

Both faces are SIL Open Font License 1.1, so they may be redistributed and bundled in the image (about
129 KB and 110 KB static TTFs, shipped with their `OFL.txt`). Geist is the web UI's own `--font-sans`. The
image ships only `fonts-dejavu-core` today. The spike renders with Pillow; a production renderer would be
Go, which needs `golang.org/x/image` (opentype and draw), a module the repo does not use yet.

## What the GPU overlay needs (NVENC)

1. **`overlay_cuda` blends alpha only onto a `yuv420p` main with a `yuva420p` overlay.** On today's nv12 main
   it refuses (`Can't overlay yuva420p on nv12`). An nv12 overlay has no alpha, so the bug would be an
   opaque box. The bug-on graph asks `scale_cuda` for `yuv420p` (or converts just before the overlay).
2. **`overlay_cuda` outputs the decoder's aligned 1920x1088 surface.** That adds 8 garbage (green) rows, and
   the SPS has no cropping (`pic_height_in_map_units_minus1 = 67`, `frame_cropping_flag = 0`). Programmes
   (bug on) and breaks (bug off) would carry different SPS in one channel stream. **Fix:**
   `scale_cuda=w=1920:h=1080:format=yuv420p:passthrough=0` right after the overlay. The bug-on SPS is then
   byte-identical to the bug-off graph's on both ffmpeg builds. It is a relabel, not a squash: the bottom
   half of bug-on vs bug-off is bit-identical (PSNR inf; a one-row shift scores 36.5 dB). A post-scale to
   `nv12` also works on n9 but faults with `CUDA_ERROR_ILLEGAL_ADDRESS` on n8.1.2.
3. **The bug is one still frame**, uploaded once. `overlay_cuda` repeats it (`repeatlast`): bug-box peak luma
   is 170 / 171 / 173 at 1 / 15 / 29.5 s, with 720 of 720 frames at 1080 lines.
4. **Blocker on the shipping ffmpeg.** In the beta.7 image (n8.1.2), `overlay_cuda` **drops the programme
   picture**: the output is all green with only the bug drawn, exit 0, correct SPS. This happens whenever
   the process decodes on NVDEC, which is every production NVENC graph:

   | beta.7 image, n8.1.2 | Picture (bottom-half YAVG, bug off = 65.7 SDR / 82.7 HDR) |
   | --- | --- |
   | naive yuv420p main | 0 (and 1088 lines) |
   | fix 2 (post-scale, passthrough=0) | 0 |
   | nv12 chain, converted to yuv420p just before the overlay | 0 |
   | HDR production path (tonemap_opencl → hwupload_cuda) + overlay | 0 |
   | CPU decode + `hwupload_cuda` + overlay (not a production graph) | 65.7 (correct) |
   | NVDEC + system-memory hop before the overlay | error (`Input frame is not in the configured hwframe context`) |

   Native n9.0.2 gives the correct picture in every variant. The image cannot move to ffmpeg 9: it breaks
   the concat advance (see the Dockerfile's n8.1 pin). `overlay_opencl` on the HDR path was tried: uploading
   the `yuva420p` bug to NVIDIA OpenCL frames failed (`Failed to allocate frame`). That path is unexplored.
   This is the #1516 class: full speed, exit 0, wrong picture. **Any watermark gate must assert picture
   luma, not only speed.** **[maintainer]** The choices: find and backport the upstream `overlay_cuda` fix
   into the pinned n8.1 build; ship NVENC with the watermark off (the "GPU or nothing" rule); or keep
   digging on `overlay_opencl`.
5. The channel icon upload stores JPEG renditions (`w500.jpg`), which have no alpha. A custom watermark
   upload can reuse the upload flow, but it needs an alpha-preserving rendition role in the image service.
6. `overlay_vaapi` (Arc) and VideoToolbox were not tested here. Arc needs the supervisor's household check,
   including a picture-luma assertion.

## Cost per stream (native n9.0.2, NVENC)

30 s of content, video only, unpaced, median of 3. `c` = CPU cores to run the stream at real time.

| Graph | Run 1: fix A, load 4.8–5.6 on 24 cores | Run 2: fix 2 (final), load 6 → 16 |
| --- | --- | --- |
| SDR 1080p, bug off → on | 15.4x → 13.8x; 0.029 → 0.042 c | 15.5x → 14.1x; 0.022 → 0.031 c |
| 4K HDR10 → 1080p (tonemap_opencl), off → on | 8.8x → 7.7x; 0.152 → 0.165 c | 10.2x → 7.3x; 0.129 → 0.166 c |
| 4 concurrent SDR, per stream, off → on | 4.3–4.4x → 3.9–4.2x (−6% aggregate); 0.040 → 0.062 c | 4.3–4.5x → 3.5–3.6x (−19%); 0.027 → 0.055 c |

Read: about +0.01–0.04 cores per stream at real time, and 9–28% of unpaced GPU headroom. The high end of
each range coincides with rising machine load from other lanes, so treat it as an upper bound. Where the
extra CPU goes was not measured.

## Anchoring **[maintainer]**

The bug is anchored to the frame's safe area. On the letterboxed 4K film it lands in the black bar, where it
is clean and legible. It sits in the same place on every programme; the alternative, anchoring to the
active picture, would make it move between programmes with different aspect ratios. Proposal: keep the frame
anchor.

## Settings fields (not designed; for the maintainer)

Install defaults go in the settings subsystem (env > db > default, hot-apply) under `playout.watermark.*`,
shown on **Settings → Channel defaults** (`/settings/defaults`) beside `filler.breaks_per_hour`:

| Key | Type | Default |
| --- | --- | --- |
| `playout.watermark.enabled` | bool | true |
| `playout.watermark.corner` | enum `top_right` / `top_left` / `bottom_right` / `bottom_left` | `top_right` |
| `playout.watermark.size` | percent of frame height | 6 |
| `playout.watermark.opacity` | percent | 65 |
| `playout.watermark.margin` | percent of frame (safe area) | 5 |

Per-channel overrides go in `schedule.OperatorPolicy` as a `Watermark` block of pointers, where nil means
"follow the default", matching `BreaksPerHour` and `BreakDuration`: `enabled *bool`, `corner *enum`,
`size`, `opacity` and `margin *int`. The source is per channel only: `mode` (auto / custom / typographic),
`callsign` (text; the LLM proposes it and it can be edited), `template` (enum of the curated templates),
`customImage` (an image-service hash with alpha), and optionally `networkLogo` (a chosen TMDB variant).
What auto resolved to is derived, not stored ("NBC: 72% of TV runtime"). The enums reach the client as
orval symbols.

The nearest existing patterns on the channel detail page, for the supervisor to take to the maintainer:

- **Image source:** `ChannelIconField` on the **Channel info** tab
  (`web/apps/web/src/components/loomarr/channels/channel-icon-field`). It shows one current preview with
  several ways in (lineup suggestions, upload, URL) and a clear button. It gates its own affordances on
  `isAdmin` and sits on the info panel because "what the family sees in the guide" belongs there. The bug is
  also what the family sees.
- **Inherit/override knobs:** the Filler tab's `Break frequency` and `Break length` selects
  (`web/apps/web/src/filler/channel-filler/channel-filler.tsx`). They offer "Follow default (N) / Off /
  Custom", link to Channel defaults, and PATCH the policy wholesale (mind the 0 = inherit vs 0 = real
  sentinel trap).

## Carry-overs

- The ffmpeg n8.1.2 `overlay_cuda` picture loss (finding 4) needs a decision before any NVENC watermark code.
- Arc `overlay_vaapi` and VideoToolbox: untested.
- Logo variant scoring (thin-stroke rejection) is a proposal, not implemented.
- The ≥ 60% network rule needs per-series network runtime. The catalog holds `Networks` names, and the TMDB
  network id is resolved on demand.
