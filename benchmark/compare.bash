#!/bin/bash
# Compare benchmarks of a base revision (default @-) against the
# working copy. Old and new runs alternate to reduce drift.
#
#   benchmark/compare.bash [BASE_REV]
#
# Results are cached by their inputs (commit ID, benchmark parameters,
# machine, CPU frequency, Go and kernel version), so revisions that
# were already measured are not run again. Each result is stored as
# <hash>.txt next to <hash>.json, which holds the inputs; <hash> is
# the SHA-256 of that JSON. Requires jq.
#
# Environment: COUNT (rounds, default 6), BENCH (regexp), CPU (-cpu
# list, default 1,2,4), OUT (output dir, default a new temp dir),
# CACHE (cache dir, default ~/.cache/go-fuse-bench), NOCACHE=1 to
# ignore cached results, ALLOW_FREQ_SCALING=1 to skip the CPU
# frequency check.
set -euo pipefail

if ! command -v jq >/dev/null; then
	echo "$0: needs jq" >&2
	exit 1
fi

repo=$(cd "$(dirname "$0")/.." && pwd)
base_rev=${1:-@-}
count=${COUNT:-6}
cpu=${CPU:-1,2,4}
bench=${BENCH:-'BenchmarkGoFuseMemoryRead|BenchmarkGoFuseMemoryWrite|BenchmarkGoFuseFDRead|BenchmarkGoFSStat|BenchmarkGoFSParallelReaddir|BenchmarkNegStat'}
out=${OUT:-$(mktemp -d)}
cache=${CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/go-fuse-bench}

# min/max frequency of each CPU; part of the cache key.
freqs=$(cat /sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_{min,max}_freq 2>/dev/null | sort -u | tr '\n' ' ' || true)

# Frequency scaling makes results noisy and incomparable.
if [ -z "${ALLOW_FREQ_SCALING:-}" ]; then
	bad=0
	found=0
	for d in /sys/devices/system/cpu/cpu[0-9]*/cpufreq; do
		[ -r "$d/scaling_min_freq" ] || continue
		found=1
		min=$(cat "$d/scaling_min_freq")
		max=$(cat "$d/scaling_max_freq")
		if [ "$min" != "$max" ]; then
			echo "$d: scaling_min_freq $min != scaling_max_freq $max" >&2
			bad=1
		fi
	done
	if [ "$found" = 0 ]; then
		echo "warning: no cpufreq info; cannot check frequency scaling" >&2
	fi
	if [ "$bad" = 1 ]; then
		echo "CPU frequency scaling is active; pin min and max frequency, eg." >&2
		echo "  sudo cpupower frequency-set -d F -u F" >&2
		echo "or set ALLOW_FREQ_SCALING=1" >&2
		exit 1
	fi
fi

cd "$repo"
base_id=$(jj log --no-graph -r "$base_rev" -T commit_id)
new_id=$(jj log --no-graph -r @ -T commit_id)

# inputs prints the JSON describing everything that the result of
# benchmarking commit $1 depends on.
inputs() {
	jq -n -S -c \
		--arg commit "$1" \
		--arg bench "$bench" \
		--arg count "$count" \
		--arg cpu "$cpu" \
		--arg host "$(hostname)" \
		--arg cpu_model "$(grep -m1 '^model name' /proc/cpuinfo | cut -d: -f2- | sed 's/^ *//')" \
		--arg freqs "$freqs" \
		--arg kernel "$(uname -r)" \
		--arg go "$(go env GOVERSION)" \
		--arg goflags "$(go env GOFLAGS)" \
		--arg goexperiment "$(go env GOEXPERIMENT)" \
		--arg goamd64 "$(go env GOAMD64)" \
		'$ARGS.named'
}

mkdir -p "$cache" "$out"
old_inputs=$(inputs "$base_id")
new_inputs=$(inputs "$new_id")
old_hash=$(printf '%s\n' "$old_inputs" | shasum -a 256 | cut -d' ' -f1)
new_hash=$(printf '%s\n' "$new_inputs" | shasum -a 256 | cut -d' ' -f1)
old_cache=$cache/$old_hash.txt
new_cache=$cache/$new_hash.txt

run_old=1
run_new=1
if [ -z "${NOCACHE:-}" ]; then
	[ -s "$old_cache" ] && run_old=0 && echo "using cached $old_cache"
	[ -s "$new_cache" ] && run_new=0 && echo "using cached $new_cache"
fi

tmp=$(mktemp -d)
base=$tmp/base
ws=bench-base-$$
cleanup() {
	cd "$repo"
	if [ -d "$base" ]; then
		jj workspace forget "$ws"
	fi
	rm -rf "$tmp"
}
trap cleanup EXIT

# BenchmarkGoFSStat runs this helper; it is not checked in.
if [ "$run_old" = 1 ]; then
	jj workspace add --name "$ws" -r "$base_id" "$base"
	make -C "$base/benchmark" bulkstat.bin
fi
if [ "$run_new" = 1 ]; then
	make -C "$repo/benchmark" bulkstat.bin
fi

rm -f "$tmp/old.txt" "$tmp/new.txt"
if [ "$run_old" = 1 ] || [ "$run_new" = 1 ]; then
	for i in $(seq "$count"); do
		if [ "$run_old" = 1 ]; then
			(cd "$base/benchmark" && go test -run '^$' -bench "$bench" -cpu "$cpu" -count 1) | tee -a "$tmp/old.txt"
		fi
		if [ "$run_new" = 1 ]; then
			(cd "$repo/benchmark" && go test -run '^$' -bench "$bench" -cpu "$cpu" -count 1) | tee -a "$tmp/new.txt"
		fi
	done
fi

# Only complete runs are cached.
if [ "$run_old" = 1 ]; then
	printf '%s\n' "$old_inputs" >"$cache/$old_hash.json"
	cp "$tmp/old.txt" "$old_cache"
fi
if [ "$run_new" = 1 ]; then
	printf '%s\n' "$new_inputs" >"$cache/$new_hash.json"
	cp "$tmp/new.txt" "$new_cache"
fi
cp "$old_cache" "$out/old.txt"
cp "$new_cache" "$out/new.txt"

# Column labels for benchstat ("label=path"; labels can't contain '=').
label() {
	jj log --no-graph -r "$1" -T 'commit_id.short(8) ++ " " ++ if(description, description.first_line(), "(no description)")' |
		tr -d '=' | cut -c1-40
}
old_label=$(label "$base_id")
new_label=$(label "$new_id")

go run golang.org/x/perf/cmd/benchstat@latest "$old_label=$out/old.txt" "$new_label=$out/new.txt" | tee "$out/benchstat.txt"
echo "results in $out"
