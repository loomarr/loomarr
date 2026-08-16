#!/bin/sh
set -eu

worker=${1:?worker executable is required}
report_dir=${2:?report directory is required}
go_cmd=${GO:-go}
cpu_profiles=${IMAGE_BENCH_CPU_PROFILES:-2,4,8}
runs=${IMAGE_BENCH_RUNS:-1}
roles=${IMAGE_BENCH_ROLES:-poster}
host_cpus=$(getconf _NPROCESSORS_ONLN)

command -v taskset >/dev/null 2>&1 || {
  echo "image-parallelism-bench: taskset is required" >&2
  exit 2
}
mkdir -p "$report_dir"

old_ifs=$IFS
IFS=,
for cpus in $cpu_profiles; do
  case "$cpus" in
    ''|*[!0-9]*)
      echo "image-parallelism-bench: invalid CPU profile $cpus" >&2
      exit 2
      ;;
  esac
  if [ "$cpus" -lt 2 ] || [ "$cpus" -gt 8 ] || [ "$cpus" -gt "$host_cpus" ]; then
    echo "image-parallelism-bench: CPU profile $cpus must be 2..8 and available on this host" >&2
    exit 2
  fi
  cpu_list="0-$((cpus - 1))"
  threads=1
  while [ "$threads" -le "$cpus" ]; do
    if [ $((cpus % threads)) -eq 0 ]; then
      workers=$((cpus / threads))
      report="$report_dir/cpu${cpus}-workers${workers}-threads${threads}.json"
      echo "image-parallelism-bench: ${cpus} CPUs, ${workers} workers x ${threads} threads"
      GOMAXPROCS=$cpus taskset --cpu-list "$cpu_list" "$go_cmd" run ./cmd/image-bench \
        --worker "$worker" --report "$report" --roles "$roles" --runs "$runs" --warmups 1 \
        --cpu-profile "$cpus" --workers "$workers" --avif-threads "$threads"
    fi
    threads=$((threads * 2))
  done
done
IFS=$old_ifs

echo "image-parallelism-bench: reports $report_dir"
