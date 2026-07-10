# godicom

**Go 实现的 DICOM 文件读写库** — pydicom 的 Go 移植版。

[![CI](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/godicom-dev/godicom/branch/main/graph/badge.svg)](https://codecov.io/gh/godicom-dev/godicom)
[![Lint](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg?job=lint)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.26-%23007d9c)
[![GoDoc](https://pkg.go.dev/badge/github.com/godicom-dev/godicom)](https://pkg.go.dev/github.com/godicom-dev/godicom)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 快速开始

```go
import "github.com/godicom-dev/godicom"

// 读取 DICOM 文件（默认选项传 nil）
ds, err := godicom.ReadFile("ct.dcm", nil)
if err != nil {
    return err
}

// 带读取选项
ds, err = godicom.ReadFile("ct.dcm", &godicom.ReadOptions{Force: true})

// 访问元素
name, _ := ds.GetString(godicom.MustTag(0x00100010))
id, _ := ds.GetString(godicom.MustTag(0x00100020))

// 修改元素
ds.Set(godicom.NewDataElement(godicom.MustTag(0x00100010), godicom.VRPN, "Anonymous"))

// 写入文件
err = ds.SaveAs("output.dcm", nil)
```

I/O 入口：`ReadFile` / `WriteFile`（或 `FileDataset.SaveAs`）。

## 像素数据解码

封装像素经 `encaps` 拆帧后，由 `pixels` 按 Transfer Syntax 调度解码（依赖 [golibjpeg](https://github.com/godicom-dev/golibjpeg)、[goopenjpeg](https://github.com/godicom-dev/goopenjpeg)、[gorle](https://github.com/godicom-dev/gorle)）：

```go
import (
    "github.com/godicom-dev/godicom"
    "github.com/godicom-dev/godicom/pixels"
)

ds, err := godicom.ReadFile("mr_j2k.dcm", nil)
if err != nil {
    return err
}

// 所有帧拼成一块 buffer（native 字节布局）
raw, err := ds.PixelBytes(pixels.WithRaw(true))
if err != nil {
    return err
}

// 或按帧解码
frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(0))
if err != nil {
    return err
}
_ = raw
_ = frames
```

**v0.2.0 像素读能力**

| 能力 | 说明 |
|------|------|
| 单帧 / 多帧 | `PixelFrames` 按 `NumberOfFrames` 拆分；`WithFrameIndex(n)` 取单帧 |
| 封装分帧 | Basic / Extended Offset Table、无 BOT + EOI 启发式 |
| 未压缩多帧 | 原生像素按帧切分（无需 encaps） |

**已支持的压缩格式（读）**

| 类别 | Transfer Syntax（示例 UID） |
|------|----------------------------|
| Native | Explicit/Implicit VR Little/Big Endian、Deflated |
| RLE Lossless | `1.2.840.10008.1.2.5` |
| JPEG Baseline / Extended / Lossless / Lossless SV1 | `1.2.840.10008.1.2.4.50` 等 |
| JPEG-LS Lossless / Near-Lossless | `1.2.840.10008.1.2.4.80` / `.81` |
| JPEG 2000 / HTJ2K | `1.2.840.10008.1.2.4.90` 等 |

**回归验证样例**（pydicom `pixels_reference` 采样点）

| 样例文件 | 内容 |
|----------|------|
| `CT_small.dcm` | Native 16-bit 单帧 |
| `MR_small*.dcm` | J2K / RLE / JPEG-LS 单帧 |
| `emri_small*.dcm` | 10 帧 native / RLE / JPEG-LS / J2K（`scripts/fetch-testdata.sh`） |
| `SC_rgb_jpeg_*.dcm` | JPEG baseline / lossless SV1 RGB |
| `JPGExtended.dcm` | JPEG extended 16-bit |

多帧测试数据不在 pydicom submodule 内，CI 与本地需先执行：

```bash
bash scripts/fetch-testdata.sh
```

**已知限制**：仅解码路径（无压缩写入）；`WithRaw(true)` 返回编解码库原始字节布局，不含 pydicom 式 reshape / YBR→RGB 后处理；像素 encode 与完整 encaps 生成未实现。细节见 [TODO.md](TODO.md)。

## DICOM JSON Model

```go
import (
    "github.com/godicom-dev/godicom"
    "github.com/godicom-dev/godicom/dicomjson"
)

ds, err := godicom.ReadFile("ct.dcm", nil)
if err != nil {
    return err
}

jsonData, err := dicomjson.MarshalDataset(ds.Dataset)
if err != nil {
    return err
}

parsed, err := dicomjson.ParseDataset(jsonData)
if err != nil {
    return err
}
_ = parsed
```

## 功能

| 功能 | 状态 |
|------|------|
| 读取 Explicit VR Little Endian | ✅ |
| 读取 Implicit VR Little Endian | ✅ |
| 读取 Explicit VR Big Endian | ✅ |
| 读取 Deflated Explicit VR Little Endian | ✅ |
| 混合编码自动切换 | ✅ |
| 文件 Meta 信息解析 | ✅ |
| 序列 (SQ) 解析 | ✅ |
| 嵌套私有 Tag | ✅ |
| `ReadOptions.SpecificTags` | ✅ |
| 写入 Explicit VR Little Endian | ✅ |
| 写入 Implicit VR Little Endian | ✅ |
| 写入 Explicit VR Big Endian | ✅ |
| 写入 Deflated Explicit VR Little Endian | ✅ |
| 写入序列 | ✅ |
| 基础 VR 值转换 | ✅ |
| DICOM 字符集 (ASCII/Latin-1/Greek 等) | 🚧 |
| DICOM 标准字典 (5189 Tag + 88 Repeater) | ✅ |
| Pixel Data 解码 (Native) | ✅ |
| Pixel Data 解码 (JPEG / JPEG-LS / JPEG 2000 / RLE) | ✅ |
| JSON 序列化 | ✅ |
| DICOMweb / WADO-RS | → **gonetdicom**（计划中独立库，非 godicom 范围） |

**v0.2.0** 起提供稳定的多帧像素**读**能力；metadata 读写与 JSON 仍在持续对齐 pydicom。完整路线图见 [TODO.md](TODO.md)。

## 测试

```bash
bash scripts/fetch-testdata.sh   # 多帧 emri_small 样例（首次或 CI）
go test -count=1 ./...
```

- 39 个测试文件，**591** 个测试用例（含 subtest，8 个包）
- 语句覆盖率见 [Codecov](https://codecov.io/gh/godicom-dev/godicom) badge
- pydicom submodule 78 个 `.dcm` + `testdata/dcm/` 5 个 `emri_small*`

## 项目结构

```
godicom/
├── tag.go / tag/           # Tag 类型与 keyword 子包
├── vr.go                   # VR 类型及分类
├── uid.go / uid/           # UID 类型与子包
├── errors.go               # 错误类型
├── element.go              # DataElement / RawDataElement / PersonName
├── dataset.go              # Dataset / FileDataset / PrivateBlock
├── sequence.go             # Sequence
├── multivalue.go           # MultiValue
├── values.go               # 值转换 (bytes → Go 类型)
├── charset.go              # DICOM 字符编码
├── dictionary.go           # 字典查询
├── dictionary_generated.go # 自动生成的 DICOM 字典
├── private_dictionary.go   # 私有字典查询与运行时扩展
├── private_dictionary_generated.go
├── io.go                   # I/O 基础
├── buffer.go               # 缓冲区工具
├── read.go                 # 文件读取
├── write.go                # 文件写入
├── pixeldata.go            # FileDataset.PixelBytes / PixelFrames
├── encaps/                 # 封装像素数据 (BOT / fragment / frame)
├── pixels/                 # 像素解码调度 (native / RLE / JPEG / J2K)
├── godicom.go              # 包文档
├── dicomjson/              # DICOM JSON Model (Part 18 Annex F)
├── generate_dict.py        # 字典生成脚本
├── cmd/godicom/            # CLI 工具 (read / readcopy)
└── pydicom/                # pydicom submodule (参考 / 测试数据)
```

## 许可

MIT

## 变更记录

See [CHANGELOG.md](CHANGELOG.md).
