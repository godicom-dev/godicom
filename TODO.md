# godicom TODO

## 项目状态

godicom 是 pydicom 的 Go 移植版本，当前完成第一期核心读写功能。

## 已完成

- [x] 核心类型：Tag, VR, UID, errors
- [x] 数据模型：DataElement, Dataset, Sequence, MultiValue, PersonName
- [x] 值转换：所有 VR 的 bytes→Go 类型转换（values.go）
- [x] 字符编码：DICOM 字符集支持（ISO-8859-x, Shift-JIS, GBK, GB18030, Big5 等）
- [x] DICOM 字典：5189 个标准 Tag + 88 个 Repeater（从 pydicom AST 解析自动生成）
- [x] 文件读取：Explicit/Implicit VR, Little/Big Endian, 混合编码自动切换
- [x] 文件写入：所有 VR 编码写入
- [x] I/O 基础：内存读写、字节序处理
- [x] CLI 工具：基本命令行读取

**验证结果：**
- 78 个 pydicom 测试 DICOM 文件全部读取成功，0 失败
- 140 个 Go 单元测试全部通过（tag / vr / uid / dataelem / dataset / values / datadict / filereader）

## 测试覆盖情况

| pydicom 测试模块 | Go 对应 | 状态 |
|---|---|---|
| `test_tag.py` | `tag_test.go` | ✅ |
| `test_valuerep.py` | `vr_test.go` | ✅ |
| `test_uid.py` | `uid_test.go` | ✅ |
| `test_dataelem.py` | `dataelem_test.go` | ✅ |
| `test_dataset.py` | `dataset_test.go` | ✅ |
| `test_values.py` | `values_test.go` | ✅ |
| `test_dictionary.py` | `datadict_test.go` | ✅ |
| `test_filereader.py` | `filereader_test.go` | ✅ |
| `test_multival.py` | — | ❌ |
| `test_sequence.py` | — | ❌ |
| `test_filewriter.py` | — | ❌ |
| `test_filebase.py` | — | ❌ |
| `test_fileutil.py` | — | ❌ |
| `test_charset.py` | — | ❌ |
| `test_config.py` | — | ❌ |
| `test_json.py` | — | ❌ |

## 待完成

### 高优先级

- [ ] **Deflated Explicit VR LE** 支持（`1.2.840.10008.1.2.1.99`）
  - 需要 zlib 解压整个 dataset
  - 参考 pydicom `read_partial` 的 deflate 处理
- [ ] **Big Endian 文件读取完善**
  - `ExplVR_BigEnd.dcm`、`MR_small_bigendian.dcm` 等文件元素数偏少
  - 需要验证 Explicit VR Big Endian 的 VR 位置（pydicom 用 `<HH2s` 始终按 LE 读 VR）
- [ ] **写入 roundtrip 验证**
  - 读取→写入→再读取，验证元素数一致
  - 需要处理文件 meta 信息写入

### 中优先级

- [ ] **Pixel Data 解码（Native/无压缩）**
  - 第一期约定只做 Native 传输语法
  - 从 Dataset 中提取像素数组
- [ ] **JSON 序列化/反序列化**
  - `Dataset.ToJSON()` 已实现骨架，需完善和测试
  - DICOM JSON Model (Part 18, Annex F)
- [ ] **Private Tag 字典支持**
  - `_private_dict.py` 有 11450 行私有 tag 定义
  - 需要生成 Go 版本或运行时加载
- [ ] **配置系统**
  - 验证模式（RAISE/WARN/IGNORE）
  - 日期时间转换开关
  - DS 精度选择（float vs decimal）

### 低优先级

- [ ] **Structured Reporting 支持**
  - CID/概念/SNOMED 字典（~88K LOC Python，纯数据）
- [ ] **Waveform 数据处理**
- [ ] **Overlay 数据处理**
- [ ] **DICOMDIR / File-set 处理**
- [ ] **DICOMweb / WADO-RS 支持**

## 架构说明

```
godicom/
├── tag.go              # Tag 类型
├── vr.go               # VR 类型及分类集合
├── uid.go              # UID 类型及已知 UID
├── errors.go           # 错误类型
├── dataelem.go         # DataElement, RawDataElement, PersonName
├── dataset.go          # Dataset, FileDataset, FileMetaDataset, PrivateBlock
├── sequence.go         # Sequence
├── multival.go         # MultiValue
├── values.go           # 值转换（bytes → Go 类型）
├── charset.go          # DICOM 字符编码
├── datadict.go         # 字典查询接口
├── dicom_dict_generated.go  # 自动生成的 DICOM 字典
├── filebase.go         # I/O 基础
├── fileutil.go         # 文件工具函数
├── filereader.go       # DICOM 文件读取
├── filewriter.go       # DICOM 文件写入
├── godicom.go          # 包文档
├── generate_dict.py    # 字典生成脚本（从 pydicom/_dicom_dict.py）
├── cmd/
│   └── godicom/main.go # CLI 工具
├── pixels/             # Pixel Data（预留）
└── pydicom/            # pydicom 源码（参考/字典生成/测试数据）
```
