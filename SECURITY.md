# Security Policy

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately through GitHub's confidential channel:

> **Security → Advisories → Report a vulnerability**
> (<https://github.com/loomarr/loomarr/security/advisories/new>)

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
- The supported Compose path puts Loomarr behind Traefik, but its default listener is still plain
  HTTP for a trusted LAN. Do not treat a reverse proxy as internet hardening or expose this default
  stack publicly. Traefik's Docker API socket bind is marked read-only at the filesystem layer;
  that does not make the Docker API itself read-only, so a compromised edge remains a host-level
  concern and the stack belongs only on a trusted Docker host.
- The `/v1/healthz`, `/v1/readyz`, and `/v1/metrics` endpoints (and their bare
  `/healthz`, `/readyz`, `/metrics` aliases) are intentionally unauthenticated on
  the LAN — their consumers are container runtimes and scrape jobs, which hold no
  session. This is declared per route as `RolePublic`, so the unauthenticated
  surface is one greppable list rather than an absence you have to infer. Metrics
  expose aggregate operational state; keep the listener on a private scrape
  network or add TLS and authentication at an operator-managed edge.
- The published image bundles third-party binaries (`yt-dlp`, `ffmpeg`, `ffprobe`, `deno`, and
  `whisper-cli` with its shared libraries and model data) pinned by the `Dockerfile`;
  vulnerabilities in those upstreams are best
  reported to their projects, but do let us know if a pin needs bumping. These used
  to ship only in the now-retired opt-in `loomarr:filler` variant; since internal playout (design
  §9.1) they are in every image we publish, so their CVE surface is everyone's.

## Supported versions

Until the first stable release, only the latest `main` / most recent published
image is supported. Once releases are tagged, this section will list the
supported version line(s).
