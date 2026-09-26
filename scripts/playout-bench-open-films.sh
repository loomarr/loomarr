#!/bin/sh
# Fetch the open films the playout bench uses as realistic content, verifying each against a pinned
# SHA-256. The films are Blender Foundation productions released under CC BY 3.0; they are
# redistributable, unlike household media, which must never enter the bench.
#
#   scripts/playout-bench-open-films.sh [DIR] [720p|1080p|4k ...]
#
# DIR defaults to $LOOMARR_ARTIFACT_DIR/playout-bench-films. Pass it to the bench as
# PLAYOUT_BENCH_FILMS=DIR. With no size arguments only Tears of Steel 720p (372 MB) is fetched; 4k
# is a 6.7 GB archive.
set -eu

base=https://download.blender.org/demo/movies/ToS
dir=${1:-${LOOMARR_ARTIFACT_DIR:-${TMPDIR:-/tmp}/loomarr-playout-bench}/playout-bench-films}
[ $# -gt 0 ] && shift
[ $# -gt 0 ] || set -- 720p

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# fetch NAME SHA256 [unzip]
fetch() {
  name=$1 want=$2 archive=${3:-}
  target="$dir/$name"
  if [ -f "$target" ] && [ "$(sha256_of "$target")" = "$want" ]; then
    echo "have $name"
  else
    echo "fetching $name"
    curl -fL --retry 3 -o "$target.part" "$base/$name"
    got=$(sha256_of "$target.part")
    if [ "$got" != "$want" ]; then
      rm -f "$target.part"
      echo "playout-bench-open-films: $name hashes to $got, pinned $want; refusing it" >&2
      exit 1
    fi
    mv "$target.part" "$target"
  fi
  if [ -n "$archive" ]; then
    unzip -oq "$target" -d "$dir"
  fi
}

mkdir -p "$dir"
for size in "$@"; do
  case "$size" in
    720p) fetch tears_of_steel_720p.mov efa9062d9cdb7a338e40ad530dfdf234806743f29ae6a1a136b97ece4e588e8f ;;
    1080p) fetch tears_of_steel_1080p.mov.zip d87a41de040d3814dbde143e9ab85ef122caf22265f660b0bebf476cd8b357a5 unzip ;;
    4k) fetch tearsofsteel_4k.mov.zip __4K_SHA__ unzip ;;
    *)
      echo "playout-bench-open-films: unknown size $size (720p, 1080p, 4k)" >&2
      exit 2
      ;;
  esac
done
echo "films in $dir; run: PLAYOUT_BENCH_FILMS=$dir make playout-bench"
