#!/usr/bin/env bash
# Fetch multi-frame pydicom-data files (not in the pydicom submodule test_files).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

manifest="$root/scripts/emri_testdata.json"
out_dir="$root/testdata/dcm"

if [[ ! -f "$manifest" ]]; then
  echo "missing $manifest" >&2
  exit 1
fi

mkdir -p "$out_dir"

python3 <<PY
import hashlib
import json
import pathlib
import urllib.request

manifest = pathlib.Path("$manifest")
out_dir = pathlib.Path("$out_dir")
entries = json.loads(manifest.read_text(encoding="utf-8"))

for name, meta in entries.items():
    path = out_dir / name
    want = meta["sha256"]
    if path.is_file():
        got = hashlib.sha256(path.read_bytes()).hexdigest()
        if got == want:
            print(f"ok {name}")
            continue
    print(f"fetch {name}")
    data = urllib.request.urlopen(meta["url"]).read()
    got = hashlib.sha256(data).hexdigest()
    if got != want:
        raise SystemExit(f"hash mismatch for {name}: got {got}, want {want}")
    path.write_bytes(data)
    print(f"wrote {path}")
PY

echo "testdata ready under $out_dir"
