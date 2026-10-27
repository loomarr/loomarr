# 0045. Loomarr recommends curated filler sources, YouTube channels included

- **Date:** 2026-10-27
- **Status:** accepted
- **Supersedes:** "Loomarr never recommends YouTube content itself"

## Context

Every commercial in the maintainer's household pool came from archive.org (#2039). Its three seeded
collections are mostly off-air VHS recordings, so the pool is soft, letterboxed and short of some
eras and countries, while cleaner copies of the same ads sit on YouTube channels that preserve
broadcast television. The rule that Loomarr never recommends YouTube content left that supply to
each operator to discover, and in practice nobody did.

## Decision

Loomarr ships a versioned list of vetted sources (`internal/filler/curated/sources.json`):
archive.org collections and YouTube channels, each with a label, era range, region and notes.

- Each upgrade offers an install the entries it has not been offered before. They arrive **switched
  off**: enabling a source is the operator's consent to fetch from it unattended
  ([0013](0013-filler-pulls-and-tombstones.md), [0015](0015-sources-fetch-on-their-own.md)). An
  entry the operator deleted is not offered again.
- A scheduled CI job checks every entry against the live archive.org metadata API and a yt-dlp flat
  listing, downloading no media. A removed or renamed entry fails the job and is fixed in the list.
- YouTube fetches the best format available and passes the same defect floor and creative identity
  as every other source (#2034, #2033).

## Consequences

- **Accepted trade-off:** YouTube's Terms of Service do not permit downloading outside its own
  clients. Loomarr already downloads what an operator points it at; now it also names channels. The
  operator still chooses: nothing is fetched until they switch a source on, and a disabled source
  stops at once.
- A curated channel is not a licence. Licence metadata stays passive provenance, and an empty licence
  is unknown, never public domain.
- A dead source on an install is reported by the existing source-health checks until the next
  release drops it.
