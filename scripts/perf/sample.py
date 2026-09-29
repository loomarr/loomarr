#!/usr/bin/env python3
"""Sample the whole Loomarr process tree over a window (#1794).

One sample every --interval seconds, for --duration seconds, of:

  * every process in the tree (the Go server plus every descendant: ffmpeg, ffprobe, whisper, the
    image worker), grouped by role: CPU cores, PSS (not RSS: children share libraries, and summed
    RSS double-counts them), bytes written;
  * CPU of children that were born AND reaped between two samples. Polling /proc cannot see a
    200 ms ffprobe, but the kernel adds every reaped child's CPU to its parent's cutime/cstime, so
    the server's cutime delta, minus the children we did see exit, is exactly that invisible work;
  * the Go runtime and Loomarr counters from /metrics (heap, goroutines, allocation rate, GC
    cycles, scheduler job executions and their wall time, HTTP requests);
  * the database file and WAL, and the HLS scratch directory (size, file count);
  * the GPU: whole-card encoder sessions and utilisation (the card is shared with other lanes, so
    these are context), and per-process encoder/SM share for the tree's own PIDs (nvidia-smi pmon).

Writes <out>/timeline.csv (one row per sample per role), <out>/metrics.csv (one row per sample),
<out>/summary.json, and prints a Markdown summary. Stdlib only.

  scripts/perf/sample.py --base http://localhost:18043 --duration 600 --out .artifacts/perf/idle
  METRICS_TOKEN_FILE=… (or LOOMARR_METRICS_TOKEN) is the /metrics bearer; without it the Go
  columns are blank and the process-tree columns still work.
"""
import argparse
import csv
import datetime as dt
import json
import os
import re
import shutil
import statistics
import subprocess
import sys
import time
import urllib.parse
import urllib.request

HZ = os.sysconf("SC_CLK_TCK")
PAGE = os.sysconf("SC_PAGE_SIZE")

# Role of a process, from its command line. First match wins; the server itself is "server".
# Playout encoders pipe fragmented MP4 (`empty_moov`, `-output_ts_offset`) to the in-process
# packager, or write under the HLS scratch root; anything else ffmpeg does is media work (filler
# measurement, splitting, clip fetch). Unmatched commands are reported by name so the table can be
# extended rather than silently lumped together.
ROLES = [
    ("playout-ffmpeg", lambda c: c[0].endswith("ffmpeg") and ("-output_ts_offset" in c or "empty_moov" in " ".join(c) or "/hls/" in " ".join(c))),
    ("media-ffmpeg", lambda c: c[0].endswith("ffmpeg")),
    ("ffprobe", lambda c: c[0].endswith("ffprobe")),
    ("whisper", lambda c: "whisper" in c[0]),
    ("image-worker", lambda c: "loomarr-image" in c[0]),
]


def role_of(cmd):
    for name, match in ROLES:
        if cmd and match(cmd):
            return name
    return "other:" + os.path.basename(cmd[0]) if cmd else "other:?"


def read_stat(pid):
    """(ppid, utime+stime ticks, cutime+cstime ticks) or None if the process is gone."""
    try:
        with open(f"/proc/{pid}/stat") as f:
            raw = f.read()
    except OSError:
        return None
    rest = raw[raw.rindex(")") + 2 :].split()
    # rest[0] is field 3 (state); fields 14..17 are utime, stime, cutime, cstime.
    return int(rest[1]), int(rest[11]) + int(rest[12]), int(rest[13]) + int(rest[14])


def read_cmd(pid):
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as f:
            return [a.decode(errors="replace") for a in f.read().split(b"\0") if a]
    except OSError:
        return []


def read_pss(pid):
    try:
        with open(f"/proc/{pid}/smaps_rollup") as f:
            for line in f:
                if line.startswith("Pss:"):
                    return int(line.split()[1]) * 1024
    except OSError:
        pass
    try:  # smaps_rollup missing: fall back to RSS
        with open(f"/proc/{pid}/statm") as f:
            return int(f.read().split()[1]) * PAGE
    except OSError:
        return 0


def read_write_bytes(pid):
    try:
        with open(f"/proc/{pid}/io") as f:
            for line in f:
                if line.startswith("write_bytes:"):
                    return int(line.split()[1])
    except OSError:
        pass
    return 0


def tree(root):
    """PIDs of root and every descendant."""
    children = {}
    for d in os.listdir("/proc"):
        if not d.isdigit():
            continue
        st = read_stat(int(d))
        if st:
            children.setdefault(st[0], []).append(int(d))
    out, todo = [], [root]
    while todo:
        p = todo.pop()
        out.append(p)
        todo.extend(children.get(p, []))
    return out


def server_pid(base):
    port = urllib.parse.urlparse(base).port
    res = subprocess.run(["ss", "-ltnpH", f"sport = :{port}"], capture_output=True, text=True)
    m = re.search(r"pid=(\d+)", res.stdout)
    if not m:
        sys.exit(f"sample: nothing listening on :{port}")
    return int(m.group(1))


METRIC_KEYS = {
    "go_memstats_heap_inuse_bytes": "heap_inuse",
    "go_memstats_heap_alloc_bytes": "heap_alloc",
    "go_memstats_sys_bytes": "go_sys",
    "go_memstats_alloc_bytes_total": "alloc_total",
    "go_memstats_mallocs_total": "mallocs_total",
    "go_goroutines": "goroutines",
    "go_threads": "threads",
    "go_gc_duration_seconds_count": "gc_count",
    "go_gc_duration_seconds_sum": "gc_pause_sum",
    "process_cpu_seconds_total": "proc_cpu",
    "process_open_fds": "open_fds",
    "loomarr_playout_sessions_active": "playout_sessions",
    "loomarr_http_requests_in_flight": "http_in_flight",
    "loomarr_database_connection_waits_total": "db_waits",
    "loomarr_database_connection_wait_duration_seconds_total": "db_wait_seconds",
}
LINE = re.compile(r"^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+(\S+)")


def scrape(base, token):
    if not token:
        return {}, {}, 0
    req = urllib.request.Request(base.rstrip("/") + "/metrics", headers={"Authorization": "Bearer " + token})
    try:
        body = urllib.request.urlopen(req, timeout=10).read().decode()
    except Exception as e:  # noqa: BLE001 — a failed scrape is a blank row, not a dead run
        print(f"sample: scrape failed: {e}", file=sys.stderr)
        return {}, {}, 0
    flat, jobs, http_total = {}, {}, 0
    for line in body.splitlines():
        m = LINE.match(line)
        if not m:
            continue
        name, labels, val = m.group(1), m.group(2) or "", float(m.group(3))
        if name in METRIC_KEYS and not labels:
            flat[METRIC_KEYS[name]] = val
        elif name == "loomarr_scheduler_job_duration_seconds_count":
            j = re.search(r'job="([^"]+)"', labels)
            if j:
                jobs.setdefault(j.group(1), [0, 0])[0] += val
        elif name == "loomarr_scheduler_job_duration_seconds_sum":
            j = re.search(r'job="([^"]+)"', labels)
            if j:
                jobs.setdefault(j.group(1), [0, 0])[1] += val
        elif name == "loomarr_http_requests_total":
            http_total += val
        elif name == "loomarr_playout_sessions_active":
            flat["playout_sessions"] = flat.get("playout_sessions", 0) + val
    return flat, jobs, http_total


def gpu_card():
    if not shutil.which("nvidia-smi"):
        return {}
    q = "encoder.stats.sessionCount,utilization.gpu,utilization.encoder,utilization.decoder,memory.used"
    res = subprocess.run(["nvidia-smi", f"--query-gpu={q}", "--format=csv,noheader,nounits"], capture_output=True, text=True)
    try:
        v = [float(x) for x in res.stdout.strip().splitlines()[0].split(",")]
    except (IndexError, ValueError):
        return {}
    return dict(zip(["gpu_enc_sessions", "gpu_util", "gpu_enc_util", "gpu_dec_util", "gpu_mem_mb"], v))


def gpu_pmon(pids):
    """Per-PID SM/encoder/decoder share for the tree's own processes (percent of the card)."""
    if not shutil.which("nvidia-smi"):
        return {}
    res = subprocess.run(["nvidia-smi", "pmon", "-c", "1", "-s", "u"], capture_output=True, text=True)
    cols, out = None, {}
    for line in res.stdout.splitlines():
        if line.startswith("#"):
            if cols is None:
                cols = line.lstrip("# ").split()
            continue
        f = line.split()
        if not cols or len(f) < len(cols) or not f[1].isdigit() or int(f[1]) not in pids:
            continue
        row = dict(zip(cols, f))
        out[int(f[1])] = {k: (float(row[k]) if row.get(k, "-") not in ("-", "") else 0.0) for k in ("sm", "enc", "dec")}
    return out


def dir_usage(path):
    total = files = 0
    for dirpath, _, names in os.walk(path):
        for n in names:
            try:
                total += os.lstat(os.path.join(dirpath, n)).st_size
                files += 1
            except OSError:
                pass
    return total, files


def size(path):
    try:
        return os.path.getsize(path)
    except OSError:
        return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default=os.environ.get("BASE", "http://localhost:8080"))
    ap.add_argument("--interval", type=float, default=5)
    ap.add_argument("--duration", type=float, default=300)
    ap.add_argument("--out", required=True)
    ap.add_argument("--label", default="")
    ap.add_argument("--db", default=".agent-data/loomarr.db", help="SQLite file (size + WAL)")
    ap.add_argument("--hls", default=".agent-data/hls", help="HLS scratch root")
    ap.add_argument("--pid", type=int, help="server PID (default: whatever listens on --base's port)")
    ap.add_argument("--no-gpu", action="store_true")
    a = ap.parse_args()

    token = os.environ.get("LOOMARR_METRICS_TOKEN", "")
    if not token and os.environ.get("METRICS_TOKEN_FILE"):
        with open(os.environ["METRICS_TOKEN_FILE"]) as f:
            token = f.read().strip()
    os.makedirs(a.out, exist_ok=True)
    root = a.pid or server_pid(a.base)
    started = dt.datetime.now().astimezone()
    print(f"sample: {a.label or a.out} — server pid {root}, {a.duration:.0f}s every {a.interval:.0f}s, from {started:%Y-%m-%d %H:%M:%S %Z}", file=sys.stderr)

    tl = open(os.path.join(a.out, "timeline.csv"), "w", newline="")
    tlw = csv.writer(tl)
    tlw.writerow(["t", "role", "procs", "cores", "pss_mb", "write_mb_s", "gpu_sm", "gpu_enc", "gpu_dec"])
    mt = open(os.path.join(a.out, "metrics.csv"), "w", newline="")
    mtw = None

    prev = {}  # pid -> (ticks, cutime, write_bytes, role, ppid)
    prev_root_c = None
    prev_flat, prev_jobs, prev_http = None, {}, None
    job_runs = {}  # job -> [executions, wall seconds] over the window
    role_rows = {}  # role -> list of (cores, pss, write rate)
    metric_rows = []
    cmd_cache = {}
    t0 = time.monotonic()
    last = t0
    while True:
        now = time.monotonic()
        dtw = now - last
        pids = tree(root)
        pidset = set(pids)
        cur, per_role = {}, {}
        for p in pids:
            st = read_stat(p)
            if not st:
                continue
            if p not in cmd_cache:
                cmd_cache[p] = read_cmd(p)
            role = "server" if p == root else role_of(cmd_cache[p])
            wb = read_write_bytes(p)
            cur[p] = (st[1], st[2], wb, role, st[0])
            r = per_role.setdefault(role, {"procs": 0, "ticks": 0, "pss": 0, "wb": 0, "sm": 0, "enc": 0, "dec": 0})
            r["procs"] += 1
            r["pss"] += read_pss(p)
            if p in prev:
                r["ticks"] += st[1] - prev[p][0]
                r["wb"] += max(0, wb - prev[p][2])
            elif prev_root_c is not None:
                r["ticks"] += st[1]  # born since the last sample: all its CPU is in this window
                r["wb"] += wb
        # Children reaped between samples: the root's cutime grew by everything its reaped children
        # used; take away what we already counted for the ones we saw alive.
        root_c = cur.get(root, (0, 0))[1]
        if prev_root_c is not None:
            seen = sum(v[0] + v[1] for p, v in prev.items() if p not in cur and v[4] == root)
            unseen = max(0, (root_c - prev_root_c) - seen)
            if unseen:
                r = per_role.setdefault("reaped-unseen", {"procs": 0, "ticks": 0, "pss": 0, "wb": 0, "sm": 0, "enc": 0, "dec": 0})
                r["ticks"] += unseen
        prev_root_c = root_c
        if not a.no_gpu:
            for p, g in gpu_pmon(pidset).items():
                if p in cur:
                    r = per_role[cur[p][3]]
                    for k in ("sm", "enc", "dec"):
                        r[k] += g[k]
        prev = cur

        flat, jobs, http_total = scrape(a.base, token)
        card = {} if a.no_gpu else gpu_card()
        hls_bytes, hls_files = dir_usage(a.hls)
        row = {"t": round(now - t0, 1), **flat, **card, "hls_mb": hls_bytes / 1e6, "hls_files": hls_files,
               "db_mb": size(a.db) / 1e6, "wal_mb": size(a.db + "-wal") / 1e6}
        if prev_flat is not None and dtw > 0:
            row["alloc_mb_s"] = (flat.get("alloc_total", 0) - prev_flat.get("alloc_total", 0)) / 1e6 / dtw
            row["gc_per_min"] = (flat.get("gc_count", 0) - prev_flat.get("gc_count", 0)) * 60 / dtw
            row["go_cores"] = (flat.get("proc_cpu", 0) - prev_flat.get("proc_cpu", 0)) / dtw
            row["http_rps"] = (http_total - prev_http) / dtw
            for j, (n, s) in jobs.items():
                pn, ps = prev_jobs.get(j, (0, 0))
                if n > pn:
                    jr = job_runs.setdefault(j, [0, 0.0])
                    jr[0] += int(n - pn)
                    jr[1] += s - ps
        prev_flat, prev_jobs, prev_http = flat, jobs, http_total
        if mtw is None:
            cols = ["t", "heap_inuse", "heap_alloc", "go_sys", "goroutines", "threads", "open_fds", "alloc_mb_s", "gc_per_min",
                    "go_cores", "http_rps", "playout_sessions", "db_mb", "wal_mb", "hls_mb", "hls_files",
                    "gpu_enc_sessions", "gpu_util", "gpu_enc_util", "gpu_dec_util", "gpu_mem_mb"]
            mtw = csv.DictWriter(mt, fieldnames=cols, extrasaction="ignore")
            mtw.writeheader()
        mtw.writerow(row)
        mt.flush()
        if "alloc_mb_s" in row:
            metric_rows.append(row)

        if now > t0:  # the first pass only primes the counters
            for role, r in per_role.items():
                cores = r["ticks"] / HZ / dtw if dtw > 0 else 0
                wr = r["wb"] / 1e6 / dtw if dtw > 0 else 0
                tlw.writerow([round(now - t0, 1), role, r["procs"], round(cores, 4), round(r["pss"] / 1e6, 1), round(wr, 3), r["sm"], r["enc"], r["dec"]])
                if prev_flat is not None and len(metric_rows):
                    role_rows.setdefault(role, []).append((cores, r["pss"] / 1e6, wr, r["enc"], r["procs"]))
            tl.flush()
        last = now
        if now - t0 >= a.duration:
            break
        time.sleep(max(0, a.interval - (time.monotonic() - now)))

    n = max(1, len(metric_rows))

    def stat(xs):
        xs = sorted(xs)
        if not xs:
            return {"mean": 0, "p95": 0, "max": 0}
        return {"mean": round(statistics.fmean(xs), 4), "p95": round(xs[min(len(xs) - 1, int(0.95 * len(xs)))], 4), "max": round(xs[-1], 4)}

    roles = {}
    for role, rows in role_rows.items():
        # A role absent from a sample used nothing then: average over every sample, not its own.
        pad = [0.0] * (n - len(rows))
        roles[role] = {"cores": stat([r[0] for r in rows] + pad), "pss_mb": stat([r[1] for r in rows]),
                       "write_mb_s": stat([r[2] for r in rows] + pad), "gpu_enc_pct": stat([r[3] for r in rows] + pad),
                       "procs_max": max(r[4] for r in rows)}
    mkeys = ["heap_inuse", "go_sys", "goroutines", "alloc_mb_s", "gc_per_min", "go_cores", "http_rps", "playout_sessions",
             "db_mb", "wal_mb", "hls_mb", "hls_files", "gpu_enc_sessions", "gpu_util", "gpu_enc_util"]
    metrics = {k: stat([float(r[k]) for r in metric_rows if k in r]) for k in mkeys}
    for k in ("heap_inuse",):
        metrics[k] = {s: round(v / 1e6, 1) for s, v in metrics[k].items()}
    for k in ("go_sys",):
        metrics[k] = {s: round(v / 1e6, 1) for s, v in metrics[k].items()}
    span = time.monotonic() - t0
    summary = {"label": a.label, "started": started.isoformat(timespec="seconds"), "seconds": round(span), "samples": n,
               "server_pid": root, "roles": roles, "metrics": metrics,
               "jobs": {j: {"runs": r[0], "wall_s": round(r[1], 3), "runs_per_hour": round(r[0] * 3600 / span, 1)}
                        for j, r in sorted(job_runs.items(), key=lambda kv: -kv[1][1])}}
    with open(os.path.join(a.out, "summary.json"), "w") as f:
        json.dump(summary, f, indent=2)

    tot = sum(r["cores"]["mean"] for r in roles.values())
    pss = sum(r["pss_mb"]["mean"] for r in roles.values())
    print(f"\n### {a.label or a.out} — {started:%Y-%m-%d %H:%M %Z}, {span:.0f}s, {n} samples\n")
    print(f"Tree total: {tot:.3f} cores mean, {pss:.0f} MB PSS mean\n")
    print("| role | procs (max) | cores mean | cores p95 | PSS MB mean | PSS MB max | write MB/s | GPU enc % |")
    print("|---|---|---|---|---|---|---|---|")
    for role, r in sorted(roles.items(), key=lambda kv: -kv[1]["cores"]["mean"]):
        print(f"| {role} | {r['procs_max']} | {r['cores']['mean']:.3f} | {r['cores']['p95']:.3f} | {r['pss_mb']['mean']:.0f} | "
              f"{r['pss_mb']['max']:.0f} | {r['write_mb_s']['mean']:.3f} | {r['gpu_enc_pct']['mean']:.1f} |")
    print("\n| metric | mean | p95 | max |\n|---|---|---|---|")
    for k, v in metrics.items():
        print(f"| {k} | {v['mean']} | {v['p95']} | {v['max']} |")
    if summary["jobs"]:
        print("\n| scheduler job | runs | runs/h | wall s |\n|---|---|---|---|")
        for j, v in summary["jobs"].items():
            print(f"| {j} | {v['runs']} | {v['runs_per_hour']} | {v['wall_s']} |")


if __name__ == "__main__":
    main()
