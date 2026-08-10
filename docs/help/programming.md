# Programming guide

Every channel carries a **policy**: the rules that decide what may play, in what order, and how
often. Loomarr's model reads your intent and *proposes* a policy; the scheduler *enforces* it the
same way every time. You can edit every part of it under **Programming** on the channel page.

The split is the thing to understand: the AI only ever proposes, and every value it proposes is
shown to you as an editable chip before anything runs. Enforcement is plain code, so a channel
behaves identically on Tuesday and on Friday.

## Scope — what's eligible

Which titles the channel may draw from: specific series, genres, an era (year range), or a
media-server collection. Anything outside scope is never scheduled, including during backfill.

**An era widens itself to fit your own picks.** If you approve a proposal containing *Alien*
(1979) on a channel the model scoped to 1982+, the range stretches to include it rather than
silently dropping a title you approved. Everything the model *didn't* pick still respects the
original range.

## Audience — the one rule that fails closed

A channel can carry a rating **ceiling** on the ladder
`TV-Y → TV-Y7 → TV-G/G → TV-PG/PG → TV-14/PG-13 → TV-MA/R/NC-17`. Anything above it is dropped.

Three behaviours worth knowing, because they are deliberate:

- **A ceiling is a kids/teen guardrail, not a general default.** Unless your intent carries a
  kids signal — "for the kids", "family", "cartoons", a named kids show — any ceiling the model
  proposes is discarded. A channel about 1980s action heroes is *supposed* to include the R-rated
  ones, and a cautious model must not quietly strip out the content the channel is about.
- **On a kids channel, unrated means excluded.** Missing rating metadata is the common real-world
  case, so a kids channel treats an unknown rating as unsafe rather than guessing. On an
  adult/general channel, unrated titles are allowed.
- **It is never relaxed.** When the pool runs thin, Loomarr degrades other things (below) and
  will happily run a filler-heavy kids channel. It will not run a less-kids one.

Ratings come from your media server's own field, at the **series** level — a mixed-rating series
clears or fails as a whole. Before you approve, the proposal tells you what the policy removed
("14 items excluded: 11 over ceiling, 3 unrated"), so you can rate your media or loosen the rule
as a deliberate choice.

## Separation and repetition — don't show the same thing twice in a row

Minimum gaps between episodes of the same series, a cap on how many play back to back, and a
no-repeat window so something that just aired doesn't come straight back.

## Ordering — making it feel like TV

- **`sequential`** — S1E1 onward, looping at the end. Right for a binge or marathon channel.
- **`shuffle`** — random, but honouring the separation rules above. Seeded, so it's reproducible.
- **`syndication`** — random *without repeats* until the pool is exhausted, then reshuffled. This
  is the weekday-rerun texture, and it's the default when a channel spans more than one series.

## Seasonality — holiday content at holiday time

A channel can restrict some content to a window of the year, so the Halloween episodes appear in
October and not in March. An empty seasonal policy still varies with the calendar — the default
is automatic, not off.

## When the pool runs dry

A small library plus a tight scope plus a long no-repeat window can leave nothing eligible.
Rather than airing dead air or silently ignoring your rules, Loomarr relaxes them **in a fixed
order**, and records every relaxation on the channel so you can see what happened:

1. Shorten the no-repeat windows (halved, never below 24 hours).
2. Relax the series gap and back-to-back cap.
3. Widen the era slightly.
4. Pad with filler pods.

**The audience ceiling and your explicit series/season choices are never relaxed.** Quality
degrades; safety and identity don't.
