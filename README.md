[![CI](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/godicom-dev/godicom/branch/main/graph/badge.svg)](https://codecov.io/gh/godicom-dev/godicom)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.26-%23007d9c)](https://go.dev/)
[![GoDoc](https://pkg.go.dev/badge/github.com/godicom-dev/godicom)](https://pkg.go.dev/github.com/godicom-dev/godicom)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

# *godicom*

*godicom* is a Go package for working with [DICOM](https://www.dicomstandard.org/) files.
It lets you read, modify and write DICOM data with an idiomatic Go API.

*godicom* is a general-purpose DICOM framework concerned with reading and writing
DICOM datasets. In order to keep the project manageable, it does not handle the
specifics of individual SOP classes or DICOM networking. Other libraries in the
[godicom-dev](https://github.com/godicom-dev) organisation build on *godicom*
for those areas — notably [gonetdicom](https://github.com/godicom-dev/gonetdicom)
for DIMSE and DICOMweb (WADO-RS / QIDO-RS / STOW-RS).

## Installation

```bash
go get github.com/godicom-dev/godicom@latest
```

Clone with the optional reference submodule (test fixtures):

```bash
git clone --recurse-submodules https://github.com/godicom-dev/godicom.git
```

## Documentation

The [pkg.go.dev API reference](https://pkg.go.dev/github.com/godicom-dev/godicom),
[CHANGELOG](CHANGELOG.md), and [PARITY](PARITY.md) coverage map vs pydicom are
the primary docs. Deferred items live in [TODO](TODO.md).

## Examples

**Change a patient's ID**

```go
package main

import (
	"log"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
)

func main() {
	ds, err := godicom.ReadFile("ct.dcm", nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := ds.SetString(tag.PatientID, "12345678"); err != nil {
		log.Fatal(err)
	}
	if err := ds.SaveAs("ct_updated.dcm", nil); err != nil {
		log.Fatal(err)
	}
}
```

Elements are accessed with typed getters and constants from the
[`tag`](https://pkg.go.dev/github.com/godicom-dev/godicom/tag) package
(`GetString`, `GetInt`, `GetFloat`, `GetBytes`, `GetSequence`, …), and written
with the matching setters (`SetString`, `SetInt`, `SetFloats`, `SetSequence`, …),
which take the VR from the data dictionary and reject values the tag's VR cannot
hold. For private tags and other tags outside the dictionary, supply the VR
yourself with `Set(godicom.NewDataElement(tag, vr, value))`.

File I/O entry points: `ReadFile` / `Read` / `ReadBytes` / `WriteFile` /
`FileDataset.SaveAs`. `Read` accepts any `io.Reader`; prefer `*os.File` /
seekable sources so `StopBeforePixels`, `DeferSize`, and `SpecificTags` can
skip large values without buffering them.

Context-aware variants (`ReadFileContext`, `WriteContext`,
`DecodeDatasetContext`, …) accept a `context.Context` for cancellation and
structured logging.

**Truncated and malformed files**

By default a read keeps whatever it parsed before the file stopped making sense,
which is what most DICOM tooling does but hides the damage. Set
`ReadOptions.OnDiagnostic` to see those anomalies — a value shorter than its
length field, a header cut off mid-element, a sequence whose declared length
runs past the end of the file, a deferred value whose source has gone away —
each reported with its tag, VR, byte offset, and enclosing sequences:

```go
ds, err := godicom.ReadFile("truncated.dcm", &godicom.ReadOptions{
	OnDiagnostic: func(d godicom.Diagnostic) error {
		log.Printf("%s", d) // keep parsing
		return nil
	},
})
```

`Diagnostic` is itself an `error`, so returning it rejects the file instead:

```go
opts := &godicom.ReadOptions{
	OnDiagnostic: func(d godicom.Diagnostic) error { return d },
}
```

**Dataset bytes (no File Meta)**

Encode or decode a dataset without a Part 10 preamble — useful for DIMSE or
multipart payloads:

```go
data, err := ds.Encode(uid.ExplicitVRLittleEndian)
parsed, err := godicom.DecodeDataset(data, uid.ExplicitVRLittleEndian)
```

Part 10 files in memory:

```go
bytes, err := ds.EncodeFile(nil)
ds2, err := godicom.ReadBytes(bytes, nil)
```

## *Pixel Data*

Compressed and uncompressed *Pixel Data* can be read as raw bytes or decoded
frames:

```go
import "github.com/godicom-dev/godicom/pixels"

ds, err := godicom.ReadFile("mr_j2k.dcm", nil)
if err != nil {
	log.Fatal(err)
}

// All frames concatenated (native layout)
raw, err := ds.PixelBytes(pixels.WithRaw(true))

// Or one frame at a time
frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(0))
```

With `WithRaw(false)` (the default), decoded frames are normalised for display
(for example YBR→RGB and planar configuration). Modality / VOI LUT helpers are
available separately and are **not** applied automatically by `PixelBytes`.

For pydicom-style access, use `PixelArray` (decoded samples + shape) and
`DisplayFrame` (8-bit display-ready bytes after modality / VOI / presentation):

```go
arr, err := ds.PixelArray(pixels.WithRaw(true))
frame, err := ds.DisplayFrame(0)
```

### Decompressing *Pixel Data*

| Format | Package |
|--------|---------|
| JPEG / JPEG-LS | [golibjpeg](https://github.com/godicom-dev/golibjpeg) |
| JPEG 2000 / HTJ2K | [goopenjpeg](https://github.com/godicom-dev/goopenjpeg) |
| RLE Lossless | [gorle](https://github.com/godicom-dev/gorle) |

These are pulled in automatically as module dependencies. Native (uncompressed)
and Deflated transfer syntaxes need no extra plugins.

### Compressing *Pixel Data*

```go
err := ds.CompressPixelData(uid.RLELossless)
err = ds.CompressPixelData(uid.JPEGLSLossless)
err = ds.CompressPixelData(uid.JPEG2000Lossless)
err = ds.CompressPixelData(uid.JPEG2000) // lossy JPEG 2000
```

Supported encode paths: native, RLE Lossless, Deflated, JPEG (baseline /
lossless / JPEG-LS), JPEG 2000 (lossless / lossy), and HTJ2K (`.201` lossless
LRCP, `.202` lossless RPCL, `.203` lossy).

## Logging

*godicom* uses Go's `log/slog`. By default it is silent (`DiscardHandler`),
similar in spirit to leaving pydicom's debugger off until you call
`config.debug()`.

```go
import (
	"context"
	"log/slog"
	"os"

	"github.com/godicom-dev/godicom"
)

h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
logger := slog.New(h)

// Per-call (preferred)
ds, err := godicom.ReadFile("ct.dcm", &godicom.ReadOptions{Logger: logger})

// Or via context (request-scoped / shared with gonetdicom)
ctx := godicom.WithLogger(context.Background(), logger)
ds, err = godicom.ReadFileContext(ctx, "ct.dcm", nil)
```

CLI: `godicom show -debug file.dcm`

Debug records use fixed attribute keys aligned with pydicom filereader
diagnostics (`component`, `offset`, `offset_hex`, `hex`, `tag`, `vr`, `len`,
`undefined_length`, `value_hex`, `value`, `transfer_syntax`, …). Messages cover
the same events as pydicom's debugger: FMI/DICM, per-element header + value
preview (first 20 bytes), defer skips, and sequence item boundaries.

## DICOM JSON Model

```go
import "github.com/godicom-dev/godicom/dicomjson"

jsonData, err := dicomjson.MarshalDataset(ds.Dataset)
parsed, err := dicomjson.ParseDataset(jsonData)

arr, err := dicomjson.MarshalDatasets([]*godicom.Dataset{ds1, ds2})
dss, err := dicomjson.ParseDatasets(arr)
```

## CLI

```bash
go install github.com/godicom-dev/godicom/cmd/godicom@latest

godicom show <file>            # print file meta + dataset
godicom show -debug <file>     # also emit reader debug logs to stderr
godicom read <file>            # alias for show
godicom readcopy <src> <dst>   # read, write, re-read
```

## Transfer syntax support

| Transfer Syntax | Read | Write |
|-----------------|------|-------|
| Explicit / Implicit VR Little Endian | ✅ | ✅ |
| Explicit VR Big Endian | ✅ | ✅ |
| Deflated Explicit VR Little Endian | ✅ | ✅ |
| RLE Lossless | ✅ | ✅ |
| JPEG Baseline / Extended / Lossless | ✅ | ✅ |
| JPEG-LS | ✅ | ✅ |
| JPEG 2000 / HTJ2K | ✅ | ✅ |

## Contributing

Bug reports, fixes, and documentation improvements are welcome. Please open an
issue or pull request on GitHub.

## License

MIT — see [LICENSE](LICENSE).
