# godicom TODO

Working notes for *godicom*. The public overview lives in [README.md](README.md);
this file tracks deferred work and known gaps.

## Status

*godicom* reads, modifies and writes DICOM datasets, pixel data, and DICOM JSON.
Core I/O, transfer-syntax coverage, pixel decode/encode (except JPEG / JPEG-LS
encode), display helpers, and `dicomjson` are in place as of **v0.23.0**.

Networking (DIMSE / DICOMweb) is **out of scope** — see
[gonetdicom](https://github.com/godicom-dev/gonetdicom).

## Deferred (need a concrete use case)

| Item | Today | Trigger |
|------|-------|---------|
| Streaming / partial read (`io.Reader`) | `ReadFile` loads the whole file; deferred load is buffer-backed | Network streams, pipes, or files larger than memory |
| String-form `DeferSize` (`"2 kB"`) | `ReadOptions.DeferSize uint32` only | API parity for string sizes |
| `GenerateUID` | Not implemented | Runtime UID generation |
| `RegisterTransferSyntax` | Not implemented | Private transfer syntaxes at runtime |
| JPEG / JPEG-LS **encode** | Decode only (`golibjpeg`) | Upstream encoder, or Accept renegotiation that needs it |
| File-set / DICOMDIR | Not implemented | Media interchange / DICOMDIR consumers |
| SR / codes / overlays / waveforms | Not implemented | Domain-specific tooling |

Do not start these without a real consumer. Prefer fixing gaps that block
gonetdicom or an application over expanding the matrix for its own sake.

## Known gaps (lower priority)

- Broader fixture / edge-case coverage vs. the full pydicom test suite
- Config / hook-style extension points (needs an explicit Go API design first)
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
