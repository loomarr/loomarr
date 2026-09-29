#!/usr/bin/env python3
"""Latency and fan-out of the routes the Guide, Home and Watch screens load (#1794).

scripts/latency-sweep.sh covers a fixed route list with an API token; this sweeps the screens'
own routes with a session cookie (what the web client sends), and reads the per-request outbound
fan-out (loomarr_http_outbound_fanout, internal/metrics/fanout.go) as the delta of the histogram's
sum/count over the sweep. The first request per route is reported separately (cold caches), then
p50/p95/p99 over the rest.

The fan-out counts OUTBOUND HTTP calls (media server, Seerr, TMDB) per inbound request. Store
queries per request are not instrumented; a CPU profile cannot count them either.

  scripts/perf/routes.py --base http://localhost:18043 --dev-login -n 30
"""
import argparse
import http.cookiejar
import json
import os
import re
import sys
import time
import urllib.request

SCREENS = {
    "guide": ["/v1/guide?from={now-30m}&to={now+4h}", "/v1/channels/now-next", "/v1/channels"],
    "home": ["/v1/guide/highlights", "/v1/discovery/ideas", "/v1/household/viewing", "/v1/titles", "/v1/channels/now-next"],
    "watch": ["/v1/channels/{channel}", "/v1/channels/now-next"],
}


def expand(path, channel):
    now = int(time.time() * 1000)
    return path.replace("{now-30m}", str(now - 1800000)).replace("{now+4h}", str(now + 14400000)).replace("{channel}", channel)


def fanout(base, token):
    req = urllib.request.Request(base + "/metrics", headers={"Authorization": "Bearer " + token})
    out = {}
    for line in urllib.request.urlopen(req, timeout=10).read().decode().splitlines():
        m = re.match(r'loomarr_http_outbound_fanout_(sum|count)\{method="GET",route="([^"]+)"\}\s+(\S+)', line)
        if m:
            out.setdefault(m.group(2), [0.0, 0.0])[0 if m.group(1) == "sum" else 1] = float(m.group(3))
    return out


def pct(xs, p):
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(p * len(xs)))] if xs else 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default=os.environ.get("BASE", "http://localhost:8080"))
    ap.add_argument("-n", type=int, default=20)
    ap.add_argument("--dev-login", action="store_true")
    ap.add_argument("--cookie-jar")
    ap.add_argument("--out")
    a = ap.parse_args()
    base = a.base.rstrip("/")
    token = os.environ.get("LOOMARR_METRICS_TOKEN", "")
    if not token and os.environ.get("METRICS_TOKEN_FILE"):
        with open(os.environ["METRICS_TOKEN_FILE"]) as f:
            token = f.read().strip()
    jar = http.cookiejar.MozillaCookieJar()
    if a.cookie_jar:
        jar.load(a.cookie_jar, ignore_discard=True, ignore_expires=True)
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def get(path, body=None):
        r = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(),
                                   method="POST" if body is not None else "GET")
        r.add_header("X-Loomarr-Csrf", "1")
        r.add_header("Content-Type", "application/json")
        t0 = time.monotonic()
        with op.open(r, timeout=60) as resp:
            n = len(resp.read())
        return (time.monotonic() - t0) * 1000, n

    if a.dev_login:
        get("/v1/auth/dev-login", {})
    chans = json.loads(op.open(urllib.request.Request(base + "/v1/channels", headers={"X-Loomarr-Csrf": "1"})).read())
    chans = chans.get("channels", chans.get("items", [])) if isinstance(chans, dict) else chans
    channel = chans[0]["id"] if chans else "none"

    rows, seen = [], set()
    for screen, paths in SCREENS.items():
        for p in paths:
            if p in seen:
                continue
            seen.add(p)
            before = fanout(base, token) if token else {}
            ms, sizes = [], []
            for _ in range(a.n):
                try:
                    t, n = get(expand(p, channel))
                except Exception as e:  # noqa: BLE001 — a failing route is a finding, not a crash
                    print(f"routes: {p}: {e}", file=sys.stderr)
                    break
                ms.append(t)
                sizes.append(n)
            after = fanout(base, token) if token else {}
            route = p.split("?")[0].replace(channel, "{id}")
            b, f = before.get(route, [0, 0]), after.get(route, [0, 0])
            calls = (f[0] - b[0]) / (f[1] - b[1]) if f[1] > b[1] else None
            if not ms:
                continue
            rows.append({"screen": screen, "route": route, "first_ms": round(ms[0], 1), "p50_ms": round(pct(ms[1:], .5), 1),
                         "p95_ms": round(pct(ms[1:], .95), 1), "p99_ms": round(pct(ms[1:], .99), 1),
                         "kb": round(sum(sizes) / len(sizes) / 1000, 1), "outbound_per_req": None if calls is None else round(calls, 2)})
    stamp = time.strftime("%Y-%m-%d %H:%M %Z")
    print(f"### routes — {stamp}, n={a.n}\n")
    print("| screen | route | first ms | p50 | p95 | p99 | KB | outbound calls/req |\n|---|---|---|---|---|---|---|---|")
    for r in rows:
        print(f"| {r['screen']} | {r['route']} | {r['first_ms']} | {r['p50_ms']} | {r['p95_ms']} | {r['p99_ms']} | {r['kb']} | {r['outbound_per_req']} |")
    if a.out:
        with open(a.out, "w") as f:
            json.dump({"at": stamp, "n": a.n, "rows": rows}, f, indent=2)


if __name__ == "__main__":
    main()
