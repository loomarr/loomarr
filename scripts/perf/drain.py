#!/usr/bin/env python3
"""Time how long encoders take to drain after the last viewer leaves (#1794).

Polls once a second: the server's loomarr_playout_sessions_active, the playout ffmpeg processes in
its process tree (every child counts, so warm neighbours (#1780), slates and grace sessions are
included), and the tree's own GPU encoder share. Prints a line whenever something changes, and the
time from --since (the last segment fetch: `viewers.py` prints it as last_fetch_epoch) to the
moment all three stay at zero for --settle seconds.

  scripts/perf/drain.py --base http://localhost:18043 --since 1790644608.7
"""
import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from sample import gpu_pmon, read_cmd, role_of, scrape, server_pid, tree  # noqa: E402


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default=os.environ.get("BASE", "http://localhost:8080"))
    ap.add_argument("--since", type=float, default=0, help="epoch of the last viewer fetch (default: now)")
    ap.add_argument("--settle", type=float, default=10)
    ap.add_argument("--timeout", type=float, default=600)
    ap.add_argument("--out")
    a = ap.parse_args()
    token = os.environ.get("LOOMARR_METRICS_TOKEN", "")
    if not token and os.environ.get("METRICS_TOKEN_FILE"):
        with open(os.environ["METRICS_TOKEN_FILE"]) as f:
            token = f.read().strip()
    since = a.since or time.time()
    root = server_pid(a.base)
    events, last_state, zero_at = [], None, None
    while True:
        now = time.time()
        pids = tree(root)
        ff = [p for p in pids if p != root and role_of(read_cmd(p)) == "playout-ffmpeg"]
        enc = sum(g["enc"] for g in gpu_pmon(set(pids)).values())
        flat, _, _ = scrape(a.base, token)
        state = (int(flat.get("playout_sessions", -1)), len(ff), round(enc))
        if state != last_state:
            ev = {"t_after_last_fetch_s": round(now - since, 1), "sessions": state[0], "playout_ffmpeg": state[1], "tree_gpu_enc_pct": state[2]}
            events.append(ev)
            print(json.dumps(ev), flush=True)
            last_state = state
        if state[0] <= 0 and state[1] == 0 and state[2] == 0:
            zero_at = zero_at or now
            if now - zero_at >= a.settle:
                break
        else:
            zero_at = None
        if now - since > a.timeout:
            print(json.dumps({"timeout": True}), flush=True)
            break
        time.sleep(1)
    result = {"since": since, "drained_after_s": round(zero_at - since, 1) if zero_at else None, "events": events}
    if a.out:
        with open(a.out, "w") as f:
            json.dump(result, f, indent=2)
    print(json.dumps({"drained_after_s": result["drained_after_s"]}))


if __name__ == "__main__":
    main()
