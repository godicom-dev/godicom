# godicom ↔ pydicom parity audit

Snapshot for **godicom v0.23.0** against the `pydicom/` submodule.

This is a **coverage map**, not a claim of full parity.

| Scale | Count |
|-------|------:|
| pydicom `def test_` (approx.) | ~2334 |
| godicom `Test*` funcs | ~460 |
| godicom tests including subtests | ~698 passed |

## Verdict

**Main production path is solid enough** for Part 10 read/write, Dataset
access, common pixel decode, RLE/Deflated/JPEG 2000 encode, DICOM JSON, and
DIMSE/DICOMweb payload bytes (what [gonetdicom](https://github.com/godicom-dev/gonetdicom) needs).

**It is not full pydicom parity.** Whole domains are missing, and overlapping
areas are thinner on edge fixtures.

## Domain map

| Domain | Status | pydicom | godicom | Note |
|--------|--------|---------|---------|------|
| Tag / standard dictionary | solid | tag, datadict | `tag/` + generated dict | Keyword constants + lookup |
| Private dictionary | solid | `_private_dict` | `private_dictionary_*` | Creators + runtime extend |
| UID dictionary | solid | uid, `_uid_dict` | `uid/` | ~490 entries |
| GenerateUID / register TS | **partial** | `generate_uid`, `register_transfer_syntax` | `uid.GenerateUID` (+ options) | `RegisterTransferSyntax` still deferred |
| VR / valuerep | partial | valuerep (~117) | DA/TM/DT/DS/IS/PN | Not full valuerep surface |
| Values conversion | partial | values (~28) | `values.go` | Main paths; thinner edges |
| DataElement | partial | dataelem (~123) | `element.go` + `RawValue` | Validation modes thinner |
| Dataset / FileDataset | partial | dataset (~205) | Get*/Set/Walk/Clone/… | No dynamic attrs (by design) |
| Sequence / MultiValue | solid | sequence, multival | present | Core ops |
| Charset / Unicode | partial | charset (~34) | `charset.go` | Major sets OK; not full `chr*` |
| File reader | partial | filereader (~110) | `ReadFile` + options | No streaming `ReadPartial` |
| File writer | partial | filewriter (~168) | `WriteFile` / `SaveAs` | Key roundtrips; not full suite |
| Dataset / Part 10 bytes API | solid | (app helpers) | Encode/Decode, EncodeFile/ReadBytes | Network payload path |
| Encapsulation | partial | encaps (~164) | `encaps/` | BOT/EOT/frames; thinner suite |
| Pixel decode | partial | pixels+handlers (~900) | native/RLE/JPEG/JLS/J2K/HTJ2K | Common TS; fixture depth << |
| Pixel encode | partial | encoders | native/RLE/Deflated/J2K | **JPEG/JPEG-LS encode missing** |
| Pixel processing | partial | processing.py | Modality/VOI/YBR/planar | Explicit; not auto in PixelBytes |
| DICOM JSON | solid | jsonrep (~30) | `dicomjson/` | Main Model + BulkDataURI |
| File-set / DICOMDIR | **gap** | fileset (~124) | — | Tag names ≠ API |
| SR / codes | **gap** | sr/ | — | Not implemented |
| Overlays | **gap** | overlays/ | — | Tag constants only |
| Waveforms | **gap** | waveforms/ | — | Tag constants only |
| Config / hooks | **gap** | config, hooks | — | Needs Go API design |
| CLI | partial | cli (~17) | show/read/readcopy | Minimal |
| Networking | out of scope | → pynetdicom | → gonetdicom | Separate library |

## Transfer syntax

| | Read | Write / pixel encode |
|--|------|----------------------|
| Native Explicit/Implicit LE/BE | yes | yes (dataset) |
| Deflated Explicit VR LE | yes | yes |
| RLE Lossless | yes | yes |
| JPEG Baseline / Extended / Lossless | yes | **no** |
| JPEG-LS | yes | **no** |
| JPEG 2000 | yes | yes |
| HTJ2K | yes | — (decode) |

## Suggested next cuts (only with a consumer)

| Priority | Item | Why |
|----------|------|-----|
| P1 | Streaming / `ReadPartial` | Large studies without full RAM |
| P2 | JPEG encode (upstream) | Accept renegotiation / some DICOMweb paths |
| P2 | Thicker writer/charset fixtures | Confidence, not new APIs |
| P3 | DICOMDIR / SR / overlay / waveform | Domain tools only |
| done | `uid.GenerateUID` | SCU / store paths invent SOP Instance UIDs |

## Method

- Inventory: `pydicom/src/pydicom` modules + `pydicom/tests` `def test_` counts.
- Probe: godicom non-test `.go` for APIs / packages (2026-07-18).
- Tag-name hits (`OverlayData`, `WaveformData`, `CodeValue`) are **not**
  treated as implementations.

See also [TODO.md](TODO.md) and [README.md](README.md).
