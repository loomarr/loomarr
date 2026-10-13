# Real titles, people and brands in web and Go fixtures (#1592)

Date: 2026-10-13. Baseline: `git archive HEAD` (1d8b8b717) of `web/`, `cmd/`, `internal/`. The working tree was being edited
concurrently while this was collected (fixtures under `web/packages/fixtures` were already partly rewritten), so only
committed content is counted.

## Method

Scanned, at HEAD: (1) `web/packages/fixtures/**`; (2) every `web/apps/*/**/*.stories.*` and `web/native-stories/**`;
(3) `web/apps/*/src/**/*.test.*`, `web/apps/*/tests/**` (e2e incl. `mock-backend.ts`, visual specs), `web/packages/*/src/**/*.test.*`,
`web/apps/web/src/test/**`, `web/packages/api/src/msw/**` (`web/apps/web/src/test/fixtures/*.ts` and `src/test/msw` are included and clean; no `__fixtures__`/`mocks` dirs exist under `web/`);
(4) `web/scripts/tv-emulator-fixture-server.mjs`, `test-tv-emulator-journey.sh`, `test-tv-emulator-journey.test.mjs`;
(5) Go: `cmd/seed`, `cmd/demo-library`, `internal/demolibrary` (minus `guard_test.go`, which is the denylist itself), `internal/api/*_test.go`,
`internal/testkit/**` except `testkit/fixtures`, and `testdata` dirs other than `internal/eval` and `internal/playout`.

Tooling: `grep -oEI -f <pattern>` (case-sensitive; one alternation of about 150 real titles, characters, people and brands, built from the seed list plus
titles found by reading every `name:`/`title:`/`label:` string literal in those files). Core of it: `Simpsons|Springfield|\bBart\b|\bLisa\b|Milhouse|Radioactive Man|Star Trek|\bTNG\b|Neutral Zone|Best of Both Worlds|First Contact|Point Break|Con Air|Face/Off|The Rock\b|The Matrix|Terminator|Die Hard|Harry Potter|Breaking Bad|Predator|\bHeat\b|Sunny D|Gushers|Frosted Flakes|Kellogg|Campbell|Tootsie|Lovelace|\bHopper\b|Katherine Johnson|Alan Turing|Mary Jackson|Michael Mann|…`.
Ids: `(movie|series):(tmdb|tvdb):N`, `(tmdb|tvdb)_?[iI]d"?\s*[:=]\s*"?N`, `Item("movie"|"series", N)`, `TMDBID:|TVDBID: N`, counting N below 90,000,000 (the guard's `ProviderIDBase`);
plus the literal archive.org/YouTube ids `CampbellsSoupAdvert`, `kelloggs-bran-flakes`, `saturday-morning-cartoons-v-110`, `youtube-IaEBrNaHfUs`, and `image.tmdb.org`/`themoviedb.org` URLs.
Hit counts are matches (string hits plus id hits), not lines. Software products (Sonarr, Emby, Plex, Ollama, YouTube/Archive.org as platforms, OpenRouter, Slack, TMDB as a service) are excluded.
Noise removed after review: "Marvelous" (substring), "Apple/Google/Target" as ordinary words or LLM provider labels, "Cars" as a category, "Voyager" (channel-icon label, ambiguous, kept as a note only).
`Heat` counts are all title uses (`name: "Heat", year: 1995`, `Include Heat`). Low ids (1, 2, 3, 7, 8, 9, 100) are counted because the guard counts them,
though they may be placeholders rather than deliberate real ids.

Real archive.org/YouTube ids found: `CampbellsSoupAdvert` (sources-panel.test.tsx), `kelloggs-bran-flakes`, `saturday-morning-cartoons-v-110`, `youtube-IaEBrNaHfUs` (testcard.ts, all `archive.org/details/...`).
Other archive/YouTube urls (`station_break_one`, `classic_tv`, `UC123`, `video-one`, `PL123`, ...) are invented. No `image.tmdb.org` poster URLs appear anywhere under `web/` (`doc-view.test.tsx` has two `themoviedb.org` links). In Go, `icons_test.go` (3), `channelicon_test.go` (1) and `channellogo_internal_test.go` (1) use `image.tmdb.org` URLs.

## Summary

| Area | Files with hits | Total hits |
|---|---|---|
| 1. `web/packages/fixtures/**` | 6 of 21 | 82 |
| 2. stories (`web/apps/*/**/*.stories.*`, `web/native-stories/**`) | 43 of 186 | 198 |
| 3a. unit tests, apps (`src/**/*.test.*`, `src/test/**`) | 57 | 603 |
| 3b. unit tests, packages (`web/packages/*/src/**/*.test.*`) | 5 | 23 |
| 3c. e2e (`web/apps/*/tests/e2e/**`, incl. `mock-backend.ts`) | 4 | 142 |
| 3d. visual specs (`web/apps/web/tests/visual/**`; mobile and tv tests are clean) | 5 | 19 |
| 3e. package tests outside `src/` (`web/packages/*/tests/**`) | 12 | 89 |
| 4. tv-emulator scripts | 2 of 3 | 11 |
| 5. Go (`internal/api` tests, `testkit`, `testdata`, `cmd/*`) | 35 of 181 | 392 |
| Web total (areas 1-4) | 122 files | 1,078 |

`cmd/seed`, `cmd/demo-library`, `internal/demolibrary` (non-test): clean, 0 hits.
`legacyFixtures` has 27 entries; all 27 still carry hits, so none can be deleted yet.

Guard coverage (`isFixture`): scans `*.stories.tsx`, `web/packages/fixtures/src/**` (non-test), `mock-backend.ts` and `tv-emulator-fixture-server.mjs`.
It does NOT scan `*.test.*`, `tests/visual|e2e` specs other than `mock-backend.ts`, `.stories.ts`, the `test-tv-emulator-journey.sh` script, or any Go test. Every file below in a "tests" or Go table is therefore unguarded.
The final column marks `y` only for files listed in `legacyFixtures`. A guard-scanned file marked `n` with hits passes only because `realTitles` does not know the string (see Denylist gaps).

## Area 1. web/packages/fixtures

| path | count | categories | distinct real strings | in legacyFixtures? |
|---|---|---|---|---|
| `web/packages/fixtures/src/testcard/testcard.ts` | 56 | title,person,brand,id | Sunny D, Gushers, Fruit by the Foot, Frosted Flakes, Grrreat, Heat, Michael Mann, Point Break, Con Air, Face/Off, The Rock, Springfield, +16 more | y |
| `web/packages/fixtures/src/client-platform/surf/surf.ts` | 9 | title | Springfield, Homediddly, Point Break, Heat, Star Trek, TNG, Best of Both Worlds | y |
| `web/packages/fixtures/src/client-platform/programmes/programmes.ts` | 7 | title | Milhouse, Fallout Boy, Radioactive Man, Springfield, Simpsons, Homediddly | y |
| `web/packages/fixtures/src/requests/requests.ts` | 5 | person | Lovelace, Hopper, Katherine Johnson, Mary Jackson | n |
| `web/packages/fixtures/src/people/people.ts` | 4 | person | Lovelace, Hopper, Katherine Johnson, Alan Turing | n |
| `web/packages/fixtures/src/testcard/filler-refinement/filler-refinement.ts` | 1 | brand | Sunny D | n |

## Area 2. Stories

| path | count | categories | distinct real strings | in legacyFixtures? |
|---|---|---|---|---|
| `web/apps/web/src/components/loomarr/channels/channel-cycle-preview/channel-cycle-preview.stories.tsx` | 46 | title,id | Die Hard, Bluey, Rugrats, Shrek, Chuckie, Predator, Breaking Bad, TNG, Next Generation, Futurama, Xmas Story, Pulp Fiction, +19 more | y |
| `web/apps/web/src/components/loomarr/ai/proposal-edit/proposal-edit.stories.tsx` | 18 | title,id | Con Air, Heat, Point Break, Simpsons, Harry Potter, tmdbId: 1701, tmdbId: 949, tmdbId: 9426, tvdbId: 71663, tmdbId: 671, tmdbId: 1241, tmdbId: 672, +2 more | y |
| `web/apps/web/src/components/loomarr/channels/channel-lineup-editor/channel-lineup-editor.stories.tsx` | 15 | title,id | Predator, Breaking Bad, Heat, Point Break, Return of the Jedi, Star Wars, tmdbId: 106, tvdbId: 81189, movie:tmdb:949, movie:tmdb:9426, movie:tmdb:1892, movie:tmdb:11 | y |
| `web/apps/web/src/components/loomarr/ai/refine-review/refine-review.stories.tsx` | 11 | title,id | Heat, Point Break, Predator, Con Air, tmdbId: 949, tmdbId: 9426, tmdbId: 106, movie:tmdb:949, movie:tmdb:9426 | y |
| `web/apps/web/src/components/loomarr/feedback/generation-progress/generation-progress.stories.tsx` | 10 | title,id | "Speed", The Matrix, Breaking Bad, The Rock, movie:tmdb:100, movie:tmdb:603, series:tmdb:1396, movie:tmdb:101 | y |
| `web/apps/web/src/components/loomarr/ai/refine-panel/refine-panel.stories.tsx` | 8 | title,id | Heat, Predator, Point Break, tmdbId: 949, tmdbId: 106, movie:tmdb:949, movie:tmdb:9426 | y |
| `web/apps/web/src/components/loomarr/channels/channel-collections-scope/channel-collections-scope.stories.tsx` | 7 | title,brand | Criterion, Oscars 2024, "Alien", Batman, Die Hard, "Rocky", "Bond" | y |
| `web/apps/web/src/components/loomarr/channels/channel-card/channel-card.stories.tsx` | 6 | title | Heat, Con Air, Animaniacs, Poirot | y |
| `web/apps/web/src/components/loomarr/people/import-dialog/import-dialog.stories.tsx` | 5 | person | Lovelace, Hopper, Katherine Johnson, Margaret Hamilton | n |
| `web/apps/web/src/design-system/identity.stories.tsx` | 5 | title | Springfield, Simpsons, Monorail | y |
| `web/apps/web/src/components/loomarr/channels/channel-rules-editor/channel-rules-editor.stories.tsx` | 4 | title,id | Breaking Bad, Predator, series:tvdb:81189, movie:tmdb:106 | y |
| `web/apps/web/src/components/loomarr/channels/channel-series-scope/channel-series-scope.stories.tsx` | 4 | title,id | Simpsons, tvdbId: 71663, series:tvdb:71663, series:tvdb:73739 | y |
| `web/apps/web/src/components/loomarr/channels/channel-upcoming/channel-upcoming.stories.tsx` | 4 | title | Chuckie, Rowdyruff | n |
| `web/apps/web/src/people/users-page/users-page.stories.tsx` | 4 | person | Dorothy Vaughan, Mary Jackson, Hopper, Katherine Johnson | n |
| `web/apps/web/src/components/loomarr/channels/channel-icon-field/channel-icon-field.stories.tsx` | 3 | title | Next Generation, Deep Space Nine, Voyager | n |
| `web/apps/web/src/components/loomarr/channels/now-next-strip/now-next-strip.stories.tsx` | 3 | title | Heat, Con Air | y |
| `web/apps/web/src/components/ui/disclosure/disclosure.stories.tsx` | 3 | brand | Coca-Cola, Pepsi, Fanta | n |
| `web/apps/web/src/components/ui/video-player/timeline-scrubber/timeline-scrubber.stories.tsx` | 3 | title | Simpsons, Bart | y |
| `web/apps/web/src/design-system/guide-surface.stories.tsx` | 3 | title | Springfield, Simpsons, Bart | y |
| `web/native-stories/identity.stories.tsx` | 3 | title | Springfield, Simpsons, Monorail | y |
| `web/apps/web/src/components/loomarr/channels/channel-breaks/channel-breaks.stories.tsx` | 2 | brand | Frosted Flakes, TMNT | y |
| `web/apps/web/src/components/loomarr/channels/channel-ident/channel-ident.stories.tsx` | 2 | title | Springfield, Star Trek | y |
| `web/apps/web/src/components/loomarr/filler/clip-player/clip-player.stories.tsx` | 2 | brand | CTV | n |
| `web/apps/web/src/components/loomarr/people/invitation-dialog/invitation-dialog.stories.tsx` | 2 | person | Lovelace, Hopper | n |
| `web/apps/web/src/components/loomarr/shell/app-shell/app-shell.stories.tsx` | 2 | person | Lovelace | n |
| `web/apps/web/src/components/ui/caption/caption.stories.tsx` | 2 | title | Simpsons, Bart | y |
| `web/apps/web/src/components/ui/video-player/video-player.stories.tsx` | 2 | brand | CTV | n |
| `web/apps/web/src/design-system/layout.stories.tsx` | 2 | title | Monorail, Springfield | n |
| `web/apps/web/src/design-system/overlay.stories.tsx` | 2 | title | Simpsons, Monorail | y |
| `web/native-stories/overlay.stories.tsx` | 2 | title | Simpsons, Monorail | y |
| `web/apps/web/src/components/loomarr/dashboard/playout-panel/playout-panel.stories.tsx` | 1 | title | Springfield | n |
| `web/apps/web/src/components/loomarr/filler/segment-filmstrip/segment-filmstrip.stories.tsx` | 1 | brand | Frosted Flakes | y |
| `web/apps/web/src/components/loomarr/filler/source-search/source-search.stories.tsx` | 1 | id | kelloggs-bran-flakes | n |
| `web/apps/web/src/components/loomarr/filler/split-review-editor/split-review-editor.stories.tsx` | 1 | brand | Sunny D | n |
| `web/apps/web/src/components/loomarr/people/invitation-join/invitation-join.stories.tsx` | 1 | person | Hopper | n |
| `web/apps/web/src/components/loomarr/people/invitation-roster/invitation-roster.stories.tsx` | 1 | person | Hopper | n |
| `web/apps/web/src/components/ui/card/card.stories.tsx` | 1 | title | Springfield | n |
| `web/apps/web/src/components/ui/label/label.stories.tsx` | 1 | title | Springfield | n |
| `web/apps/web/src/design-system/surf-rail.stories.tsx` | 1 | title | Star Trek | y |
| `web/apps/web/src/filler/incoming/incoming.stories.tsx` | 1 | brand | Tootsie | n |
| `web/native-stories/layout.stories.tsx` | 1 | title | Monorail | n |
| `web/native-stories/tv-parity-fixtures.ts` | 1 | title | Blue Planet | n |
| `web/native-stories/watching-surface.stories.tsx` | 1 | title | Blue Planet | n |

## Area 3. Test fixtures (unit, e2e, visual)

| path | count | categories | distinct real strings | in legacyFixtures? |
|---|---|---|---|---|
| `web/apps/web/tests/e2e/channel-review.spec.ts` | 85 | title,id | Harry Potter, Wizarding World, Heat, The Matrix, Point Break, Con Air, Face/Off, The Rock, True Lies, "Speed", Crimson Tide, Air Force One, +8 more | n |
| `web/apps/web/src/components/loomarr/ai/proposal-edit/proposal-edit.test.tsx` | 75 | title,id | Heat, Simpsons, Harry Potter, Con Air, Face/Off, tmdbId: 949, tvdbId: 71663, tmdbId: 671, tmdbId: 672, tmdbId: 673, tmdbId: 1241, movie:tmdb:949, +7 more | n |
| `web/apps/web/src/suggest/channel-suggest-panel/channel-suggest-panel.test.tsx` | 68 | title,id | Ferris Bueller, Heat, Con Air, The Matrix, Face/Off, tmdbId: 9377, tmdbId: 949, movie:tmdb:9377, tmdbId: 1701, tmdbId: 603, tmdbId: 754, movie:tmdb:754, +1 more | n |
| `web/apps/web/src/components/loomarr/ai/proposal-review/proposal-review.test.tsx` | 47 | title,id | Heat, Con Air, Face/Off, The Matrix, Point Break, Great Indian Family, tmdbId: 949, tmdbId: 1701, tmdbId: 754, movie:tmdb:949, movie:tmdb:1, movie:tmdb:1701, +5 more | n |
| `web/apps/web/tests/e2e/mock-backend.ts` | 46 | title,id | Wizarding World, Harry Potter, The Rock, "Speed", Crimson Tide, Air Force One, The Matrix, Con Air, True Lies, Enemy of the State, Face/Off, Heat, +21 more | y |
| `web/apps/web/src/components/loomarr/channels/channel-lineup-editor/channel-lineup-editor.test.tsx` | 43 | title,id | Heat, Point Break, Predator, The Matrix, Star Wars, Breaking Bad, movie:tmdb:949, movie:tmdb:9426, movie:tmdb:106, movie:tmdb:603, movie:tmdb:11, tmdbId: 106, +3 more | n |
| `web/apps/web/src/components/loomarr/ai/refine-review/refine-review.test.tsx` | 25 | title,id | Heat, Point Break, Predator, Con Air, tmdbId: 949, tmdbId: 9426, tmdbId: 106, movie:tmdb:949, movie:tmdb:9426 | n |
| `web/apps/web/src/components/loomarr/channels/channel-cycle-preview/channel-cycle-preview.test.tsx` | 21 | title,id | Die Hard, Rugrats, South Park, King of the Hill, movie:tmdb:562, movie:tmdb:10719, movie:tmdb:772, series:tmdb:3022, series:tmdb:2190, series:tmdb:2122, movie:tmdb:1 | n |
| `web/apps/web/src/components/loomarr/channels/channel-series-scope/channel-series-scope.test.tsx` | 20 | title,id | Simpsons, Heat, series:tvdb:73739, series:tvdb:71663, tvdbId: 71663, tmdbId: 949 | n |
| `web/apps/web/src/filler/sources-panel/sources-panel.test.tsx` | 19 | brand | Campbell, CampbellsSoupAdvert | n |
| `web/apps/web/src/queue/request-detail/request-detail.test.tsx` | 19 | title,id | RoboCop, "Speed", tmdbId: 7, tmdbId: 8, tmdbId: 9, tmdbId: 181812, movie:tmdb:100 | n |
| `web/apps/web/src/test/filler.test.tsx` | 19 | brand | Frosted Flakes, TMNT | n |
| `web/apps/web/src/suggest/approval-queue/approval-queue.test.tsx` | 16 | title,id | Heat, Simpsons, tmdbId: 949, tvdbId: 71663, tmdbId: 456, movie:tmdb:949 | n |
| `web/apps/web/src/suggest/intent-form/intent-form.test.tsx` | 12 | title | Heat, Point Break, Con Air, Predator | n |
| `web/packages/core/src/provision/provision.test.ts` | 12 | id | tvdbId: 71663, tmdbId: 456, series:tvdb:71663, tmdbId: 4229, series:tmdb:4229, tmdbId: 603, movie:tmdb:603, movie:tmdb:0, tmdbId: 0, tvdbId: 0 | n |
| `web/apps/web/src/components/loomarr/ai/approval-queue-item/approval-queue-item.test.tsx` | 11 | title,id | Simpsons, tmdbId: 5001, tmdbId: 5004, tmdbId: 456, tmdbId: 1, tmdbId: 2, tmdbId: 3, tmdbId: 4 | n |
| `web/apps/web/src/components/loomarr/people/import-dialog/import-dialog.test.tsx` | 11 | person | Lovelace, Hopper, Margaret Hamilton | n |
| `web/apps/web/src/channels/use-channel-save/use-channel-save.test.tsx` | 10 | title,id | Heat, Point Break, Predator, movie:tmdb:949, movie:tmdb:9426, movie:tmdb:106 | n |
| `web/apps/web/src/channels/use-channel-lineup/use-channel-lineup.test.tsx` | 9 | title,id | Heat, Point Break, Predator, Breaking Bad, movie:tmdb:949, movie:tmdb:9426, movie:tmdb:106, series:tvdb:81189 | n |
| `web/apps/web/src/components/loomarr/channels/channel-rules-editor/channel-rules-editor.test.tsx` | 9 | title,id | Breaking Bad, series:tvdb:81189 | n |
| `web/apps/web/src/components/loomarr/feedback/generation-progress/generation-progress.test.tsx` | 9 | title,id | "Speed", The Matrix, movie:tmdb:100, tmdbId: 100, movie:tmdb:603, tmdbId: 603 | n |
| `web/apps/web/src/components/loomarr/filler/source-search/source-search.test.tsx` | 9 | brand,id | Kellogg, Bran Flakes, kelloggs-bran-flakes | n |
| `web/apps/web/tests/e2e/channel-recovery.spec.ts` | 9 | title | Heat, Point Break | n |
| `web/apps/web/src/suggest/manual-channel-builder/manual-channel-builder.test.tsx` | 8 | title,id | Heat, movie:tmdb:1, movie:tmdb:2, tmdbId: 1, tmdbId: 2, tmdbId: 949 | n |
| `web/apps/web/src/suggest/movie-collection-choices/movie-collection-choices.test.tsx` | 8 | id | tmdbId: 671, tmdbId: 1399, tmdbId: 67, tmdbId: 672, movie:tmdb:671, movie:tmdb:672, movie:tmdb:24 | n |
| `web/apps/web/src/test/channel-save.test.tsx` | 8 | title,id | Heat, Point Break, movie:tmdb:949, movie:tmdb:9426 | n |
| `web/apps/web/tests/visual/filler-split-review.spec.ts` | 8 | brand | Sunny D | n |
| `web/apps/web/src/components/loomarr/filler/clip-card/clip-card.test.tsx` | 7 | brand | Sunny D, Frosted Flakes | n |
| `web/apps/web/src/suggest/suggestion-expansion/suggestion-expansion.test.ts` | 7 | title,id | Face/Off, Point Break, movie:tmdb:754 | n |
| `web/apps/web/src/suggest/use-suggestion-run/use-suggestion-run.test.tsx` | 7 | title,id | "Speed", Heat, movie:tmdb:1, tmdbId: 949 | n |
| `web/apps/web/tests/visual/guide-surface-contract.spec.ts` | 7 | title | Springfield, Simpsons, Bart, Lisa Gets | n |
| `web/apps/web/src/components/loomarr/channels/channel-collections-scope/channel-collections-scope.test.tsx` | 6 | title,brand | Criterion, Alien Collection, Batman | n |
| `web/apps/web/src/discovery-feedback/discovery-feedback.test.tsx` | 6 | title,id | Heat, movie:tmdb:949 | n |
| `web/apps/web/src/filler/clip-details/clip-details.test.tsx` | 6 | brand | Tootsie | n |
| `web/packages/core/src/guide/guide.test.ts` | 6 | title | Radioactive Man, Simpsons | n |
| `web/apps/web/src/components/loomarr/ai/refine-panel/refine-panel.test.tsx` | 5 | title,id | Predator, tmdbId: 106 | n |
| `web/apps/web/src/components/loomarr/channels/channel-ident/channel-ident.test.tsx` | 5 | title | Springfield, Star Trek | n |
| `web/apps/web/src/components/loomarr/filler/split-review-editor/split-review-editor.test.tsx` | 5 | brand | Toys R Us, Sunny D | n |
| `web/apps/web/src/components/loomarr/people/people-roster/people-roster.test.tsx` | 5 | person | Hopper, Alan Turing, Lovelace | n |
| `web/apps/web/src/test/guide-page.test.tsx` | 5 | id | movie:tmdb:1, movie:tmdb:2, movie:tmdb:3, tmdbId: 3 | n |
| `web/apps/web/src/test/users.test.tsx` | 5 | person | Hopper, Lovelace | n |
| `web/apps/web/src/components/loomarr/channels/now-next-strip/now-next-strip.test.tsx` | 4 | title | Heat, Con Air | n |
| `web/apps/web/src/components/loomarr/filler/pod-timeline/pod-timeline.test.tsx` | 4 | brand | Sunny D | n |
| `web/apps/web/src/filler/incoming/incoming.test.tsx` | 4 | brand | Tootsie, HP Sauce, Magna-Doodle | n |
| `web/apps/web/src/suggest/suggestion-language/suggestion-language.test.ts` | 4 | title,id | Heat, series:tmdb:3921, movie:tvdb:123 | n |
| `web/apps/web/src/components/loomarr/channels/channel-card/channel-card.test.tsx` | 3 | title | Heat, Con Air | n |
| `web/apps/web/src/components/loomarr/filler/clip-player/clip-player.test.tsx` | 3 | brand | Frosted Flakes | n |
| `web/apps/web/src/components/loomarr/shell/search-command/search-command.test.tsx` | 3 | title | Heat, Con Air | n |
| `web/apps/web/src/components/ui/video-player/timeline-scrubber/timeline-scrubber.test.tsx` | 3 | title | Simpsons, Indiana Jones | n |
| `web/apps/web/src/components/ui/video-player/video-player.test.tsx` | 3 | brand | Frosted Flakes | n |
| `web/apps/web/src/suggest/use-proposal-review-edit/use-proposal-review-edit.test.tsx` | 3 | id | movie:tmdb:949 | n |
| `web/apps/web/src/components/loomarr/ai/episode-selection-label/episode-selection-label.test.ts` | 2 | title | Heat, Simpsons | n |
| `web/apps/web/src/components/loomarr/filler/clip-row/clip-row.test.tsx` | 2 | brand | Sunny D | n |
| `web/apps/web/src/components/loomarr/people/invitation-join/invitation-join.test.tsx` | 2 | person | Hopper | n |
| `web/apps/web/src/components/ui/panel-row/panel-row.test.tsx` | 2 | title | Springfield | n |
| `web/apps/web/src/components/ui/sheet/sheet.test.tsx` | 2 | person | Lovelace | n |
| `web/apps/web/src/filler/channel-filler/channel-filler.test.tsx` | 2 | brand | Frosted Flakes | n |
| `web/apps/web/src/filler/channel-filler/filler-clip-list/filler-clip-list.test.tsx` | 2 | brand | Frosted Flakes | n |
| `web/apps/web/src/help/doc-view/doc-view.test.tsx` | 2 | id | themoviedb.org | n |
| `web/apps/web/src/lib/policy-overrides/policy-overrides.test.ts` | 2 | id | series:tmdb:1 | n |
| `web/apps/web/src/queue/request-lists/request-lists.test.tsx` | 2 | id | tmdbId: 7 | n |
| `web/apps/web/src/suggest/live-proposal-outlook/live-proposal-outlook.test.tsx` | 2 | id | movie:tmdb:949 | n |
| `web/apps/web/tests/e2e/temporal-reviewer.spec.ts` | 2 | title | "Speed" | n |
| `web/apps/web/tests/visual/surf-rail-contract.spec.ts` | 2 | title | Star Trek, Best of Both Worlds | n |
| `web/packages/core/src/guide/guide-controller/guide-controller.test.ts` | 2 | title | Radioactive Man, Fallout Boy | n |
| `web/packages/core/src/requests/request-status/request-status.test.ts` | 2 | title,id | "Speed", movie:tmdb:100 | n |
| `web/apps/web/src/suggest/use-channel-refine/use-channel-refine.test.tsx` | 1 | title | Heat | n |
| `web/apps/web/src/test/story-coverage.test.ts` | 1 | title | Next Generation | n |
| `web/apps/web/tests/visual/identity-contract.spec.ts` | 1 | title | Monorail | n |
| `web/apps/web/tests/visual/people.spec.ts` | 1 | person | Katherine Johnson | n |
| `web/packages/core/src/requests/progress-line/progress-line.test.ts` | 1 | id | movie:tmdb:1 | n |

### 3e. Package tests outside `src/` (`web/packages/*/tests/**`)

Added after review: the scan above covered `web/packages/*/src/**` only, and `ui`, `ui-tv` and `player` keep their
tests in a sibling `tests/` directory. Counted at HEAD with the core pattern from the Method section (case-insensitive,
so lowercase ids such as `ch-springfield` count). The web total above excludes this row.

| File | Hits | Categories | Distinct real strings |
|---|---|---|---|
| `web/packages/ui/tests/guide.test.tsx` | 17 | title | Bart, Simpsons, Springfield |
| `web/packages/ui/tests/guide-journey.test.tsx` | 13 | title | Bart, Simpsons, Springfield |
| `web/packages/ui/tests/surf-rail.test.tsx` | 11 | title | Homediddly, Radioactive Man, Springfield |
| `web/packages/ui-tv/tests/focus-registry.test.ts` | 7 | title | bart, springfield (ids) |
| `web/packages/ui-tv/tests/surf-navigation.test.ts` | 6 | title | springfield (ids) |
| `web/packages/ui/tests/identity.test.tsx` | 6 | title | Monorail, Simpsons, Springfield |
| `web/packages/ui/tests/surf-data.test.ts` | 5 | title | Simpsons |
| `web/packages/player/tests/player-recovery.test.ts` | 5 | title | Simpsons |
| `web/packages/ui/tests/surf-journey.test.tsx` | 4 | title | Fallout Boy, Radioactive Man |
| `web/packages/ui/tests/overlay.test.tsx` | 4 | title | Monorail, Simpsons |
| `web/packages/ui/tests/requests.test.tsx` | 3 | person | Hopper, Lovelace |
| `web/packages/player/tests/catalog-refresher.test.ts` | 2 | title | Simpsons |

## Area 4. TV emulator scripts

| path | count | categories | distinct real strings | in legacyFixtures? |
|---|---|---|---|---|
| `web/scripts/test-tv-emulator-journey.sh` | 7 | title | Neutral Zone | n |
| `web/scripts/tv-emulator-fixture-server.mjs` | 4 | title | Simpsons, Neutral Zone, Star Trek | y |

(`test-tv-emulator-journey.sh` asserts on the on-screen text "The Neutral Zone" in 7 places; it has no `legacyFixtures` entry and the guard does not scan it. `test-tv-emulator-journey.test.mjs` is clean.)

## Area 5. Go

| path | count | categories | distinct real strings | in legacyFixtures? |
|---|---|---|---|---|
| `internal/api/channels_test.go` | 92 | title,id | TNG, South Park, King of the Hill, The Matrix, Terminator, Heat, Predator, Breaking Bad, Point Break, Star Wars, movie:tmdb:1, series:tvdb:655, +9 more | n |
| `internal/api/proposals_test.go` | 79 | title,id | The Matrix, "Speed", Simpsons, Predator, Heat, TMDBID: 603, tmdbId":100, movie:tmdb:100, tmdbId":603, movie:tmdb:603, movie:tmdb:1, tmdbId":456, +3 more | n |
| `internal/api/channel_lineup_test.go` | 22 | title,id | The Matrix, movie:tmdb:603, tmdbId":603, movie:tmdb:604, tmdbId":604 | n |
| `internal/api/guide_test.go` | 18 | title,brand,id | Heat, Predator, "Alien", Sunny D, Indiana Jones, Kellogg, Frosted Flakes, movie:tmdb:89 | n |
| `internal/api/movie_collections_test.go` | 16 | title,id | Harry Potter, TMDBID: 1241, TMDBID: 671, TMDBID: 672, movie:tmdb:671, movie:tmdb:673, series:tmdb:1399, movie:tvdb:671, movie:tmdb:550 | n |
| `internal/api/season_window_test.go` | 16 | title,id | Simpsons, Futurama, series:tvdb:71663, series:tmdb:615 | n |
| `internal/api/channeltimeline_test.go` | 15 | title,id | Indiana Jones, Springfield, Simpsons, Lisa, Bart, movie:tmdb:89, series:tmdb:456 | n |
| `internal/api/discovery_feedback_test.go` | 12 | id | movie:tmdb:603, movie:tvdb:603, movie:tmdb:0603 | n |
| `internal/api/icons_test.go` | 12 | title,id | Star Trek, Next Generation, image.tmdb.org, Deep Space Nine | n |
| `internal/api/filler_test.go` | 10 | brand | Kellogg, Campbell, McDonald | n |
| `internal/api/channelideas_test.go` | 9 | id | movie:tmdb:1, TMDBID: 1, TMDBID: 2, tmdbId": 1, tmdbId": 2, TMDBID: 9, tmdbId": 9 | n |
| `internal/api/pods_test.go` | 9 | brand | Frosted Flakes, Criterion | n |
| `internal/testkit/channeljourney/fixtures.go` | 9 | title,id | Simpsons, TVDBID: 762, TVDBID: 767, TVDBID: 760, TMDBID: 456 | n |
| `internal/api/api_test.go` | 8 | id | tmdbId":1111867, movie:tmdb:1111867, tmdbId":5, tmdbId":1, movie:tmdb:404, movie:tmdb:9, TMDBID: 9 | n |
| `internal/api/auth_flow_test.go` | 8 | title,id | "Speed", Con Air, Face/Off, tmdbId":1, tmdbId":42, tmdbId":9, tmdbId":77, tmdbId":100 | n |
| `internal/api/proposaljourneys_test.go` | 8 | title,id | "Speed", movie:tmdb:1, movie:tmdb:603, movie:tmdb:100, TMDBID: 100 | n |
| `internal/api/playoutguide_test.go` | 7 | title | Transformers, Bart, Simpsons, Heat, Predator | n |
| `internal/api/channelicon_test.go` | 6 | title,id | Star Trek, image.tmdb.org | n |
| `internal/api/harness_test.go` | 5 | id | tmdbId":41, tmdbId":42, movie:tmdb:42 | n |
| `internal/testkit/tmdb.go` | 5 | title | The Rock, "Speed", The Matrix, Breaking Bad | n |
| `internal/api/fillerresearchpresentation_test.go` | 3 | brand | Tootsie | n |
| `internal/api/search_test.go` | 3 | title,id | The Matrix, "Alien", TMDBID: 603 | n |
| `internal/api/device_flow_test.go` | 2 | title | Heat | n |
| `internal/api/ontheway_test.go` | 2 | id | TVDBID: 90001, TMDBID: 90002 | n |
| `internal/api/programmingchanges_test.go` | 2 | id | movie:tmdb:1 | n |
| `internal/api/sizeconfidencewire_test.go` | 2 | title,id | The Matrix, tmdbId": 603 | n |
| `internal/fillerenrichment/testdata/hp-sauce.info.json` | 2 | brand | HP Sauce | n |
| `internal/fillerenrichment/testdata/tootsie-pop.info.json` | 2 | brand | Tootsie | n |
| `internal/recommend/testdata/channel-recommendation-snapshots-v2.json` | 2 | brand | Fanta | n |
| `internal/api/channellogo_internal_test.go` | 1 | id | image.tmdb.org | n |
| `internal/api/events_test.go` | 1 | id | movie:tmdb:1 | n |
| `internal/api/guidehighlights_internal_test.go` | 1 | id | movie:tmdb:1 | n |
| `internal/api/invitations_test.go` | 1 | person | Hopper | n |
| `internal/api/newthisweek_test.go` | 1 | id | movie:tmdb:1 | n |
| `internal/testkit/channeljourney/controls.go` | 1 | title | Simpsons | n |

## Denylist gaps

Distinct real strings found that `realTitles` in `internal/demolibrary/guard_test.go` does not match (the denylist is case-insensitive on word boundaries, so
"Kellogg" without `'s` and "Grrreat" without "They're" also miss). Counts are hits across all scanned web and Go files; the number in brackets is hits inside files the guard
actually scans (`*.stories.tsx`, `web/packages/fixtures/src/**`, `mock-backend.ts`, `tv-emulator-fixture-server.mjs`), where the gap means a real title passes today.

Franchise titles and characters (add to `realTitles`):
Heat [16], Harry Potter [12], Predator [6], Breaking Bad [5], Springfield [14], Bart [5], Lisa (Simpsons use; `\bLisa\b`), Homer [1], Milhouse [1], Monorail (Marge vs. the Monorail) [6], Chuckie (Chuckie's a Lefty) [4], Lisa Gets an A, Bart the Genius, Bart the Mother, Kidney Trouble, Fallout Boy [1], Radioactive Man [2], Homediddly [2], Neutral Zone [1], The Best of Both Worlds / Best of Both Worlds [2], First Contact [1], TNG [4], The Next Generation / Next Generation [4], Deep Space Nine [1], Voyager (ambiguous), The Rock [3], "Speed" [3], Ferris Bueller's Day Off, RoboCop (RoboCop 2), True Lies [1], Enemy of the State [1], Crimson Tide [1], Air Force One [1], Alien (Alien Collection), Rocky, Bond, Indiana Jones (Temple of Doom), Back to the Future [1], Return of the Jedi [1], Pulp Fiction [1], The Goonies [1], Poirot [1], The Great Indian Family, Blue Planet [1], Oscars 2024 [1], Wizarding World [1], Rugrats [1], Animaniacs [2], Rowdyruff (The Rowdyruff Boys) [2], Gargoyles [1], Dexter's Laboratory [1], Bluey [2], South Park, King of the Hill, Shrek [2], Futurama S2E4 "Xmas Story" [1], Michael Mann [1].

Brands and products (add): Sunny D [8], Gushers [4], Fruit by the Foot [3], Kellogg (bare) [1], Bran Flakes [1], Grrreat (bare) [1], Campbell (Campbell's Soup), Tootsie (Tootsie Pop) [1], Toys R Us, TMNT, HP Sauce, Magna-Doodle, Criterion [1], CTV (CTV Life with Bonnie) [4], Coca-Cola [1], Pepsi [1], Fanta [1], McDonald (Go filler_test only), TV network/brand "Nickelodeon", "Cartoon Network", "HBO" (seeded in the brief; no hits in scanned files).

People used as fixture user names (not in the denylist at all): Ada Lovelace / Lovelace [8], Grace Hopper / Hopper [7], Katherine Johnson [4], Mary Jackson [2], Alan Turing [1], Margaret Hamilton, Dorothy Vaughan, Michael Mann [1].
`web/packages/fixtures/src/people/people.ts` and `requests/requests.ts` carry these and are guard-scanned yet not in `legacyFixtures`, so they pass today.

Real ids and artwork not caught by `realProviderID`:
- archive.org item ids `CampbellsSoupAdvert`, `kelloggs-bran-flakes`, `saturday-morning-cartoons-v-110`, `youtube-IaEBrNaHfUs` (only title/brand words, no id pattern exists for archive.org or YouTube ids).
- `themoviedb.org` and `image.tmdb.org` URLs (Go icon and logo tests; web doc-view test link text).
- Provider ids written without an `id` suffix or `Item("movie", N)` form are the only forms the regex knows. All forms found in scanned files (`movie:tmdb:N`, `series:tvdb:N`, `tmdbId: N`, `TMDBID: N`, `tmdbId":N`) are covered, but only inside guarded files; the same ids appear in about 40 unguarded web test files and 20 Go tests (below), so the guard's scope, not its regex, is the gap.

Scope gaps (strings the denylist would catch, but in files the guard never reads): all `*.test.*` files, `tests/visual/*.spec.ts`, `tests/e2e/channel-review.spec.ts` and the other e2e specs, `web/scripts/test-tv-emulator-journey.sh`, `.stories.ts` (non-tsx), and every Go test under `internal/api` and `internal/testkit`.

### Guard-scanned files that carry real strings and are NOT in `legacyFixtures` (pass only because of the denylist gaps)

| path | hits | distinct real strings |
|---|---|---|
| `web/apps/web/src/components/loomarr/channels/channel-icon-field/channel-icon-field.stories.tsx` | 3 | Next Generation, Deep Space Nine, Voyager |
| `web/apps/web/src/components/loomarr/channels/channel-upcoming/channel-upcoming.stories.tsx` | 4 | Chuckie, Rowdyruff |
| `web/apps/web/src/components/loomarr/dashboard/playout-panel/playout-panel.stories.tsx` | 1 | Springfield |
| `web/apps/web/src/components/loomarr/filler/clip-player/clip-player.stories.tsx` | 2 | CTV |
| `web/apps/web/src/components/loomarr/filler/source-search/source-search.stories.tsx` | 1 | kelloggs-bran-flakes |
| `web/apps/web/src/components/loomarr/filler/split-review-editor/split-review-editor.stories.tsx` | 1 | Sunny D |
| `web/apps/web/src/components/loomarr/people/import-dialog/import-dialog.stories.tsx` | 5 | Lovelace, Hopper, Katherine Johnson, Margaret Hamilton |
| `web/apps/web/src/components/loomarr/people/invitation-dialog/invitation-dialog.stories.tsx` | 2 | Lovelace, Hopper |
| `web/apps/web/src/components/loomarr/people/invitation-join/invitation-join.stories.tsx` | 1 | Hopper |
| `web/apps/web/src/components/loomarr/people/invitation-roster/invitation-roster.stories.tsx` | 1 | Hopper |
| `web/apps/web/src/components/loomarr/shell/app-shell/app-shell.stories.tsx` | 2 | Lovelace |
| `web/apps/web/src/components/ui/card/card.stories.tsx` | 1 | Springfield |
| `web/apps/web/src/components/ui/disclosure/disclosure.stories.tsx` | 3 | Coca-Cola, Pepsi, Fanta |
| `web/apps/web/src/components/ui/label/label.stories.tsx` | 1 | Springfield |
| `web/apps/web/src/components/ui/video-player/video-player.stories.tsx` | 2 | CTV |
| `web/apps/web/src/design-system/layout.stories.tsx` | 2 | Monorail, Springfield |
| `web/apps/web/src/filler/incoming/incoming.stories.tsx` | 1 | Tootsie |
| `web/apps/web/src/people/users-page/users-page.stories.tsx` | 4 | Dorothy Vaughan, Mary Jackson, Hopper, Katherine Johnson |
| `web/native-stories/layout.stories.tsx` | 1 | Monorail |
| `web/native-stories/watching-surface.stories.tsx` | 1 | Blue Planet |
| `web/packages/fixtures/src/people/people.ts` | 4 | Lovelace, Hopper, Katherine Johnson, Alan Turing |
| `web/packages/fixtures/src/requests/requests.ts` | 5 | Lovelace, Hopper, Katherine Johnson, Mary Jackson |
| `web/packages/fixtures/src/testcard/filler-refinement/filler-refinement.ts` | 1 | Sunny D |

(Some of these are bare "Springfield" used as a place name in a card or label story; it is still a Simpsons reference per #1592 and is listed for a decision.)

## Out of scope: recorded provider evidence

`internal/testkit/fixtures/**`: 52 captured provider responses (Emby, Jellyfin, Plex, Tunarr, Seerr, TMDB shapes); 14 of the 52 contain at least one match from the pattern above.
The existing guard deliberately excludes this directory (recorded evidence of real servers' wire shapes, never rendered). Not itemised here.

Also not inventoried (semantic-eval corpora and media test data, never rendered in the web app): `internal/eval/testdata` 74 files (41 with matches, mostly real TMDB-keyed
query-expansion and planner catalogs), `internal/playout/testdata` 147 files (0 matches), plus `internal/recommend/testdata`, `internal/fillerenrichment/testdata` (HP Sauce, Tootsie Pop captured yt-dlp info) which appear in the Go table above.
