# On-screen logo (watermark) customisation — design review candidate

Review checkpoint for [#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not
an approved behaviour change. No production component, token, route, API contract or renderer changes;
the maintainer chose "draft a mock first, build after approval".

Open [the mock](../prototypes/watermark.html) directly in a browser. The review bar switches Built
today / Draft, the channel (three invented channels in three eras), the state (normal, own image
uploaded, server can't draw it), 4 corners / 3 × 3 position control, and desktop / 390 px. A second
screen shows every look on three frames beside a feasibility table. A link can open a state directly,
for example `watermark.html#screen=looks&ch=mm` or `#frame=phone&tall=1&look=lockup&scene=box`.
Every fixture is invented and the pictures are shapes in token colours. A third screen, "Round 2",
holds the holiday versions, the motifs at real size, the AI prompt and the feasibility table; a link such
as `watermark.html#screen=holiday` or `#ch=mm&today=2026-10-31&sug=0` opens it directly.

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

## Round 2: holiday versions, "design it for me", TMDB signals

Maintainer, 2026-10-13: "for the watermark, we should have some holiday rep as well and also use TMDB
or AI to help design it." The mock gains a **Holiday touches** field, a **Suggest a design** button
with a result card, a **Colours from this lineup** option in Colour, an opt-in **network logo** field
(marked new work), and a third screen, "Round 2", with holiday versions of three looks, every picture at
real size, the AI prompt and its validation, and a feasibility table. Review-bar controls: *Pretend today
is*, *AI set up / not set up*, *Lineup colours snapped / as sampled*.

Hard constraints kept: every version is one still PNG with straight alpha from `render.go`; no
animation or live text; **no image generation**, because the maintainer rejected AI-rendered art and the
LLM providers (`internal/llm`, `Provider.Chat`) are text-only. Fixtures stay invented: posters are colour
swatches, the network is "Kite TV" and its mark is a drawn kite.

### F. Holiday versions

**Where the bug is resolved.** `packager_hls.go:583`: once per item, at Open, with the encoder and output
size (`it.Watermark(ctx, host.Encoder, out.Width, out.Height)`); the packager source builds the closure
in `watermarkFor` (`playoutpackager.go:28`) for library programmes only, so breaks, bumpers, IDs and
filler never carry it. `channelWatermarks.For` (`internal/app/watermark.go:120`) then reads the channel
and builds the cache key `source|height|size|opacity|renderVersion`. **There is no date input today**, so
the key cannot change by itself on Oct 1.

**A date input fits, and the right one already exists.** `playout.Airing.StartedAt` is documented as the
authoritative wall-clock boundary, "stable across retries and mid-program tune-ins" (`timeline.go:26`).
Change `WatermarkFor` to `func(ctx, enc, w, h, at time.Time)`, pass `airing.StartedAt` from
`watermarkFor`, and have `For` resolve the holiday from `at` and the channel's `policy.seasonal`. The
holiday id and motif id join the key (`…|hol:halloween:pumpkin`) and `renderVersion` bumps once, so
existing cached bugs stay valid. Using the airing start, not `time.Now()`, means a retried Open or a
tune-in mid-programme gets the same bug as the first open, and the swap lands exactly on a programme
boundary: a programme already airing keeps its logo until it ends (at most one longest-programme late).
Nothing re-resolves mid-item, so there is no mid-picture switch and no flicker.

**Reuse of the calendar.** `schedule.builtinCalendar` (`seasonal.go:33`) is unexported; expose one
`ActiveHolidayAt(t, selected []string) string` beside `activeHolidays` so the bug can never disagree with
what the scheduler boosts. Selection follows the channel's own `policy.seasonal` (`mode`, `holidays`;
empty = all). A channel with seasonal mode "off" gets no holiday logo and the field says why.

**A date-boundary question to confirm in the build, unverified.** The windows are built in UTC
(`fixedWindow` uses `time.UTC`) and `holidayActive` compares instants. If the scheduler passes an
instant, the window turns over at 00:00 UTC: for a household in US Eastern the Halloween logo would start
at 8 pm on Sep 30 and stop at 8 pm on Oct 31, losing trick-or-treat night. I did not run it. See
question 14.

| Cost | Per item | Notes |
| --- | --- | --- |
| Renderer | Motif is a small path set drawn in the existing 8× supersample, composed beside the look | Eight motifs: pumpkin, bat, ghost, snowflake, holly, heart, firework, leaf. Each is 1–3 flat shapes in tokens or white; holes (pumpkin face, ghost eyes, leaf vein) are alpha holes. Not measured; the mock draws them in SVG |
| Cache | One more PNG per (channel, size, look, holiday) | At most five holidays per channel, rendered on first use under the existing `render` mutex, a few ms each (not measured) |
| Jobs / LLM | None | No scheduled job: the first item after Oct 1 pays one render |
| Pipeline | Unchanged GPU path | Coloured motifs need the colour work in section C (BT.709 on the CUDA path, coloured self-check); white-only motifs (bat, ghost) do not. A white-silhouette fallback of every motif is the safe degrade |

**What the real-size tiles show.** Each motif was drawn at its true pixel height: 65 px bug at 1080p
(motif 55 px), 29 px at 480p (motif 24 px). All eight read as a distinct shape at 55 px on the dark tile.
At 24 px (480p) the solid shapes (pumpkin, snowflake, heart, holly, leaf) are still recognisable; the
firework's thin rays are the weakest, so the mock thickens them, and a build should keep every stroke
at 8+ units of 100. **At the approved default 40% opacity on a bright frame, every coloured motif and
plate washes out** (the round-1 white-text finding, again), the firework and leaf worst. Recommendation: holiday versions keep the user's
opacity, but the preview shows this, and the build should not silently raise it. Question 12 asks.

A first draft put the motif as a corner sticker on the Number badge; in the same colour it disappeared
into the badge. The mock now puts every motif beside the look (a 6-unit gap), as the Round 2 screen shows.

### G. "Suggest a design" (AI, closed vocabulary)

**What it is.** One stateless call on a button press, never on the airtime path. Proposed endpoint
`POST /v1/channels/{id}/watermark/suggestion` (admin), returning `{look, color, size, corner, callsign,
motifs{holidayId: motifId}, reason, source: "ai"|"rules"}`. Apply writes ordinary `policy.watermark`
fields through the same autosave PATCH; nothing is stored for the suggestion itself, so there is no new
table or job. The existing `CallsignProposer` seam (`watermark.go`, one small call after channel
creation, deliberately unwired) generalises to a `DesignProposer`.

**Prompt shape** (short, one list per field, JSON answer; weak-model friendly per the
"optimize for weak models" rule):

```
Design a TV channel logo. Answer with JSON only.
Channel: Saturday Cartoons (channel 7)
Years: 1980s
Genres: Animation, Family
Titles: Gummi Rangers; Captain Comet; Pixel Pals; Robo Bakery; Moon Patrol
Choose every value from its list. Do not make up values.
look: plate|text|outline|small-plate|tint|twotone|badge|lockup|stripe
color: era|lineup|signal|tune|lock|onair|suggest|caution
size: small|standard|large
corner: top-left|top-right|bottom-left|bottom-right
callsign: up to 12 letters or digits, or ""
halloween: pumpkin|bat|ghost
reason: at most 8 words
{"look":"","color":"","size":"","corner":"","callsign":"","halloween":"","reason":""}
```

About 250–400 prompt tokens, 60–120 out. `ChatOptions{JSONMode: true, Temperature: ~0.3, MaxTokens: 160,
ReasoningEffort: "none"}`, no tools, 12 s timeout, one attempt (Try again is the retry). The holiday keys
list only holidays the channel honours that have more than one motif. Glass and 3 × 3 positions are
omitted from the lists (open questions 1 and 3).

**Validation, per field, never all-or-nothing:** a value outside its list is dropped and filled from the
rules; `lineup` without posters falls back to `era`; the callsign is upper-cased, filtered to letters,
digits, space, `&` and `-`, cut to 12, and blank means auto; the reason keeps its first 8 words; any
extra key is ignored, so a model cannot ask for a hex colour, font, image or an unlisted corner. A
non-JSON answer, timeout, provider error, or no provider configured returns the rules version with
`source: "rules"` and no error toast.

**Offline rules (the deterministic version):** Colour plate when the channel has a date window, else
Text; colour "Match the era"; Standard; top right; derived callsign (`deriveCallsign`); the default motif
of each included holiday. Same inputs, same output, unit-testable, free. It is also what a new channel
gets at creation (question 15).

**Budget.** One call per click; a household clicks a handful of times in a channel's life. Local models
answer in about a second warm (the suggester notes ~9 s cold-load, ~0.5 s warm for an 8B). Hosted
providers send the channel name, decade, genres and up to five titles: the mock says so in one line on
the card ("Sent to your AI: …"), and the data class is the same as the lineup suggester's.

### H. Lineup palette from TMDB posters

**No new TMDB call.** Posters are already adopted into the image service (`PosterURL` comment: "the
caller ADOPTS this URL", `internal/tmdb/tmdb.go:807`), and each image record keeps `DominantHex`
(`internal/images/image.go:173`). That field is a single **average** colour for skeleton tinting: the
average of a red-and-blue poster is a muddy purple, so it is not a palette. A hue sampler (decode at most
8 small cached poster thumbnails, histogram by hue) is the new work, a few ms each at about 100 px.

**Snapped to tokens or raw: recommend snapped to the six app colours, by hue only.** Tokens fix
brightness, so the lineup decides *which* hue; the mock's `snapToken` does this and the *Lineup colours*
review control shows both. Reasons: (1) raw colours are usually dull and mid-tone, and a coloured plate
has to stay legible on bright and dark frames at 40%; (2) the coloured self-check (section C) can
verify the six known colours per host, but cannot pre-verify an arbitrary RGB, so raw colour would
bypass the safety gate; (3) arbitrary hues bleed unpredictably in 4:2:0; (4) it keeps the app's look.
Cost of snapping: two quite different lineups can land on the same token (Midnight Movies' navy and teal
posters both go to cyan); the card shows the three chosen colours, so the household sees what it got.

**Stored as a snapshot on Apply**, not recomputed live: choosing "Colours from this lineup" saves up to
three token names in the policy (`color: "lineup"` plus the resolved list), so a library change never
recolours a channel unprompted. A "Refresh colours" action would re-run the sampler (question 17).

### I. Network logo (new work, opt-in)

Not wired today: `source()` carries the comment "The automatic TMDB network logo … is not wired yet".
Needed: TMDB `GET /tv/{id}` already returns `networks[].logo_path` and `/movie/{id}` returns
`production_companies`; `internal/tmdb` resolves network *names* (`resolveNetwork`,
`loadNetworkIdentities`) but never reads a logo. Per channel: the network that carries a majority
(proposed ≥ 50%) of the lineup's series; films have studios, not one network, so the field is disabled
with a sentence (Midnight Movies in the mock); a mixed lineup is disabled (Cozy Mysteries). The logo is
fetched once, adopted into the image service like a poster, then rendered through the existing
`RenderImage` path an upload uses (keeps its own colours). Cost: one TMDB request per series, cached,
none per programme. Order: your own upload > network logo (if on) > generated look. **Network logos
are third-party trademarks, so it is off by default and per channel**; holiday touches pause while it is
on (the logo airs unchanged). Colour logos take the same colour-pipeline check as section C.

### J. What the mock shows vs not

Shows: the controls, the consequence lines ("From Oct 1 to Oct 31 this channel shows the Halloween
logo"), the holiday on the three frames for Halloween and Christmas (and Thanksgiving, New Year,
Valentine's Day via the *Show as* row), three looks × two holidays, eight motifs at real size, the
AI result card with the offline version beside it, and a set-up-AI-off state. Does not show: any real
render, a real LLM answer, a real poster, or timing.

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

Round 2 (holiday, AI, TMDB), continuing the numbering:

11. **Holiday touches default.** On or off for a channel that has never touched it?
    Recommendation: **on**, whenever the channel's seasonal mode is not "Ignore holidays" (the seasonal
    default is Auto), and no field at all when it is. A household that asked for spooky October evenings
    gets the matching logo without finding a setting; the switch is one click to turn off, and the
    consequence line says what it does.
12. **Holiday logo on a bright picture.** At the default 40% every coloured motif and plate washes out on a
    bright frame (section F). Recommendation: the holiday version keeps the channel's opacity (no silent
    raise); say so in review, and let question 5's answer decide for both.
13. **Which holidays at launch, and is the picture a choice?** Recommendation: build all five calendar
    holidays on one mechanism with one default picture each (pumpkin, leaf, holly, firework, heart), and
    offer a choice only where there are two or three (Halloween: pumpkin, bat, ghost; Christmas: holly,
    snowflake), as the mock does. Halloween and Christmas are the two the household asked about; the
    rest cost one small shape each.
14. **Holiday dates and time zone.** The calendar windows are UTC dates; if the seasonal code compares
    instants, a US Eastern household's Halloween ends at 8 pm on Oct 31. Recommendation: evaluate the
    window in the household's configured time zone for both the scheduler and the logo, and add one
    shared `ActiveHolidayAt` so they cannot disagree. I did not run it; confirm before building.
15. **When does a suggestion run?** Recommendation: **only on a click**. A new channel gets the rules
    design immediately (no call, nothing sent anywhere); the AI is the "Suggest a design" button.
    Automatic calls at creation cost a model turn per channel, send data to a hosted provider
    unprompted, and replace a design the household never asked to change.
16. **Does the lineup palette replace "Match the era" as the default colour?** Recommendation: **no**.
    The era colour is stable and works for an empty or new lineup; a lineup-derived default would
    recolour a channel when its library changes. Lineup colours is an opt-in swatch and one of the AI's
    answers.
17. **Lineup palette: snapshot or live?** Recommendation: snapshot the (at most three) colour names when
    the household picks it or applies a suggestion, with a "Refresh colours" action. No job, no TMDB call,
    no surprise recolour.
18. **Lineup colours: snapped to the six app colours, or raw?** Recommendation: snapped by hue only
    (section H). Raw colours are duller, cannot be verified by the coloured self-check, and bleed in
    4:2:0. The mock's *Lineup colours* control shows both so you can look.
19. **Network logo: build it at all, and when?** Recommendation: last, after holiday versions and the
    colour check ship. Off by default, per channel, only when one network carries at least half the
    lineup's series, never for a film channel; it is a third-party trademark and the repo is public, so
    no logo asset is bundled or committed (it is fetched at runtime from TMDB). Say if you would rather
    not offer it.
20. **Holiday touches with an uploaded image or a network logo.** Recommendation: the image or logo airs
    unchanged and the field says "paused", as the mock shows. Compositing a motif onto a household's own
    artwork is more surprising than useful.
21. **Sending lineup facts to a hosted AI.** "Suggest a design" sends the channel name, decade, genres and
    up to five titles. Recommendation: allowed for any configured provider, with the one-line "Sent to
    your AI" note on the card and nothing stored; no separate setting. Say so if you want it limited to
    local providers.

## Gates

Docs and prototype only. No Go, TypeScript or generated file changed. The mock was screenshotted in
headless Chromium (no Playwright suite) at 390 px and 1440 px: the channel section (normal, with a
suggestion open, AI off, a channel that ignores holidays, a film channel with no network logo, a
different "today"), the looks screen and the Round 2 screen. In-page JavaScript raised no console errors.
No renderer, LLM, TMDB or timing was run; every claim about cost is read from code, not measured.
