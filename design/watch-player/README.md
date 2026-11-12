# Watch player — the mock of record (variant J, #3449)

The design for **every Loomarr video player**: web Watch, the phone apps, Android TV and Apple TV. The
maintainer chose **variant J** on 2026-11-09 ([#3449](https://github.com/loomarr/loomarr/issues/3449)):
the calmer band of variant G with four changes. It replaces variant H (#2768), which the player was
first built from, and the #2088 mock before that. The rules it follows are
[`docs/frontend-design.md` §9](../../docs/frontend-design.md#9-player-chrome--one-design-for-every-player);
this folder says what the mock shows.

## Source

J lives on the throwaway branch
[`prototype/design-direction`](https://github.com/loomarr/loomarr/tree/prototype/design-direction),
mainly `web/apps/web/src/prototype-direction/player/chrome-tv.tsx` (with `refined`), `panels-j.tsx`,
`tune-j.tsx` and `chrome-line.tsx`, at `584e138ac`. Run it with Vite and open

```text
/prototype/player?variant=J&state=<state>&device=<desktop|tv|portrait|landscape>
```

`portrait` and `landscape` are an iPhone held upright and on its side, with its safe areas drawn. Other
parameters: `bug=<top-right|top-left|bottom-right|bottom-left|off>` places the channel watermark;
`stress=long` puts long names everywhere and `stress=bare` is a library with no art; `t=<ms>` freezes
the tune at that moment and `loop=1` replays it. The prototype is not product code: build from this
mock and §9, never by copying it. Programme art in the mock is drawn from tokens, or comes from a
gitignored `real-art/` folder when present; the renders here use the drawn stand-ins.

## Renders

`sheet-<device>.webp` is every state below on one frame: `desktop` (1440 × 900, pointer), `tv`
(960 × 540, remote), `portrait` (390 × 844) and `landscape` (844 × 390). Each tile is labelled with its
state.

| State | What it shows |
| --- | --- |
| `playing` | Controls up: Leave top left, the band bottom left, the control cluster bottom right. Desktop and TV: the film strip is the band's foot. |
| `idle` | Controls hidden; only the picture and the channel's watermark. |
| `chrome-top` | A preview card open over a block of the film strip (desktop: pointed at; TV: focused). |
| `surfing` | Flipping channels: the channel cards replace the band and the readout is up. |
| `tune`, `cold-tune` | The warm flip (the picture dips, the bars run once along the band's top edge) and the cold start's full signature, each frozen mid-way. |
| `behind`, `commercials` | Behind live (Go live in place of Live) and a break (`Back in n min`). |
| `station-id` | The ident card after a settled tune. |
| `subtitles`, `audio`, `more`, `picture`, `volume` | What the keys open: a menu over the keys on desktop and TV, a bottom sheet upright, the right-hand sheet on its side. |
| `details` | Playback details: a panel upper left (two columns on TV), a sheet on a phone. |
| `info` | The on-air block's card in full (desktop, TV), a sheet on a phone. |
| `channels` | The Channels sheet. |
| `unmute` | The muted first play's pill (web only). |
| `no-channel`, `no-schedule` | A typed number no channel has; a channel with nothing scheduled. |
| `error-server`, `error-not-started`, `error-stopped`, `off-air` | The covers, with a white Retry. |

## What J settles

1. **Four changes to G.** The big channel readout shows only on a channel change, never beside the
   band. The progress line is white, so amber is on the channel block alone. Subtitles, Audio and
   Fullscreen are a small row of icons over the keys, and Leave is a back arrow in the top left corner.
   On desktop the film strip is the band's progress row, always shown with the band, with a preview card
   for the block under the pointer.
2. **From G.** A narrower channel block with soft corners; "Now" and "Next" as small plain labels; a
   white play key; keys in the order Last channel, Channel down, Play, Channel up, (Volume,) More, with
   their legends on hover and focus only; no colour-bar edge at rest.
3. **Typed titles.** The band's title is always typed. Logos go on the preview card's fanart, the cold
   start's hold and the ident, sized by area.
4. **Flat.** Solid surfaces, hairlines and one bottom scrim.
5. **The tune never adds waiting time** (from H): the warm flip dips the picture alone; the full
   signature runs only on a cold start, for as long as the real wait.
6. **The watermark, per corner.** Only the chrome in the bug's corner steps clear of it.
7. **Every device.** Television: four keys, an amber focus ring, 5 % clear at every edge. iPhone: safe
   areas; upright, a toolbar above the picture and nothing on it.
8. **Plain words.** Technical terms only in Playback details.

## Not drawn yet

An iPad, an Android phone, and what the remote's left and right do while the chrome is hidden.
