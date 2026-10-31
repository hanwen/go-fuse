#!/bin/bash
# Compare benchmarks of a base revision (default @-) against the
# working copy. Old and new runs alternate to reduce drift.
#
#   benchmark/compare.bash [BASE_REV]
#
# Environment: COUNT (rounds, default 6), BENCH (regexp), CPU (-cpu
# list, default 1,2,4), OUT (output dir, default a new temp dir).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
base_rev=${1:-@-}
count=${COUNT:-6}
cpu=${CPU:-1,2,4}
bench=${BENCH:-'BenchmarkGoFuseMemoryRead|BenchmarkGoFuseFDRead|BenchmarkGoFSStat|BenchmarkGoFSParallelReaddir|BenchmarkNegStat'}
out=${OUT:-$(mktemp -d)}
tmp=$(mktemp -d)
base=$tmp/base
ws=bench-base-$$

cd "$repo"
jj workspace add --name "$ws" -r "$base_rev" "$base"
trap 'cd "$repo"; jj workspace forget "$ws"; rm -rf "$tmp"' EXIT

# BenchmarkGoFSStat runs this helper; it is not checked in.
make -C "$base/benchmark" bulkstat.bin
make -C "$repo/benchmark" bulkstat.bin

rm -f "$out/old.txt" "$out/new.txt"
for i in $(seq "$count"); do
	(cd "$base/benchmark" && go test -run '^$' -bench "$bench" -cpu "$cpu" -count 1) | tee -a "$out/old.txt"
	(cd "$repo/benchmark" && go test -run '^$' -bench "$bench" -cpu "$cpu" -count 1) | tee -a "$out/new.txt"
done

go run golang.org/x/perf/cmd/benchstat@latest "$out/old.txt" "$out/new.txt" | tee "$out/benchstat.txt"
echo "results in $out"
