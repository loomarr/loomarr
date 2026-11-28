# Library — the mock of record (#2151, #4476)

The design for the **Library** nav item, **Settings → Library** and the wizard's **Library** step,
on desktop and responsive mobile web (decision 31). Build from these sheets,
[`docs/frontend-design.md`](../../docs/frontend-design.md) and the design-system primitives named
below.

**The 2026-11-21 sheets are the mock of record for the Library wall, Needs you and the title page**
(#4476, approved in review on 2026-11-21). They replace those screens of the 2026-11-02 sheets.
Settings → Library and the wizard step stay as the 2026-11-02 sheets draw them (#2151, chosen in
review on 2026-11-02).

| Sheet | Frame | Screens | Mock of record for |
| --- | --- | --- | --- |
| [`sheet-desktop-4476.webp`](sheet-desktop-4476.webp) | 1440 × 900, pointer | 26 | The wall, Needs you, the title page |
| [`sheet-phone-4476.webp`](sheet-phone-4476.webp) | 390 × 844, touch | 26 | The wall, Needs you, the title page |
| [`sheet-desktop.webp`](sheet-desktop.webp) | 1440 × 900, pointer | 21 | Settings → Library, the wizard step |
| [`sheet-phone.webp`](sheet-phone.webp) | 390 × 844, touch | 8 | Settings → Library, the wizard step |

The 2026-11-21 sheets show each screen twice over, once for desktop and once for phone. The wall
appears with titles needing a look, all clear, importing, loading, a search with no results and
home videos, plus Collections with none made yet. A member sees the wall, an empty search and
Collections. Needs you appears with several files waiting, after Ignore with its undo, after a
folder-wide Ignore, on the Ignored tab and all caught up. The title page shows a movie as it opens,
with the Fix menu open, with Technical details open, with an extra copy and after removing it. It
shows a series as it opens, with one episode opened and with episodes that can't be played, a home
video, and a member's movie and series.

The 2026-11-02 sheets show the Library in six scan states, Needs you, a series, a movie, a series
with Technical details open, Collections, the collection editor, Settings → Library in the same six
states, and the wizard step fresh, importing and with an offline folder.

**Source.** The 2026-11-21 sheets are rendered from the throwaway branch
[`prototype/library-4476`](https://github.com/loomarr/loomarr/tree/prototype/library-4476). Run Vite
and open
`/prototype/library?screen=<wall|needs|title>&state=<state>&role=<admin|member>&title=<movie|series|home>&device=<desktop|phone>`
to see one screen at full size. The route file lists every state. The 2026-11-02 sheets are
rendered from [`prototype/library-2151`](https://github.com/loomarr/loomarr/tree/prototype/library-2151),
variant **D** ("Appliance, plain words"), at
`/prototype/library?variant=D&screen=<screen>&scan=<state>&device=<desktop|phone>`. Its word sheet
is `web/apps/web/src/library/library-prototype/PLAIN-WORDS.md` on that branch. Neither prototype is
product code; build from this mock, never by copying it.

**Stand-ins.** Posters, backdrops and stills are drawn art in the title's colours, with the title
set in the poster; the real ones are each title's artwork (decision 19). Cast without a photo show
their initials. The stub data is invented for the mock.

## Settled in review (#2151, 2026-11-02)

The maintainer raised these in order; they amend decisions 23–28 and 34 of #2151.

1. **An appliance for a household, not an operator console.** Variants A (dense table), B and C
   read as "AI techie". Leading with artwork and plain words replaced them.
2. **Plain words everywhere.** "Needs you", not "Unmatched"; "Yes, that's it / Pick a different
   title / Ignore this", not "Confirm / Search / Not a title"; "Rating", not "Certification";
   "Update details / Check files again / Wrong title?", not "Refresh / Re-read / Fix match".
3. **Technical facts sit behind one "Technical details" disclosure** on the title page:
   where the details came from, the title match, file, size, picture, sound, subtitles, length.
   This replaces the per-field provenance chips of decision 25.
4. **Not childish:** stand-in artwork is dark and quiet, the chrome is neutral, and the artwork
   carries the colour.
5. **Built from the design system, not beside it.** Amber is focus and the one primary action per
   view. Import progress is `ProgressTrack` in its `tune` tone (preparation), neutral when paused.

## Settled in review (#4476, 2026-11-21)

1. **Posters first.** The wall opens on the posters. Needs you is one line above them, with a
   count and Review, not a strip of cards.
2. **The wall has a count, tabs, sort, genre and a letter rail.** The tabs are All, Movies, Shows
   and Collections. The letter rail is on desktop only. On a phone the wall keeps one row, the
   search and a "Filters" control that counts what narrows it. The tabs, sort, genre and the other
   filters sit in a sheet that control opens, which also says how many titles they leave (A5
   critique, #4476). This supersedes the inline rows in the phone mock.
3. **Members browse read-only.** A member sees the wall and the title pages, with no Needs you, no
   Fix and no settings. Members have their own phone bar, with Library in it.
4. **Needs you is grouped by folder, and the file name leads each card.** It has two tabs, Waiting
   and Ignored. Every Ignore has an undo. Ignore works for one file or a whole folder. A home video
   gets "It's ours, keep it as is".
5. **The title page is a TV-app page.** A backdrop with the title over it, then the facts, the
   story and the channels. It has:
   - the Fix menu, on a solid surface over any backdrop;
   - cast, with initials until a photo is imported;
   - next airing, under "On these channels";
   - Technical details with "More about this file";
   - per-episode details: an episode row opens to that episode's own file;
   - the extra-copy card and the can't-be-played card;
   - for a member, one plain technical line in place of Technical details.
6. **Mono type is for file names only.** A long file name is shortened in the middle, so its end
   stays readable.
7. **One action name: "Check files again".**

## Screens (2026-11-02)

The Library, Needs you and title page entries below are replaced by the 2026-11-21 sheets. They stay
for the decisions they cite.

- **Library** (decision 22): a one-line status from `ScanHealth` with "Library settings"; the
  **Needs you** strip (first four, "Show N more"); `Tabs` All · Movies · Shows · Collections; a
  `Field` search; the poster grid (6 / 4 / 3 across). A title that is not ready yet is dimmed with
  a "Getting ready" badge; one that needs a look carries a warning `StatusDot`.
- **Needs you** (decision 27, was Unmatched): one card per group, best candidate first, the reason
  in one sentence ("The name and year match."), the files in a `Disclosure`. Answering locks the
  card in (static → clear, 200 ms) and it leaves. The nav badge is the same count as the heading.
- **Title page** (decisions 25–26): poster, title, year and length, rating `Badge`, genres,
  overview, Update details / Check files again / Wrong title? ("Sonarr decides what this is"
  links out for arr-grounded items). An extra copy is a warning card with Remove extra copy
  (decision 37). "On these channels" uses `ChannelIdentity`. Series list seasons as
  `Disclosure`s; specials say where they air.
- **Collections** (decision 28): Yours (editable, New collection) and Ready-made sets (TMDB,
  "you have 3 of 4", Make my own copy). Delete is disabled with the reason while a channel uses it.
- **Settings → Library** (decision 29): Status, the titles found while importing (shown, not
  said), Keeping up to date only when the watch, Sonarr or Radarr is waiting, Media folders as cards with "Loomarr can open this folder", where it
  was found, when it was checked, and Use this folder / Stop using. A held removal and an offline
  folder are calm warning callouts with one primary action.
- **Wizard step** (decision 30): "Where's your media?", the same folder cards, Continue never
  waits for the import.

## Gaps in the design system, for the build

- `ArtworkFrame` fixes a 16:9 ratio; posters need 2:3. The mock sizes it from a 2:3 box.
- `Tabs` draw buttons; the wall and Needs you use underline tabs.
- There is no letter rail for jumping through the wall.
- There is no toast with an Undo action.
- There is no file name that shortens in the middle.
- There is no hero: a backdrop with a scrim and the title over it.
- There is no person avatar that falls back to initials.
- The shell's search field is 12 px wider than the side navigation, so a full-bleed page clips it.
  The mock insets the title page's backdrop to keep clear of it.
