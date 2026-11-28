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
| `cross`, `playing` | Beat two: the film strip for the tuned channel. In a window both arms are always up, and the state is `playing`. On an upright phone `playing` shows the list with the strip folded away. |
| `peek` | Desktop in a window: pointing at a channel shows its day in the strip before a click tunes. |
| `later` | A later programme pointed at or focused: its facts as type (a sheet on an upright phone). |
| `find`, `typing` | The find control, or typing letters, filters the column; typing digits replaces the numeral and counts down to the tune. |
| `focus`, `paused` | Keyboard focus drawn on the tuned row, and paused, which marks the picture itself. |
| `strip` | Upright phone: the tuned channel tapped, its film strip open under the lower third. |
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
   put the channel list over the picture and keep the day in the Guide. A window narrower than about
   900 px stacks as the phone does.
3. **Flipping tunes; the list looks** (maintainer, 2026-11-27). Up and Down on the bare picture
   change channel at once and show only the lower third. **Channels** opens the list: moving through
   it looks, OK or a click tunes, and Back or Esc closes it. The list is one scrollable list on every
   device, with the channel you are watching pinned in it as the lower third, so it stays in view
   however far the list is scrolled. Nothing that holds focus, the pointer or an open layer closes
   on a timer; only the bare picture goes quiet.
4. **A pointer looks before it tunes.** In a window, pointing at a channel shows its day and
   pointing at a programme shows its facts. A click on a row tunes. The wheel scrolls the column and
   never flips channels.
5. **The Guide is a view of the same screen**, not a page of its own, so the picture keeps playing in
   the corner. It is the cross with every row open on one clock. What is on now carries its artwork
   with the aired part dimmed. The tuned channel's row is the film strip at full height. A run of one
   series is one block that says how many episodes. A later programme shows its start time. A break
   is a thin tick. A click on a row tunes; a click on a later programme shows its facts.
   - **Television:** the header is one line beside a small picture, so eight channels fit.
   - **Phone:** the Guide is a time view too: one channel's day per row, with its number and name on
     the line above the row.
6. **Channels and Guide are always named.** Over the picture, the quiet row leads with two controls
   that carry their words, **Channels** and **Guide**, on every wake: on desktop full screen, a
   television and a phone on its side. Channels opens the column; it is not a pair of up and down
   keys. A desktop window and an upright phone keep the column in view instead, with the Guide
   control in the column's header or pinned at the end of the tabs. The Guide nav item opens the
   same view.
   - **"Later today" is the Guide** (maintainer, 2026-11-27). Over the picture there is no film
     strip of its own: the Guide opens focused on the row of the channel you are watching, which is
     that strip at full height. A desktop window keeps the strip under the picture.
   - **A television's focus map.** Up and Down change channel. OK on the bare picture pauses and
     resumes. Left and Right wake the quiet row and move along it: Channels, Guide, Pause,
     Subtitles, Audio, More, with Go live leading the row while behind live. OK activates the
     focused control. Back clears focus, then goes Home. In the open list, Up from the first channel
     reaches the group tabs, Left and Right move between them, OK chooses one and Down returns. The
     key that wakes the screen also does its job. One hint line says the first three.
   - **A television has no typed find** (maintainer, 2026-11-27). Its group tabs and the Guide are
     how it narrows 30 channels; find stays on desktop and phone.
7. **30 channels and more, in one order.** Every list of channels is in number order, scrolled so
   the tuned channel sits on the crossing. It never wraps on screen, though flipping past the last
   channel still reaches the first. On an upright phone the whole list sits under the lower third,
   scrolled to the tuned channel, which is marked "watching". Group
   tabs (Favourites, All, Recent, By decade) head the column, the Guide and the wall, and the wall
   follows the same order after the last watched. Digits tune direct. A find control opens the
   query, `/` does the same, and results list straight under it. Typing letters does not open find:
   building the prototype showed that letters are already shortcuts (G, L, C, A, M, F).
8. **Decades get colours of their own** (maintainer, 2026-11-27). The channel you are watching takes
   its decade's colour on its numeral, the large one; every other channel number is plain. The
   decade colours are six soft tints that reuse none of the system's accents, so a decade is never
   read as live, a link, a warning or focus: 1950s `#E6CFA8`, 1960s `#F0A8B8`, 1970s `#C9E08A`,
   1980s `#B9A6F2`, 1990s `#9DB4F5`, 2000s `#8FD3C8`. They are new tokens: the build adds them to
   `docs/frontend-design.md` §2.1 and the token source under the `tokens` claim, and the contrast
   check must pass them on the dark and light grounds. Breaks are neutral grey. Amber means focus
   and typed digits, red means live, and cyan stays the break's "Back in n min".
   - **Pointing is not focus.** A pointer over a programme draws a plain ring; the amber ring is for
     keyboard and remote focus alone.
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
15. **Focus is always drawn.** On a television the list's cursor row, the focused tab or the focused
    control carries the amber ring, and so does the focused programme in the Guide. On desktop the
    same ring shows for keyboard focus.
16. **The rating reads with the programme's facts** in the lower third, not beside the back arrow.
17. **Sizes.** The numeral and title are 64 and 34 px full screen, 56 and 28 px in a window, 52 and
    30 px on a television, 44 and 22 px on an upright phone.

## Unchanged from variant J

Not redrawn here; build them from [`../watch-player/`](../watch-player/README.md): the Audio menu and
the Picture page, Playback details, the server, not-started and stream-stopped covers, casting on an
iPhone, the watermark's corner rule, and every word in the covers and menus.

## Not drawn yet

A desktop window narrower than about 900 px, the Guide on a phone on its side, the Guide's other days
and its loading and empty states, a visible close on the touch cross and sheets, an iPad, and an
Android phone. The prototype draws By decade's headings and the wall on a television; the stills do
not.

## Prototype

`prototype.html` is the core loop made clickable: open on the picture, change channel, look later in
the day, open the Guide, find a channel by name, pause, go Home and come back. It is one
self-contained file, throwaway, and not product code.

- **Desktop** takes a pointer and a keyboard; `F` swaps the window for full screen.
- **Television** takes arrow keys, Enter for OK and Backspace for Back, and ignores the pointer.
- **Phone** is an upright phone; a click is a tap.
- **Tasks** opens a checklist of six things to try. Each ticks itself when done, by any route, and
  records how many seconds it took: an instrument for five minutes with someone who has never seen
  the design.

`?device=tv|phone|full`, `&tasks=1` and `&cold=0` (skip the tuning-in wait) set it up from the URL.
It adds what stills could not show: a close on every menu, a next step on the covers, a tooltip
naming each icon key, one first-run hint per device that goes after the first channel change, and a
shortcuts sheet under More. Its artwork is the mock's stand-ins, and the picture is a still.

### What using it changed

Two reviews by use (six tasks, cold, on each layout) found these. The prototype carries the fixes;
the mock's stills and sheets do not yet, so where they differ the prototype and the list above win.

- **One model for the channel list** on every device (item 3). The first prototype gave each layout
  its own: a list that opened on a flip and closed on a timer, a Today control, a strip opened by
  tapping your row. Both reviews scored it lowest on control and consistency, so those went.
- **"Later today" is the Guide** over the picture (item 6). The Today control and the strip over
  the picture are gone.
- **A television's remote reaches everything** (item 6): OK pauses, the group tabs are reachable,
  Go live is in the quiet row, and Back on the bare picture goes Home, where arrows move between
  tiles and OK watches one.
- **Fast flipping holds one wash.** The picture is covered at once and uncovered once, 220 ms after
  the last flip, so a run of flips never shows the pictures in between.
- **Back goes one step.** Inside Playback details, Keyboard shortcuts or Remote buttons it returns
  to More.
- **A phone's find works by tapping.** The prototype draws a stand-in keyboard, and the Guide
  control steps aside while find is open.
- **Focus never leaves the screen, and there is one ring.** Whatever carries the ring is scrolled
  into view, the pinned channel carries it when the list's cursor is there, and a re-render keeps
  every list where it was scrolled.
- **The Guide moves through the day.** Earlier and Later page it by an hour; on a television Right
  past the last programme drawn does the same. A programme that has ended is dimmed.
- **By decade is headed by its decades**, each heading in its decade's colour. In the open list
  over the picture, Left and Right change the group, and the Channels control names a group other
  than All, because the group also steers Up and Down.
- **A message shows when it is raised**, inside the Guide too: OK on a later programme says when it
  starts.
- **A click outside a menu closes it** and does nothing else. A second click or tap on the spot a
  tune just re-centred does nothing until the pointer moves or the finger lifts.
- **Looking is not playing.** Pointing at another channel says "Looking at 9 Movie Matinee · still
  playing 7 Coast 90s". After a click the list re-centres, and the pointer must move before it
  looks at anything, so a second click cannot tune a channel that slid under it.
- **Looking is not playing in the Guide either.** With the cursor on another channel the header
  says "Looking at Game Night", and the docked picture keeps its own caption, "Playing 9 Movie
  Matinee".
- **A phone answers every tap.** A later programme shows its facts under the Guide, the Guide's
  control reads "Channels" while the Guide is up, and Go live leads its line.
- **The first-run hint** sits at the top of the picture, clear of the lists, and fades with the
  controls. More holds Keyboard shortcuts, or Remote buttons on a television.
- **The group tabs filter**, and "no match" offers "Show all channels".

## Reviews

Two independent critiques scored the mock's stills: 23 of 40, then 24 of 40 after the first rework.
Both praise the lower third, the crossing and the tune. The low scores sat where still frames cannot
answer (control, error prevention, recovery, help), so the checks moved to the prototype.

Three independent reviews by use have scored the prototype: 25 of 40, 24 of 40, then 27 of 40 after
the move to one channel list, with all 24 task attempts passing and nothing blocking. The third
review's faults (a repeat click tuning the wrong channel, messages that never showed, missing focus
rings, an unlabelled By decade, a Guide that could not move through the day) are fixed in the
prototype and listed above. The next check is a person who has never seen it, with the Tasks panel.
