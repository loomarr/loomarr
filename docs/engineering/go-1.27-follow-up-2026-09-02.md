# Go 1.27 follow-up evidence

**Measured 2026-09-02 on Darwin/arm64 (Apple M5 Pro).** This records the bounded pilots from
[issue #679](https://github.com/loomarr/loomarr/issues/679). The measurements characterize these
specific test and command boundaries; they do not claim an application-wide Go 1.27 performance
gain.

## In-memory HTTP test server: adopted

`internal/api/playout_test.go` is a suitable pilot because every request already uses the server's
injected client. Ten repetitions of the complete `TestPlayout_*` set were measured three times
before the change and five times after it:

| server | real-time observations | median |
|---|---|---:|
| loopback `httptest.NewServer` | 1.43 s, 1.39 s, 1.40 s | 1.40 s |
| in-memory `httptest.NewTestServer` | 1.38 s, 1.50 s, 1.50 s, 1.36 s, 1.36 s | 1.38 s |

The difference is within run-to-run noise, so speed is not the reason to adopt it. The value is
removing loopback-port allocation and receiving automatic test cleanup and handler-panic reporting.

One compatibility difference was useful to expose: the in-memory server has no loopback URL before
client initialization. Tests now construct requests with the stable absolute origin
`http://example.com`; the injected client deliberately routes any absolute HTTP or HTTPS origin to
the in-memory handler. The live-stream and client-cancellation tests pass through that transport.
This is a pilot, not authorization for a repository-wide mechanical replacement.

## Strict JSON v2 boundary: adopted narrowly

`cmd/internal/fillerbakeoffio.ReadStrictJSON` reads complete immutable evidence and attestation
artifacts. The prior v1 decoder rejected unknown fields and trailing values but accepted three
ambiguous inputs in a red test: duplicate member names, case variants, and invalid UTF-8. Direct
`encoding/json/v2` with `RejectUnknownMembers(true)` rejects all five adversarial classes.

| input property | decision |
|---|---|
| exact known member names | accept |
| unknown member | reject |
| case-variant member | reject |
| duplicate member | reject |
| invalid UTF-8 | reject |
| trailing JSON value | reject |
| `null` collection | accept and preserve `nil` |
| empty collection | accept and preserve non-nil empty value |

Writers remain on `encoding/json` v1. The canonical fixture bytes and SHA-256 remain exactly
`5609d16ddd08b226203f229fdb378f558dc3a04bc4f8db4b7f5de5387d1afd07`. One consumer test had pinned
the old decoder's error wording; it now asserts rejection rather than treating implementation prose
as a contract.

`BenchmarkReadStrictJSON`, which includes the boundary's file read, reported 9,847–10,696 ns/op,
1,098–1,099 B/op, and 9 allocs/op across five runs. It is a regression baseline only: no before/after
speed claim is made because stricter semantics, not throughput, justify this migration. External
clients and public DTOs remain on v1.

## Go fix modernizers: accepted selectively

The issue-named suggestions were reviewed individually and retained because they make intent safer
or more direct without changing ordering:

- `internal/app/application.go`: two reverse lifecycle traversals use `slices.Backward`.
- `internal/filler/splitrescue.go`: reverse timestamp-component parsing uses `slices.Backward`.
- `internal/schedule/separation.go`: two reverse separation scans use `slices.Backward`.
- `internal/suggest/worker_test.go`: the concurrent ID generator uses `atomic.Int64`, preventing
  accidental non-atomic access.

A broad `go fix -diff` also proposed extensive unrelated literal, range-loop, tag, and test-helper
churn. None of that was accepted. The three production packages and the suggestion package pass
their focused race-enabled tests after the selected changes.
