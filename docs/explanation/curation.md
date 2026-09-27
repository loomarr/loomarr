# Programming guide

Every channel carries a **policy**: the rules deciding what may play, in what order, and how
often. The model proposes one from your intent; the scheduler enforces it. You can edit every
part of it under **Programming** on the channel page.

Everything the model proposes is shown as an editable chip before anything runs.

## Scope — what's eligible

Which titles the channel may draw from: specific series, genres, a year range, or a media-server
collection. Anything outside scope is never scheduled.

A year range stretches to fit your own picks. Approve a proposal containing *Alien* (1979) on a
channel scoped to 1982+, and the range widens rather than dropping a title you approved.

## Audience

A channel can cap ratings on the ladder
`TV-Y → TV-Y7 → TV-G/G → TV-PG/PG → TV-14/PG-13 → TV-MA/R/NC-17`. Anything above it is dropped.

Three things worth knowing:

- **A cap is for kids and teens, not a general default.** Unless your intent says something like
  "for the kids", "family" or "cartoons", any cap the model proposes is discarded. A channel
  about 1980s action heroes is meant to include the R-rated ones.
- **On a kids channel, unrated means excluded.** Missing rating metadata is common, so a kids
  channel treats unknown as unsafe. On other channels, unrated titles are allowed.
- **It's never relaxed.** When the pool runs thin, Loomarr relaxes other rules and will run a
  filler-heavy kids channel rather than a less-kids one.

Ratings come from your media server, at the series level — a mixed-rating series clears or fails
as a whole. Before you approve, the proposal shows what the policy removed ("14 items excluded:
11 over ceiling, 3 unrated").

## Separation and repetition

Minimum gaps between episodes of the same series, a cap on how many play back to back, and a
no-repeat window so something that just aired doesn't come straight back.

## Ordering

- **`sequential`** — S1E1 onward, looping. Good for a binge channel.
- **`shuffle`** — random, honouring the separation rules. Seeded, so it's reproducible.
- **`syndication`** — random without repeats until the pool is exhausted, then reshuffled. This
  is the weekday-rerun feel, and the default when a channel spans more than one series.

## Seasonality

A channel can restrict some content to a window of the year, so Halloween episodes appear in
October. An empty seasonal policy still varies with the calendar — the default is automatic,
not off.

## When the pool runs dry

A small library plus a tight scope plus a long no-repeat window can leave nothing eligible.
Loomarr relaxes the rules in a fixed order and records what it did on the channel:

1. Shorten the no-repeat windows (halved, never below 24 hours).
2. Relax the series gap and back-to-back cap.
3. Widen the year range slightly.
4. Pad with filler.

The audience cap and your explicit series choices are never relaxed.
