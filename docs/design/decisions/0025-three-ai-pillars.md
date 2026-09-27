# 0025. Three independently certified AI pillars

- **Date:** 2026-09-02
- **Status:** accepted

## Context

Loomarr uses models for three different jobs: recommending channel concepts, curating a chosen
Intent into a Proposal, and classifying filler clips. They share infrastructure and may share a
model family, which invited treating one passing scorecard as proof for all three.

## Decision

Model work is three pillars with separate authority and release contracts:

1. **Channel recommendation** proposes Channel Concepts (a name, a draft Intent, cited context
   signal ids). A Concept is not a Proposal and carries no effectful field.
2. **Channel curation** turns an Intent into a grounded Proposal and ChannelPolicy.
3. **Filler curation** classifies, tags and ranks clips from bounded evidence; admission and
   scheduling authority stay with deterministic filler policy.

Each pillar owns its own corpus, thresholds, scorecard and ship decision. Certification never
transfers between pillars, even for the same weights.

## Consequences

- Corpora are versioned and frozen before live inference; development corpora must be disjoint
  from certification holdouts, and a prompt or schema change needs a new untouched holdout.
- Recommendation certification uses synthetic, digest-pinned snapshots, never household identity
  or viewing history.
- Frozen holdouts and their no-ship scorecards stay immutable; `channel-recommendation-v2`
  replaced v1 with new, mechanically disjoint cases rather than editing it.
