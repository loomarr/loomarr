# Concepts

The short version of how Loomarr thinks.

## Who does what

Loomarr decides **what plays and in what order**. It doesn't stream or transcode —
**Tunarr** does that, and your **media server** owns the library. If you stop using
Loomarr, the channels keep playing.

## Intent → proposal → channel

You describe a channel; the suggester returns a **proposal**:

- a **lineup** of titles you already have, and
- an **acquisition list** of titles you don't.

Every pick is **grounded** — a real title from your library or TMDB. The model can't invent
one. If nothing grounds, the run fails clearly instead of making something up.

## Approval — the one gate

A proposal does nothing until an **admin approves** it. Approval is the only place
resources get spent: it starts the downloads and **creates the channel**. Members can
suggest and review; only admins approve.

## Backfill

Titles move **wanted → downloading → available**. A channel is built from what's available
*now*; anything missing becomes a **pending** slot filled with commercials, and is swapped
for the real program **the moment it lands**. The channel improves itself — that's
backfill.

## Series

A movie is one program. A **series** expands into one program per episode you actually
have, in the channel's order. New episodes join on the next refresh.

## Filler & pods

Between programs, Loomarr leaves gaps and fills them with **pods** — short runs of
bumpers/commercials from your [filler](filler) folder, matched to the channel. No filler
just means no commercials; the channel still plays.

## Policy — a kids channel stays a kids channel

A themed channel carries a **policy**: which content, an audience rating cap, no repeats,
ordering. The model *extracts* it from your intent ("for the kids" → a `TV-Y7` cap); the
scheduler *enforces* it, the same way every time. Anything over the cap (or unrated) is
dropped — fail-closed, so a kids channel never leaks something it shouldn't.
