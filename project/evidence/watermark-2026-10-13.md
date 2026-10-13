# On-screen logo (watermark) customisation — design review candidate

Review checkpoint for [#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not
an approved behaviour change. No production component, token, route, API contract or renderer changes;
the maintainer chose "draft a mock first, build after approval".

Open [the mock](../prototypes/watermark.html) directly in a browser. The review bar switches Built
today / Draft, the channel (three invented channels in three eras), the state (normal, own image
uploaded, server can't draw it), 4 corners / 3 × 3 position control, and desktop / 390 px. A second
screen shows every look on three frames beside a feasibility table. A link can open a state directly,
for example `watermark.html#screen=looks&ch=mm` or `#frame=phone&tall=1&look=lockup&scene=box`.
Every fixture is invented and the pictures are shapes in token colours.

## Why

Household report, 2026-10-13: "The watermark is cool, but a little bland. Build a way to make it more
interesting, and let the user customize its look and position." The maintainer added that every
relevant setting must be exposed per channel and be friendly.

## What exists (sourced, main @ `d81538b40`)

- **Per-channel policy already carries everything.** `WatermarkPolicy`
  (`internal/schedule/watermark_policy.go`): `enabled`, `corner` (4 corners), `opacity` 0.1–1, `look`
  (`plate`, `text`, `outline`, `small-plate`), `size` 0.02–0.15 (default 0.06), `margin` 0–0.2 (default
  0.05), `image` (custom upload), `callsign` (≤ 12 characters). A nil field means the default.
- **No screen sets any of it per channel.** The channel route's tabs are Watch, Channel info,
  Programming, Filler, Danger zone (`routes/_authed/channels/$id/route.tsx:42-46`). The only UI is two
  install-wide settings under Settings → Playback → Picture and sound: "Channel logo style" and
  "Channel logo opacity" (`playback.tsx`, `internal/settings/declared.go:500-520`). The upload endpoint
  is `POST /v1/channels/{id}/watermark` (`docs/design/playout.md`).
- **The renderer draws white silhouettes.** `internal/watermark/render.go`: Geist Bold, drawn 8×
  supersampled, RGB white and only alpha carries the shape, equal-area sizing (`sizeFor`), opacity baked
  into alpha, soft shadow baked in. Its comment gives the reason for white: "the 4:2:0 overlay cannot
  fringe (white has neutral chroma)". An uploaded image already keeps its own colours (`RenderImage`),
  so colour reaches the screen today, just not from a generated look.
- **The callsign is derived**: `deriveCallsign` in `internal/app/watermark.go` (initials, decade, or
  `CH<n>`); the channel number is available at the same call site (`store.Channel.Number`).
- **The bug is one still PNG per programme item.** `internal/playout/watermark.go`: "The programme's
  frames never leave the GPU for it; only the bug's own single frame is prepared on the CPU, once".
  NVIDIA: `movie=…,format=yuva420p,hwupload_cuda` into `overlay_cuda=x=…:y=…`
  (`pipeline.go:~705`). VAAPI: Loomarr's own OpenCL kernel on a full NV12 bug frame. Software, generic,
  VideoToolbox and HDR10 never draw it. Breaks, bumpers and IDs are encoded without it.
- **Position is a pure function.** `Watermark.position` takes a `Corner`, anchors to the measured
  active picture (decision 0036), insets by the margins, rounds to even, and clamps. `bugPosition()`
  passes the resulting numbers to `overlay_cuda`; the kernel path pads the bug onto a full-frame canvas
  at the same x, y.
- **The self-check asserts white.** `docs/design/playout.md`: the test bug's blend is 65% white over the
  background, "neutral in chroma", with the SPS and PPS byte-identical to a bug-off item.

## What the mock proposes

### A. A per-channel "On-screen logo" section (Channel info tab, admin only)

Same row shape as the neighbouring Programming fields: label, `?` help, "Default / Channel override"
badge and "Reset to default" (`PolicyField` + `PolicyFieldSource`), then the control.

| Field | Control | Wire field | Default |
| --- | --- | --- | --- |
| Show the channel logo | switch | `enabled` | on |
| Style | picker of the 4 built looks + new looks | `look` | Text (install setting) |
| Colour | 6 token swatches + "Match the era" | new field | Match the era |
| Name on the logo | text, placeholder shows the derived name | `callsign` | derived |
| Position | 2 × 2 corners (or 3 × 3, see Q3) | `corner` | top right |
| Size | select: Small / Standard / Large / Extra large | `size` | Standard (0.06) |
| Distance from the edge | select: Tight / Standard / Roomy | `margin` | Standard (0.05) |
| How see-through | select: Faint 25 / Subtle 40 / Clear 70 / Solid 100 % | `opacity` | 40% (install setting) |
| Your own logo | upload button, thumbnail, Remove | `image` | none |

Size, margin and opacity are selects with named steps because their neighbours (Longest programme,
Max from one series) are inputs and selects; the repo has no slider primitive, and a named step says
what the choice does. The API stays continuous.

A live preview sits beside the controls (above them at 390 px) over three frames: bright, dark and an
old 4:3 film pillarboxed inside 16:9. The bug is placed in the active picture, as in production. Under
the preview, three plain lines state the consequence: what you will see, where it shows (movies and
episodes, never commercials, bumpers or IDs), and when (the channel's next programme; the one airing
is unchanged, per `docs/design/playout.md`). A fourth state, "this server can't draw the logo", shows
the existing `watermark.disabled` condition in words and says the choices are saved.

Reuse: the radio-style buttons are the Filler Library `.views` radiogroup pattern. Two parts are new
and would be `@loomarr/ui` primitives: the look picker (the same button with a thumbnail) and the
position grid (a 2 × 2 or 3 × 3 radiogroup). Nothing goes in the legacy `components/ui` stack.

### B. New candidate looks

All are one PNG with alpha, so the GPU path does not change. The mock draws them from the same
silhouette idea as `render.go`; the real renderer would need a new mask for each.

| Look | What it is | PNG | Renderer cost | Pipeline impact |
| --- | --- | --- | --- | --- |
| Colour plate | Name knocked out of a coloured badge | One PNG | Small: a fill colour instead of white | Colour, see below |
| Number + name | Channel number in a coloured tab beside a white name plate | One PNG | Small–medium: two shapes, one mask; needs the channel number passed in | Colour |
| Number badge | Rounded square with the channel number only | One PNG | Small | Colour |
| Number \| name | Number, thin divider, name; white text, like a network ID | One PNG | Small: text only | None, white as today |
| Glass | Frosted see-through plate with a bright edge (gradient alpha; the picture behind is not blurred) | One PNG | Medium: gradient alpha and an edge pass | None, but the self-check's 65% test target doesn't cover mid-range alpha |
| Era stripe | Name with a three-colour stripe in the channel's era palette | One PNG | Small: three rectangles | Colour |

"Match the era" picks colours from the channel's own date window (the `eraOf` the Programming tab
already uses): 1970s caution/signal/onair, 1980s onair/signal/tune, 1990s tune/suggest/lock, from the
design tokens only. That is a proposal, not something to read as approved.

**What the mock shows that is easy to miss.** White at the default 40% nearly disappears on a bright
picture (see the Bright frame and the Text, Plate and Outline cards on the looks screen). The coloured
looks hold up there. That is the strongest argument for colour.

### C. Colour: what the pipeline needs (unverified, to prove on hardware)

- **NVIDIA path.** The bug is decoded to RGBA and converted to `yuva420p` by ffmpeg's inserted scaler
  with no stated matrix. White is immune to the matrix; a saturated colour is not, and ffmpeg's RGB→YUV
  default is BT.601 where the main is BT.709, which would shift hue. The VAAPI kernel path already
  forces `scale=out_color_matrix=bt709:out_range=tv` for the bug, so the fix is the same one step on the
  CUDA path. I did not run it: it needs the GeForce.
- **4:2:0 chroma.** Coloured edges blend at half chroma resolution, which can bleed colour about one
  pixel at the edge of a plate. White has none. Check at 480p output as well as 1080p.
- **Self-check.** It proves white's range and neutrality per host. A coloured look should add a coloured
  test bug with the same ±6 assertion. If a host fails it, coloured looks should fall back to the white
  version of the same style (so the programme still carries a logo) and say so in Diagnostics, as the
  white bug does today.

### D. Position beyond four corners

`Watermark.position` is a pure function of a `Corner` and the active picture, so a 3 × 3 grid is small:
extend the `Corner` enum to nine values (centres use `(a.W - w)/2`), the policy enum and `validate`,
then regenerate OpenAPI and the client. The overlay filter graph does not change, because it already
receives numeric x and y. The mock shows 3 × 3 behind the "Position control" switch.

### E. Not possible today (marked "future" in the mock)

- **A logo that alternates** (number, then name). The bug is one image per programme item on both
  overlay paths. Two images need a timed `enable` on `overlay_cuda` and the OpenCL kernel (not verified
  for either) or splitting the item.
- **Animation** (shimmer, fade-in) and **live text** (time, next programme): each frame would need its
  own input; the bug is rendered before the encode starts.
- **A blurred backdrop** behind the glass look: the blend is a plain alpha mix that never reads the
  picture.

## Gaps this does not cover

- The mock does not exercise a real `watermark.png`; the looks are SVG stand-ins drawn to match the
  shape rules (equal area, 6% height, 5% margin). Real glyph widths differ slightly from the Geist Bold
  render.
- No timing was measured. The runtime cost of every new look is the existing one blend per frame; the
  10 ms/frame self-check budget is unchanged. Render time (once per channel, size and look, cached by
  `channelWatermarks.cached`) is not measured because no new renderer exists.
- The TMDB network logo ("between the two in the maintainer's order") is still unwired
  (`internal/app/watermark.go`, `source`), and this draft does not change that.

## Open questions for the maintainer

1. **Which looks first?** Recommendation: build Number \| name first (white, zero pipeline risk) with
   the whole UI. Then, once the colour check below passes, Colour plate, Number + name, Number badge and
   Era stripe. Drop Glass unless you like it in the mock: it has the highest renderer cost and the weakest
   fit with the "no per-frame work" rule.
2. **Allow colour at all?** Recommendation: yes. It fixes the bright-frame problem, and uploaded
   images already air in colour. Gate it on the explicit BT.709 step, a coloured self-check and a white
   fallback (section C).
3. **Four corners or a 3 × 3 grid?** Recommendation: keep the four corners. A centre or mid-edge logo sits
   on the action, and no US network bug does. The grid is cheap if you want it (section D).
4. **Named steps or free sliders for size, margin and opacity?** Recommendation: named steps (matches
   the neighbouring fields, needs no new primitive). If you want a continuous slider later, it is a new
   `@loomarr/ui` primitive.
5. **Bright-frame legibility.** The approved default is white at 40%. Recommendation: leave it, and let
   the colour looks answer it. Say if you would rather raise the default opacity.
6. **Where does it live?** Recommendation: an "On-screen logo" section on the Channel info tab,
   admin-only like Programming and Filler. A new tab for nine fields is more navigation than it earns.
7. **Naming.** Settings → Playback says "Channel logo style" and "Channel logo opacity"; the brief says
   "On-screen logo". Recommendation: one name. Use "On-screen logo" for the channel section and rename
   the two Settings labels to match, in the build PR.
8. **Default colour.** Recommendation: "Match the era", and amber for a channel with no date window. The
   alternative is a fixed amber for every channel.
9. **Alternating number/name.** Recommendation: defer. Run a spike on `overlay_cuda` timeline `enable`
   and the OpenCL kernel before promising it.
10. **Autosave.** The mock follows the Programming fields (one PATCH of `policy.watermark`, #1895).
    Recommendation: same, with the consequence lines as the only confirmation.

## Gates

Docs and prototype only. No Go, TypeScript or generated file changed. The mock was screenshotted at
1100 px and 390 px (headless Chromium, no Playwright suite) in: normal, own image uploaded, server can't
draw it, 4:3 frame, and the looks screen.
