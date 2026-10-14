#!/usr/bin/env python3
"""Attribute a Go pprof profile to Loomarr subsystems (#1794).

`go tool pprof -top` ranks functions; a resource audit needs the cost per SUBSYSTEM (API,
scheduler/guide, filler, curation, library sync, images, database, playout, jobs runtime, GC), and
needs to know what STARTED the work (an HTTP request, a River job, a cron tick, the playout loop).
Two views, from the same samples:

  subsystem  who did the work: walking leaf→root, the first frame that names a subsystem. SQLite
             and the store are "database" wherever they are called from, so the database's share is
             not smeared across its callers. Runtime GC workers are "GC"; allocation inside a
             function (mallocgc, GC assist) stays with that function's subsystem.
  initiator  what started it: walking root→leaf, the first frame that names an entry point.

Reads `go tool pprof -raw` (so it works on CPU, heap and allocs profiles alike) and prints Markdown.

  scripts/perf/attribute.py cpu.pb.gz                     CPU seconds per subsystem
  scripts/perf/attribute.py heap.pb.gz --index inuse_space
  scripts/perf/attribute.py heap.pb.gz --index alloc_space --top 15
"""
import argparse
import re
import subprocess
import sys

MOD = "github.com/loomarr/loomarr/"

# internal/<pkg> → subsystem. Anything unlisted reports as its own package name, so a new package
# shows up rather than vanishing into "other".
PKG = {}
for sub, pkgs in {
    "API": "api httpx auth web setup invitation contact secretprotection",
    "scheduler/guide": "schedule binder programmer channels plannerreference channelmaint",
    "jobs runtime": "scheduler bgexec",
    "filler": "filler filleradmission fillerairworthiness fillercorpus fillerdecision fillerenrichment fillereval "
              "fillerresearch fillersafety fillerstructure fillerstructuremedia fillerstructurewindow "
              "fillerstructurewindowopenrouter clipfetch mediameasure media mediatools episodeevidence",
    "curation": "suggest recommend proposaloutlook proposalworkflow recurate ideas taxonomy holidayvocab textmatch "
                "moviecollections tmdb catalog reference quality",
    "library sync": "library inventory requester provision reconcile",
    "images": "images",
    "database": "store fillerstore",
    "playout": "playout watermark viewing playoutbench",
    "platform": "settings config notifications diagnostics recovery retention storagegovernor events activity app "
                "landiscovery installationlocation logchange releasenotes buildinfo backendtransition proctree",
    "llm": "llm openroutercatalog openroutermedia eval",
    "metrics scrape": "metrics",
}.items():
    for p in pkgs.split():
        PKG[p] = sub

# Frames outside the module that still name a subsystem.
EXTERNAL = [
    ("modernc.org/sqlite", "database"),
    ("modernc.org/libc", "database"),
    ("database/sql.", "database"),
    ("github.com/jackc/pgx", "database"),
    ("github.com/riverqueue/river/riverdriver", "database"),
    ("github.com/riverqueue/river", "jobs runtime"),
    ("github.com/prometheus/", "metrics scrape"),
    ("runtime/pprof.", "profiler (self)"),
    ("net/http/pprof.", "profiler (self)"),
]
GC = ("runtime.gcBgMarkWorker", "runtime.bgsweep", "runtime.bgscavenge", "runtime.gcDrain", "runtime.markroot",
      "runtime.gcMarkTermination", "runtime.gcStart")

ENTRY = [
    ("net/http.(*conn).serve", "HTTP request"),
    ("github.com/riverqueue/river", "River job"),
    (MOD + "internal/scheduler", "cron job"),
    (MOD + "internal/playout", "playout loop"),
    ("main.main", "startup (retained since boot)"),
]


def subsystem_of(fn):
    if fn.startswith(GC):
        return "GC"
    if fn.startswith(MOD + "internal/"):
        pkg = fn[len(MOD + "internal/") :].split(".")[0].split("/")[0]
        return PKG.get(pkg, "pkg:" + pkg)
    if fn.startswith(MOD + "cmd/"):
        return "platform"
    for prefix, sub in EXTERNAL:
        if fn.startswith(prefix):
            return sub
    return None


def entry_of(stack_root_first):
    for fn in stack_root_first:
        if fn.startswith(GC):
            return "GC"
        for prefix, name in ENTRY:
            if fn.startswith(prefix):
                return name
    for fn in stack_root_first:
        s = subsystem_of(fn)
        if s:
            return "background: " + s
    return "runtime/other"


def load(path, index, base=None):
    cmd = ["go", "tool", "pprof", "-raw"] + (["-diff_base", base] if base else []) + [path]
    raw = subprocess.run(cmd, capture_output=True, text=True, check=True).stdout
    lines = raw.splitlines()
    i = lines.index("Samples:")
    cols = lines[i + 1].split()
    names = [c.split("/")[0] for c in cols]
    if index is None:
        # cpu profiles, heap profiles, and the wall-clock blocking profiles `go tool trace -pprof=…` writes
        index = next((n for n in ("cpu", "delay", "inuse_space") if n in names), names[-1])
    if index not in names:
        sys.exit(f"attribute: no sample index {index!r}; have {names}")
    col = names.index(index)
    unit = cols[col].split("/")[1].split("[")[0]  # "bytes[dflt]" marks the default index
    samples = []
    j = i + 2
    sample_re = re.compile(r"^\s*([-\d\s]+):\s*([\d\s]*)$")  # a -diff_base profile has negative values
    while j < len(lines) and not lines[j].startswith("Locations"):
        m = sample_re.match(lines[j])
        if m:
            vals = m.group(1).split()
            samples.append((int(vals[col]), [int(x) for x in m.group(2).split()]))
        j += 1
    locs, cur = {}, None
    loc_re = re.compile(r"^\s*(\d+):\s+0x[0-9a-f]+\s+M=\d+\s+(\S+)")
    for line in lines[j + 1 :]:
        if line.startswith("Mappings"):
            break
        m = loc_re.match(line)
        if m:
            cur = int(m.group(1))
            locs[cur] = [m.group(2)]
        elif cur is not None and line.startswith(" " * 13) and line.strip():
            locs[cur].append(line.split()[0])  # an inlined caller of the line above
    return samples, locs, index, unit


def fmt(v, unit):
    if unit == "nanoseconds":
        return f"{v / 1e9:.2f}s"
    if unit == "bytes":
        for u, d in (("GB", 1e9), ("MB", 1e6), ("kB", 1e3)):
            if v >= d:
                return f"{v / d:.1f}{u}"
        return f"{v}B"
    return str(v)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("profile")
    ap.add_argument("--base", help="subtract this earlier profile (e.g. allocs at the window's start: what the window allocated)")
    ap.add_argument("--index", help="sample index: cpu, inuse_space, alloc_space, … (default: cpu or inuse_space)")
    ap.add_argument("--top", type=int, default=8, help="functions listed under each of the top subsystems")
    a = ap.parse_args()
    samples, locs, index, unit = load(a.profile, a.index, a.base)
    total = sum(v for v, _ in samples) or 1
    by_sub, by_entry, cross, funcs = {}, {}, {}, {}
    for v, ids in samples:
        if not v:
            continue
        stack = [fn for lid in ids for fn in locs.get(lid, ["?"])]  # leaf first
        sub = next((s for s in map(subsystem_of, stack) if s), "runtime/other")
        if any(fn.startswith(GC) for fn in stack):
            sub = "GC"
        ent = entry_of(list(reversed(stack)))
        by_sub[sub] = by_sub.get(sub, 0) + v
        by_entry[ent] = by_entry.get(ent, 0) + v
        cross[(ent, sub)] = cross.get((ent, sub), 0) + v
        leaf = next((fn for fn in stack if subsystem_of(fn) == sub), stack[0] if stack else "?")
        funcs.setdefault(sub, {})
        funcs[sub][leaf] = funcs[sub].get(leaf, 0) + v
    print(f"### {a.profile} — {index}, total {fmt(total, unit)}\n")
    print("| subsystem | " + index + " | share |\n|---|---|---|")
    for s, v in sorted(by_sub.items(), key=lambda kv: -kv[1]):
        print(f"| {s} | {fmt(v, unit)} | {100 * v / total:.1f}% |")
    print("\n| initiator | " + index + " | share | biggest subsystems under it |\n|---|---|---|---|")
    for e, v in sorted(by_entry.items(), key=lambda kv: -kv[1]):
        subs = sorted(((s, x) for (ee, s), x in cross.items() if ee == e), key=lambda kv: -kv[1])[:3]
        print(f"| {e} | {fmt(v, unit)} | {100 * v / total:.1f}% | " + ", ".join(f"{s} {100 * x / total:.1f}%" for s, x in subs) + " |")
    print()
    for s, _ in sorted(by_sub.items(), key=lambda kv: -kv[1])[:4]:
        print(f"<details><summary>{s}: first {s} frame from the leaf</summary>\n")
        for fn, v in sorted(funcs[s].items(), key=lambda kv: -kv[1])[: a.top]:
            print(f"- {fmt(v, unit)} ({100 * v / total:.1f}%) `{fn}`")
        print("\n</details>\n")


if __name__ == "__main__":
    main()
