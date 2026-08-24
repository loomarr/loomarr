# Curated client diagnostics

Parent: [#509](https://github.com/loomarr/loomarr/issues/509)  
Delivery issue: [#513](https://github.com/loomarr/loomarr/issues/513)

## Outcome

When playback fails, buffers, drifts, or crosses a programme/commercial boundary, an administrator
or authenticated support agent can correlate what the web or Android TV player observed with the
server request, Channel, scheduled block, and retained Process-run evidence. Playback never waits
for diagnostics, and clients cannot turn the ingestion route into arbitrary remote logging.

## Interface

The deep `diagnostics` module exposes one client-ingestion operation. Its implementation owns the
closed event vocabulary, validation, severity/subsystem mapping, attribute projection, actor-scoped
rate limiting, and admission to the existing non-blocking Recorder. The HTTP adapter only derives
the authenticated actor and translates module errors to `400`, `429`, or `503`.

Web and Android TV implement the same small reporter interface: enqueue one typed observation and
flush a bounded batch through their native authenticated transport. Player code reports facts at
the lifecycle seam where it already knows them; it never assembles wire JSON or starts network work.

## Delivery sequence

1. Amend design §17 with the exact vocabulary, bounds, privacy rules, schedule-block identity, and
   failure behaviour.
2. Add module-level red tests for valid projection, adversarial fields, clock skew, whole-batch
   rejection, actor derivation, rate limiting, and non-blocking recorder admission.
3. Add the authenticated typed route and prove member/device acceptance, API-token identity,
   anonymous refusal, admin-only reads, and generated-contract coverage.
4. Give Guide and Playout the same opaque scheduled-block identity and prove programme, filler,
   restart/recompute, and Process-run joins.
5. Add the bounded web reporter, global caught/unhandled and typed-request failure capture, and
   player lifecycle/transition/drift observations.
6. Add the bounded Android TV reporter and the matching Media3 lifecycle/transition/drift
   observations.
7. Run focused race, web, and Android tests while editing; after stabilization run `make check`,
   Postgres conformance, complete frontend/visual/e2e/tuner gates, shared clients, and Android.
8. Publish one focused auto-merge PR closing #513, then continue #514.

## Verification matrix

| Concern | Evidence |
| --- | --- |
| Closed vocabulary | Every accepted event projects only its named fields; unknown names/fields reject the batch. |
| Privacy | Authorization, cookies, URLs, headers, stacks, DOM/form values, actor ids, and arbitrary maps have no input field and adversarial JSON fails validation. |
| Identity | Cookie and paired-device calls persist their resolved user id; API-token calls receive a server-owned machine identity. |
| Correlation | Web and TV schedule transitions use the same opaque block id stored on the corresponding Process run. |
| Backpressure | Client queues and server recorder admission are bounded; a stalled transport/store cannot stall playback. |
| Time | Client occurrence and server receipt remain distinct; outlandish skew is rejected and ordinary skew stays visible. |
| Authorization | Anonymous is `401`; members may ingest only; retained reads remain admin-only. |
| Rate | The actor-scoped burst succeeds, the next batch is `429`, and another actor remains independent. |
| Clients | Web and Android emit the same event names and correlation meanings under browser/Media3 lifecycle tests. |

## Non-goals

- Arbitrary frontend logging, console mirroring, remote Android logs, crash dumps, and stack upload.
- Client-side durable log storage or background telemetry when the user is signed out.
- Automatic outbound support submission.
- Rendering the retained events; #514 owns the operator UI and #515 owns downloads/bundles.
