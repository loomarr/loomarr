# 0002. Code-first OpenAPI with Huma v2

- **Date:** 2026-07-13
- **Status:** accepted

## Context

Loomarr's API is consumed by its own web and native clients, by generated TypeScript clients, and by
household scripts. Hand-maintained API documentation drifts from the code, and a spec-first workflow
adds a contract-review step to every change for a project with one team.

## Decision

Define each operation once in Go (input and output types, tags, required role) with Huma v2 on the
stdlib `ServeMux`. Huma emits the OpenAPI 3.1 spec, validates requests from the same schema and
serves the docs. The committed `api/openapi.yaml` is exported from the same constructor as the
running server, and CI fails on drift. Hand-written API docs are not allowed.

Rejected: spec-first `oapi-codegen` (review ceremony for no gain here) and annotation-based
`swaggo` (the weakest drift guarantee).

## Consequences

- The spec is the API reference; prose describes behaviour, not routes.
- Every route, including binary, SSE, multipart and redirect routes, is a Huma operation, so one
  fail-closed authorization middleware covers all of them.
- Client hooks, zod schemas and MSW wiring are generated from the spec by `orval`.
