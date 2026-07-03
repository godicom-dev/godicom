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

PYDICOM_DATA_REF="fb1f24e4f0418008757766d8e79ec92dc2ab9855"
charset_base="https://raw.githubusercontent.com/pydicom/pydicom/${PYDICOM_DATA_REF}/src/pydicom/data/charset_files"

declare -a charset_names=(
  chrRuss.dcm
  chrFren.dcm
  chrGreek.dcm
  chrX1.dcm
  chrSQEncoding.dcm
)
declare -a charset_urls=(
  "${charset_base}/chrRuss.dcm"
  "${charset_base}/chrFren.dcm"
  "${charset_base}/chrGreek.dcm"
  "${charset_base}/chrX1.dcm"
  "${charset_base}/chrSQEncoding.dcm"
)
declare -a charset_hashes=(
  e82d8856b7d9fb407a80a2824dc7adf8daf7ced265450d698955f754a9af1730
  8363f3d2e55b448a688ed7863675bf9645e77a919486ddfb8faa12227e28b097
  cdbdf7820642c13c26b49e496d571f390418d3078e75b2c070e5c668861ed49b
  133232a666587ee884804cb07aaa4becf36f720bc5919a0781732ce691f5dedc
  b124a74bcf2f258ee8c99c354208eb7ceb3969e7f2e827d9dd6e41905facaa7e
)

fetch_verified() {
  local name="$1"
  local url="$2"
  local want="$3"
  local dir="$4"
  local path="$dir/$name"

  if [[ -f "$path" ]]; then
    got=$(sha256_file "$path")
    if [[ "$got" == "$want" ]]; then
      echo "ok $name"
      return 0
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
}

for i in "${!names[@]}"; do
  fetch_verified "${names[$i]}" "${urls[$i]}" "${hashes[$i]}" "$out_dir"
done

charset_dir="testdata/charset"
mkdir -p "$charset_dir"
for i in "${!charset_names[@]}"; do
  fetch_verified "${charset_names[$i]}" "${charset_urls[$i]}" "${charset_hashes[$i]}" "$charset_dir"
done

echo "testdata ready under $out_dir and $charset_dir"
