# Security Policy

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately through GitHub's confidential channel:

> **Security → Advisories → Report a vulnerability**
> (<https://github.com/mantonx/loomarr/security/advisories/new>)

This opens a private advisory visible only to you and the maintainers. Please
include:

- affected version / image tag (or commit),
- a description and, if possible, steps to reproduce or a proof of concept,
- the impact as you see it.

We aim to acknowledge a report within a few days and to keep you updated as we
investigate and fix. Coordinated disclosure is appreciated: give us a reasonable
window to ship a fix before any public write-up.

## Scope notes

- Loomarr is designed to run on a **trusted LAN / internal Docker network and is
  not intended to be exposed directly to the public internet** (see the operator
  runbook in `docs/design.md`). Reports should assume that deployment model.
- The `/healthz`, `/readyz`, and `/metrics` endpoints are intentionally
  unauthenticated on the LAN.
- The `loomarr:filler` image bundles third-party binaries (`yt-dlp`, `ffmpeg`,
  `deno`) pinned by the `Dockerfile`; vulnerabilities in those upstreams are best
  reported to their projects, but do let us know if a pin needs bumping.

## Supported versions

Until the first stable release, only the latest `main` / most recent published
image is supported. Once releases are tagged, this section will list the
supported version line(s).
