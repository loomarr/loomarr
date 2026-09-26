# Playout bench baselines

One accepted report per hardware family, named for the family the bench reports: `vaapi.json`,
`nvenc.json`, `software.json`, `videotoolbox.json`. Hosted-runner families live under `ci/` because
their timing is a different, noisier population than the maintainer's hardware; a `ci/` baseline is
never compared with a maintainer baseline.

A baseline is committed only from a supervised run on the real hardware, with `make playout-bench`
and `PLAYOUT_BENCH_ACCEPT=1`. `make playout-bench` refuses to accept a report that fails a
threshold or regresses. See [the playout bench guide](../../dev/playout-bench.md).

No baseline exists yet for any family; until one is accepted the bench judges thresholds only.
