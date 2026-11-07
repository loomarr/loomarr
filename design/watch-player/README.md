# Watch player — the mock of record (#2088)

The design for **every Loomarr video player**: web Watch first, then the phone and TV clients
(maintainer, 2026-10-29). The rules it follows are written down in
[`docs/frontend-design.md` §9](../../docs/frontend-design.md#9-player-chrome--one-design-for-every-player);
this folder is what they look like.

| Sheet | Frame | States |
| --- | --- | --- |
| [`sheet-desktop.webp`](sheet-desktop.webp) | 1440 × 900, pointer | all 13 |
| [`sheet-landscape.webp`](sheet-landscape.webp) | 844 × 390, touch | all 13 |
| [`sheet-portrait.webp`](sheet-portrait.webp) | 390 × 844, touch | all 13 |

The states: playing · idle (controls hidden) · station-id · tuning · behind (paused in the 15-min
window) · commercials · info · settings · playback-info · drawer (channels) · help (shortcuts) ·
error · off-air.

**Source.** The sheets are rendered from the step-0 prototype, variant **B2** ("lower third ·
hover previews"), which lives on the throwaway branch
[`prototype/web-watch-2088`](https://github.com/loomarr/loomarr/tree/prototype/web-watch-2088)
at `b6b348e1f`. Run it with Vite and open `/prototype/watch?variant=B2&state=<state>&device=<frame>`
to see one state at full size, or to compare it with the rejected variants (A overlay classic, B
revealed strip, B1 filmstrip, C side rail). The prototype is not product code; build from this mock
and §9, never by copying it.

**Stand-ins.** Programme stills and clip thumbnails are hue-lit stand-ins drawn from tokens; the
real ones are the airing's still and each clip's thumbnail. The stub data (channel 7 "Coast 90s",
its programmes and breaks) is invented for the mock.

**Settled in review** (in the order the maintainer raised them):

1. The lower third (B) over the overlay classic (A) and the side rail (C).
2. The playbar shows what's coming as previews — programmes with their stills, breaks with every
   spot in them — on hover (B2), with the filmstrip (B1) as the touch-portrait form.
3. Depth, not flat: lit stills, a glass tray, separate planes for picture, band, bar and card.
4. **No growing.** Depth comes from light only; segments never change height or lift.
5. The toolbar: one solid primary (play), glass capsules grouped by job, one glyph weight, an amber
   dot for active, shortcut tooltips.
6. No Timeline toggle — the preview bar is always there with the controls.
