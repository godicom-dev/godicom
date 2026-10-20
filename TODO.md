# godicom TODO

Working notes for *godicom*. The public overview lives in [README.md](README.md);
this file tracks deferred work and known gaps.

## Status

*godicom* reads, modifies and writes DICOM datasets, pixel data, and DICOM JSON.
As of **v0.30.0** the released surface covers Part 10 and dataset encode/decode,
transfer syntaxes through HTJ2K, pixel decode and encode (native, RLE, Deflated,
JPEG, JPEG-LS, JPEG 2000, HTJ2K), `PixelArray` / `DisplayFrame`, `dicomjson`,
`uid.GenerateUID`, typed getters and setters, `Dictionary` with a per-read
private dictionary, `PrivateBlock`, read/write diagnostic hooks, and streaming
`Read`. `main` also has `ReadOptions.OnElement`; that commit is not tagged yet.

Networking (DIMSE / DICOMweb) is **out of scope** — see
[gonetdicom](https://github.com/godicom-dev/gonetdicom).

## Deferred (need a concrete use case)

| Item | Today | Trigger |
|------|-------|---------|
| String-form `DeferSize` (`"2 kB"`) | `ReadOptions.DeferSize uint32` only | API parity for string sizes |
| `RegisterTransferSyntax` | Not implemented | Private transfer syntaxes at runtime |
| File-set / DICOMDIR write | Read-only tree and File ID lookup (`OpenFileSet`) | Media interchange that must write a DICOMDIR |
| SR / codes / overlays / waveforms | Not implemented | Domain-specific tooling |

Do not start these without a real consumer. Prefer fixing gaps that block
gonetdicom or an application over expanding the matrix for its own sake.

## Known gaps (lower priority)

- Broader fixture / edge-case coverage vs. the full pydicom test suite
- CLI parity beyond `show` / `read` / `readcopy`

## Out of scope

| Concern | Where |
|---------|-------|
| DIMSE (C-ECHO, C-STORE, C-FIND, C-MOVE, C-GET, DIMSE-N) | [gonetdicom](https://github.com/godicom-dev/gonetdicom) |
| DICOMweb (WADO-RS / QIDO-RS / STOW-RS) | gonetdicom |
| HTTP servers / PACS integration | gonetdicom |

## Engineering notes

- Prefer Go-idiomatic APIs: typed getters + [`tag`](./tag) constants, not dynamic attribute names.
- Keep dictionaries generated from authoritative sources (`generate_*.py`); do not hand-edit generated tables.
- When behaviour is ambiguous, validate with golden bytes / value-level roundtrips, not “it opens without error”.
- If godicom is blocking a consumer (especially gonetdicom), fix godicom first.

## See also

- [README.md](README.md)
- [CHANGELOG.md](CHANGELOG.md)
- [PARITY.md](PARITY.md) — domain-level coverage vs pydicom (not full parity)
