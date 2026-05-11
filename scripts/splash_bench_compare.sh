#!/usr/bin/env bash
# Run splash benchmarks against the package and report.
# Optionally compare to a baseline saved by a previous run.
#
# Usage: ./scripts/splash_bench_compare.sh [baseline.txt]
#
# Outputs current run to tmp/splash_bench.txt. When a baseline argument is
# supplied AND the file exists, runs benchstat to print delta vs baseline.
# Requires `benchstat` (go install golang.org/x/perf/cmd/benchstat@latest).

set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p tmp

# Run benchmarks across the splash package and its xpath sub-package.
go test -bench=. -benchmem -count=10 \
    ./internal/infrastructure/splash/... \
    | tee tmp/splash_bench.txt

if [[ -n "${1:-}" ]] && [[ -f "$1" ]]; then
    if ! command -v benchstat >/dev/null; then
        echo "Install benchstat: go install golang.org/x/perf/cmd/benchstat@latest" >&2
        exit 1
    fi
    benchstat "$1" tmp/splash_bench.txt
fi
