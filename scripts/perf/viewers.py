#!/usr/bin/env python3
"""Simulated HLS viewers for the resource pass (#1794).

Each viewer does what the web player does on the wire: POST /v1/channels/{id}/play-url, fetch the
master, pick a variant, then reload the media playlist and download every new segment (and the
init segment) until it leaves. A viewer never announces leaving (neither does the player); it just
stops fetching, so the packager's grace timer is what ends the session. That is what drain timing
(scripts/perf/drain.py) measures from the "last fetch" this script prints.

  scripts/perf/viewers.py --viewers 4 --duration 600                 4 steady viewers, spread over channels
  scripts/perf/viewers.py --viewers 1 --surf 20 --duration 300       one viewer surfing every 20 s
  scripts/perf/viewers.py --viewers 1 --channels ch_x --premium      the 4K HEVC HDR variant

Auth: --dev-login (lane backends) or --cookie-jar (a curl -c jar). Writes --out JSON with every
tune (time to master, time to first segment) and per-viewer totals; stdlib only.
"""
import argparse
import http.cookiejar
import json
import os
import re
import sys
import threading
import time
import urllib.parse
import urllib.request

PREMIUM_PROFILE = {"video": ["hevc"], "audio": ["eac3", "ac3"], "video10bit": True, "hdr": True}


class Client:
    def __init__(self, base, jar):
        self.base = base.rstrip("/")
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def req(self, path, body=None, timeout=30):
        url = path if path.startswith("http") else self.base + path
        data = None if body is None else json.dumps(body).encode()
        r = urllib.request.Request(url, data=data, method="POST" if body is not None else "GET")
        r.add_header("X-Loomarr-Csrf", "1")
        if body is not None:
            r.add_header("Content-Type", "application/json")
        with self.op.open(r, timeout=timeout) as resp:
            return resp.read(), resp.geturl()


def parse_master(text, base_url, premium):
    variants = []
    lines = text.splitlines()
    for i, line in enumerate(lines):
        if line.startswith("#EXT-X-STREAM-INF"):
            h = re.search(r"RESOLUTION=\d+x(\d+)", line)
            variants.append((int(h.group(1)) if h else 0, urllib.parse.urljoin(base_url, lines[i + 1].strip())))
    if not variants:
        return base_url  # already a media playlist
    if premium:
        return max(variants)[1]
    return variants[0][1]  # the baseline is listed first; hls.js starts there too


def parse_media(text, base_url):
    target, init, segs, seq = 4.0, None, [], 0
    for line in text.splitlines():
        if line.startswith("#EXT-X-TARGETDURATION:"):
            target = float(line.split(":", 1)[1])
        elif line.startswith("#EXT-X-MEDIA-SEQUENCE:"):
            seq = int(line.split(":", 1)[1])
        elif line.startswith("#EXT-X-MAP:"):
            m = re.search(r'URI="([^"]+)"', line)
            init = urllib.parse.urljoin(base_url, m.group(1)) if m else None
        elif line and not line.startswith("#"):
            segs.append(urllib.parse.urljoin(base_url, line.strip()))
    return target, init, [(seq + i, u) for i, u in enumerate(segs)]


class Viewer(threading.Thread):
    def __init__(self, n, client, channels, a, stop, log):
        super().__init__(daemon=True)
        self.n, self.c, self.channels, self.a, self.stop, self.log = n, client, channels, a, stop, log
        self.tunes, self.segments, self.bytes, self.stalls, self.errors = [], 0, 0, 0, []
        # Media playlist reloads: how often, how big, how many signed entries (every entry is
        # re-signed per reload server-side, and the list grows with the DVR horizon).
        self.reloads, self.playlist_bytes, self.playlist_max_bytes, self.playlist_max_entries = 0, 0, 0, 0
        self.last_fetch = 0.0

    def tune(self, ch):
        t0 = time.monotonic()
        body = PREMIUM_PROFILE if self.a.premium else {}
        out, _ = self.c.req(f"/v1/channels/{ch}/play-url", body)
        url = json.loads(out)["url"]
        master, murl = self.c.req(url)
        t_master = time.monotonic() - t0
        return parse_master(master.decode(), murl, self.a.premium), t0, t_master

    def run(self):
        k = self.n  # stagger: viewer n starts on channel n
        while not self.stop.is_set():
            ch = self.channels[k % len(self.channels)]
            k += 1
            dwell_end = time.monotonic() + (self.a.surf if self.a.surf else 1e9)
            rec = {"viewer": self.n, "channel": ch, "at": time.time()}
            try:
                media, t0, rec["master_s"] = self.tune(ch)
                seen, init_done, first = set(), False, None
                idle_since = time.monotonic()
                while not self.stop.is_set() and time.monotonic() < dwell_end:
                    text, murl = self.c.req(media)
                    target, init, segs = parse_media(text.decode(), murl)
                    self.reloads += 1
                    self.playlist_bytes += len(text)
                    self.playlist_max_bytes = max(self.playlist_max_bytes, len(text))
                    self.playlist_max_entries = max(self.playlist_max_entries, len(segs))
                    if init and not init_done:
                        b, _ = self.c.req(init)
                        self.bytes += len(b)
                        init_done = True
                    new = [s for s in segs if s[0] not in seen]
                    if not seen and len(new) > 3:
                        for s in new[:-3]:  # join at the live edge, as hls.js does
                            seen.add(s[0])
                        new = new[-3:]
                    for sq, u in new:
                        b, _ = self.c.req(u)
                        seen.add(sq)
                        self.segments += 1
                        self.bytes += len(b)
                        self.last_fetch = time.time()
                        if first is None:
                            first = time.monotonic() - t0
                            rec["first_segment_s"] = round(first, 3)
                    if new:
                        idle_since = time.monotonic()
                    elif time.monotonic() - idle_since > 3 * target:
                        self.stalls += 1
                        idle_since = time.monotonic()
                    self.stop.wait(target / 2 if first is None else target)
            except Exception as e:  # noqa: BLE001 — a failed tune is a result, recorded and survived
                rec["error"] = f"{type(e).__name__}: {e}"[:200]
                self.errors.append(rec["error"])
                self.stop.wait(2)
            rec["master_s"] = round(rec.get("master_s", 0), 3)
            self.tunes.append(rec)
            self.log(rec)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default=os.environ.get("BASE", "http://localhost:8080"))
    ap.add_argument("--viewers", type=int, default=1)
    ap.add_argument("--duration", type=float, default=300)
    ap.add_argument("--surf", type=float, default=0, help="seconds per channel; 0 = stay on one channel")
    ap.add_argument("--channels", help="comma-separated channel ids (default: every channel, 4K ones last)")
    ap.add_argument("--premium", action="store_true", help="opt in to the premium (4K HEVC HDR) variant")
    ap.add_argument("--dev-login", action="store_true")
    ap.add_argument("--cookie-jar")
    ap.add_argument("--out")
    a = ap.parse_args()

    jar = http.cookiejar.MozillaCookieJar()
    if a.cookie_jar:
        jar.load(a.cookie_jar, ignore_discard=True, ignore_expires=True)
    c = Client(a.base, jar)
    if a.dev_login:
        c.req("/v1/auth/dev-login", {})
    if a.channels:
        channels = a.channels.split(",")
    else:
        raw, _ = c.req("/v1/channels")
        data = json.loads(raw)
        items = data.get("channels", data.get("items", data)) if isinstance(data, dict) else data
        channels = [ch["id"] for ch in sorted(items, key=lambda ch: ("4K" in ch.get("name", ""), ch.get("number", 0)))]
    lock = threading.Lock()

    def log(rec):
        with lock:
            print(json.dumps(rec), file=sys.stderr, flush=True)

    stop = threading.Event()
    vs = [Viewer(i, c, channels, a, stop, log) for i in range(a.viewers)]
    started = time.time()
    for v in vs:
        v.start()
        time.sleep(0.5)
    try:
        stop.wait(a.duration)
    except KeyboardInterrupt:
        pass
    stop.set()
    for v in vs:
        v.join(timeout=60)
    tunes = [t for v in vs for t in v.tunes]
    firsts = sorted(t["first_segment_s"] for t in tunes if "first_segment_s" in t)
    last = max((v.last_fetch for v in vs), default=0)
    summary = {
        "started": time.strftime("%Y-%m-%dT%H:%M:%S%z", time.localtime(started)),
        "viewers": a.viewers, "surf_s": a.surf, "premium": a.premium, "channels": channels,
        "tunes": len(tunes), "tune_errors": sum(1 for t in tunes if "error" in t),
        "first_segment_s": {"p50": firsts[len(firsts) // 2] if firsts else None, "max": firsts[-1] if firsts else None},
        "segments": sum(v.segments for v in vs), "mb": round(sum(v.bytes for v in vs) / 1e6, 1),
        "playlist_reloads": sum(v.reloads for v in vs), "playlist_mb": round(sum(v.playlist_bytes for v in vs) / 1e6, 1),
        "playlist_max_kb": round(max((v.playlist_max_bytes for v in vs), default=0) / 1e3, 1),
        "playlist_max_entries": max((v.playlist_max_entries for v in vs), default=0),
        "stalls": sum(v.stalls for v in vs), "last_fetch_epoch": last, "errors": sorted({e for v in vs for e in v.errors})[:10],
        "tune_log": tunes,
    }
    if a.out:
        with open(a.out, "w") as f:
            json.dump(summary, f, indent=2)
    print(json.dumps({k: v for k, v in summary.items() if k != "tune_log"}))


if __name__ == "__main__":
    main()
