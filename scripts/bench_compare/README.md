# DICOM 性能对比基准

对比 **godicom**、**gonetdicom**、**pydicom**、**pynetdicom**、**DCMTK** 的文件 I/O 与 C-STORE 网络吞吐。

结果见 [REPORT.md](./REPORT.md)（运行 `./run.sh` 可刷新）。

## 依赖

### Go

- 本仓库（`replace` 到 `../..`）
- 同级目录 [gonetdicom](https://github.com/godicom-dev/gonetdicom)（`replace` 到 `../../../gonetdicom`）
- `pydicom` submodule 测试文件：`git submodule update --init pydicom`

### Python

```bash
pip install pydicom pynetdicom pylibjpeg pylibjpeg-libjpeg pylibjpeg-openjpeg pillow
```

pynetdicom 可使用 gonetdicom 仓库内 vendored 版本：

```bash
pip install -e ../gonetdicom/pynetdicom
```

### DCMTK

macOS Homebrew：`brew install dcmtk`

需支持 `storescu --repeat`。若不在默认 PATH，设置：

```bash
export DCMTK_BIN=/opt/homebrew/bin
```

## 运行

```bash
cd scripts/bench_compare
./run.sh              # 完整基准 + 更新 REPORT.md
python3 bench.py      # JSON 到 stdout
go run .              # 仅 godicom 文件 I/O + gonetdicom 网络部分
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `GODICOM_ROOT` | godicom 仓库根目录（默认自动推断） |
| `GONETDICOM_ROOT` | gonetdicom 仓库根目录（默认同级 `../gonetdicom`） |
| `DCMTK_BIN` | DCMTK 可执行文件目录 |

## 网络基准说明

C-STORE 按**栈**分列（见 REPORT），而非 SCU→SCP 交叉矩阵：

- **SCU**：该栈发送，对端为 DCMTK `storescp --ignore`
- **SCP**：该栈接收，对端为 DCMTK `storescu --repeat`
- **loopback**：同栈 SCU↔SCP

对象：`gonetdicom/pynetdicom/.../CTImageStorage.dcm`（约 38KB），单 association 内重复发送。
