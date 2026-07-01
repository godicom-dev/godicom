# godicom TODO

## 项目状态

godicom 是 pydicom 的 Go 移植版本。当前实现覆盖核心 DICOM metadata 读写子集，但距离 pydicom 完整功能仍有较大差距。

**当前阶段**：metadata 读写闭环基本完成；字典层与 DICOM JSON Model 已对齐 pydicom 主路径；下一步大块功能：Native Pixel Data。

## 暂缓项（当前阶段明确不做，保留记录）

以下功能**不是永远不做**，而是 metadata 主路径已通、投入产出比低，待有明确需求再单独立项：

| 项 | 现状 | 触发条件 |
|----|------|----------|
| `read_partial` / 流式 rawread | `ReadFile` 仍整文件进内存；deferred 在内存 buffer 上延迟加载 | 需要 `io.Reader` 网络流/管道、单文件大于内存 |
| `defer_size` 字符串形式（`"2 kB"`） | Go 仅有 `ReadOptions.DeferSize uint32` | 需要与 pydicom 测试完全一致的字符串 API |
| `generate_uid()` | pydicom UID 生成器未移植 | 需要运行时生成 DICOM UID |
| `register_transfer_syntax()` | 私有传输语法运行时注册未移植 | 需要自定义私有 TS + 编码声明 |

## 迁移原则

- **默认**：行为、测试、边界条件全部参照 pydicom 源码与测试（`pydicom/src/pydicom/` + `pydicom/tests/`）
- **例外**：仅当 Python 动态特性或 Go 类型系统无法直接等价时，先向人类确认 API（已确认：keyword 访问用显式 getter + `tag` 常量；原始字节用 `Element.RawValue`）
- 不手写可由 pydicom 字典派生的数据；必须从源字典生成并验证覆盖
- 每实现一个 pydicom 功能块，必须同步迁移对应测试用例或建立 Go 等价测试
- 不以“能读不报错”作为完整验证；必须增加值、VR、VM、tag、file meta、transfer syntax、roundtrip 断言

### API 设计决策（已确认）

pydicom 的 `ds.PatientName` **不移植**为 Go 动态属性。统一采用 **显式 getter + `tag` 常量**：

```go
import (
    "github.com/godicom-dev/godicom"
    "github.com/godicom-dev/godicom/tag"
)

name, ok := ds.GetString(tag.PatientName)
id, ok := ds.GetString(tag.PatientID)
rows, ok := ds.GetInt(tag.Rows)
```

- Tag 定位：`tag` 子包提供 keyword 常量；根包 `MustTag("PatientName")` / `ParseTag` 作为便捷入口
- 取值：`GetString` / `GetInt` / `GetFloat` / `GetBytes` / `GetSequence`（及 `*Value` 别名）
- 文件 I/O：`ReadFile(path, opts)`、`WriteFile(path, ds, opts)`、`FileDataset.SaveAs(path, opts)`；`DcmRead` / `DcmReadFile` / `DcmWrite` 为已废弃别名
- **不做**：`ds.PatientName`、字符串 keyword 下标 `ds["PatientName"]`、代码生成 accessor
- 其他 Python 动态特性（Dataset 切片语义、config/hooks、pixel handler plugin）迁移前仍需单独确认

## 当前已实现

- [x] 核心类型：`Tag`, `VR`, `UID`, errors
- [x] Go 风格子包：`tag`、`uid`
- [x] `tag` 包完整生成 DICOM keyword 常量和 keyword/tag 双向映射（从 `dictionary_generated.go` 派生）
- [x] 数据模型：`Element`/`DataElement`, `RawDataElement`, `Dataset`, `FileDataset`, `FileMetaDataset`, `Sequence`, `MultiValue`, `PersonName`
- [x] 基础 Dataset API：Set/Get/Delete/Has/Iter/SortedTags/typed getters/private block/SaveAs
- [x] 值转换：基础 VR bytes → Go 类型转换（strings、PN、UI、AT、int、float、binary、DS/IS multi-values）
- [x] DICOM 字典：标准字典、keyword 映射、repeater tag 匹配
- [x] UID 字典：从 `_uid_dict.py` 生成 490 条；`uid.Dictionary` / `Lookup` / 传输语法属性
- [x] 私有字典：从 `_private_dict.py` 生成 449 creators / 10545 entries；`PrivateDictionaryVR/VM/Description`、`AddPrivateDictEntry`
- [x] 字符集辅助函数：`DecodeString`、`EncodeString`、`DecodeBytes`
- [x] 文件读取：Explicit/Implicit VR、Little/Big Endian、Deflated Explicit VR Little Endian、file meta 分离、`SpecificTags`、`Force`、`StopBeforePixels`、`DeferSize`
- [x] 文件写入：Dataset/Element/Sequence 基础写入，Explicit/Implicit VR、Little/Big Endian 选项，保留 `FileDataset.FileMeta` 与 `Preamble`
- [x] DICOM JSON Model：`dicomjson` 子包；对齐 `test_json.py` 主路径（全 VR roundtrip、CT_small 完整往返、PN/AT/数值/空值/BulkDataURI/UN InlineBinary、fixture roundtrip）
- [x] 基础 CLI：`godicom read`、`godicom readcopy`

## 项目结构（Go 源码）

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
├── private_dictionary_generated.go # 自动生成的私有字典
├── generate_dict.py        # 标准字典生成脚本
├── generate_uid_dict.py    # UID 字典生成脚本
├── generate_private_dict.py # 私有字典生成脚本
├── uid/dictionary_generated.go
├── uid/generated.go
├── io.go                   # I/O 基础（原 filebase）
├── buffer.go               # 缓冲区工具（原 fileutil）
├── read.go                 # 文件读取（原 filereader）
├── write.go                # 文件写入（原 filewriter）
├── dicomjson/              # DICOM JSON Model (Part 18 Annex F)
├── cmd/godicom/            # CLI 工具
└── pydicom/                # pydicom submodule（参考 / 测试数据）
```

## pydicom 参照清单

迁移任何功能前，先对照下表中的 pydicom 源码和测试文件；涉及 Python 动态特性时必须先确认 Go API 设计。

| 功能领域 | pydicom 源码参照 | pydicom 测试参照 | Go 当前状态 |
|---|---|---|---|
| Tag 类型/解析 | `pydicom/src/pydicom/tag.py` | `pydicom/tests/test_tag.py` | 部分实现；`tag` 子包 keyword 常量已生成，子包测试已补 |
| DICOM 标准字典 | `pydicom/src/pydicom/_dicom_dict.py`, `pydicom/src/pydicom/datadict.py` | `pydicom/tests/test_dictionary.py` | 标准字典已实现；`dictionary_test.go` 覆盖查找/repeater |
| Private Dictionary | `pydicom/src/pydicom/_private_dict.py`, `pydicom/src/pydicom/datadict.py` | `pydicom/tests/test_dictionary.py` | 已实现；`private_dictionary_generated.go` + 查询 API + `AddPrivateDictEntry` |
| UID / UID 字典 | `pydicom/src/pydicom/uid.py`, `pydicom/src/pydicom/_uid_dict.py` | `pydicom/tests/test_uid.py` | 字典已生成（490 条）；`generate_uid` / `register_transfer_syntax` 暂缓 |
| VR / valuerep | `pydicom/src/pydicom/valuerep.py` | `pydicom/tests/test_valuerep.py` | 大量缺失；当前只有 VR 常量/分类和简化 PN |
| values 转换 | `pydicom/src/pydicom/values.py`, `pydicom/src/pydicom/valuerep.py` | `pydicom/tests/test_values.py` | 部分实现；需逐项迁移转换边界测试 |
| DataElement / RawDataElement | `pydicom/src/pydicom/dataelem.py` | `pydicom/tests/test_dataelem.py` | 部分实现；validation、deferred/buffered 等不足；`element_test.go` 已覆盖 String/ReprValue/Equal |
| Dataset / FileDataset | `pydicom/src/pydicom/dataset.py` | `pydicom/tests/test_dataset.py` | 部分实现；keyword 访问已定为显式 getter + `tag` 常量；walk/copy/validation 等待补 |
| Sequence | `pydicom/src/pydicom/sequence.py` | `pydicom/tests/test_sequence.py` | 基础实现 |
| MultiValue | `pydicom/src/pydicom/multival.py` | `pydicom/tests/test_multival.py` | 基础实现 |
| Charset / Unicode | `pydicom/src/pydicom/charset.py` | `pydicom/tests/test_charset.py`, `pydicom/tests/test_unicode.py` | 部分实现；`charset_test.go` 覆盖当前支持范围；ISO-2022 与读写路径集成未充分验证 |
| File base / DICOM IO | `pydicom/src/pydicom/filebase.py`, `pydicom/src/pydicom/dicomio.py` | `pydicom/tests/test_filebase.py` | 部分实现；`io.go` + `io_test.go` 覆盖 tag/uint 读写 |
| File util | `pydicom/src/pydicom/fileutil.py`, `pydicom/src/pydicom/misc.py` | `pydicom/tests/test_fileutil.py`, `pydicom/tests/test_misc.py`, `pydicom/tests/test_util.py` | 部分实现；`buffer.go` + `buffer_test.go` 覆盖 buffer length/equality 等 |
| File reader | `pydicom/src/pydicom/filereader.py` | `pydicom/tests/test_filereader.py`, `pydicom/tests/test_rawread.py` | 基础 transfer syntax 读取已实现；`DeferSize` 延迟加载已实现；**流式 rawread 暂缓**（见暂缓项） |
| File writer | `pydicom/src/pydicom/filewriter.py` | `pydicom/tests/test_filewriter.py` | 基础写入器；已保留 FileDataset file meta/preamble；已补 string VR padding、OB odd padding、OD/OL/UC/UR/UN 字节布局；group length、ambiguous VR、undefined length、roundtrip 字节级仍需补 |
| DICOM JSON Model | `pydicom/src/pydicom/jsonrep.py`, `pydicom/src/pydicom/dataset.py` | `pydicom/tests/test_json.py` | 已实现；`dicomjson` 对齐主测试路径；刻意不做：`dump_handler`、元素级 `from_json`、BulkDataURI warn |
| Encapsulated Pixel Data | `pydicom/src/pydicom/encaps.py` | `pydicom/tests/test_encaps.py` | 部分实现；`encaps` 包覆盖 BOT/fragment/frame 拆分（核心路径 + 部分 pydicom 用例） |
| Pixel Data 通用工具 | `pydicom/src/pydicom/pixels/common.py`, `pydicom/src/pydicom/pixels/utils.py`, `pydicom/src/pydicom/pixel_data_handlers/util.py` | `pydicom/tests/pixels/test_common.py`, `pydicom/tests/pixels/test_utils.py`, `pydicom/tests/test_handler_util.py` | 未实现 |
| Native Pixel Decode/Encode | `pydicom/src/pydicom/pixels/decoders/native.py`, `pydicom/src/pydicom/pixels/encoders/native.py`, `pydicom/src/pydicom/pixel_data_handlers/numpy_handler.py` | `pydicom/tests/pixels/test_decoder_native.py`, `pydicom/tests/pixels/test_encoder_pydicom.py`, `pydicom/tests/test_numpy_pixel_data.py` | 部分实现；`pixels` 包 native 路径 + `FileDataset.PixelBytes` |
| RLE Pixel Data | `pydicom/src/pydicom/pixel_data_handlers/rle_handler.py` | `pydicom/tests/test_rle_pixel_data.py` | 部分实现；经 `gorle` 解码（`MR_small_RLE.dcm` 回归） |
| JPEG/JPEG-LS/JPEG2000 handlers | `pydicom/src/pydicom/pixel_data_handlers/*.py`, `pydicom/src/pydicom/pixels/decoders/*.py`, `pydicom/src/pydicom/pixels/encoders/*.py` | `pydicom/tests/test_gdcm_pixel_data.py`, `test_pillow_pixel_data.py`, `test_pylibjpeg.py`, `test_jpeg_ls_pixel_data.py`, `pydicom/tests/pixels/test_decoder_*.py`, `pydicom/tests/pixels/test_encoder_*.py` | 部分实现；JPEG/JPEG-LS/J2K 经 `golibjpeg`/`goopenjpeg` 解码（baseline、lossless SV1、JPEG-LS、J2K 回归） |
| Pixel processing | `pydicom/src/pydicom/pixels/processing.py` | `pydicom/tests/pixels/test_processing.py` | 未实现 |
| File-set / DICOMDIR | `pydicom/src/pydicom/fileset.py` | `pydicom/tests/test_fileset.py` | 未实现 |
| SR / codes | `pydicom/src/pydicom/sr/codedict.py`, `coding.py`, `_cid_dict.py`, `_concepts_dict.py`, `_snomed_dict.py` | `pydicom/tests/test_codes.py` | 未实现 |
| Overlay | `pydicom/src/pydicom/overlays/numpy_handler.py` | `pydicom/tests/test_overlay_np.py` | 未实现 |
| Waveform | `pydicom/src/pydicom/waveforms/numpy_handler.py` | `pydicom/tests/test_waveform.py` | 未实现 |
| Config | `pydicom/src/pydicom/config.py` | `pydicom/tests/test_config.py` | 未实现；Python 全局配置迁移前需确认 Go API |
| Hooks | `pydicom/src/pydicom/hooks.py` | `pydicom/tests/test_hooks.py` | 未实现；Python hook 机制迁移前需确认 Go API |
| CLI | `pydicom/src/pydicom/cli/main.py`, `cli/show.py`, `cli/codify.py` | `pydicom/tests/test_cli.py` | 部分实现，仅 read/readcopy |
| Data manager / test data | `pydicom/src/pydicom/data/data_manager.py`, `data/download.py`, `data/retry.py` | `pydicom/tests/test_data_manager.py` | 未实现，当前复用 pydicom submodule 测试数据（78 个 `.dcm` 文件） |
| Env info / examples | `pydicom/src/pydicom/env_info.py`, `examples/__init__.py` | `pydicom/tests/test_env_info.py`, `pydicom/tests/test_examples.py` | 未实现 |
| Errors | `pydicom/src/pydicom/errors.py` | `pydicom/tests/test_errors.py` | 部分实现；`errors_test.go` 已覆盖 |

## 当前测试与覆盖率

最新统计（`go test ./... -count=1`）：

- Go 测试包：`godicom`（根包）、`tag`、`uid`、`dicomjson`、`encaps`、`pixels`（共 6 个有测试的包）
- Go 测试文件：23 个（见下表）
- Go 测试用例：**355** 个（`go test ./... -count=1`）
- pydicom 测试数据：78 个 `.dcm` 文件（`pydicom/src/pydicom/data/test_files/`）
- pydicom pytest 测试定义：约 2392 个
- pydicom pytest 文件：约 55 个
- `go test ./...`：**通过**
- 语句覆盖率：**71.3%**

### Go 测试覆盖现状

| Go 测试文件 | 覆盖领域 | 状态 |
|---|---|---|
| `tag_test.go` | 根包 Tag 构造/属性/私有 tag/JSON key | 部分覆盖 |
| `tag/tag_test.go` | 子包 API 与生成 keyword 映射校验 | 已覆盖 |
| `vr_test.go` | VR 分类集合 | 部分覆盖 |
| `uid_test.go` | 根包 UID 常量/校验/字典 | 已覆盖 |
| `uid/uid_test.go` | 子包 UID 字典/metadata/传输语法属性 | 已覆盖 |
| `element_test.go` | DataElement String/ReprValue/Equal/Name/VM | 部分覆盖 |
| `dataset_test.go` | Dataset set/get/delete/typed getters/private block | 部分覆盖 |
| `values_test.go` | 基础 VR 转换 | 部分覆盖 |
| `dictionary_test.go` | 标准字典查找/repeater + 私有字典查询/运行时扩展 | 已覆盖 |
| `deferred_test.go` | DeferSize 延迟加载 | 已覆盖 |
| `ambiguous_vr_test.go` | CorrectAmbiguousVR | 已覆盖 |
| `read_test.go` | pydicom 测试文件读取、值级断言、SpecificTags、Big Endian/Deflated、RTPlan sequence、roundtrip | 部分覆盖 |
| `write_test.go` | 写入/readback/sequence/VR padding/OD/OL/UC/UR/UN 字节布局 | 部分覆盖 |
| `multivalue_test.go` | MultiValue 基础操作 | 部分覆盖 |
| `sequence_test.go` | Sequence 基础操作 | 部分覆盖 |
| `charset_test.go` | ASCII/Latin-1/Greek encode/decode | 部分覆盖 |
| `io_test.go` | tag/uint 读写 | 部分覆盖 |
| `buffer_test.go` | buffer length/equality | 部分覆盖 |
| `errors_test.go` | 错误类型 | 部分覆盖 |
| `dicomjson/json_test.go` | PN/AT/SQ/binary/BulkDataURI/fixtures | 已覆盖 |
| `dicomjson/roundtrip_test.go` | 全 VR roundtrip、CT_small 完整往返 | 已覆盖 |
| `dicomjson/edges_test.go` | PN 边界、AT 非法值、数值、嵌套 SQ roundtrip | 已覆盖 |

### 仍待覆盖或弱覆盖

- [x] DICOM JSON Model（`dicomjson` 子包）
  - pydicom 参照：`pydicom/tests/test_json.py`
  - 已覆盖：全 VR `test_roundtrip`、CT_small 完整 dataset 往返、PN 组件边界、AT 空值/非法值、数值类型、嵌套 SQ、`test_PN.json`/`test1.json`、BulkDataURI/InlineBinary/UN
  - 刻意不做：Python `dump_handler`、元素级 `DataElement.from_json`、BulkDataURI 无 handler 时的 UserWarning（Go 静默）
- [ ] Reader rawread 流式 API（**暂缓**，见暂缓项）
- [x] Reader deferred 机制（`DeferSize` uint32；字符串形式暂缓）
- [x] Reader encapsulated pixel data 读取（undefined length OB/OW item 流）
  - pydicom 参照：`pydicom/src/pydicom/fileutil.py` `_try_read_encapsulated_pixel_data`
- [x] Pixel decode 基础路径（`encaps` + `pixels` + `FileDataset.PixelBytes`/`PixelFrames`）
  - pydicom 参照：`pydicom/tests/test_encaps.py`（部分）、`MR_small_jp2klossless.dcm`、`MR_small_RLE.dcm`、`CT_small.dcm`
  - 待补：完整 encaps 测试移植、JPEG/JPEG-LS 全覆盖、reshape/YBR→RGB、encode 路径
- [ ] Writer group length、ambiguous VR、undefined length、roundtrip 字节级兼容
  - pydicom 参照：`pydicom/tests/test_filewriter.py`
- [ ] ISO-2022 字符集完整行为与读写路径集成
  - pydicom 参照：`pydicom/tests/test_charset.py`, `pydicom/tests/test_unicode.py`
- [x] UID 字典生成与查询（490 条；`generate_uid` / `register_transfer_syntax` 暂缓）

## pydicom 对比：大块缺失功能

### 最高优先级缺口

- [x] **pydicom 动态属性迁移设计**
  - Python: `ds.PatientName` → Go: `ds.GetString(tag.PatientName)`（显式 getter + `tag` 常量）
  - 不做动态属性、字符串 keyword 访问、生成 accessor

- [ ] **Dataset API 完整性**
  - pydicom 覆盖约 209 个 dataset 测试定义
  - Go 当前只有基础 map-backed dataset + typed getters
  - 缺少/待实现：walk/recursive iteration、copy/clone、validation、file meta 行为、pixel helper、private creator 完整语义
  - 不做：slice-like 行为（Python 特有）

- [ ] **File Reader 完整性**
  - pydicom 有 `test_filereader.py` + `test_rawread.py`，约 130 个测试定义
  - Go 当前完成：file meta 分离、SpecificTags、Big Endian、Deflated、CT/MR 值级断言、defined length SQ 基础读取、RTPlan 深层 sequence 断言、78 个测试文件 bulk read
  - 缺少/待实现：read_partial/raw 流式 API（**暂缓**）、malformed file recovery、hooks/callbacks
  - 已完成：deferred 机制（`DeferSize` uint32）、encapsulated pixel data 读取

- [ ] **File Writer 完整性**
  - pydicom `test_filewriter.py` 约 178 个测试定义
  - Go 当前是基础写入器，已补部分 padding 与 explicit VR 字节布局测试
  - 缺少/待实现：file meta enforcement、group length/version、ambiguous VR 修正、defined/undefined length 细节、完整 roundtrip 兼容

- [ ] **Value Representation / valuerep**
  - pydicom `test_valuerep.py` 约 117 个测试定义
  - Go 当前没有完整 `valuerep` 等价层
  - 缺少：DA/TM/DT 强类型、DSfloat/DSdecimal 设计、PersonName 完整行为、validation mode、original string preservation

### 高优先级缺口

- [ ] **Pixel Data 解码/编码**
  - pydicom legacy pixel handlers + 新 `pydicom.pixels` 测试约 900+ 个测试定义
  - Go 已有 `pixels` 包：native / RLE / JPEG / J2K 解码调度（`golibjpeg`、`goopenjpeg`、`gorle`）
  - 待补：JPEG-LS 16-bit RGB、多帧/extended offset table 全覆盖、reshape、photometric 后处理、encode 路径

- [ ] **Encapsulated Pixel Data**
  - pydicom `encaps.py` + `test_encaps.py` 约 164 个测试定义
  - Go 已有 `encaps` 包：BOT、fragment 拆分、frame 合并（核心路径 + 部分测试）
  - 待补：完整测试移植、encapsulation generation、extended offset table 边界

- [x] **DICOM JSON Model**
  - `dicomjson` 子包；对齐 `test_json.py` 主路径（全 VR roundtrip、CT_small 完整往返等）
  - 刻意不做：Python `dump_handler`、元素级 `from_json`、BulkDataURI 无 handler 时的 warn

- [x] **Private Dictionary**
  - 从 `_private_dict.py` 生成 449 creators / 10545 entries
  - `PrivateDictionaryVR/VM/Description`、`AddPrivateDictEntry`、Element 私有名查询

- [x] **UID 字典**
  - 从 `_uid_dict.py` 生成 490 条；`uid.Dictionary` / `Lookup` / 传输语法属性
  - `generate_uid()` / `register_transfer_syntax()` **暂缓**（见暂缓项）

- [ ] **Charset / Unicode 完整性**
  - pydicom `test_charset.py` + `test_unicode.py` 约 34 个测试定义
  - Go 当前仅辅助函数，读写路径集成不足
  - 缺少：多字符集、ISO-2022 escape、PN 多组件字符集、read/write integration

### 中优先级缺口

- [ ] **File-set / DICOMDIR**
  - pydicom `fileset.py` + `test_fileset.py` 约 124 个测试定义
  - Go 当前无实现

- [ ] **SR / codes**
  - pydicom `sr` 包 + `test_codes.py` 约 23 个测试定义，大量 SNOMED/CID 数据
  - Go 当前无实现

- [ ] **Overlay / Waveform**
  - pydicom overlay/waveform tests 合计约 28 个测试定义
  - Go 当前无实现

- [ ] **Config / Hooks**
  - pydicom `config.py`、`hooks.py` 有独立测试
  - Go 当前无等价设计
  - 涉及 Python 特有全局配置/钩子机制，迁移前必须先问人类确认 API

- [ ] **CLI parity**
  - pydicom CLI 有 show/codify 等
  - Go 当前只有 read/readcopy

## pydicom 测试规模参考

| pydicom 测试领域 | 约测试定义数 | Go 状态 |
|---|---:|---|
| Dataset | 209 | 部分实现/部分测试 |
| DataElement | 123 | 部分实现/部分测试 |
| File reader/raw read | 130 | 基础实现/部分测试 |
| File writer | 178 | 基础实现/部分测试 |
| Value representation | 117 | 大量缺失 |
| Values conversion | 28 | 部分实现 |
| Pixel data legacy handlers | 239 | 未实现 |
| `pydicom.pixels` API | 689 | 未实现 |
| Encapsulation | 164 | 未实现 |
| Charset/Unicode | 34 | 部分实现/部分测试 |
| File-set/DICOMDIR | 124 | 未实现 |
| UID | 50 | 字典已实现/部分测试（生成器与私有 TS 注册暂缓） |
| Tag | 51 | 部分实现/子包测试已补 |
| Dictionary | 18 | 标准 + 私有字典已实现/已测试 |
| JSON | 30 | 已实现/已测试（`dicomjson`） |
| CLI | 17 | 部分实现 |
| SR/codes | 23 | 未实现 |
| Sequence | 10 | 基础实现 |
| MultiValue | 17 | 基础实现 |
| Filebase/Fileutil | 31 | 部分实现/部分测试（`io.go`/`buffer.go`） |

## 迁移规则

- [x] Dataset keyword 访问：显式 getter + `tag` 常量
- [x] 原始字节保留：`Element.RawValue`（对齐 pydicom `is_raw`）
- [x] 默认逻辑参照 pydicom；仅 Python/Go 语法差异需单独确认
- [ ] 其他 Python 特有行为（Dataset 切片语义、config/hooks、pixel handler plugin）迁移前仍需确认
- [ ] 不手写可由 pydicom 字典派生的数据；必须从源字典生成并验证覆盖
- [ ] 每实现一个 pydicom 功能块，必须同步迁移对应测试用例或建立 Go 等价测试
- [ ] 不以“能读不报错”作为完整验证；必须增加值、VR、VM、tag、file meta、transfer syntax、roundtrip 断言

## 建议下一步

1. **补测试基础设施** ✅ 已完成
   - [x] 为 `tag` 子包补完整生成覆盖测试（`tag/tag_test.go`）
   - [x] 为 `uid` 子包补测试（`uid/uid_test.go`）
   - [x] 为 `charset.go`、`io.go`、`buffer.go` 补基础测试（`charset_test.go`、`io_test.go`、`buffer_test.go`）

2. **优先补齐 metadata 读写闭环** ✅ 核心路径已完成
   - [x] 实现并测试 `ReadOptions.SpecificTags`
   - [x] 完善 file meta 写入保留与读取分离
   - [x] 完善 Big Endian / Deflated 读取测试与实现
   - [x] 增加典型 pydicom 文件的 CT/MR 值级断言
   - [x] 修复 defined length SQ 读取，迁移 `test_filereader.py::test_RTPlan` 的基础断言
   - [x] 继续补 `test_filereader.py::test_RTPlan` 更深层嵌套断言：`ReferencedDoseReferenceSequence`、`BeamLimitingDevicePositionSequence`

3. **确认 Python 动态 API 的 Go 设计** ✅ 已确认
   - [x] `ds.PatientName` → `ds.GetString(tag.PatientName)`
   - [x] 不做字符串 keyword 访问、不做生成 accessor

4. **补齐 metadata 边角** ✅ 已完成
   - [x] `Element.RawValue` 原始字节保留与写入（对齐 pydicom `is_raw` / `write_like_original`）
   - [x] 跳过 retired Group Length 写入（PS3.5 §7.2）
   - [x] 字节级 roundtrip 测试：`CT_small` / `MR_small` / `rtplan` / `rtdose`
   - [x] undefined-length sequence item 写入（`IsUndefinedLengthSequenceItem`）
   - [x] `CorrectAmbiguousVR`（对齐 `correct_ambiguous_vr`）
   - [x] `ValidateFileMeta` + `EnforceFileFormat` 基础语义（对齐 `validate_file_meta` / `enforce_file_format`）
   - [x] preamble 写入规则（无 preamble 且 `EnforceFileFormat=false` 时不写 preamble/DICM）
   - [x] Reader deferred 机制（`DeferSize` uint32、延迟加载、`LoadDeferred`）
   - [x] UID Dictionary 生成（`generate_uid_dict.py` → `uid/dictionary_generated.go`）
   - [x] Private Dictionary 生成（`generate_private_dict.py` → `private_dictionary_generated.go`）
   - [ ] Reader `read_partial` / rawread 流式 API（**暂缓**，见暂缓项）
   - [ ] `defer_size` 字符串形式（**暂缓**，见暂缓项）

5. **再进入大块功能** ⬅️ 当前
   - [x] DICOM JSON Model（`dicomjson`，对齐 `test_json.py` 主路径）
   - [ ] Native Pixel Data
   - [ ] Encapsulated Pixel Data
