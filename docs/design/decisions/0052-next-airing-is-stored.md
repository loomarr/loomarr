# 0052. A title's next airing is stored, not computed per request

- **Date:** 2026-11-25
- **Status:** accepted

## Context

A title's page names the channels whose lineup holds it, and the household wants to know when it
is next on (#4476). The guide already answers that for one channel: its resolver walks the
channel's timeline, window by window, through the same cycle playout airs. Asking every channel
that walk for every page view costs one timeline walk per channel per request, each re-arranging
forecast windows, and a Library page would pay it for every title it shows.

## Decision

- **Reconcile stores each internal-playout channel's programmes for the next 48 hours** in
  `channel_upcoming_airings`, after it commits the channel and plans the next window's breaks. The
  rows come from the guide's own walk (`Resolver.UpcomingProgrammes`), so they cannot disagree
  with the guide.
- **One indexed read answers "when is this next on, and where?"** A movie matches on its library
  item id; a series matches through the title key its lineup entry names, the same join that lists
  a title's channels.
- **The write replaces a channel's rows in one transaction, guarded by the channel's revision.** A
  write computed from a stale revision changes nothing. It is best-effort: a failed write is logged
  and never fails a reconcile.
- **Tunarr channels are not stored**, the same as the scheduled ledger: Tunarr loops its list on its
  own clock.

## Consequences

- A title page reads its next airings in one query, whatever the number of channels.
- Forecast rows, those past the window on air, can lag the guide until the channel's next sweep
  (by default within ten minutes); the window on air never does. No extra reconcile runs at a
  window's turn.
- Pausing or detaching a channel saves it without a reconcile, so the read filters the channel's
  status rather than trusting the rows; purging a channel deletes its rows with it.
