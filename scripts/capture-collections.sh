#!/usr/bin/env bash
# Capture the media server's COLLECTIONS contract, so the adapter is written against pinned
# truth rather than remembered field names (CLAUDE.md: "Write parsers against the fixtures").
#
# ⚠ READ-ONLY. Every request is a GET. Nothing is created, renamed or deleted on your server.
#
# ⚠ Phase-0-style live contact, so it is MAINTAINER-RUN by design — an agent must not touch a
# real media server. It reads LIBRARY_URL / LIBRARY_TOKEN / LIBRARY_FLAVOR from .env, the same
# three keys the smoke uses.
#
# What it answers, and why each matters for scope.collections:
#   1. Which item type collections are          → what to ask for when LISTING them
#   2. What a collection item looks like        → the id + name the picker shows
#   3. How MEMBERSHIP resolves                  → the query that turns "collection X" into keys
#   4. Whether a member carries provider ids    → whether membership maps to a provisioning key
#      at all, or needs a second lookup per item (a real cost difference in the engine)
#
# Output: internal/testkit/fixtures/emby/collections_*.json + FINDINGS-collections.md
# Scrubbed of ids/paths/hostnames before writing — see scrub() below.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/internal/testkit/fixtures/emby"
[ -f "$ROOT/.env" ] || { echo "no .env — this needs your real LIBRARY_URL/LIBRARY_TOKEN"; exit 1; }

# shellcheck disable=SC1091
set -a; . "$ROOT/.env"; set +a
: "${LIBRARY_URL:?LIBRARY_URL not set in .env}"
: "${LIBRARY_TOKEN:?LIBRARY_TOKEN not set in .env}"
FLAVOR="${LIBRARY_FLAVOR:-emby}"

# Header auth only — never the api_key query param, which leaks into server logs (§6).
if [ "$FLAVOR" = "jellyfin" ]; then
  AUTH=(-H "Authorization: MediaBrowser Client=\"Loomarr\", Device=\"capture\", DeviceId=\"loomarr-capture\", Version=\"0.0.0\", Token=\"$LIBRARY_TOKEN\"")
else
  AUTH=(-H "X-Emby-Token: $LIBRARY_TOKEN")
fi

get() { curl -sS --max-time 20 "${AUTH[@]}" "$LIBRARY_URL$1"; }

# scrub — remove anything that identifies YOUR install before the fixture is committed:
# real item/user/server ids become stable placeholders, absolute media paths are dropped, and
# the host disappears. The SHAPE is what we need pinned; the contents are yours.
scrub() {
  # ⚠ The piped JSON goes to a TEMP FILE, not stdin: the heredoc below already occupies stdin,
  # so `sys.stdin.read()` would consume the Python source rather than the payload. And scrub
  # takes NO arguments — referencing `$1` here is what broke the first version under `set -u`,
  # found by running the script against a stub server rather than a real one.
  local tmp; tmp="$(mktemp)"; cat > "$tmp"
  python3 - "$tmp" <<'PY'
import json, re, sys
raw = open(sys.argv[1]).read()
try:
    doc = json.loads(raw)
except Exception:
    print(raw); raise SystemExit
seen = {}
def fake(v, kind):
    if v not in seen:
        seen[v] = f"{kind}-{len(seen)+1:03d}"
    return seen[v]
def walk(o):
    if isinstance(o, dict):
        out = {}
        for k, v in o.items():
            if k in ("Path", "ContainerStartTimeTicks"):        # absolute media paths
                continue
            if k in ("Id", "ParentId", "SeriesId", "ServerId") and isinstance(v, str):
                out[k] = fake(v, "id"); continue
            if k == "Name" and isinstance(v, str) and len(v) > 60:
                out[k] = v[:60]; continue
            out[k] = walk(v)
        return out
    if isinstance(o, list):
        return [walk(x) for x in o]
    if isinstance(o, str):
        return re.sub(r"https?://[^\s\"']+", "http://media.local", o)
    return o
print(json.dumps(walk(doc), indent=2))
PY
  rm -f "$tmp"
}

mkdir -p "$OUT"
echo "→ capturing from $LIBRARY_URL (flavor: $FLAVOR) — read-only"

# 1. LIST collections. BoxSet is the type Emby/Jellyfin use for a collection; asking for it
#    explicitly (rather than filtering client-side) is what the adapter would do.
echo "  [1/4] listing collections (IncludeItemTypes=BoxSet)"
get "/Items?IncludeItemTypes=BoxSet&Recursive=true&Fields=ProviderIds,ChildCount&SortBy=SortName" \
  | scrub > "$OUT/collections_list.json"

FIRST_ID=$(python3 -c "
import json;d=json.load(open('$OUT/collections_list.json'))
items=d.get('Items') or []
print(items[0]['Id'] if items else '')" 2>/dev/null || echo "")

# The REAL id, not the scrubbed one — needed to make the next call. Read from a raw re-fetch
# so the scrubbed fixture never has to carry it.
RAW_ID=$(get "/Items?IncludeItemTypes=BoxSet&Recursive=true&Limit=1" \
  | python3 -c "
import json,sys
try:
    items=json.load(sys.stdin).get('Items') or []
    print(items[0]['Id'] if items else '')
except Exception: print('')" 2>/dev/null || echo "")

if [ -z "$RAW_ID" ]; then
  echo "  ⚠ no collections found on this server — steps 2-4 skipped."
  echo "     Make one in Emby (any two movies) and re-run, or the adapter has nothing to parse."
else
  # 2. The collection itself, in full — every field it carries.
  echo "  [2/4] one collection in full"
  get "/Items?Ids=$RAW_ID&Fields=ProviderIds,ChildCount,Overview,Genres" | scrub > "$OUT/collections_one.json"

  # 3. MEMBERSHIP. This is the load-bearing one: whether ParentId works for a BoxSet, and what
  #    a member item looks like coming back.
  echo "  [3/4] membership via ParentId"
  get "/Items?ParentId=$RAW_ID&Recursive=true&Fields=ProviderIds,ParentId" | scrub > "$OUT/collections_members.json"

  # 4. Do members carry PROVIDER IDS? If yes, membership maps straight to a provisioning key
  #    (§3) and the engine filter is a set-membership test. If no, every member needs a second
  #    lookup — a materially different design, and the reason this is captured rather than assumed.
  echo "  [4/4] provider ids on members"
  get "/Items?ParentId=$RAW_ID&Recursive=true&Fields=ProviderIds&Limit=3" | scrub > "$OUT/collections_member_ids.json"
fi

# A findings note in the same shape as the other captures, filled in from what came back.
python3 - "$OUT" <<'PY'
import json, os, sys
out = sys.argv[1]
def load(n):
    p = os.path.join(out, n)
    return json.load(open(p)) if os.path.exists(p) else None
lst, mem = load("collections_list.json"), load("collections_members.json")
n_coll = len((lst or {}).get("Items") or [])
members = (mem or {}).get("Items") or []
with_ids = [m for m in members if m.get("ProviderIds")]
lines = [
    "# FINDINGS — media-server collections (scope.collections)",
    "",
    "Captured live by `scripts/capture-collections.sh` (read-only GETs). Scrubbed of real ids,",
    "paths and hostnames; the SHAPE is what is pinned.",
    "",
    f"- Collections found: **{n_coll}** (queried as `IncludeItemTypes=BoxSet&Recursive=true`)",
    f"- Members in the sampled collection: **{len(members)}** (queried as `ParentId=<id>&Recursive=true`)",
    f"- Members carrying `ProviderIds`: **{len(with_ids)}/{len(members)}**",
    "",
    "## Why the last line decides the design",
    "",
    "If members carry `ProviderIds`, collection membership maps straight onto a provisioning",
    "key (§3: `<mediatype>:tmdb:<id>` / `series:tvdb:<id>`) and the engine filter is a set-",
    "membership test with no extra I/O. If they do NOT, every member needs a second lookup, and",
    "`scope.collections` becomes an N+1 on the scheduling path — a different feature.",
    "",
    "## Fields observed on a collection",
    "",
    "```",
    ", ".join(sorted((((lst or {}).get("Items") or [{}])[0]).keys())) or "(none — no collections on the server)",
    "```",
    "",
    "## Fields observed on a member",
    "",
    "```",
    ", ".join(sorted((members[0] if members else {}).keys())) or "(none)",
    "```",
]
open(os.path.join(out, "FINDINGS-collections.md"), "w").write("\n".join(lines) + "\n")
PY

echo
echo "✓ wrote:"
ls -1 "$OUT"/collections_*.json "$OUT/FINDINGS-collections.md" 2>/dev/null | sed 's|.*/|    |'
echo
echo "Review the files (they are scrubbed, but YOU should confirm before committing), then"
echo "paste back FINDINGS-collections.md — that decides the adapter's shape."
