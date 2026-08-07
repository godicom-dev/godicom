#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

echo "Building Go bench..." >&2
go build -o bench_compare .

echo "Running benchmarks (may take a few minutes)..." >&2
python3 bench.py --report

echo "Done. See REPORT.md" >&2
