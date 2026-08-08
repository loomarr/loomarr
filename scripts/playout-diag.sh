#!/usr/bin/env bash
# playout-diag.sh — a forensic snapshot of live internal playout (§9.1).
#
# WHY THIS EXISTS. Debugging a channel that plays black in the browser repeatedly needs the
# same three answers, none of which the app surfaces at shell level: (1) what ffmpeg processes
# are live and how they map to channels/targets, (2) what the GPU + the resident LLM are doing
# (they share VRAM — §8.2, §9.1 V47), and (3) per channel, what codec is airing and whether it
# is direct-played or transcoded, on hardware or software. This session hand-rolled all three a
# dozen times; this script makes them one command.
#
# It reads ONLY — it never kills a process, never touches the DB, never restarts anything. Safe
# to run against the live dev box or a real deployment at any time.
#
# Usage:
#   scripts/playout-diag.sh           # full snapshot
#   scripts/playout-diag.sh --json    # machine-readable (for piping / CI)
#
# Env (optional): OLLAMA_URL (default http://localhost:11434), FFMPEG (default ffmpeg).

set -u

OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"
FFMPEG="${FFMPEG:-ffmpeg}"
JSON=0
[ "${1:-}" = "--json" ] && JSON=1

# --- helpers ---------------------------------------------------------------

# cmdline PID → space-joined argv (NUL-separated in /proc), empty if the process is gone.
cmdline() { tr '\0' ' ' < "/proc/$1/cmdline" 2>/dev/null; }

# ffmpeg_pids — every live ffmpeg process (by exact comm, so a shell mentioning "ffmpeg"
# in its own argv is not matched — a trap this session hit repeatedly).
ffmpeg_pids() { pgrep -x ffmpeg 2>/dev/null; }

# role CMD → parent(concat) | remux(hls) | program(child) | card(offline) | other.
#
# Keyed off what each tier's ACTUAL argv carries (verified against live processes), which is not
# always the URL you'd expect:
#   - parent: reads the ffconcat playlist over HTTP — argv has `-f concat` and `/playlist/`.
#   - remux:  the `-c copy -f hls` repackager — argv has `-f hls` and a `/loomarr-hls-*/` dir.
#   - program child: spawned server-side with the RESOLVED FILE PATH directly — argv has
#     `-i /path -f mpegts` and NO `/program/` (that URL is the parent's, not the child's). This
#     was the trap: matching `/program/` found nothing and mislabeled every child as "other".
#   - card: the synthetic offline card — lavfi/drawtext.
role() {
  case "$1" in
    *" -f concat "*|*"/playlist/"*) echo "parent" ;;
    *loomarr-hls-*|*" hls "*)       echo "remux" ;;
    *lavfi*|*drawtext*)             echo "card" ;;
    *" -i "*" -f mpegts "*)         echo "program" ;;
    *)                              echo "other" ;;
  esac
}

# field CMD REGEX → first match of REGEX in CMD (empty if none).
field() { echo "$1" | grep -oE "$2" | head -1; }

# --- GPU + LLM (the shared-VRAM picture) -----------------------------------

gpu_line() {
  if command -v nvidia-smi >/dev/null 2>&1; then
    timeout 8 nvidia-smi --query-gpu=name,memory.used,memory.total,utilization.gpu,utilization.encoder \
      --format=csv,noheader,nounits 2>/dev/null | head -1
  fi
}

llm_resident() {
  # Ollama's /api/ps lists models currently in memory + their VRAM footprint.
  curl -s -m 4 "$OLLAMA_URL/api/ps" 2>/dev/null | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
for m in d.get("models", []):
    print(f"{m.get(\"name\",\"?\")}\t{round(m.get(\"size_vram\",0)/1e9,1)}")
' 2>/dev/null
}

# --- per-process classification --------------------------------------------

# Emits one TSV row per live ffmpeg: pid \t role \t channel \t target \t encoder \t input
classify() {
  for pid in $(ffmpeg_pids); do
    cmd=$(cmdline "$pid")
    [ -z "$cmd" ] && continue
    r=$(role "$cmd")
    ch=$(field "$cmd" 'ch_[a-f0-9]{16}')
    tgt=$(field "$cmd" 'target=[a-z]+'); tgt=${tgt#target=}
    enc=$(field "$cmd" '\-c:v (copy|libx264|h264_[a-z0-9]+|hevc_[a-z0-9]+)'); enc=${enc#-c:v }
    # Input path can contain spaces (real library files do), so match up to the next flag/redirect
    # (` -` or ` pipe:`) rather than the first space — else a "Star Trek …" file truncates to "Star".
    inp=$(echo "$cmd" | grep -oE '\-i /[^ ].*\.(mkv|avi|mp4|ts|m3u8|mov|m4v)' | head -1); inp=${inp#-i }
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$pid" "$r" "${ch:-–}" "${tgt:-–}" "${enc:-–}" "${inp:-–}"
  done
}

# --- render ----------------------------------------------------------------

if [ "$JSON" = "1" ]; then
  gpu=$(gpu_line)
  export DIAG="$0"
  python3 - "$gpu" <<'PY'
import sys, subprocess, json, os
gpu = sys.argv[1] if len(sys.argv) > 1 else ""
rows = []
out = subprocess.run(["bash", os.environ["DIAG"], "--_rows"], capture_output=True, text=True).stdout
for line in out.splitlines():
    p = line.split("\t")
    if len(p) == 6:
        rows.append(dict(pid=p[0], role=p[1], channel=p[2], target=p[3], encoder=p[4], input=p[5]))
llm = []
lout = subprocess.run(["bash", os.environ["DIAG"], "--_llm"], capture_output=True, text=True).stdout
for line in lout.splitlines():
    q = line.split("\t")
    if len(q) == 2:
        llm.append(dict(model=q[0], vramGiB=float(q[1])))
gp = gpu.split(", ") if gpu else []
print(json.dumps(dict(
    gpu=(dict(name=gp[0], memUsedMiB=gp[1], memTotalMiB=gp[2], gpuUtilPct=gp[3], encUtilPct=gp[4]) if len(gp) == 5 else None),
    residentModels=llm,
    processes=rows,
), indent=2))
PY
  exit 0
fi

# Hidden sub-commands the JSON path shells back into (keeps one source of truth).
if [ "${1:-}" = "--_rows" ]; then classify; exit 0; fi
if [ "${1:-}" = "--_llm" ]; then llm_resident; exit 0; fi

echo "=== GPU + shared VRAM ======================================================"
gpu=$(gpu_line)
if [ -n "$gpu" ]; then
  echo "  $gpu"
  echo "  (name, mem used MiB, mem total MiB, gpu util %, encoder-engine util %)"
else
  echo "  no nvidia-smi / no GPU detected — encoders run on CPU (software)"
fi
echo "  resident LLM (shares this VRAM — §8.2):"
llm=$(llm_resident)
if [ -n "$llm" ]; then
  echo "$llm" | while IFS="$(printf '\t')" read -r model vram; do echo "    - $model  ${vram}GB"; done
else
  echo "    (none resident — full VRAM available to encoders)"
fi

echo
echo "=== live ffmpeg by channel + target ========================================"
rows=$(classify)
if [ -z "$rows" ]; then
  echo "  (no ffmpeg running — no channel is being streamed right now)"
else
  printf '  %-8s %-8s %-20s %-11s %-13s %s\n' PID ROLE CHANNEL TARGET ENCODER INPUT
  echo "$rows" | while IFS="$(printf '\t')" read -r pid r ch tgt enc inp; do
    # Trim the input to a basename for readability.
    base=$(basename "$inp" 2>/dev/null); [ "$inp" = "–" ] && base="–"
    printf '  %-8s %-8s %-20s %-11s %-13s %s\n' "$pid" "$r" "$ch" "$tgt" "$enc" "$base"
  done

  echo
  echo "=== pipeline summary ======================================================="
  # Honest scope: a program CHILD is spawned with just its file path (no channel id / target in
  # argv), and a REMUX writes to a HASHED dir — so shell forensics cannot reliably map either back
  # to its channel. That mapping lives in the app, which spawned them: `GET /v1/playout/sessions`
  # (per (channel,target) encoder + speed) and the doctor endpoint are the per-channel truth. What
  # the process table CAN answer authoritatively is the shape and the load:
  n_parent=$(echo "$rows"  | awk -F'\t' '$2=="parent"'  | wc -l | tr -d ' ')
  n_remux=$(echo "$rows"   | awk -F'\t' '$2=="remux"'   | wc -l | tr -d ' ')
  n_program=$(echo "$rows" | awk -F'\t' '$2=="program"' | wc -l | tr -d ' ')
  n_copy=$(echo "$rows"    | awk -F'\t' '$5=="copy"'    | wc -l | tr -d ' ')
  n_soft=$(echo "$rows"    | awk -F'\t' '$5=="libx264"' | wc -l | tr -d ' ')
  n_hw=$(echo "$rows"      | awk -F'\t' '$5 ~ /^(h264|hevc)_/' | wc -l | tr -d ' ')
  echo "  sessions (concat parents):  $n_parent"
  echo "  HLS remuxes (browser Watch): $n_remux"
  echo "  program children live now:   $n_program  (direct-copy: $n_copy, hardware: $n_hw, SOFTWARE: $n_soft)"
  echo
  # The one cross-cutting risk this table CAN flag: concurrent SOFTWARE transcodes are the CPU
  # ceiling — several at once is the shape most likely to fall below realtime and stall a channel.
  if [ "${n_soft:-0}" -ge 2 ]; then
    echo "  ⚠ $n_soft concurrent SOFTWARE transcodes — the CPU realtime ceiling. If a channel"
    echo "    stalls, this is the first suspect (an old codec with no hardware decoder, §9.1 ladder)."
  fi
  parents_browser=$(echo "$rows" | awk -F'\t' '$2=="parent" && $4=="browser"' | wc -l | tr -d ' ')
  if [ "${parents_browser:-0}" -gt "${n_remux:-0}" ]; then
    echo "  ⚠ $parents_browser browser session(s) but only $n_remux remux(es) — a browser session"
    echo "    with no remux serves no HLS (the <video> element gets nothing). Check the doctor endpoint."
  fi
fi
