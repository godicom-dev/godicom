#!/usr/bin/env bash
# Fetch multi-frame pydicom-data files not shipped in the pydicom submodule test_files.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

urls_json="$root/pydicom/src/pydicom/data/urls.json"
hashes_json="$root/pydicom/src/pydicom/data/hashes.json"
out_dir="$root/pydicom/src/pydicom/data/test_files"

if [[ ! -f "$urls_json" ]]; then
  echo "missing $urls_json — initialize pydicom submodule first" >&2
  exit 1
fi

names=(
  emri_small.dcm
  emri_small_big_endian.dcm
  emri_small_RLE.dcm
  emri_small_jpeg_ls_lossless.dcm
  emri_small_jpeg_2k_lossless.dcm
)

python3 <<PY
import hashlib
import json
import pathlib
import urllib.request

urls_path = "$urls_json"
hashes_path = "$hashes_json"
out_dir = "$out_dir"
names = [
    "emri_small.dcm",
    "emri_small_big_endian.dcm",
    "emri_small_RLE.dcm",
    "emri_small_jpeg_ls_lossless.dcm",
    "emri_small_jpeg_2k_lossless.dcm",
]

with open(urls_path, encoding="utf-8") as f:
    urls = json.load(f)
with open(hashes_path, encoding="utf-8") as f:
    hashes = json.load(f)

for name in names:
    if name not in urls:
        raise SystemExit(f"unknown test file: {name}")
    path = pathlib.Path(out_dir) / name
    want = hashes[name]
    if path.is_file():
        got = hashlib.sha256(path.read_bytes()).hexdigest()
        if got == want:
            print(f"ok {name}")
            continue
    print(f"fetch {name}")
    data = urllib.request.urlopen(urls[name]).read()
    got = hashlib.sha256(data).hexdigest()
    if got != want:
        raise SystemExit(f"hash mismatch for {name}: got {got}, want {want}")
    path.write_bytes(data)
    print(f"wrote {path}")
PY

echo "testdata ready under $out_dir"
