# 2026-09-28 Profile 记录

这轮运行验证了最终版 `-Profile` 路径，并为 `BenchmarkLimitOrderMatch` 保留 CPU、内存 profile 和对应测试二进制。参数是 `-Seconds 1 -Count 1 -Profile`，原始输出在 [bench.txt](bench.txt)。

## CPU 观察

`go tool pprof -top .\profiles\cpu.out` 显示：

| 项 | 累计占比 |
| --- | ---: |
| `orderbook.(*OrderBook).ProcessLimitOrder` | 65.04% |
| `github.com/shopspring/decimal.Decimal.Add` | 23.58% |
| `github.com/shopspring/decimal.Decimal.Div` | 15.04% |
| `orderbook.(*OrderBook).processQueue` | 17.07% |
| `runtime.mallocgc` | 17.07% |

热点集中在 decimal 运算、订单对象创建和 GC。成交后计算均价的 `Div`，以及部分成交时的 `Sub/Add`，都是明确的优化入口。

## 内存观察

`go tool pprof -sample_index=alloc_space -top .\profiles\mem.out` 显示：

| 项 | 分配占比 |
| --- | ---: |
| `math/big.nat.make` | 31.96% |
| `orderbook.NewOrder` | 21.84% |
| `decimal.Decimal.Add` | 13.12% |
| `decimal.Decimal.QuoRem` | 9.06% |
| `decimal.Decimal.Sub` | 8.79% |
| `decimal.Decimal.rescale` | 8.56% |

后续若要优化吞吐，优先减少每笔成交创建的 `Order` 对象、避免不必要的 decimal 大数运算，并重新审视均价精度与舍入策略。改动后用相同参数重跑，并用 `benchstat` 对比两次原始结果。
