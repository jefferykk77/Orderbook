# 2026-09-28 Profile 记录

这轮运行用于验证 `-Profile` 路径，并为 `BenchmarkLimitOrderMatch` 保留 CPU 与内存 profile。参数是 `-Seconds 1 -Count 1 -Profile`，完整原始输出在 [bench.txt](bench.txt)。

## CPU 观察

`go tool pprof -top profiles/cpu.out` 显示：

| 项 | 累计占比 |
| --- | ---: |
| `orderbook.(*OrderBook).ProcessLimitOrder` | 70.04% |
| `github.com/shopspring/decimal.Decimal.Add` | 20.22% |
| `github.com/shopspring/decimal.Decimal.Div` | 19.85% |
| `orderbook.(*OrderBook).processQueue` | 16.10% |
| `runtime.mallocgc` | 17.23% |

限价撮合的热点集中在 decimal 运算、订单对象创建和 GC。特别是成交后计算均价的 `Div`，以及队列部分成交时的 `Sub/Add`。

## 内存观察

`go tool pprof -sample_index=alloc_space -top profiles/mem.out` 显示：

| 项 | 分配占比 |
| --- | ---: |
| `math/big.nat.make` | 31.57% |
| `orderbook.NewOrder` | 21.81% |
| `decimal.Decimal.Add` | 13.22% |
| `decimal.Decimal.Sub` | 8.93% |
| `decimal.Decimal.rescale` | 8.60% |
| `decimal.Decimal.QuoRem` | 8.57% |

结论：若后续要优化撮合吞吐，优先方向是减少每笔成交创建的 `Order` 对象、避免不必要的 decimal 大数运算，并重新审视成交均价的精度/舍入策略。优化后必须用相同参数重跑并保留新的 profile。
