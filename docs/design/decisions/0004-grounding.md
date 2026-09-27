# 0004. The LLM never supplies trusted identity

- **Date:** 2026-07-13
- **Status:** accepted

## Context

The Suggester's output can trigger real downloads and fill real channels. A language model will
confidently name titles that do not exist, attach the wrong external id to a real one, or claim a
title belongs to a set it does not.

## Decision

Identity comes only from the Catalog. The model chooses among candidates that a read-only Catalog
operation surfaced in the same run and copies their canonical key; code checks every pick against
that surfaced set. Model-authored names are search input at most, never evidence. Membership in a
named set needs user-supplied or resolved-reference evidence, and acquisitions are re-validated
against TMDB and the library before they are actionable.

## Consequences

- A weak or misbehaving model degrades to "no valid proposal", never to a hallucinated download.
- Every recovery path (retries, the exact-name fallback, source completion) must re-enter the same
  surfaced-key chokepoint; none may widen authority.
- Library, TMDB and reference-page text in prompts is untrusted data and cannot change tools,
  quotas or identity.
