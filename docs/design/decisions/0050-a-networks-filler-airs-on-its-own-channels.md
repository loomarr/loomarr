# 0050. A network's branded filler airs only on channels of that network

- **Date:** 2026-11-21
- **Status:** accepted

## Context

A network's own station ID aired on a channel of a show from another network. On the
maintainer's install that day, one network's station IDs aired 19–27 times a day on at least
four channels, two of them carrying only another network's shows. Nothing linked a filler piece
to a network: no clip had one recorded, and break selection never read the field (#4432).

Most filler that names a network is not that network's: an ad taped off a network affiliate says
only where the tape was made. A station ID, a network bumper or a network's promo for its own show
carries the network's identity on screen, and on any other network's channel it reads as wrong.

## Decision

- **A piece that presents a network airs only on that network's channels** (maintainer,
  2026-11-21). A channel's network is the one most of its lineup's series premiered on: each
  series counts its first network in the Library once, a movie counts nothing, and a tie gives
  none. A channel with no network airs no network's pieces.
- **An ad only taped off a network is neutral** and airs anywhere. A clip records a network only
  when the piece itself presents it.
- **It is a boundary, like geography.** It is applied before pins and the ladder, so a pinned
  piece of another network does not air either, and a clip that fits no channel because of it is
  idle like any other. A per-clip fit says `network`.
- The channel's network has no setting: it is read from the lineup. Each series' networks are
  stamped onto its lineup entry once at reconcile, as box sets are, so choosing filler does no
  library I/O.

## Consequences

- Until clips record the network they present, the boundary only keeps pieces off channels; the
  clip side is #4432's next step.
- A channel mixing networks evenly carries none of their identities. A real station carries one
  network's, so that is the honest answer.
