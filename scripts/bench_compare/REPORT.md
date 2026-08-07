# DICOM 性能对比报告

最后更新：**2026-08-07**

## 环境

| 项 | 值 |
|---|---|
| 平台 | arm64 |
| pydicom | 3.0.2 |
| pynetdicom | 3.1.0.dev0 |
| DCMTK | $dcmtk: dcmdump v3.7.0 2025-12-15 $ |

方法：本地 loopback TCP；C-STORE 为**单 association** 内重复发送同一实例（`storescu --repeat` / 等价逻辑）；
文件 I/O 为 median of 15 runs（warmup 3）。

## 文件 I/O

### 读 metadata（StopBeforePixels / dcmdump --quiet）

| 测试文件 | pydicom | godicom | DCMTK |
|---|---:|---:|---:|
| CT_small 38KB native | 0.41 ms | 0.44 ms | 10.20 ms |
| JPGExtended JPEG | 0.35 ms | 0.27 ms | 10.08 ms |
| MR JPEG-LS | 0.19 ms | 0.13 ms | 9.18 ms |
| MR_small_RLE 8KB | 0.19 ms | 0.30 ms | 8.78 ms |
| RTImageStorage 2MB | 0.16 ms | 0.09 ms | 9.49 ms |

### 解码像素（pydicom pixel_array / godicom PixelBytes / dcmj2pnm）

| 测试文件 | pydicom | godicom | DCMTK |
|---|---:|---:|---:|
| CT_small 38KB native | 0.56 ms | 0.42 ms | 18.05 ms |
| JPGExtended JPEG | 1.34 ms | 1.43 ms | 39.89 ms |
| MR JPEG-LS | 0.61 ms | 0.47 ms | 16.60 ms |
| MR_small_RLE 8KB | 0.35 ms | 0.25 ms | 16.84 ms |
| RTImageStorage 2MB | 0.38 ms | 0.72 ms | 62.56 ms |

**结论**：pydicom 与 godicom 在同一量级，均显著快于 DCMTK CLI 管道。

## 网络 C-STORE 吞吐

对象：`CTImageStorage.dcm`（约 38KB）。
横向对比时 **SCU 行** 表示该栈作为发送方（对端固定 DCMTK `storescp --ignore`），
**SCP 行** 表示该栈作为接收方（对端固定 DCMTK `storescu --repeat`）。
**loopback** 为同栈 SCU↔SCP。

### DCMTK

| 角色 | ×100 | ×1000 |
|---|---:|---:|
| SCU（→ DCMTK SCP） | 1,260 obj/s | 1,826 obj/s |
| SCP（← DCMTK SCU） | 1,233 obj/s | 1,838 obj/s |
| loopback | 1,219 obj/s | 1,828 obj/s |

### pynetdicom

| 角色 | ×100 | ×1000 |
|---|---:|---:|
| SCU（→ DCMTK SCP） | 260 obj/s | 286 obj/s |
| SCP（← DCMTK SCU） | 237 obj/s | 256 obj/s |
| loopback | 133 obj/s | 137 obj/s |

### gonetdicom

| 角色 | ×100 | ×1000 |
|---|---:|---:|
| SCU（→ DCMTK SCP） | 2,539 obj/s | 5,024 obj/s |
| SCP（← DCMTK SCU） | 1,482 obj/s | 2,066 obj/s |
| loopback | 4,322 obj/s | 5,565 obj/s |

**结论**：
- gonetdicom SCU 可达 **~5k obj/s**（×1000，对 DCMTK SCP），SCP ~2k obj/s。
- DCMTK 自环 ~1.9k obj/s，作为通用对端基准合理。
- pynetdicom 无论 SCU/SCP 均在 **~250–290 obj/s**，常为混合栈场景瓶颈。

## 复现

```bash
cd scripts/bench_compare
./run.sh          # 跑基准并更新 REPORT.md
python3 bench.py  # 仅输出 JSON
```

依赖见 [README.md](./README.md)。
