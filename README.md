# godicom

**Go 实现的 DICOM 文件读写库** — pydicom 的 Go 移植版。

[![CI](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml/badge.svg)](https://github.com/godicom-dev/godicom/actions/workflows/ci.yml)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.22-%23007d9c)
[![GoDoc](https://pkg.go.dev/badge/github.com/godicom-dev/godicom)](https://pkg.go.dev/github.com/godicom-dev/godicom)
[![License](https://img.shields.io/github/license/godicom-dev/godicom)](LICENSE)

## 快速开始

```go
import "github.com/godicom-dev/godicom"

// 读取 DICOM 文件
ds, err := godicom.DcmReadFile("ct.dcm")
// 或使用选项
ds, err := godicom.ReadFile("ct.dcm", &godicom.ReadOptions{Force: true})

// 访问元素
name, _ := ds.GetString(godicom.MustTag(0x00100010))
id, _ := ds.GetString(godicom.MustTag(0x00100020))

// 修改元素
ds.Set(godicom.NewDataElement(godicom.MustTag(0x00100010), godicom.VRPN, "Anonymous"))

// 写入文件
ds.SaveAs("output.dcm", nil)
```

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
| 写入序列 | ✅ |
| 基础 VR 值转换 | ✅ |
| DICOM 字符集 (ASCII/Latin-1/Greek 等) | 🚧 |
| DICOM 标准字典 (5189 Tag + 88 Repeater) | ✅ |
| Pixel Data 解码 (Native) | ❌ |
| Pixel Data 解码 (JPEG/JPEG-LS/JPEG-2000/RLE) | ❌ |
| JSON 序列化 | ✅ |
| DICOMweb / WADO-RS | ❌ |

当前聚焦 **metadata 读写子集**；完整路线图见 [TODO.md](TODO.md)。

## 测试

```
go test -count=1 ./...
```

- 17 个测试文件，282 个测试用例（含 subtest）
- 语句覆盖率约 **71%**
- 使用 pydicom submodule 中 78 个真实 `.dcm` 测试文件

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
├── io.go                   # I/O 基础
├── buffer.go               # 缓冲区工具
├── read.go                 # 文件读取
├── write.go                # 文件写入
├── godicom.go              # 包文档
├── dicomjson/              # DICOM JSON Model (Part 18 Annex F)
├── generate_dict.py        # 字典生成脚本
├── cmd/godicom/            # CLI 工具 (read / readcopy)
└── pydicom/                # pydicom submodule (参考 / 测试数据)
```

## 许可

MIT
