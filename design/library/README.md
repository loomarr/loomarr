# Library — the mock of record (#2151)

The design for the **Library** nav item, **Settings → Library** and the wizard's **Library** step,
on desktop and responsive mobile web (decision 31). The maintainer chose it in review on
2026-11-02. Build from these sheets, [`docs/frontend-design.md`](../../docs/frontend-design.md) and
the design-system primitives named below.

| Sheet | Frame | Screens |
| --- | --- | --- |
| [`sheet-desktop.webp`](sheet-desktop.webp) | 1440 × 900, pointer | 21 |
| [`sheet-phone.webp`](sheet-phone.webp) | 390 × 844, touch | 8 |

Desktop shows the Library in six scan states (idle, importing, paused, held removal, folder
offline, fresh install), then Needs you, a series, a movie, a series with Technical details open,
Collections, the collection editor, Settings → Library in the same six states, and the wizard step
fresh, importing and with an offline folder. Phone shows the Library, Needs you, a series, a movie,
Collections, the collection editor, Settings → Library and the wizard step.

**Source.** The sheets are rendered from the step-2 prototype, variant **D** ("Appliance, plain
words"), on the throwaway branch
[`prototype/library-2151`](https://github.com/loomarr/loomarr/tree/prototype/library-2151). Run Vite
and open `/prototype/library?variant=D&screen=<screen>&scan=<state>&device=<desktop|phone>` to see
one screen at full size, or to compare it with the rejected variants A, B and C. The prototype is
not product code; build from this mock, never by copying it. Its word sheet is
`web/apps/web/src/library/library-prototype/PLAIN-WORDS.md` on that branch.

**Stand-ins.** Posters are `ArtworkFrame` on the artwork-placeholder token with the title set in
it; the real ones are each title's artwork (decision 19). The stub data is invented for the mock.

## Settled in review

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

## Screens

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
- **Settings → Library** (decision 29): Status, Ready so far while importing, Keeping up to date
  (watch, Sonarr, Radarr), Media folders as cards with "Loomarr can open this folder", where it
  was found, when it was checked, and Use this folder / Stop using. A held removal and an offline
  folder are calm warning callouts with one primary action.
- **Wizard step** (decision 30): "Where's your media?", the same folder cards, Continue never
  waits for the import.

## Gaps in the design system, for the build

- `ArtworkFrame` fixes a 16:9 ratio; posters need 2:3. The mock sizes it from a 2:3 box.
- `Disclosure` passes `nativeID` through to the DOM, which React warns about on web.
- `Field` sets typed text in the data (mono) face; fine for paths, odd for a title search.
