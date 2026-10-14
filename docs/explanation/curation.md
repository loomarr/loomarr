# How curation works

**For:** household admins who want to understand, or change, what a channel plays.
**You'll get:** what each part of a channel's policy does, and how the scheduler applies it.

Every channel carries a **policy**: the rules deciding what may play, in what order, and how
often. The model proposes one from your intent; the scheduler enforces it. You can edit every
part of it under **Programming** on the channel page.

Everything the model proposes is shown as an editable chip before anything runs.

## Scope — what's eligible

Which titles the channel may draw from: specific series, genres, a year range, or a media-server
collection. Anything outside scope is never scheduled.

A year range stretches to fit your own picks. Approve a proposal containing a 1979 space
horror film on a channel scoped to 1982 and later, and the range widens rather than dropping a
title you approved.

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
- **`shuffle`** — random, honoring the separation rules. Seeded, so it's reproducible.
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

## Filler in the breaks

Filler follows the same idea as programming: the channel's policy decides, and the scheduler
applies it the same way every time. [Set up filler](../guides/filler.md) covers the steps.

### What Loomarr knows about a clip

Matching a clip to a channel needs several separate facts:

- **Kind:** commercial, bumper, station ID, PSA, trailer or interstitial. Kind decides where a
  clip may play.
- **Era** and **audience:** scheduling facts, each checked on its own.
- **Brand:** shown only when the clip's own text or picture supports it.
- **Geography:** national or local, with a country and an optional market. Unknown means it
  still needs review; it never means local.
- **Tags:** what the clip is about (products and topics, format, seasonal and audience cues),
  from the vocabulary under **Filler → Manage**.

A tag describes; it doesn't change where a clip may play. Tags form a hierarchy, so picking a
broad tag such as Food also matches clips tagged Cereal. A clip stores only the tags chosen for
it, so reshaping the hierarchy later never rewrites old decisions.

When an AI model is set up, Loomarr uses it to fill in facts the clip's own details don't answer.
It can pick only tags that already exist; it can't invent one. It can also look up a clip's
likely era and place in public sources; [Privacy](privacy.md) lists what that sends.

### Safety comes first

- **Unknown audience is never safe for children.** Commercials with no tags are a last resort,
  and only on general and late-night channels. Kids and family channels never get them.
- **A new channel starts from its own policy.** A channel created from a proposal draws filler
  matching its era and audience: a kids channel starts from kids-safe filler, and any other
  channel from general-audience filler. Loomarr doesn't guess product categories from the
  shows' genres.
- **Geography is never loosened.** A channel set to a US market takes US-national clips and that
  market's local clips. Clips from other countries or markets stay in the library but never
  enter that channel's pool, whatever the fallback. The guide's time zone only changes how times
  are shown; it never sets a country.
