# godicom TODO

## 项目状态

godicom 是 pydicom 的 Go 移植版本。当前实现覆盖核心 DICOM metadata 读写子集，但距离 pydicom 完整功能仍有较大差距。

## 当前已实现

- [x] 核心类型：`Tag`, `VR`, `UID`, errors
- [x] Go 风格子包：`tag`、`uid`
- [x] `tag` 包完整生成 DICOM keyword 常量和 keyword/tag 双向映射（从 `dicom_dict_generated.go` 派生）
- [x] 数据模型：`Element`/`DataElement`, `RawDataElement`, `Dataset`, `FileDataset`, `FileMetaDataset`, `Sequence`, `MultiValue`, `PersonName`
- [x] 基础 Dataset API：Set/Get/Delete/Has/Iter/SortedTags/typed getters/private block/SaveAs
- [x] 值转换：基础 VR bytes → Go 类型转换（strings、PN、UI、AT、int、float、binary、DS/IS multi-values）
- [x] DICOM 字典：标准字典、keyword 映射、repeater tag 匹配
- [x] 字符集辅助函数：`DecodeString`、`EncodeString`、`DecodeBytes`
- [x] 文件读取：Explicit/Implicit VR、Little/Big Endian 基础处理、file meta 分离、sequence 读取、`Force`、`StopBeforePixels`、`DeferSize`
- [x] 文件写入：Dataset/Element/Sequence 基础写入，Explicit/Implicit VR、Little/Big Endian 选项
- [x] 基础 CLI：`godicom read`、`godicom readcopy`

## pydicom 参照清单

迁移任何功能前，先对照下表中的 pydicom 源码和测试文件；涉及 Python 动态特性时必须先确认 Go API 设计。

| 功能领域 | pydicom 源码参照 | pydicom 测试参照 | Go 当前状态 |
|---|---|---|---|
| Tag 类型/解析 | `pydicom/src/pydicom/tag.py` | `pydicom/tests/test_tag.py` | 部分实现；`tag` 子包完整 keyword 常量已生成，需补子包测试 |
| DICOM 标准字典 | `pydicom/src/pydicom/_dicom_dict.py`, `pydicom/src/pydicom/datadict.py` | `pydicom/tests/test_dictionary.py` | 标准字典部分实现；需补完整生成验证测试 |
| Private Dictionary | `pydicom/src/pydicom/_private_dict.py`, `pydicom/src/pydicom/datadict.py` | `pydicom/tests/test_dictionary.py`, `pydicom/tests/test_private_dict.py` 若上游存在 | 未实现；当前 private lookup 是 placeholder |
| UID / UID 字典 | `pydicom/src/pydicom/uid.py`, `pydicom/src/pydicom/_uid_dict.py` | `pydicom/tests/test_uid.py` | 部分实现；缺完整 UID 字典生成 |
| VR / valuerep | `pydicom/src/pydicom/valuerep.py` | `pydicom/tests/test_valuerep.py` | 大量缺失；当前只有 VR 常量/分类和简化 PN |
| values 转换 | `pydicom/src/pydicom/values.py`, `pydicom/src/pydicom/valuerep.py` | `pydicom/tests/test_values.py` | 部分实现；需逐项迁移转换边界测试 |
| DataElement / RawDataElement | `pydicom/src/pydicom/dataelem.py` | `pydicom/tests/test_dataelem.py` | 部分实现；validation、deferred/buffered、repr 等不足 |
| Dataset / FileDataset | `pydicom/src/pydicom/dataset.py` | `pydicom/tests/test_dataset.py` | 部分实现；Python 动态属性迁移需先确认 API |
| Sequence | `pydicom/src/pydicom/sequence.py` | `pydicom/tests/test_sequence.py` | 基础实现 |
| MultiValue | `pydicom/src/pydicom/multival.py` | `pydicom/tests/test_multival.py` | 基础实现 |
| Charset / Unicode | `pydicom/src/pydicom/charset.py` | `pydicom/tests/test_charset.py`, `pydicom/tests/test_unicode.py` | 部分实现；读写路径集成和 ISO-2022 未充分验证 |
| File base / DICOM IO | `pydicom/src/pydicom/filebase.py`, `pydicom/src/pydicom/dicomio.py` | `pydicom/tests/test_filebase.py` | 部分实现；覆盖率不足 |
| File util | `pydicom/src/pydicom/fileutil.py`, `pydicom/src/pydicom/misc.py` | `pydicom/tests/test_fileutil.py`, `pydicom/tests/test_misc.py`, `pydicom/tests/test_util.py` | 部分实现；很多函数 placeholder 或未覆盖 |
| File reader | `pydicom/src/pydicom/filereader.py` | `pydicom/tests/test_filereader.py`, `pydicom/tests/test_rawread.py` | 基础实现；read_partial/raw/deferred/SpecificTags/deflated 等缺失 |
| File writer | `pydicom/src/pydicom/filewriter.py` | `pydicom/tests/test_filewriter.py` | 基础实现；file meta、ambiguous VR、padding、undefined length 等需补 |
| DICOM JSON Model | `pydicom/src/pydicom/jsonrep.py`, `pydicom/src/pydicom/dataset.py` | `pydicom/tests/test_json.py` | 骨架；缺 Part 18 Annex F 完整兼容和 from_json |
| Encapsulated Pixel Data | `pydicom/src/pydicom/encaps.py` | `pydicom/tests/test_encaps.py` | 未实现 |
| Pixel Data 通用工具 | `pydicom/src/pydicom/pixels/common.py`, `pydicom/src/pydicom/pixels/utils.py`, `pydicom/src/pydicom/pixel_data_handlers/util.py` | `pydicom/tests/pixels/test_common.py`, `pydicom/tests/pixels/test_utils.py`, `pydicom/tests/test_handler_util.py` | 未实现 |
| Native Pixel Decode/Encode | `pydicom/src/pydicom/pixels/decoders/native.py`, `pydicom/src/pydicom/pixels/encoders/native.py`, `pydicom/src/pydicom/pixel_data_handlers/numpy_handler.py` | `pydicom/tests/pixels/test_decoder_native.py`, `pydicom/tests/pixels/test_encoder_pydicom.py`, `pydicom/tests/test_numpy_pixel_data.py` | 未实现 |
| RLE Pixel Data | `pydicom/src/pydicom/pixel_data_handlers/rle_handler.py` | `pydicom/tests/test_rle_pixel_data.py` | 未实现 |
| JPEG/JPEG-LS/JPEG2000 handlers | `pydicom/src/pydicom/pixel_data_handlers/*.py`, `pydicom/src/pydicom/pixels/decoders/*.py`, `pydicom/src/pydicom/pixels/encoders/*.py` | `pydicom/tests/test_gdcm_pixel_data.py`, `test_pillow_pixel_data.py`, `test_pylibjpeg.py`, `test_jpeg_ls_pixel_data.py`, `pydicom/tests/pixels/test_decoder_*.py`, `pydicom/tests/pixels/test_encoder_*.py` | 未实现 |
| Pixel processing | `pydicom/src/pydicom/pixels/processing.py` | `pydicom/tests/pixels/test_processing.py` | 未实现 |
| File-set / DICOMDIR | `pydicom/src/pydicom/fileset.py` | `pydicom/tests/test_fileset.py` | 未实现 |
| SR / codes | `pydicom/src/pydicom/sr/codedict.py`, `coding.py`, `_cid_dict.py`, `_concepts_dict.py`, `_snomed_dict.py` | `pydicom/tests/test_codes.py` | 未实现 |
| Overlay | `pydicom/src/pydicom/overlays/numpy_handler.py` | `pydicom/tests/test_overlay_np.py` | 未实现 |
| Waveform | `pydicom/src/pydicom/waveforms/numpy_handler.py` | `pydicom/tests/test_waveform.py` | 未实现 |
| Config | `pydicom/src/pydicom/config.py` | `pydicom/tests/test_config.py` | 未实现；Python 全局配置迁移前需确认 Go API |
| Hooks | `pydicom/src/pydicom/hooks.py` | `pydicom/tests/test_hooks.py` | 未实现；Python hook 机制迁移前需确认 Go API |
| CLI | `pydicom/src/pydicom/cli/main.py`, `cli/show.py`, `cli/codify.py` | `pydicom/tests/test_cli.py` | 部分实现，仅 read/readcopy |
| Data manager / test data | `pydicom/src/pydicom/data/data_manager.py`, `data/download.py`, `data/retry.py` | `pydicom/tests/test_data_manager.py` | 未实现，当前只复用 submodule 测试数据 |
| Env info / examples | `pydicom/src/pydicom/env_info.py`, `examples/__init__.py` | `pydicom/tests/test_env_info.py`, `pydicom/tests/test_examples.py` | 未实现 |
| Errors | `pydicom/src/pydicom/errors.py` | `pydicom/tests/test_errors.py` | 部分实现 |

## 当前测试与覆盖率

最新统计：

- Go 测试文件：11 个
- Go 顶层 `Test*`：约 73 个
- pydicom pytest 测试定义：约 2392 个
- pydicom pytest 文件：约 55 个
- `go test ./...`：通过
- 当前总覆盖率：约 **44.0% statements**
- 根包覆盖率：约 **50.6% statements**

### Go 测试覆盖现状

| Go 测试文件 | 覆盖领域 | 状态 |
|---|---|---|
| `tag_test.go` | 根包 Tag 构造/属性/私有 tag/JSON key | 部分覆盖 |
| `vr_test.go` | VR 分类集合 | 部分覆盖 |
| `uid_test.go` | 根包 UID 常量/校验 | 部分覆盖 |
| `dataelem_test.go` | DataElement 创建/VM/Name/Keyword/PN | 部分覆盖 |
| `dataset_test.go` | Dataset set/get/delete/typed getters/private block | 部分覆盖 |
| `values_test.go` | 基础 VR 转换 | 部分覆盖 |
| `datadict_test.go` | 字典查找/repeater | 部分覆盖 |
| `filereader_test.go` | 若干 pydicom 测试文件读取、roundtrip、StopBeforePixels/DeferSize | 浅层覆盖 |
| `filewriter_test.go` | 基础写入/readback/sequence/VR sample | 浅层覆盖 |
| `multival_test.go` | MultiValue 基础操作 | 部分覆盖 |
| `sequence_test.go` | Sequence 基础操作 | 部分覆盖 |

### 明显未覆盖或弱覆盖

- [ ] `tag` 子包自身无直接测试，完整生成的 5182 tag 常量未做覆盖校验测试
  - pydicom 参照：`pydicom/tests/test_tag.py`
  - 说明：子包是 Go 新增结构；但 tag 行为、json key、构造/比较等 pydicom 已覆盖，Go 还需增加子包级等价测试和生成覆盖校验
- [ ] `uid` 子包自身无直接测试，完整 UID 字典尚未实现
  - pydicom 参照：`pydicom/tests/test_uid.py`
  - pydicom 源码：`pydicom/src/pydicom/uid.py`, `pydicom/src/pydicom/_uid_dict.py`
- [ ] `charset.go` 覆盖率 0%，非 ASCII、多字符集、ISO-2022 场景未验证
  - pydicom 参照：`pydicom/tests/test_charset.py`, `pydicom/tests/test_unicode.py`
  - 重点测试：`test_encodings`, `test_nested_character_sets`, `test_inherited_character_set_in_sequence`, `test_single_byte_multi_charset_personname`, ISO-2022 escape/unknown charset/invalid decode 场景
- [ ] `filebase.go` / `fileutil.go` 大量 0% 覆盖，`DicomBytesIO.Bytes()` 当前未验证
  - pydicom 参照：`pydicom/tests/test_filebase.py`, `pydicom/tests/test_fileutil.py`
  - 重点测试：`DicomIO.read_tag/write_tag/read_US/write_US/read_UL/write_UL/read_exact`, `DicomBytesIO`, `buffer_length`, `buffer_equality`, `read_buffer`, `check_buffer`
- [ ] `Dataset.ToJSON()` / `writeJSONValue()` 覆盖率 0%，且仅为骨架实现
  - pydicom 参照：`pydicom/tests/test_json.py`
  - 重点测试：`test_to_json`, `test_from_json`, `test_json_from_dicom_file`, `test_roundtrip`, `InlineBinary`, `BulkDataURI`, PN JSON object, empty value handling
- [ ] `DataElement.String()` / `ReprValue()` / `Equal()` 未覆盖或弱覆盖
  - pydicom 参照：`pydicom/tests/test_dataelem.py`
  - 重点测试：`test_equality_standard_element`, `test_equality_private_element`, `test_equality_sequence_element`, `test_repeater_str`, `test_str_no_vr`, `test_repr_seq`, `test_repval_strange_type`, `test_empty_*`, `TestBufferedDataElement`
- [ ] Reader 的 `SpecificTags` 是公开选项，但当前未实现/未测试
  - pydicom 参照：`pydicom/tests/test_filereader.py`
  - 重点测试：`test_specific_tags`, `test_specific_tags_with_other_unknown_length_tags`, `test_specific_tags_with_unknown_length_SQ`, `test_specific_tags_with_unknown_length_tag`
- [ ] Reader 多数 pydicom 测试文件只验证 “能读不报错”，缺少值、VR、sequence、file meta、pixel data、transfer syntax 断言
  - pydicom 参照：`pydicom/tests/test_filereader.py`, `pydicom/tests/test_rawread.py`, 以及 pixel handler 相关测试
  - 重点测试：CT/MR/RT 文件的具体值断言、`test_CT_PixelData`, `test_read_file_meta_info`, file meta 缺失/异常 TransferSyntaxUID、undefined length SQ/raw read 场景
- [ ] Writer 多数测试只看 readback 元素数，缺少文件 meta、group length、padding、undefined length、ambiguous VR、roundtrip 字节级兼容测试
  - pydicom 参照：`pydicom/tests/test_filewriter.py`
  - 重点测试：`test_write_no_ts`, `test_write_double_filemeta`, `test_write_removes_grouplength`, `test_write_*_with_padding`, `test_write_OB_odd`, `test_write_new_ambiguous`, `test_correct_ambiguous_*`, `test_write_explicit_vr_big_endian`, `test_file_meta_*`, `test_read_write_identical`
- [ ] Big Endian、Deflated、compressed/encapsulated pixel data 缺少针对性测试
  - pydicom 参照：`pydicom/tests/test_filereader.py`, `pydicom/tests/test_rawread.py`, `pydicom/tests/test_encaps.py`, `pydicom/tests/test_rle_pixel_data.py`, `pydicom/tests/test_gdcm_pixel_data.py`, `pydicom/tests/test_pillow_pixel_data.py`, `pydicom/tests/test_pylibjpeg.py`, `pydicom/tests/pixels/test_decoder_*.py`, `pydicom/tests/pixels/test_encoder_*.py`
  - 重点测试：`test_deflate`, `test_explicit_VR_big_endian_no_meta`, `test_big_endian_explicit`, encaps fragment/frame/offset table 测试，各压缩 transfer syntax pixel decode/encode 测试


## pydicom 对比：大块缺失功能

### 最高优先级缺口

- [ ] **pydicom 动态属性迁移设计**
  - Python: `ds.PatientName`
  - Go 不能直接等价；涉及 API 设计时必须先问人类确认
  - 候选：`ds.GetString(tag.PatientName)`、`ds.StringValue(tag.PatientName)`、生成强类型 accessor、或其他方案

- [ ] **Dataset API 完整性**
  - pydicom 覆盖约 209 个 dataset 测试定义
  - Go 当前只有基础 map-backed dataset
  - 缺少/待确认：keyword access 替代设计、walk/recursive iteration、copy/clone、slice-like 行为、validation、file meta 行为、pixel helper、JSON roundtrip、private creator 完整语义

- [ ] **File Reader 完整性**
  - pydicom 有 `test_filereader.py` + `test_rawread.py`，约 130 个测试定义
  - Go 当前是基础读取器
  - 缺少/待实现：read_partial/raw/deferred 机制、SpecificTags、deflated transfer syntax、malformed file recovery、hooks/callbacks、完整 file meta 校验、compressed pixel/encapsulated 数据处理

- [ ] **File Writer 完整性**
  - pydicom `test_filewriter.py` 约 178 个测试定义
  - Go 当前是基础写入器
  - 缺少/待实现：file meta enforcement、group length/version、ambiguous VR 修正、padding/validation 边界、defined/undefined length 细节、完整 roundtrip 兼容

- [ ] **Value Representation / valuerep**
  - pydicom `test_valuerep.py` 约 117 个测试定义
  - Go 当前没有完整 `valuerep` 等价层
  - 缺少：DA/TM/DT 强类型、DSfloat/DSdecimal 设计、PersonName 完整行为、validation mode、original string preservation

### 高优先级缺口

- [ ] **Pixel Data 解码/编码**
  - pydicom legacy pixel handlers + 新 `pydicom.pixels` 测试约 900+ 个测试定义
  - Go 当前没有 `pixels` 包/handler pipeline
  - 缺少：native pixel array、RLE、JPEG/JPEG-LS/JPEG2000、GDCM/Pillow/pylibjpeg 等等价策略

- [ ] **Encapsulated Pixel Data**
  - pydicom `encaps.py` + `test_encaps.py` 约 164 个测试定义
  - Go 当前无等价实现
  - 缺少：Basic Offset Table、fragment/frame iterator、encapsulation generation、extended offset table

- [ ] **DICOM JSON Model**
  - pydicom `test_json.py` 约 30 个测试定义
  - Go 当前 `Dataset.ToJSON()` 是简单骨架
  - 缺少：Part 18 Annex F 兼容、from_json、BulkDataURI、InlineBinary、PN object formatting、roundtrip

- [ ] **Private Dictionary**
  - pydicom 有 `_private_dict.py`
  - Go 当前 `privateDictLookup`、`privateDictionaryVR` 是 placeholder
  - 需要生成 Go 私有字典或设计运行时扩展 API

- [ ] **UID 字典完整性**
  - pydicom 有 `_uid_dict.py` 和约 50 个 UID 测试定义
  - Go 当前只有少量 UID 常量/metadata
  - 需要完整 UID dictionary 生成与查询 API

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
| File reader/raw read | 130 | 基础实现/浅层测试 |
| File writer | 178 | 基础实现/浅层测试 |
| Value representation | 117 | 大量缺失 |
| Values conversion | 28 | 部分实现 |
| Pixel data legacy handlers | 239 | 未实现 |
| `pydicom.pixels` API | 689 | 未实现 |
| Encapsulation | 164 | 未实现 |
| Charset/Unicode | 34 | 部分实现/未覆盖 |
| File-set/DICOMDIR | 124 | 未实现 |
| UID | 50 | 部分实现 |
| Tag | 51 | 部分实现；Go 子包需补测试 |
| Dictionary | 18 | 标准字典部分实现；私有字典缺失 |
| JSON | 30 | 骨架 |
| CLI | 17 | 部分实现 |
| SR/codes | 23 | 未实现 |
| Sequence | 10 | 基础实现 |
| MultiValue | 17 | 基础实现 |
| Filebase/Fileutil | 31 | 部分实现/未覆盖 |

## 迁移规则

- [ ] 任何 Python 特有属性/动态行为迁移到 Go 前必须先问人类确认 API
  - 例如：`ds.PatientName`、动态 keyword access、Dataset 切片语义、config/hooks、pixel handler plugin、bulk data callback
- [ ] 不手写可由 pydicom 字典派生的数据；必须从源字典生成并验证覆盖
- [ ] 每实现一个 pydicom 功能块，必须同步迁移对应测试用例或建立 Go 等价测试
- [ ] 不以“能读不报错”作为完整验证；必须增加值、VR、VM、tag、file meta、transfer syntax、roundtrip 断言

## 建议下一步

1. **补测试基础设施**
   - [ ] 为 `tag` 子包补完整生成覆盖测试
   - [ ] 为 `uid` 子包补测试
   - [ ] 为 `charset.go`、`filebase.go`、`fileutil.go` 补基础测试

2. **优先补齐 metadata 读写闭环**
   - [ ] 实现并测试 `ReadOptions.SpecificTags`
   - [ ] 完善 file meta 写入/校验
   - [ ] 完善 Big Endian / Deflated 测试与实现
   - [ ] 增加典型 pydicom 文件的值级断言

3. **确认 Python 动态 API 的 Go 设计**
   - [ ] 确认 `ds.PatientName` 在 Go 中的推荐 API
   - [ ] 确认 Dataset keyword access、typed accessor、生成 accessor 是否需要

4. **再进入大块功能**
   - [ ] DICOM JSON Model
   - [ ] Private Dictionary
   - [ ] UID Dictionary
   - [ ] Native Pixel Data
   - [ ] Encapsulated Pixel Data
