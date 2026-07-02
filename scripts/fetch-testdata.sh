#!/usr/bin/env bash
# Fetch multi-frame pydicom-data files (not in the pydicom submodule test_files).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

out_dir="testdata/dcm"
mkdir -p "$out_dir"

# pydicom-data URLs and SHA256 (from pydicom src/pydicom/data/{urls,hashes}.json)
declare -a names=(
  emri_small.dcm
  emri_small_big_endian.dcm
  emri_small_RLE.dcm
  emri_small_jpeg_ls_lossless.dcm
  emri_small_jpeg_2k_lossless.dcm
)
declare -a urls=(
  "https://github.com/pydicom/pydicom-data/raw/39a2eb31815eec435dc26c322c27aec5cfcbddb6/data/emri_small.dcm"
  "https://github.com/pydicom/pydicom-data/raw/39a2eb31815eec435dc26c322c27aec5cfcbddb6/data/emri_small_big_endian.dcm"
  "https://github.com/pydicom/pydicom-data/raw/39a2eb31815eec435dc26c322c27aec5cfcbddb6/data/emri_small_RLE.dcm"
  "https://github.com/pydicom/pydicom-data/raw/39a2eb31815eec435dc26c322c27aec5cfcbddb6/data/emri_small_jpeg_ls_lossless.dcm"
  "https://github.com/pydicom/pydicom-data/raw/39a2eb31815eec435dc26c322c27aec5cfcbddb6/data/emri_small_jpeg_2k_lossless.dcm"
)
declare -a hashes=(
  151233ec63f64ebb63b979df51aa827cd612a53422c073f6ef341770c7bc9a56
  8e18ed3542bc4df70dc6acda87eab5095b19e2b4c1b7fb72ba457e7c217b1ab7
  93c19bca3fb6b7202dcd067de8d16cb6b3f7c6e9a0632e474aab81175ee45266
  24de03c9c0f8b5aa75d7fbcc894f94e612b66702175b4936589a0849ec9f87b4
  b2b4063359a08ed3b0afa9f4e4f72f84af79e5116515b446d9a30da9dc7f1888
)

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "no sha256sum or shasum available" >&2
    return 1
  fi
}

for i in "${!names[@]}"; do
  name="${names[$i]}"
  url="${urls[$i]}"
  want="${hashes[$i]}"
  path="$out_dir/$name"

  if [[ -f "$path" ]]; then
    got=$(sha256_file "$path")
    if [[ "$got" == "$want" ]]; then
      echo "ok $name"
      continue
    fi
  fi

  echo "fetch $name"
  curl -fsSL "$url" -o "$path"
  got=$(sha256_file "$path")
  if [[ "$got" != "$want" ]]; then
    echo "hash mismatch for $name: got $got, want $want" >&2
    exit 1
  fi
  echo "wrote $path"
done

echo "testdata ready under $out_dir"
