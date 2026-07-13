# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.13.0] - 2026-07-13

### Added
- **Charset**: pydicom-aligned write tests — changed SpecificCharacterSet re-encode, WritePN/WriteText multi-charset, Japanese ISO-2022 PN encode

### Fixed
- **Charset / File Writer**: changing `SpecificCharacterSet` forces text/PN re-encode from decoded Unicode (no longer writes stale `RawValue`)
- **Charset**: `EncodeString` rejects non-ASCII for default/ISO_IR 6 so ISO-2022 extensions are used
- **Charset**: multi-charset encode splits on `^`/`=`/`\` so escapes survive delimiter repertoire resets

### Removed
- Known limitation: Japanese ISO-2022 multi-byte encode (now covered for PN roundtrip)

**Tests**: 633 passed (v0.12.0: 623)

## [0.12.0] - 2026-07-13

### Added
- **File Writer**: implicit VR byte-layout tests for OD/OL/UC/UR/UN; UC multi-value explicit cases; file meta unchanged on non-enforced save
- **Reader / File Writer**: private-tag VR resolution from private dictionary during implicit reads; implicit↔explicit conversion keeps private creator/data VRs

### Fixed
- **Reader**: private creator tags resolve to LO; private data tags use creator + private dictionary instead of always UN on implicit VR read

**Tests**: 623 passed (v0.11.0: 615)

## [0.11.0] - 2026-07-13

### Added
- **File Writer**: pydicom-aligned encoding tests — EnforceFileFormat file-meta fill/SOP sync, private/invalid transfer syntax, mismatch checks, command-set reject, unknown VR, implicit↔explicit VR conversion

### Fixed
- **File Writer**: `determineWriteEncoding` rejects non-transfer-syntax UIDs, requires encoding args for private TS, and checks ImplicitVR/LittleEndian mismatches independently
- **File Writer**: reject Command Set group `(0000,eeee)` elements on file write; reject unknown VRs when encoding elements

**Tests**: 615 passed (v0.10.0: 603)

## [0.10.0] - 2026-07-10

### Added
- **File Writer**: reject group 2 elements in dataset; implicit VR + big endian validation; explicit/implicit SQ ambiguous VR via tag-number access
- **File Writer**: `writeFileMetaInfo` validates non-group-2 elements; raw ambiguous element conversion tests

### Fixed
- **File Writer**: ambiguous US/SS/LUTData correction converts `RawValue`-only payloads; non-ambiguous overlay OB no longer forced to OW

**Tests**: 603 passed (v0.9.0: 591)

## [0.9.0] - 2026-07-10

### Added
- **File Writer**: non-standard file-meta-only write (no preamble), preamble+file-meta roundtrip, dataset unchanged on save
- **File Writer**: `writeFileMetaInfo` non-standard tests (missing TransferSyntaxUID, progressive missing elements)
- **File Writer**: byte-identical roundtrip for `chrH31.dcm` and `chrFrenMulti.dcm`

**Tests**: 591 passed (v0.8.0: 583)

## [0.8.0] - 2026-07-10

### Added
- **File Writer**: pydicom-aligned ambiguous VR tests for LUTDescriptor VM3, LUTData, PixelData, and implicit sequence pixel-representation nearest lookup
- **File Writer**: non-standard write paths without enforced file format (no preamble, empty file meta, dataset-only)

### Fixed
- **File Writer**: `lutDescriptorFirstValue` handles `MultiValue[uint64]` / `MultiValue[int64]` so LUTData resolves to US when descriptor first value is 1

**Tests**: 583 passed (v0.7.0: 571)

## [0.7.0] - 2026-07-06

### Added
- **Charset**: fetch all 17 pydicom `FILE_PATIENT_NAMES` charset fixtures via `scripts/fetch-testdata.sh`
- **Charset**: read, write roundtrip, and byte-identical tests for Arabic, German, Hebrew, Korean (chrI2), GB18030 (chrX2), Japanese multi-charset PN (chrH31/H32), and related fixtures
- **Charset**: `chrFrenMulti` multi-valued PN/LO tests; `chrSQEncoding1` sequence charset inheritance test

### Fixed
- **Charset**: ISO-2022 IR 149 (Korean) decode strips `\x1b$)C` escapes before EUC-KR payload decode

### Known limitations
- Japanese ISO-2022 multi-byte **encode** not fully covered (read works, e.g. `chrSQEncoding.dcm`)
- `chrJapMulti` / `chrKoreanMulti` write roundtrip is value-identical but not byte-identical (SpecificCharacterSet padding differs)

**Tests**: 571 passed (v0.6.0: 531)

## [0.6.0] - 2026-07-04

### Added
- **File Writer**: byte-identical roundtrip tests for undefined-length sequences, private nested SQ, `MR_small_implicit.dcm`, and additional pydicom fixtures
- **File Writer**: implicit nested ambiguous VR via sequence owner/parent ancestor chain; pydicom-aligned P2 tests (index/nearest access, parent change, `(FFFF,FFFF)`, no-preamble/prefix)
- **File Writer**: `CorrectAmbiguousVRPreservingRaw` for implicit writes with in-memory values while keeping raw byte roundtrips
- **Reader**: undefined-length UN values parsed as sequences (PS3.5 6.2.2); raw byte preservation for other undefined-length elements
- **Reader**: `Force` read of small files without preamble; propagate transfer syntax encoding into nested sequence items
- Transfer syntax encoding from file meta; TS conversion roundtrip tests (BE↔LE); lazy ambiguous VR correction on `Get` for file-read elements
- `UN_sequence.dcm` semantic read test; overlay/waveform ambiguous VR tests; file meta validation/group length tests

### Fixed
- **File Writer**: nested sequences with repeated tags no longer skip items due to incorrect cycle detection
- **File Writer**: re-encode from typed values when output transfer syntax differs from read encoding (instead of writing stale RawValue bytes)
- **File Writer**: `encodeInts` handles `[]int` / `[]int64` (fixes empty LUTDescriptor on implicit write)
- **Ambiguous VR**: `WaveformBitsAllocated` tag lookup (was wrongly using `WaveformBitsStored`)

**Tests**: 531 passed (v0.5.0: 503)

## [0.5.0] - 2026-07-04

### Added
- **Charset**: ISO-2022 decode on read and encode on write for PN / LO / LT / SH / ST / UT / UC
- **Charset**: multi-charset code extensions, SQ item charset inheritance
- **Charset**: integration tests on pydicom `charset_files` (Latin, Greek, Russian, UTF-8, Japanese SQ)
- **Charset**: UTF-8 (`ISO_IR 192`) write/read roundtrip (`chrX1.dcm`)
- **PersonName**: component helpers (`FamilyName`, `GivenName`, `Formatted`, …) and `Dataset.GetPN`
- **Encaps**: `CountFragments` and expanded unit / real-DICOM integration tests
- **Docs**: `CHANGELOG.md`

### Changed
- Nested sequence ambiguous VR handling and `convertInts` odd-length validation

### Known limitations
- Japanese ISO-2022 multi-byte **encode** not fully covered (read works, e.g. `chrSQEncoding.dcm`)
- Broader `chr*.dcm` matrix not exhaustive

**Tests**: 503 passed (v0.4.0: 429)

## [0.4.0] - 2026-07-03

### Added
- **Value Representation**: `DA` / `TM` / `DT` types and `GetDA` / `GetTM` / `GetDT`
- **Value Representation**: `DS` / `IS` typed values and `GetDS` / `GetIS` (with `Original` string preserved)
- **CLI**: `godicom show` / `read` with file meta + dataset display
- **CLI**: `-t` / `--top` tag filter and `--no-meta` flag
- **File Writer**: pydicom `test_filewriter.py` subset — empty AT/LO, DA/TM/DT single/multi-value, empty sequence
- **File Writer**: ambiguous VR resolution on explicit write; nested SQ ambiguous VR tests
- **Reader**: nested SQ ambiguous VR read fixes

### Fixed
- `convertInts` rejects odd-length byte values

**Tests**: 429 passed (v0.3.1: 389)

## [0.3.1] - 2026-07-02

### Fixed
- Metadata roundtrip gaps from v0.3.0 follow-up

## [0.3.0] - 2026-07-02

### Added
- File writer metadata roundtrip improvements
- `Dataset.Walk` and `Dataset.Clone`

## [0.2.0] - 2026-07-02

### Added
- Pixel read path: `PixelBytes` / `PixelFrames`, encaps frame splitting
- JPEG / JPEG-LS / JPEG 2000 / RLE decode via `golibjpeg`, `goopenjpeg`, `gorle`
- DICOM JSON Model (`dicomjson` subpackage)
- Deferred read (`DeferSize`), encapsulated pixel data read
- Private dictionary query API and code-generated UID dictionary

## [0.1.0] - 2026-07-02

### Added
- Initial release: DICOM file read/write, tag dictionary, basic VR conversion
- pydicom test file compatibility for core read paths

[Unreleased]: https://github.com/godicom-dev/godicom/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/godicom-dev/godicom/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/godicom-dev/godicom/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/godicom-dev/godicom/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/godicom-dev/godicom/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/godicom-dev/godicom/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/godicom-dev/godicom/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/godicom-dev/godicom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/godicom-dev/godicom/releases/tag/v0.1.0
