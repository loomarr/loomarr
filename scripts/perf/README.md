# Resource pass tools (#1794)

Measure CPU, memory, disk and GPU across the whole Loomarr process tree and its clients. Every
number in the #1794 findings comes from one of these scripts, run on a lane backend with the demo
library. They are investigation tools, not gates: the numbers depend on the machine and dataset.

## Setup (a lane backend)

```sh
DATABASE_URL=sqlite://$PWD/.agent-data/loomarr.db make seed
LOOMARR_PPROF=1 LOOMARR_METRICS_TOKEN=<any string> make dev-be      # /debug/pprof and /metrics
# pause filler-pipeline, filler-split-sweep, filler-fetch, channel-maintenance (POST /v1/jobs/{name}/pause)
make demo-library & make demo-seed                                   # 6 channels, one 4K HDR10
pnpm --filter @loomarr/web build (in web/), then restart the backend  # the web tools need the embedded SPA
```

Export `METRICS_TOKEN_FILE` (or `LOOMARR_METRICS_TOKEN`) with the same token for every script.

## The tools

| script | measures |
|---|---|
| `server-pass.sh idle\|evening\|load\|premium` | one state end to end: the sampler, a CPU profile, a 15 s execution trace (wall-clock blocking), the allocations of the window, heap, goroutines, viewers, drain. Writes `.artifacts/perf/<date>/<state>/report.md` |
| `sample.py` | the process tree by role (server, playout ffmpeg, media ffmpeg, ffprobe, whisper, image worker, and children reaped between samples via the parent's `cutime`): cores, PSS, bytes written, GPU encoder share; Go heap, goroutines, allocation rate, GC cycles; scheduler job runs and wall time; DB, WAL and HLS scratch size |
| `attribute.py` | a pprof profile (CPU, heap, allocs, or a `go tool trace -pprof=` blocking profile) by subsystem (who did the work) and initiator (what started it) |
| `viewers.py` | HLS viewers as the web player does them on the wire: play-url, master, media playlist, segments; `--surf N` changes channel every N s; `--premium` takes the 4K HEVC HDR variant |
| `drain.py` | seconds from the last viewer fetch until sessions, playout ffmpeg and GPU encode are all gone |
| `routes.py` | latency (first, p50/p95/p99) and outbound calls per request for the routes Guide, Home and Watch load |
| `web-client.mjs routes\|soak` | Chromium: cold per-screen transfer by type, long tasks, TBT, LCP; and a watch-then-surf soak sampling heap, detached DOM nodes, listeners, renderer PSS and the hls.js buffer |

## States

- **idle**: nobody watching, one hour (warm-up vs. after 1 h), filler jobs paused as a lane keeps them.
- **evening**: one steady viewer and one surfing every 2 min, with the filler jobs UNPAUSED for this
  window only (capped at 20 min; the script re-pauses them on exit). Run it alone on the machine.
- **load**: `VIEWERS=4` and `VIEWERS=8` steady viewers plus one surfing every 15 s.
- **premium**: one viewer on the 4K channel's premium variant (`CHANNELS=<its id>`).

Viewer states hold `flock /tmp/loomarr-gpu.lock` for the viewers and the drain, and nothing else.

## Reading the numbers

- A CPU profile shows only running time. Waiting on SQLite I/O, a lock or the network is in the
  `wall-*.md` reports (from the execution trace), and outbound calls per request come from
  `routes.py`.
- PSS, not RSS: children share libraries, and summed RSS counts them once per process.
- The GPU is shared with other lanes. The card-wide columns are context; the per-role
  `GPU enc %` is this tree's own share (`nvidia-smi pmon`).
