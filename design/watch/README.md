# Watching: The Cross (decided 2026-11-27, not yet built)

The next design for watching Loomarr on every client: web, the phone apps, Android TV and Apple TV.
The maintainer locked the structure, **The Cross**, on 2026-11-27 after a critique of the shipped
Home, Guide and Watch surfaces. It covers more than the player: the landing route, the channel
column, the film strip, the Guide and Home's wall.

**The shipped player is still variant J.** [`../watch-player/`](../watch-player/README.md) stays the
mock of record for what is built, and [`docs/frontend-design.md`
§9](../../docs/frontend-design.md#9-player-chrome--one-design-for-every-player) stays its rules,
until the build lands. Each build PR amends §9 and the design docs for the part it changes.

## The idea

Channels run down, the day runs across, and they meet at what is on.

- **Loomarr opens on the picture**: the last channel, playing. Home is where the back arrow leads.
- **Three renderings of the channels, one job each.** The channel column flips. The wall on Home
  browses with artwork. The Guide plans across time. The Watch nav item, the Guide's Now & next
  list, the player's surf cards, its Channels sheet, the band, the on-screen channel keys and the
  white Play key go.
- **The lower third is type on a scrim**: the channel's number as a numeral in its decade colour,
  the title, then the channel, episode and minutes left. Nothing has a box.
- **Controls recede.** One quiet row of icons. Pause is a click on the picture, or Space.

## Source

`mock.html` draws every state from one file. Open it in a browser for the gallery, or one state with

```text
mock.html?device=<desk|desktop|tv|portrait|landscape>&state=<state>
```

`desk` is desktop in a window and `desktop` is desktop full screen. `&art=0` draws a library with no
artwork. The mock is not product code: build from it and the rules below, never by copying it. The
picture is the drawn diner still from variant J's sheet; every other programme's artwork is a drawn
two-colour stand-in, so real stills will be richer and less tidy. `static.webp` is a generated noise
tile standing in for the design system's `AnalogSnow`. Fonts load from Google Fonts and
fall back to the system face offline.

## Renders

Each `sheet-<device>.webp` is every drawn state for that device, labelled.

| Sheet | Frame | Input |
| --- | --- | --- |
| `sheet-desktop.webp` | 1440 × 900, in a window | pointer and keyboard |
| `sheet-desktop-full-screen.webp` | 1440 × 900, full screen | pointer and keyboard |
| `sheet-tv.webp` | 960 × 540 | remote |
| `sheet-portrait.webp` | 390 × 844 | touch |
| `sheet-landscape.webp` | 844 × 390 | touch |

| State | What it shows |
| --- | --- |
| `rest` | Full screen at rest: the picture and the channel's own watermark. |
| `third` | A tune or a wake: the lower third, what is next, and the quiet row. |
| `column` | Beat one: the channel column. Moving through it tunes at once. |
| `cross`, `playing` | Beat two: the film strip for the tuned channel. In a window and on an upright phone both arms are always up, and the state is `playing`. |
| `peek` | Desktop in a window: pointing at a channel shows its day in the strip before a click tunes. |
| `later` | A later programme pointed at or focused: its facts as type (a sheet on an upright phone). |
| `find`, `typing` | The find control, or typing letters, filters the column; typing digits replaces the numeral and counts down to the tune. |
| `focus`, `paused` | Keyboard focus drawn on the tuned row, and the paused state with its key. |
| `tune`, `cold` | Changing channel (a short burst of static, the seven bars along the crossing) and tuning in (static for the real wait, the bars in grey). |
| `break`, `behind`, `nothing` | In a break, behind live with Go live, and nothing scheduled. |
| `subtitles`, `more` | What a key opens: a menu from the quiet row, a sheet on a phone. |
| `unmute`, `off-air` | The web's muted first play, and a cover. |
| `guide`, `guide-later` | The Guide, and the Guide with a later programme pointed at. |
| `bright` | The crossing over a washed-out frame, to prove the scrims. |
| `wall` | Home: every channel with its artwork, the last watched first. |
| `*-no-artwork` | A library with no art: flat tones in place of stills, and words inside each block. |

## What The Cross settles

1. **The picture is the front door** for members and admins. With no last channel it is the
   lowest-numbered playable one.
2. **Beside the picture where there is room, over it where there is not.** In a desktop window the
   column runs down the left and the film strip under the picture, and nothing covers it. An upright
   phone stacks the same parts under the picture. Full screen, a television and a phone on its side
   put the same cross over the picture. A window narrower than about 900 px stacks as the phone does.
3. **Two beats over the picture.** Up or Down opens the column. Right, or resting on a row, opens the
   film strip. Enter or Esc returns to the picture.
4. **A pointer looks before it tunes.** In a window, pointing at a channel shows its day and
   pointing at a programme shows its facts. A click on a row tunes. The wheel scrolls the column and
   never flips channels.
5. **The Guide is a view of the same screen**, not a page of its own, so the picture keeps playing in
   the corner. It is the cross with every row open on one clock. What is on now carries its artwork
   with the aired part dimmed. The tuned channel's row is the film strip at full height. A run of one
   series is one block that says how many episodes. A later programme shows its start time. A break
   is a thin tick. A click on a row tunes; a click on a later programme shows its facts.
   - **Television:** the header is one line beside a small picture, so eight channels fit.
   - **Phone:** the Guide is a list, not a grid. Each channel is a row with what is on now and its
     next two programmes.
6. **Getting to the Guide.** One control, always with its word, "Guide": in the column's header on
   desktop, at the end of the group tabs over the picture, and pinned at the end of the tabs on a
   phone. The Guide nav item opens the same view. On a television OK on the picture opens the Guide,
   as it does today, and Up or Down opens the column; one hint line says so.
7. **30 channels and more, in one order.** Every list of channels is in number order, scrolled so
   the tuned channel sits on the crossing. It never wraps on screen, though flipping past the last
   channel still reaches the first. On an upright phone the lower third is the tuned row, so the
   list under it carries on from the next channel. Group tabs (Favourites, All, Recent, By decade)
   head the column, the Guide and the wall. Digits tune direct. A find control opens the query,
   typing letters does the same, and results list straight under it.
8. **Decade colour belongs to the channel you are watching.** Its numeral, the large one, takes the
   decade's colour; every other channel number is plain. Breaks are neutral grey. So amber means
   focus and typed digits, red means live, and neither is borrowed for anything else.
9. **The tune is static and seven bars, and it is never a nuisance** (maintainer, 2026-11-27). The
   station-ident card goes: the lower third on a settled tune does its job.
   - **Changing channel** (the next channel is already streaming): the picture dips into a burst of
     static for about 150 ms, inside the same 340 ms the flip takes today, and the seven bars run
     once along the crossing. It never adds waiting time.
   - **Tuning in** (the first tune, or a channel that must spin up): the held picture is drained and
     under static for as long as the real wait. The lower third already names the channel and says
     "Tuning in…", in the place it stays. The seven bars wait in grey along the crossing and lock to
     colour when the picture arrives. There is no full-screen colour-bar band and no centred readout.
   - **The static is dim**: fine grey snow over a darkened picture, never a white flash.
   - **Fast flipping does not strobe.** While flips keep coming the static holds as one low wash and
     resolves once when they stop, so nothing flashes more than three times a second.
   - **Reduced motion** gets a plain crossfade and no static.
   - Sound is unchanged: the web's Channel-change sound switch still decides it.
10. **One key set, with its omissions written down.** In order: Pause, Subtitles, Audio, Volume,
    Full screen, More. A television drops Pause, Volume and Full screen, which its remote has. A
    phone drops Volume and Full screen and adds AirPlay; upright, Pause and More sit by the lower
    third, under the thumb, and the rest at the top. Home is always named beside its arrow.
11. **More holds only what has no key of its own**: Last channel, Picture, Playback details, Picture
    in picture on desktop, and Manage this channel for an admin. Channels and Info left it: the
    column and the synopsis under the strip do those jobs.
12. **Administration leaves the watching path.** "Request a channel" and the Guide's per-row Actions
    menu move to channel management, reached from More.
13. **Words over the picture always read.** The left scrim is as wide as what stands on it, list rows
    do not fade, and facts set over the picture stand on a solid ink plate. `bright` proves it over a
    washed-out frame on desktop, a television and a phone on its side.
14. **Every touch control is at least 44 px**, and no text is under 12 px.
15. **Focus is always drawn.** On a television the tuned row carries the amber ring while the column
    is up, and a strip block carries it when focus moves there. On desktop the same ring shows for
    keyboard focus.
16. **The rating reads with the programme's facts** in the lower third, not beside the back arrow.
17. **Sizes.** The numeral and title are 64 and 34 px full screen, 56 and 28 px in a window, 52 and
    30 px on a television, 44 and 22 px on an upright phone.

## Unchanged from variant J

Not redrawn here; build them from [`../watch-player/`](../watch-player/README.md): the Audio menu and
the Picture page, Playback details, the server, not-started and stream-stopped covers, casting on an
iPhone, the watermark's corner rule, and every word in the covers and menus.

## Not drawn yet

A desktop window narrower than about 900 px, the Guide on a phone on its side, the Guide's other days
and its loading and empty states, the wall on a television, an iPad, and an Android phone.
