# godicom ↔ pydicom parity audit

Snapshot of **main** against the `pydicom/` submodule. The latest tag is
**v0.34.0**, and it matches main.

This is a **coverage map**, not a claim of full parity.

| Scale | Count |
|-------|------:|
| pydicom `def test_` | 2334 |
| godicom `Test*` funcs | 619 |

## Verdict

**Main production path is solid enough** for Part 10 read/write, Dataset
access, common pixel decode, pixel encode (native, RLE, Deflated, JPEG,
JPEG-LS, JPEG 2000, HTJ2K), DICOM JSON, and DIMSE/DICOMweb payload bytes
(what [gonetdicom](https://github.com/godicom-dev/gonetdicom) needs).

**It is not full pydicom parity.** Whole domains are missing, and overlapping
areas are thinner on edge fixtures.

## Domain map

| Domain | Status | pydicom | godicom | Note |
|--------|--------|---------|---------|------|
| Tag / standard dictionary | solid | tag, datadict | `tag/` + generated dict | Keyword constants + lookup |
| Private dictionary | solid | `_private_dict` | `Dictionary`, `NewPrivateDictionary`, `PrivateBlock` | Per-read dictionary; blocks reserve a creator |
| UID dictionary | solid | uid, `_uid_dict` | `uid/` | 494 generated constants |
| GenerateUID / register TS | **partial** | `generate_uid`, `register_transfer_syntax` | `uid.GenerateUID` (+ options) | `RegisterTransferSyntax` still deferred |
| VR / valuerep | partial | valuerep (~117) | DA/TM/DT/DS/IS/PN | Not full valuerep surface |
| Values conversion | partial | values (~28) | `values.go` | Main paths; thinner edges |
| DataElement | partial | dataelem (~123) | `element.go` + `RawDataElement` | Diagnostics via `OnDiagnostic`, not a mode enum |
| Dataset / FileDataset | partial | dataset (~205) | Get*/Set*/Walk/Clone/… | Typed setters; no dynamic attrs (by design) |
| Sequence / MultiValue | solid | sequence, multival | present | Core ops |
| Charset / Unicode | partial | charset (~34) | `charset.go` | Major sets OK; not full `chr*` |
| File reader | solid | filereader (~110) | `Read` / `ReadFile` / `ReadBytes` | Seekable skip; deferred values use `io.ReaderAt`; `OnElement` keep / skip / stop; no string `DeferSize` |
| File writer | partial | filewriter (~168) | `WriteFile` / `SaveAs` | Key roundtrips; not full suite |
| Dataset / Part 10 bytes API | solid | (app helpers) | Encode/Decode, EncodeFile/ReadBytes | Network payload path |
| Encapsulation | partial | encaps (~164) | `encaps/` | BOT/EOT/frames; thinner suite |
| Pixel decode | partial | pixels+handlers (~900) | native/RLE/JPEG/JLS/J2K/HTJ2K | Common TS; fixture depth << |
| Pixel encode | partial | encoders | native/RLE/Deflated/JPEG/JLS/J2K/HTJ2K | |
| Pixel processing | partial | processing.py | Modality/VOI/YBR/planar, `PixelArray`, `DisplayFrame`, `ApplyColorLUT` | Explicit; not auto in PixelBytes. Supplemental and enhanced palettes unsupported |
| DICOM JSON | solid | jsonrep (~30) | `dicomjson/` | Main Model + BulkDataURI |
| File-set / DICOMDIR | **partial** | fileset (~124) | `OpenFileSet`, record tree, `Find` | Write, add/remove, copy, orphan re-inclusion |
| SR / codes | **gap** | sr/ | — | Not implemented |
| Overlays | **partial** | overlays/ | `OverlayArray` | 1-bit `(60xx,3000)`; not bits embedded in pixel data |
| Waveforms | **gap** | waveforms/ | — | Tag constants only |
| Config / hooks | **partial** | config, hooks | slog + `OnDiagnostic` | No global validation-mode enum |
| CLI | partial | cli (~17) | show/read/readcopy | Minimal |
| Networking | out of scope | → pynetdicom | → gonetdicom | Separate library |

## Transfer syntax

| | Read | Write / pixel encode |
|--|------|----------------------|
| Native Explicit/Implicit LE/BE | yes | yes (dataset) |
| Deflated Explicit VR LE | yes | yes |
| RLE Lossless | yes | yes |
| JPEG Baseline / Extended / Lossless | yes | yes |
| JPEG-LS | yes | yes |
| JPEG 2000 | yes | yes |
| HTJ2K | yes | yes (.201–.203) |

## Suggested next cuts (only with a consumer)

| Priority | Item | Why |
|----------|------|-----|
| P2 | Thicker writer/charset fixtures | Confidence, not new APIs |
| P3 | DICOMDIR write / SR / waveform | Read-only File-set and overlay planes are in |
| done | `uid.GenerateUID` | SCU / store paths invent SOP Instance UIDs |
| done | `Read` streaming / seekable skip | Large studies without full RAM |
| done | `ReadOptions.OnElement` | Per-element keep / skip / stop, since v0.31.0 |

## Method

- Inventory: `pydicom/src/pydicom` modules. The parenthetical pydicom counts in the domain map are the 2026-07-18 pass and were not re-counted. `pydicom/tests` `def test_` was re-counted on 2026-10-20 (2334).
- Probe: godicom non-test `.go` for APIs / packages, `Test*` funcs (619), and `uid/generated.go` constants (494) on 2026-10-20. The old "~698 passed" figure was a `go test` run at v0.23.0 and is not repeated here.
- Tag-name hits (`OverlayData`, `WaveformData`, `CodeValue`) are **not**
  treated as implementations.

See also [TODO.md](TODO.md) and [README.md](README.md).
