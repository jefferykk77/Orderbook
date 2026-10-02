# 2026-09-28 基线记录

## 目的

为当前撮合引擎建立第一份可复现压测基线，覆盖挂单/撤单、限价撮合、市价撮合、深度计算、序列化、文本输出和独立订单簿并行吞吐。

## 环境与代码

| 项目 | 值 |
| --- | --- |
| 时间 | 2026-09-28 18:59:22 +08:00 |
| 分支 | `feat/注释翻译` |
| 提交 | `aeec46e`，另有本次新增压测文件 |
| OS/CPU | Windows amd64，AMD Ryzen 7 8845H，8 核 16 线程 |
| Go | `go1.27.1 windows/amd64` |
| 参数 | `-Seconds 1 -Count 3` |

机器处于日常桌面环境，未专门进入安静模式。该结果适合作为教学和快速回归基线；对外发布或对比优化时，建议用 `-Seconds 3 -Count 10` 重跑。

## 结果摘要

| 基准 | 平均耗时 | 平均吞吐 | 分配 |
| --- | ---: | ---: | ---: |
| `LimitOrderNoCross` | 1294.7 ns/op | 77.2 万轮/s，每轮含挂单和撤单 | 664 B/op，23 allocs/op |
| `LimitOrderMatch` | 1010.9 ns/op | 98.9 万单/s | 720 B/op，26 allocs/op |
| `MarketOrderPartialFill` | 242.2 ns/op | 412.8 万单/s | 200 B/op，7 allocs/op |
| `CalculateMarketPrice` | 1344.3 ns/op | 74.4 万次/s | 960 B/op，36 allocs/op |
| `Depth` | 26421.7 ns/op | 3.78 万次/s | 14752 B/op，710 allocs/op |
| `MarshalJSON` | 2695.6 us/op | 371.1 次/s | 约 1.55 MB/op，14759 allocs/op |
| `ParallelIndependentBooks` | 343.2 ns/op | 291.4 万单/s 聚合 | 720 B/op，26 allocs/op |
| `BookString` | 70528.7 ns/op | 1.42 万次/s | 29281 B/op，1717 allocs/op |

当前最明显的重路径是 JSON 序列化和 `String()` 调试输出。核心撮合路径的分配次数也偏高，后续优化时应优先看 `pprof` 的 CPU 和 alloc profile。

## 验证

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test -race ./...` | 未运行到测试断言，测试二进制以 `exit status 0xc0000139` 退出；最小用例同样失败，表现为本机 Windows race runtime 初始化问题 |

`benchstat` 当前未安装。后续需要统计对比时，可执行 `go install golang.org/x/perf/cmd/benchstat@latest`。

同日的 profile 记录在 [../20260928-190446/README.md](../20260928-190446/README.md)。
