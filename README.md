# godicom

**Go 实现的 DICOM 核心库** — [pydicom](https://github.com/pydicom/pydicom) 的 Go 移植版，覆盖文件读写、数据集操作、像素编解码与 DICOM JSON。

[![CI](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/godicom-dev/godicom/branch/main/graph/badge.svg)](https://codecov.io/gh/godicom-dev/godicom)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.26-%23007d9c)
[![GoDoc](https://pkg.go.dev/badge/github.com/godicom-dev/godicom)](https://pkg.go.dev/github.com/godicom-dev/godicom)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 定位

godicom 只做 pydicom 核心库的事：**文件 / 数据集 / 像素 / JSON**。

- 行为、测试、边界条件一律对齐 pydicom 源码与测试（`pydicom/` 子模块）。
- 网络层（DIMSE / DICOMweb / WADO-RS / QIDO-RS / STOW-RS）**不在本库**，由上层库 [gonetdicom](https://github.com/godicom-dev/gonetdicom) 提供，依赖 godicom。
- 不移植 pydicom 的 Python 动态特性（`ds.PatientName` 属性、`ds["Keyword"]` 下标）；统一用**显式 getter + `tag` 常量**。

```
gonetdicom ──▶ godicom ──▶ golibjpeg / goopenjpeg / gorle
```

## 安装

```bash
go get github.com/godicom-dev/godicom@latest
```

克隆（含 pydicom 参考子模块）：

```bash
git clone --recurse-submodules https://github.com/godicom-dev/godicom.git
```

## 快速开始

```go
package main

import (
    "fmt"

    "github.com/godicom-dev/godicom"
    "github.com/godicom-dev/godicom/tag"
)

func main() {
    // 读取 DICOM 文件（默认选项传 nil）
    ds, err := godicom.ReadFile("ct.dcm", nil)
    if err != nil {
        panic(err)
    }

    // 用 tag 常量 + 显式 getter 取值
    name, _ := ds.GetString(tag.PatientName)
    id, _   := ds.GetString(tag.PatientID)
    rows, _ := ds.GetInt(tag.Rows)
    fmt.Println(name, id, rows)

    // 修改 / 新增元素
    ds.Set(godicom.NewDataElement(tag.PatientName, godicom.VRPN, "Anonymous"))

    // 写回（Part 10 文件）
    if err := ds.SaveAs("output.dcm", nil); err != nil {
        panic(err)
    }
}
```

文件 I/O 入口：`godicom.ReadFile` / `godicom.WriteFile` / `FileDataset.SaveAs`。

带读取选项：

```go
ds, _ = godicom.ReadFile("ct.dcm", &godicom.ReadOptions{
    Force:        true,   // 无 preamble / 非 Part 10 也读
    SpecificTags: []godicom.Tag{tag.PatientName, tag.PatientID},
    StopBeforePixels: true,
})
```

## 数据集 API

不提供 `ds.PatientName` 这类动态属性。取值统一走 getter，定位用 `tag` 子包常量或根包 `MustTag`：

```go
name, ok := ds.GetString(tag.PatientName)
id,   ok := ds.GetString(tag.PatientID)
rows, ok := ds.GetInt(tag.Rows)
f,    ok := ds.GetFloat(tag.RescaleSlope)
fs,   ok := ds.GetFloats(tag.WindowCenter)
pn,   ok := ds.GetPN(tag.PatientName)
seq,  ok := ds.GetSequence(tag.ReferencedImageSequence)
b,    ok := ds.GetBytes(tag.PixelData)
```

| 类别 | API |
|------|-----|
| 取值 | `GetString` / `GetInt` / `GetFloat` / `GetFloats` / `GetBytes` / `GetSequence` / `GetPN` / `GetDA` / `GetTM` / `GetDT` / `GetDS` / `GetIS`（及 `*Value` 别名） |
| 增删 | `Set` / `Delete` / `Has` / `Pop` / `Clear` / `Update` |
| 遍历 | `Iter` / `IterAll` / `SortedTags` / `Walk` / `GroupDataset` |
| 私有 | `PrivateBlock` / `RemovePrivateTags` |
| 编码状态 | `IsOriginalEncoding` / `SetOriginalEncoding` / `SetWriteEncoding` |
| 展示 | `String` / `Top` / `FormattedLines` |
| 比较 | `Equal` / `Clone` / `ElementByKeyword` |

## 像素解码

封装像素先经 `encaps` 拆帧，再由 `pixels` 按 Transfer Syntax 调度到 [golibjpeg](https://github.com/godicom-dev/golibjpeg)（JPEG）、[goopenjpeg](https://github.com/godicom-dev/goopenjpeg)（JPEG 2000 / HTJ2K）、[gorle](https://github.com/godicom-dev/gorle)（RLE）。

```go
import "github.com/godicom-dev/godicom/pixels"

ds, _ := godicom.ReadFile("mr_j2k.dcm", nil)

// 所有帧拼成一块 native 字节布局
raw, err := ds.PixelBytes(pixels.WithRaw(true))

// 或按帧取
frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(0))

// 或解包成 float64 样本（含 bitsStored / pixelRepresentation 处理）
samples, err := ds.PixelSamples(pixels.WithRaw(true))
```

`WithRaw(true)` 返回编解码库原始字节布局；`WithRaw(false)`（默认）会在解码后做 YBR→RGB、PlanarConfiguration 归一化等显示后处理（对标 pydicom）。

显示管线（对标 pydicom `apply_modality_lut` / `apply_voi_lut`，**不会**在 `PixelBytes` 里自动执行）：

```go
hu, err  := ds.ApplyModalityLUT(samples)       // Rescale 或 Modality LUT
win, err  := ds.ApplyVOILUT(hu, 0, true)        // VOI LUT 或窗宽窗位
shape, err := ds.ApplyPresentationLUTShape(hu) // Presentation LUT Shape
```

底层 `pixels` 包还导出 `ApplyRescale` / `ApplyWindowing` / `ApplyVOI` / `ApplyVOILUT` / `ApplyModalityLUT` / `InvertValues` / `UnpackSamples` / `ConvertColorSpace` / `ExpandYBR422` / `PlanarToColorByPixel` / `ColorByPixelToPlanar`。

## 像素编码 / 压缩

```go
// 重新编码 Pixel Data 并更新 Transfer Syntax（原地修改数据集）
err := ds.CompressPixelData(string(uid.RLELossless))
err  = ds.CompressPixelData(string(uid.JPEG2000Lossless))
err  = ds.CompressPixelData(string(uid.JPEG2000)) // 默认 lossy
```

`CompressPixelData` 支持：native、RLE Lossless、Deflated、**JPEG 2000（lossless / lossy）**。

> JPEG / JPEG-LS **encode 暂不可用** —— `golibjpeg` / pylibjpeg-libjpeg 上游没有编码器；解码路径完整。详见 [TODO.md](TODO.md)。

底层封装写入（对标 pydicom encaps write path）：`encaps.Encapsulate` / `EncapsulateExtended` / `FragmentFrame` / `ItemizeFragment`，`pixels.EncodeFrame` / `EncodeFrames`，以及 `FileDataset.SetEncodedPixelData`。

## 数据集编解码（DIMSE / DICOMweb 载荷）

不带 preamble / File Meta 的数据集字节，供上层网络库拼 C-STORE / C-FIND 载荷或处理 DICOMweb multipart body：

```go
// 编码（可指定 Transfer Syntax；支持 Deflated）
data, err := ds.Encode(string(uid.ExplicitVRLittleEndian))
data, err  = ds.EncodeEncoding(false, true) // implicit VR, little endian

// 解码
parsed, err := godicom.DecodeDataset(data)
parsed, err  = godicom.DecodeDatasetEncoding(data, false, true)
```

Part 10 文件级内存编解码（DICOMweb STOW 上传 / WADO 下载）：

```go
bytes, err := ds.EncodeFile(nil)          // FileDataset → Part 10 字节
ds2, err    := godicom.ReadBytes(bytes)    // Part 10 字节 → FileDataset
err         = ds.Write(w, nil)             // 流式写
```

## DICOM JSON Model

```go
import "github.com/godicom-dev/godicom/dicomjson"

jsonData, err := dicomjson.MarshalDataset(ds.Dataset)
parsed, err     := dicomjson.ParseDataset(jsonData)

// QIDO-RS / WADO-RS 元数据数组
arr, err := dicomjson.MarshalDatasets([]*godicom.Dataset{ds1, ds2})
dss, err := dicomjson.ParseDatasets(arr)
```

对齐 pydicom `test_json.py` 主路径：全 VR roundtrip、`CT_small` 完整往返、PN/AT/数值/空值/BulkDataURI/UN InlineBinary、fixture roundtrip。

## CLI

```bash
go install github.com/godicom-dev/godicom/cmd/godicom@latest

godicom show <file>            # 打印 file meta + dataset
godicom read <file>            # show 别名
godicom readcopy <src> <dst>   # 读 → 写 → 回读校验
```

`show` 支持 `-t <tag>` 过滤、`--top` 只看顶层。

## 支持的 Transfer Syntax

| 类别 | 读 | 写 |
|------|----|----|
| Explicit VR Little Endian | ✅ | ✅ |
| Implicit VR Little Endian | ✅ | ✅ |
| Explicit VR Big Endian | ✅ | ✅ |
| Deflated Explicit VR Little Endian | ✅ | ✅ |
| RLE Lossless | ✅ | ✅ |
| JPEG Baseline / Extended | ✅ | — |
| JPEG Lossless / Lossless SV1 | ✅ | — |
| JPEG-LS Lossless / Near-Lossless | ✅ | — |
| JPEG 2000 / HTJ2K | ✅ | ✅（JPEG 2000） |

## 功能矩阵

| 功能 | 状态 |
|------|------|
| Explicit/Implicit VR、Little/Big Endian、Deflated 读写 | ✅ |
| 混合编码自动切换 | ✅ |
| File Meta 解析 / 同步 | ✅ |
| 序列 (SQ) / 嵌套私有 Tag | ✅ |
| `ReadOptions.SpecificTags` / `Force` / `StopBeforePixels` / `DeferSize` | ✅ |
| 私有字典 VR 解析（implicit 读）+ 运行时扩展 | ✅ |
| 基础 VR 值转换（string / PN / UI / AT / int / float / DS / IS …） | ✅ |
| DICOM 字符集（ASCII / Latin-1 / Greek / 日文 / 韩文 / GB18030 等） | ✅ |
| DICOM 标准字典（5189 Tag + 88 Repeater） | ✅ |
| 私有字典（449 creators / 10545 entries） | ✅ |
| UID 字典（490 条） | ✅ |
| Pixel Data 解码（Native / JPEG / JPEG-LS / JPEG 2000 / HTJ2K / RLE） | ✅ |
| Pixel Data 编码（Native / RLE / Deflated / JPEG 2000） | ✅ |
| 显示管线（Modality LUT / VOI LUT / 窗宽窗位 / 反转 / P-LUT Shape） | ✅ |
| 数据集编解码（无 File Meta，DIMSE 载荷） | ✅ |
| Part 10 内存编解码（`EncodeFile` / `ReadBytes` / `Write`） | ✅ |
| DICOM JSON Model（单 / 多数据集） | ✅ |
| CLI（`show` / `read` / `readcopy`） | ✅ |

## 测试

```bash
bash scripts/fetch-testdata.sh   # 拉取多帧 emri_small 等样例（首次或 CI）
go test -count=1 ./...
```

- ~700 个测试，覆盖 8 个包；pydicom submodule 78 个 `.dcm` + `testdata/dcm/` 5 个 `emri_small*`。
- 回归样例取自 pydicom `pixels_reference` 采样点：`CT_small.dcm`、`MR_small*.dcm`、`emri_small*.dcm`、`SC_rgb_jpeg_*.dcm`、`JPGExtended.dcm`。
- 覆盖率见 [Codecov](https://codecov.io/gh/godicom-dev/godicom) badge。

## 项目结构

```
godicom/
├── tag.go / tag/             # Tag 类型与 keyword 子包
├── vr.go                     # VR 类型及分类
├── uid.go / uid/             # UID 类型与子包
├── errors.go                 # 错误类型
├── element.go                # DataElement / RawDataElement / PersonName
├── dataset.go                # Dataset / FileDataset / PrivateBlock
├── sequence.go               # Sequence
├── multivalue.go             # MultiValue
├── values.go                 # 值转换 (bytes → Go 类型)
├── valuerep*.go              # DA / TM / DT / DS / IS / PN 值表示
├── charset.go                # DICOM 字符编码
├── dictionary*.go            # 标准字典（含生成产物）
├── private_dictionary*.go    # 私有字典
├── read.go / write.go        # 文件读写
├── decode.go / encode*.go    # 数据集编解码（DIMSE 载荷）
├── encode_file*.go           # Part 10 内存编解码
├── pixeldata.go              # FileDataset.PixelBytes / PixelFrames
├── pixel_encode.go           # CompressPixelData / SetEncodedPixelData
├── pixel_processing.go       # Modality / VOI / 显示管线
├── encaps/                   # 封装像素 (BOT / fragment / frame)
├── pixels/                   # 像素编解码调度 (native / RLE / JPEG / J2K)
├── dicomjson/                # DICOM JSON Model (Part 18 Annex F)
├── cmd/godicom/              # CLI (show / read / readcopy)
├── scripts/                  # fetch-testdata.sh
├── generate_dict.py          # 字典生成脚本
└── pydicom/                  # pydicom submodule（参考 / 测试数据）
```

## 范围之外

以下明确**不在 godicom**，避免把 pydicom 单体塞进一个库：

| 项 | 归属 |
|----|------|
| DIMSE（C-ECHO / C-STORE / C-FIND / C-MOVE / C-GET / DIMSE-N） | [gonetdicom](https://github.com/godicom-dev/gonetdicom) |
| DICOMweb（WADO-RS / QIDO-RS / STOW-RS） | gonetdicom |
| HTTP 服务 / PACS 集成 | gonetdicom |

暂缓项（待有明确需求再单独立项，见 [TODO.md](TODO.md)）：`read_partial` / 流式 rawread、`defer_size` 字符串形式、`generate_uid()`、`register_transfer_syntax()`。

## 许可

MIT — 见 [LICENSE](LICENSE)。

## 变更记录

见 [CHANGELOG.md](CHANGELOG.md)。
