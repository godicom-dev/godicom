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
  chrArab.dcm
  chrFren.dcm
  chrFrenMulti.dcm
  chrGerm.dcm
  chrGreek.dcm
  chrH31.dcm
  chrH32.dcm
  chrHbrw.dcm
  chrI2.dcm
  chrJapMulti.dcm
  chrJapMultiExplicitIR6.dcm
  chrKoreanMulti.dcm
  chrRuss.dcm
  chrSQEncoding.dcm
  chrSQEncoding1.dcm
  chrX1.dcm
  chrX2.dcm
)
declare -a charset_urls=(
  "${charset_base}/chrArab.dcm"
  "${charset_base}/chrFren.dcm"
  "${charset_base}/chrFrenMulti.dcm"
  "${charset_base}/chrGerm.dcm"
  "${charset_base}/chrGreek.dcm"
  "${charset_base}/chrH31.dcm"
  "${charset_base}/chrH32.dcm"
  "${charset_base}/chrHbrw.dcm"
  "${charset_base}/chrI2.dcm"
  "${charset_base}/chrJapMulti.dcm"
  "${charset_base}/chrJapMultiExplicitIR6.dcm"
  "${charset_base}/chrKoreanMulti.dcm"
  "${charset_base}/chrRuss.dcm"
  "${charset_base}/chrSQEncoding.dcm"
  "${charset_base}/chrSQEncoding1.dcm"
  "${charset_base}/chrX1.dcm"
  "${charset_base}/chrX2.dcm"
)
declare -a charset_hashes=(
  7020ecdbb68bdd13264daeb29fabe636a28d79a1a83ecbb65a0d996c68458c1e
  8363f3d2e55b448a688ed7863675bf9645e77a919486ddfb8faa12227e28b097
  ea01c20111b638327d6d9c003631a0396167267bfc69c53815f055c76e2dafc1
  49a285554a4ef62ae97c31c1c8c15e9fc3289570a0f56f446e6a652c799dae5d
  cdbdf7820642c13c26b49e496d571f390418d3078e75b2c070e5c668861ed49b
  37b1165fc2b35cbe12f0b036a439d1c69412adb34ce5a387d23191fc2d285f48
  de42af715ac11d701d493ac34b1cf477d3e1f2d5d05e3441738a68d495f7708f
  5065bb5c8e558ecc85cc48113a1b0bf07701eaf2e9fa1b7b733669c7ad8ef9c4
  1d2ed1aa27c01ca85ed2482d6ffe97249d3f276661fa65b99c1b1103b78aa0cc
  9f69e68ba12651b28d0f76047b0f77628e3054cc16461746ad3622fa8bd4a945
  bd45c304b43e1f894942c30239f463156e0e63986fb1dad4c2625b59aa32e01f
  f07d6193310e593fd7f8ecf3e301da01c82c47dc22973896f2a671c69d7fdd4d
  e82d8856b7d9fb407a80a2824dc7adf8daf7ced265450d698955f754a9af1730
  b124a74bcf2f258ee8c99c354208eb7ceb3969e7f2e827d9dd6e41905facaa7e
  1ee6189b45e1610731762b7a823f3ce329b70f9cda9aad1936972966a8aac661
  133232a666587ee884804cb07aaa4becf36f720bc5919a0781732ce691f5dedc
  c626f310e03012138456589f5167547c5bfecf7adef13bb1ed43a21dcb274ee8
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
