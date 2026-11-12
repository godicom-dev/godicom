- 以瞎猜接口为耻，以认真查询为荣。
- 以模糊执行为耻，以寻求确认为荣。
- 以臆想业务为耻，以人类确认为荣。
- 以创造接口为耻，以复用现有为荣。
- 以跳过验证为耻，以主动测试为荣。
- 以破坏架构为耻，以遵循规范为荣。
- 以假装理解为耻，以诚实无知为荣。
- 以盲目修改为耻，以谨慎重构为荣。
- pydicom 迁移必须严格参考 pydicom 源码和测试；每个功能先定位对应的 pydicom 文件和测试。
- 遇到 Python 语法/动态特性与 Go 设计不一致时，必须先向人类确认 API，不得自作主张。

## pydicom → Go 决策规则

判断依据只有一条：**删掉这个 Python 构造，Go 还能不能表达同样的语义？**
能，就只抄行为；不能，就抄形状，并在注释里写清为什么不能照搬。

| pydicom 构造 | Go 做法 | 取值 |
|---|---|---|
| 抛异常（`ValueError`） | 返回 `error`，需要判别时定义错误类型 | 抄行为 |
| `config.validation_mode` 全局枚举 | `OnDiagnostic` 回调返回值决定继续/中断 | 抄行为 |
| 校验返回 `(bool, str)` 元组 | 返回 `[]string`，空即通过 | 抄形状 |
| `value: Any` + `isinstance` 分派 | 静态类型 + 按 VR 的 `map[VR]func` 表 | 抄形状 |
| `ds.PatientName` 动态属性 | 类型化 getter + tag 常量 | 不移植 |
| 模块级全局可变状态 | options struct，零值即默认 | 抄行为 |
| 关键字默认参数 | struct 零值字段 | 抄行为 |

抄形状时保持**顺序、数量、字面量**一致，parity 测试才逐条对得上；抄行为时按 Go 习惯重写签名，但语义必须能反推回原文。任何有意的偏离都要在代码注释里写明「与 pydicom 不同之处 + 原因」，并配一条测试锁住该行为。

## 推送与发版纪律

- **推送成功的唯一判据是 `git ls-remote` 报出的 SHA 等于本地 SHA。** 不是退出码，也不是 `Everything up-to-date`：代理不稳时 git 会在协议层失败（`RPC failed; curl 52`），下一次尝试又会回报成功，而远端仍停在旧提交。用 `scripts/push.sh` 代替裸 `git push`。
- **不要在环境里设 `HTTP_PROXY` 跑 git。** 需要代理时用 `GIT_PROXY=http://host:port scripts/push.sh`，让 `-c http.proxy` 只作用于这次调用；环境变量会让每个请求都走代理并产生 502。
- **打 tag 不保证被验证。** `ci.yml` 的 tag 触发只跑 test matrix，其余 job 在 main 上已跑过同一提交；而且 tag 触发要等 workflow 本身已在默认分支上才生效。发版顺序固定为：补齐 CHANGELOG → 推 main → 等 main 的 run 绿 → 打并推 tag → 再看 tag 的 run。
- **发版前核对 `CHANGELOG [Unreleased]` 是否覆盖本次全部用户可见变更**（新增导出 API、行为变更、修复）。Release 正文逐字取自该版本段，漏记就是永久漏记。
- 生成 fixture 优先用 pydicom 参照实现而非 godicom 自己，否则编码和解码共享同一个错误，测试只会互相印证。

