# Library prototype — plain words (PROTOTYPE, #2151 step 2)

The maintainer's verdict on the first pass: "it looks very AI techie — the language used". The Library
is for a household admin, not an operator. Every visible word follows this sheet. Anything technical
lives behind ONE "Technical details" disclosure on a title page, and even there uses whole words.

## Rules

1. Say what it means for the household, not what the machine did. "Ready to air", not "✓ measured".
2. No jargon on the surface: never show identity source, facts state, provenance, probe, header, walk,
   root, webhook, change notify, NFO, TVDB/TMDB/IMDb ids, revision, codec names, files/s, paths.
3. No monospace for words. Monospace only for a file path inside Technical details.
4. A problem is a sentence with a way out: what happened, what Loomarr is doing, the one action.
5. Counts in words people use: "7 seasons", "3 need you", "about 8 minutes left".
6. Sentence case. No "·"-chains of more than two facts.

## Words

| Was (techie)                                                  | Now (plain)                                                                                                                                                                   |
| ------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Items / Unmatched / Collections tabs                          | All · Movies · Shows · Collections, plus a "Needs you (3)" strip on top                                                                                                       |
| Unmatched                                                     | "We're not sure what this is" (in Needs you)                                                                                                                                  |
| Confirm / Search… / Not a title                               | "Yes, that's it" · "Pick a different title" · "Ignore this"                                                                                                                   |
| best candidate, agreement ticks title/year/runtime            | "Is this Home Movies (1999)?" + one reason line: "The name and length match; the year doesn't."                                                                               |
| identity: Sonarr / Radarr / NFO / Linked / Embedded           | hidden; Technical details: "Recognised from Sonarr"                                                                                                                           |
| facts: waiting / reading / ✓ read / ✓ measured / ! failed     | nothing when fine; "Getting ready" while new; "1 episode can't be played" when broken                                                                                         |
| 118/154 read                                                  | "Getting ready — 118 of 154 episodes" (only while not ready)                                                                                                                  |
| stray copy 8.1 GB                                             | "An extra copy is taking 8.1 GB — it never plays" + "Remove extra copy"                                                                                                       |
| 1 file unreadable / probe failed                              | "1 episode can't be played — the file may be damaged" + "Check again"                                                                                                         |
| certification                                                 | Rating                                                                                                                                                                        |
| source chip / provenance popover / rule                       | Technical details → "Where details come from": "Description and rating from TMDB. Episode names from Sonarr."                                                                 |
| Refresh metadata / Re-read files / Fix match                  | "Update details" · "Check files again" · "Wrong title?"                                                                                                                       |
| Fix match owned by Sonarr                                     | "Sonarr decides what this is. Change it in Sonarr." + "Open Sonarr"                                                                                                           |
| Used by CH 4                                                  | "On Saturday Morning (channel 4)"                                                                                                                                             |
| Up to date · last change 3m ago                               | "Up to date — checked 3 minutes ago"                                                                                                                                          |
| Importing your library + stages + 12,400/33,135 · 41 files/s  | "Adding your media — about 8 minutes left" + bar. Titles appear as they're ready. No stages.                                                                                  |
| Import paused while CH 4 plays                                | "Paused so Saturday Morning plays smoothly — picks up when it's done"                                                                                                         |
| /media/movies is offline                                      | "Can't reach your Movies folder. Nothing has been removed; channels skip those movies until it's back." + "Check again"                                                       |
| 2,406 movies look removed (held)                              | "2,406 movies seem to be gone from your Movies folder. We haven't removed anything — if a drive is unplugged, plug it back in." + "Check again" / "They're gone, remove them" |
| Confirm where your media is                                   | "Tell Loomarr where your media is"                                                                                                                                            |
| Scan roots / root / walked / readable ✓                       | "Media folders" · folder · "Checked 12 minutes ago" · "Loomarr can open this folder" / "Loomarr can't open this folder"                                                       |
| Confirm root / Remove / Add root                              | "Use this folder" · "Stop using" · "Add a folder"                                                                                                                             |
| path mappings (derived)                                       | Advanced: "Folders have different names in other apps"                                                                                                                        |
| Identity sources / Use as library source / webhook registered | "Sonarr and Radarr" · "Let Sonarr name my shows" · "Sonarr tells Loomarr about new downloads ✓"                                                                               |
| Live changes / change notify                                  | "Loomarr notices new and deleted files on its own ✓"                                                                                                                          |
| Metadata language / country                                   | "Details language" · "Ratings country"                                                                                                                                        |
| Refresh all metadata                                          | "Update all details"                                                                                                                                                          |
| TMDB collection, 4 of 6 owned, read-only                      | "From TMDB — you have 4 of 6"; can't be edited, can be copied: "Make my own copy"                                                                                             |
| from Emby                                                     | "Brought over from Emby"                                                                                                                                                      |
