# Product

<!-- impeccable:product-schema 1 -->

This is the product record that design work reads first. It holds confirmed product facts only.
Behaviour lives in `docs/design/`; the look and its rules live in `docs/frontend-design.md` and
`design/`.

## Platform

adaptive

## Users

- **The admin.** The one person in a household who runs its media server and sets Loomarr up.
  Technical enough to run a home server; wants channels that keep themselves running, not a
  console to operate.
- **Household members.** Everyone else. They watch the channels, ask for titles, and browse what
  the household has. They are not technical and never administer anything.

## Product Purpose

Describe a TV channel in a sentence; Loomarr builds it, plays it, and keeps it running. It picks a
lineup from the household's own library, requests what is missing, schedules it with breaks, and
streams it as live TV. Success is a household that turns on a channel and watches, with nobody
tending it.

## Positioning

The channels are made from the household's own files and run themselves. Every pick is a real
title from the library or the catalogue, never an invented one, and nothing downloads until an
admin approves. Loomarr is an appliance for a household, not an operator console.

## Operating Context

- Runs on a home server beside Emby or Jellyfin, with optional Sonarr, Radarr and a requester.
- The admin works mostly in the web app on a desktop. Members use the web app, the phone app and
  the TV apps (Apple TV, Android TV).
- Libraries are messy: years of files named by different tools and by hand. That is the normal
  case, not an edge case.

## Capabilities and Constraints

- Two roles: admin and member. Identifying files, fixing titles, collections and settings are the
  admin's.
- Decided, not yet built (#4476): a member browses the Library read-only. Today the Library is
  admin-only.
- The Library is Loomarr's own scan of the media folders. A title it cannot identify waits in
  "Needs you" for the admin.
- Words used in the product: "Needs you", "Getting ready", "Technical details", "Update details",
  "Check files again", "Wrong title?".
- The repository is public. Fixtures, stories and screenshots use an invented catalogue, never a
  household's real titles or paths.

## Brand Commitments

- The name is Loomarr.
- Plain words everywhere: a non-technical household member understands every screen. Technical
  facts sit behind one "Technical details" disclosure.
- The voice is warm broadcast, used sparingly: "Dead air", "All caught up".
- It must not read as childish or as an admin tool written for engineers.

## Evidence on Hand

- Mocks of record: `design/library/`, `design/watch-player/`, and the prototypes in `design/`.
- A demo library with an invented catalogue and generated artwork: `make demo-library`.
- There are no customer quotes, usage numbers or press. Do not invent any.

## Product Principles

1. It maintains itself. A person is asked only when Loomarr cannot decide, and then in one plain
   question.
2. The household's titles come first. Chores, status and machinery never stand between a person
   and what they came to see.
3. Messy libraries are normal. Every screen is designed for the library that was never tidied.
4. Honest about what it knows. It says when it is not sure, and it never invents a title.

## Accessibility & Inclusion

Text and controls meet WCAG AA contrast, checked by the token generator and by axe on every story.
Required of every surface: it works by keyboard, by touch and, on TV, by remote, and its motion
respects reduced motion.
