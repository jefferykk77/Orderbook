# 撮合引擎压测指南

这个仓库是一个 Go 库，没有 HTTP 入口，所以压测入口使用 Go 基准测试，而不是 `wrk`、`k6` 或 `ab` 这类网络负载工具。基准文件在 [benchmark_test.go](../benchmark_test.go)，每个用例都保持固定深度和固定订单路径，避免因为测试循环不断加深订单簿导致前后数据不可比。

## 压测负载

| 基准名 | 场景 | 关注指标 |
| --- | --- | --- |
| `BenchmarkLimitOrderNoCross` | 买入价 1，不触发成交，随后撤单 | 挂单加撤单的基础开销 |
| `BenchmarkLimitOrderMatch` | 买单按 100 吃一个巨额卖单，持续部分成交 | 核心撮合路径 |
| `BenchmarkMarketOrderPartialFill` | 市价买单持续吃同一个巨额卖单 | 市价单路径 |
| `BenchmarkCalculateMarketPrice` | 在 50 档、1000 笔订单中计算成交价 | 深度遍历 |
| `BenchmarkDepth` | 生成两侧各 50 档深度快照 | 查询和分配开销 |
| `BenchmarkMarshalJSON` | 序列化完整订单簿 | 序列化瓶颈 |
| `BenchmarkParallelIndependentBooks` | 每个 worker 独立订单簿并行撮合 | 多订单簿分片时的扩展性 |
| `BenchmarkBookString` | 生成人类可读订单簿文本 | 调试输出开销 |

注意：`OrderBook` 本身没有内部锁。多个 goroutine 同时修改同一个 `OrderBook` 会产生数据竞争，这不是有效压测。并发基准使用的是“每个 goroutine 一个独立订单簿”的模型，适合一个交易品种一个订单簿的分片架构。

## 快速运行

```powershell
cd D:\trade\OrderBook\orderbook
go test ./...
go test -run '^$' -bench . -benchmem -benchtime 1s -count 3
```

推荐使用 `count` 至少为 3。单次结果容易受电源模式、后台进程、Windows 更新和 CPU 频率波动影响。正式记录时使用脚本：

```powershell
cd D:\trade\OrderBook\orderbook
.\scripts\run-bench.ps1 -Seconds 1 -Count 3
```

脚本会把结果写入 `benchmarks/<时间戳>/bench.txt`，并记录时间、Go 版本、操作系统、CPU、逻辑核心数、模块路径、提交号、测试结果和完整命令。需要 CPU 与内存 profile 时加 `-Profile`：

```powershell
.\scripts\run-bench.ps1 -Seconds 3 -Count 1 -Profile
```

Profile 会保存在 `benchmarks/<时间戳>/profiles/`，可用以下命令分析：

```powershell
go tool pprof -http=:0 .\benchmarks\<时间戳>\profiles\cpu.out
go tool pprof -http=:0 .\benchmarks\<时间戳>\profiles\mem.out
```

文本模式可省略 `-http=:0`，例如在模块根目录执行：

```powershell
go tool pprof -top .\benchmarks\<时间戳>\profiles\cpu.out
go tool pprof -sample_index=alloc_space -top .\benchmarks\<时间戳>\profiles\mem.out
```

## 对比两次结果

安装 `benchstat` 后可以做统计对比：

```powershell
go install golang.org/x/perf/cmd/benchstat@latest
benchstat .\benchmarks\旧时间戳\bench.txt .\benchmarks\新时间戳\bench.txt
```

重点看三列：

| 指标 | 含义 |
| --- | --- |
| `ns/op` | 每轮操作耗时，越小越好；每秒吞吐约为 `1,000,000,000 / ns/op` |
| `B/op` | 每轮操作分配的字节数，越小越好 |
| `allocs/op` | 每轮分配次数，越小越好 |

不要只看吞吐百分比。若 `allocs/op` 明显上升，即使耗时暂时下降，高负载下也可能放大 GC 压力。遇到多个基准回归时，优先用 profile 确认热点集中在哪个函数。

## 记录规范

每次正式压测建议在 `benchmarks/<时间戳>/README.md` 中补充：

| 字段 | 示例 |
| --- | --- |
| 目的 | 验证某个优化或回归 |
| 代码状态 | commit、分支、是否有本地修改 |
| 机器状态 | 电源模式、插电、后台负载 |
| 参数 | `-Seconds 3 -Count 10` |
| 结论 | 哪些场景提升或回归，幅度是多少 |
| 后续动作 | 是否需要 profile、是否需要修复 |

发布或归档时保留原始 `bench.txt`，不要只保留整理后的表格。原始输出包含 `goos`、`goarch`、`pkg`、`cpu` 和逐轮数据，是之后复现和审计的依据。
