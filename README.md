# godicom

*godicom* is a Go package for working with [DICOM](https://www.dicomstandard.org/) files.
It lets you read, modify and write DICOM datasets with an idiomatic Go API.

*godicom* is a general-purpose DICOM framework concerned with reading and writing
DICOM datasets, pixel data, and the DICOM JSON Model. It does not handle DICOM
networking. For DIMSE and DICOMweb (WADO-RS / QIDO-RS / STOW-RS), use
[gonetdicom](https://github.com/godicom-dev/gonetdicom), which builds on *godicom*.

[![CI](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/godicom-dev/godicom/branch/main/graph/badge.svg)](https://codecov.io/gh/godicom-dev/godicom)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.26-%23007d9c)
[![GoDoc](https://pkg.go.dev/badge/github.com/godicom-dev/godicom)](https://pkg.go.dev/github.com/godicom-dev/godicom)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Installation

```bash
go get github.com/godicom-dev/godicom@latest
```

Clone with the optional reference submodule (test fixtures):

```bash
git clone --recurse-submodules https://github.com/godicom-dev/godicom.git
```

## Quick start

```go
package main

import (
	"fmt"
	"log"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

func main() {
	ds, err := godicom.ReadFile("ct.dcm", nil)
	if err != nil {
		log.Fatal(err)
	}

	name, _ := ds.GetString(tag.PatientName)
	id, _ := ds.GetString(tag.PatientID)
	fmt.Println(name, id)

	ds.Set(godicom.NewDataElement(tag.PatientID, godicom.VRLO, "12345678"))
	ds.Set(godicom.NewDataElement(tag.SOPInstanceUID, godicom.VRUI, string(uid.MustGenerateUID())))
	if err := ds.SaveAs("ct_updated.dcm", nil); err != nil {
		log.Fatal(err)
	}
}
```

File I/O: `ReadFile` / `Read` / `ReadBytes` / `WriteFile` / `FileDataset.SaveAs`.

`Read` accepts any `io.Reader`. Prefer `*os.File` / seekable sources — the parser walks tags without `ReadAll`, so `StopBeforePixels`, `DeferSize`, and `SpecificTags` can skip large values without buffering them. Deferred values reload by reopening the file path.

Elements are accessed with typed getters and constants from the [`tag`](https://pkg.go.dev/github.com/godicom-dev/godicom/tag) package
(`GetString`, `GetInt`, `GetFloat`, `GetBytes`, `GetSequence`, …), not dynamic attribute names.

## Pixel Data

Compressed and uncompressed *Pixel Data* can be read as raw bytes or decoded frames:

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

With `WithRaw(false)` (the default), decoded frames are normalized for display
(for example YBR→RGB and planar configuration). Modality / VOI LUT helpers are
available separately and are **not** applied automatically by `PixelBytes`:

```go
samples, err := ds.PixelSamples(pixels.WithRaw(true))
hu, err := ds.ApplyModalityLUT(samples)
win, err := ds.ApplyVOILUT(hu, 0, true)
```

### Decompressing Pixel Data

| Format | Package |
|--------|---------|
| JPEG / JPEG-LS | [golibjpeg](https://github.com/godicom-dev/golibjpeg) |
| JPEG 2000 / HTJ2K | [goopenjpeg](https://github.com/godicom-dev/goopenjpeg) |
| RLE Lossless | [gorle](https://github.com/godicom-dev/gorle) |

These are pulled in automatically as module dependencies. Native (uncompressed)
and Deflated transfer syntaxes need no extra plugins.

### Compressing Pixel Data

```go
import "github.com/godicom-dev/godicom/uid"

err := ds.CompressPixelData(string(uid.RLELossless))
err = ds.CompressPixelData(string(uid.JPEG2000Lossless))
err = ds.CompressPixelData(string(uid.JPEG2000)) // lossy JPEG 2000
```

Supported encode paths today: native, RLE Lossless, Deflated, and JPEG 2000
(lossless / lossy). JPEG and JPEG-LS **encode** are not available yet (upstream
decoders only).

## Dataset bytes (no File Meta)

Encode or decode a dataset without a Part 10 preamble — useful for DIMSE or
multipart payloads:

```go
data, err := ds.Encode(string(uid.ExplicitVRLittleEndian))
parsed, err := godicom.DecodeDataset(data)
```

Part 10 files in memory:

```go
bytes, err := ds.EncodeFile(nil)
ds2, err := godicom.ReadBytes(bytes)
```

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
| JPEG Baseline / Extended / Lossless | ✅ | — |
| JPEG-LS | ✅ | — |
| JPEG 2000 / HTJ2K | ✅ | ✅ (JPEG 2000) |

## Documentation

- [pkg.go.dev API reference](https://pkg.go.dev/github.com/godicom-dev/godicom)
- [CHANGELOG](CHANGELOG.md)
- [TODO](TODO.md) — deferred items and known gaps
- [PARITY](PARITY.md) — coverage map vs pydicom (not full parity)

## License

MIT — see [LICENSE](LICENSE).
