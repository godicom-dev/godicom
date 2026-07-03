# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/godicom-dev/godicom/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/godicom-dev/godicom/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/godicom-dev/godicom/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/godicom-dev/godicom/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/godicom-dev/godicom/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/godicom-dev/godicom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/godicom-dev/godicom/releases/tag/v0.1.0
