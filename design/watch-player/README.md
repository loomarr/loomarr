# Watch player — the mock of record (#2088, superseded by variant H, #2768)

The design for **every Loomarr video player**: web Watch first, then the phone and TV clients. The
maintainer picked **variant H**, "Loomarr TV · signature tune", on 2026-11-08 (design review,
[#2719](https://github.com/loomarr/loomarr/issues/2719)). It is channel-first, flat, and feels like live
TV rather than a streaming app. It replaces the #2088 mock (variant B2, "lower third · hover
previews") and the design review's four-control layout. The rules it follows are
[`docs/frontend-design.md` §9](../../docs/frontend-design.md#9-player-chrome--one-design-for-every-player);
this folder says what the mock shows.

## Source

H lives on the throwaway branch
[`prototype/design-direction`](https://github.com/loomarr/loomarr/tree/prototype/design-direction), file
`web/apps/web/src/prototype-direction/player/chrome-tune.tsx`, at `3a253dfa0`. Run it with Vite and open

```text
/prototype/player?variant=H&state=<state>&device=<desktop|portrait|landscape>
```

Other parameters: `bug=<top-right|top-left|bottom-right|bottom-left|off>` places the channel
watermark; `t=<ms>` freezes the tune at that moment; `loop=1` replays it. The prototype is not product
code: build from this mock and §9, never by copying it. Programme art in the mock is drawn from tokens,
or comes from a gitignored `real-art/` folder when present; real art is the airing's logo and backdrop
(#2567).

## States

Each state is drawn at three frames: **desktop** (1440 × 900, pointer), **portrait** (390 × 844, touch)
and **landscape** (844 × 390, touch).

| State | What it shows |
| --- | --- |
| `playing` | Controls up: top row, band, keys, over one bottom scrim. |
| `idle` | Controls hidden after three seconds; only the picture (and the channel watermark). |
| `surfing` | A flip with the controls hidden: the big channel readout on a solid backing. |
| `tune` | The warm flip: about 340 ms, the picture dips, the colour bars sweep the band's edge only. |
| `cold-tune` | A cold start (about two seconds): the full signature, static resolving to signal, the bars, the number, the hand-off into the band. |
| `slow-tune` | A slow stream (about three seconds): the bars fold into a band across the frame, the programme's fanart dimmed above and below, "Tuning in…". |
| `subtitles`, `audio` | Right-hand sheets in the band's style; phones show both on one "Subtitles & audio" sheet. |
| `details` | Playback details: the one place technical terms live (desktop: a panel top left; phone: a sheet). |
| `more` | Schedule, Last channel (phone), Picture, Playback details, Picture in picture, Channels, Info, Shortcuts. |
| `channels` | The Channels sheet. |
| `behind`, `commercials` | Behind live (Go live in place of Live) and a break (the programme's title with "Back in n min"). |
| `covers` | `error-server`, `error-not-started` and `off-air`. |

## What H settles

1. **Channel first.** The band is the channel block (amber, `CH` over the number) beside a broadcast
   band: channel name, Live and the clock, NOW with the programme's logo (text when there is none) and
   episode, one progress line, NEXT. The channel is named once at a time: the band names it, so the big
   readout only shows when the band is hidden.
2. **Flat.** Solid surfaces, hairlines and one bottom scrim. No glass, blur, drop or inset shadows,
   gradients other than the scrim, or bevels.
3. **Keys.** Same on desktop and phone: Last (hidden until there is a channel to return to), a Channel up
   and down rocker, Play/Pause (the one amber key), Volume (pointer), More. Keys have room around them.
4. **Top row.** Upper right: Subtitles, Audio, Fullscreen (pointer), Leave. Portrait phones keep Leave and
   one "Subtitles & audio" key; landscape adds More. Esc closes the top layer, then leaves.
5. **Amber discipline.** Amber is the channel block, the play key and focus rings. Progress, volume,
   the readout and active states are white. Live stays red.
6. **Type.** Letter-spaced mono capitals only on the NOW and NEXT chips. Everything else is Geist in
   sentence case. Mono for channel numbers, clocks and Playback details' values.
7. **The tune never adds waiting time.** The warm flip is quiet and short. The full signature runs only
   on a cold start, for as long as the real wait; a wait of 960 ms or less is the warm flip.
8. **The watermark.** Playout burns the channel's watermark into the picture. The player never moves it;
   it keeps its own chrome out of the watermark's box (`bug=` shows each corner).
9. **Plain words.** Subtitles, Audio and Picture speak plainly; technical terms are only in Playback
   details.

## Superseded

The #2088 sheets (`sheet-*.webp`, variant B2: glass tray, hover-preview playbar, capsules) are gone from
this folder; they are in git history at the commit before #2768. Where that mock and H disagree, H wins:
depth from glass, capsules and the hover-preview playbar are replaced by the flat band and keys. H's band
has no seek bar; the schedule playbar lives in More.

**Not yet rendered.** The state sheets (`sheet-desktop`, `sheet-portrait`, `sheet-landscape`) are rendered
from the running prototype with Playwright, which the lane that wrote this could not run. Until they are
added, the prototype commit above is the picture.
