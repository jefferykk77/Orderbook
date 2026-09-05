# Go orderbook

用 Go（Golang）实现的改进版撮合引擎

> 本文为 [README.md](README.md) 的中文译本。

[![Go Report Card](https://goreportcard.com/badge/github.com/i25959341/orderbook)](https://goreportcard.com/report/github.com/i25959341/orderbook)
[![GoDoc](https://godoc.org/github.com/i25959341/orderbook?status.svg)](https://godoc.org/github.com/i25959341/orderbook)
[![gocover.run](https://gocover.run/github.com/i25959341/orderbook.svg?style=flat&tag=1.10)](https://gocover.run?tag=1.10&repo=github.com%2Fi25959341%2Forderbook)
[![Stability: Active](https://masterminds.github.io/stability/active.svg)](https://masterminds.github.io/stability/active.html)
[![Build Status](https://travis-ci.org/i25959341/orderbook.svg?branch=master)](https://travis-ci.org/i25959341/orderbook)

## 特性

- 标准价格-时间优先（price-time priority）
- 同时支持市价单和限价单
- 支持撤单
- 高性能（每秒超过 30 万笔成交）
- 内存占用优化
- 支持 JSON 序列化与反序列化
- 可按指定数量计算市价

## 用法

使用订单簿时，先创建对象：

```go
import (
  "fmt" 
  ob "github.com/muzykantov/orderbook"
)

func main() {
  orderBook := ob.NewOrderBook()
  fmt.Println(orderBook)
}

```

然后即可调用以下核心方法：

```go

func (ob *OrderBook) ProcessLimitOrder(side Side, orderID string, quantity, price decimal.Decimal) (done []*Order, partial *Order, err error) { ... }

func (ob *OrderBook) ProcessMarketOrder(side Side, quantity decimal.Decimal) (done []*Order, partial *Order, quantityLeft decimal.Decimal, err error) { .. }

func (ob *OrderBook) CancelOrder(orderID string) *Order { ... }

```

## 核心方法说明

### ProcessLimitOrder

```go
// ProcessLimitOrder 将新订单放入订单簿
// 参数：
//      side     - 买卖方向（ob.Sell 或 ob.Buy）
//      orderID  - 盘口中的唯一订单 ID
//      quantity - 希望买入或卖出的数量
//      price    - 限价：买入不高于该价，卖出不低于该价
//      * 创建 decimal 请使用 decimal.New()
//        详见 https://github.com/shopspring/decimal
// 返回值：
//      error   - 数量（或价格）小于等于 0，或给定 ID 的订单已存在时不为 nil
//      done    - 若你的订单导致其他订单完全成交，这些订单会加入 "done" 切片。
//                若你的订单本身也完全成交，同样会放入该数组
//      partial - 若你的订单已处理但对手盘最优订单未完全成交；或你的订单部分成交后
//                以剩余数量挂入订单簿，则 partial 为该剩余订单
//      partialQuantityProcessed - 当 partial 不为 nil 时，表示该部分成交订单已成交的数量
func (ob *OrderBook) ProcessLimitOrder(side Side, orderID string, quantity, price decimal.Decimal) (done []*Order, partial *Order, err error) { ... }
```

示例：
```
ProcessLimitOrder(ob.Sell, "uinqueID", decimal.New(55, 0), decimal.New(100, 0))

asks: 110 -> 5      110 -> 5
      100 -> 1      100 -> 56
--------------  ->  --------------
bids: 90  -> 5      90  -> 5
      80  -> 1      80  -> 1

done    - nil
partial - nil

```

```
ProcessLimitOrder(ob.Buy, "uinqueID", decimal.New(7, 0), decimal.New(120, 0))

asks: 110 -> 5
      100 -> 1
--------------  ->  --------------
bids: 90  -> 5      120 -> 1
      80  -> 1      90  -> 5
                    80  -> 1

done    - 2（或更多订单）
partial - uinqueID 订单

```

```
ProcessLimitOrder(ob.Buy, "uinqueID", decimal.New(3, 0), decimal.New(120, 0))

asks: 110 -> 5
      100 -> 1      110 -> 3
--------------  ->  --------------
bids: 90  -> 5      90  -> 5
      80  -> 1      90  -> 5

done    - 1 笔价格为 100 的订单（可能还有若干价格为 110 的订单）+ uinqueID 订单
partial - 1 笔价格为 110 的订单

```

### ProcessMarketOrder

```go
// ProcessMarketOrder 按市价立即从订单簿吃掉指定数量
// 参数：
//      side     - 买卖方向（ob.Sell 或 ob.Buy）
//      quantity - 希望买入或卖出的数量
//      * 创建 decimal 请使用 decimal.New()
//        详见 https://github.com/shopspring/decimal
// 返回值：
//      error        - 价格小于等于 0 时不为 nil
//      done         - 若市价单导致其他订单完全成交，这些订单会加入 "done" 切片
//      partial      - 若你的订单已处理但对手盘最优订单未完全成交，则不为 nil
//      partialQuantityProcessed - 当 partial 不为 nil 时，表示该部分成交订单已成交的数量
//      quantityLeft - 盘口深度不足以吃完全部数量时大于 0
func (ob *OrderBook) ProcessMarketOrder(side Side, quantity decimal.Decimal) (done []*Order, partial *Order, quantityLeft decimal.Decimal, err error) { .. }
```

示例：
```
ProcessMarketOrder(ob.Sell, decimal.New(6, 0))

asks: 110 -> 5      110 -> 5
      100 -> 1      100 -> 1
--------------  ->  --------------
bids: 90  -> 5      80 -> 1
      80  -> 2

done         - 2（或更多订单）
partial      - 1 笔价格为 80 的订单
quantityLeft - 0

```

```
ProcessMarketOrder(ob.Buy, decimal.New(10, 0))

asks: 110 -> 5
      100 -> 1
--------------  ->  --------------
bids: 90  -> 5      90  -> 5
      80  -> 1      80  -> 1
                    
done         - 2（或更多订单）
partial      - nil
quantityLeft - 4

```

### CancelOrder

```go
// CancelOrder 按 ID 从订单簿中移除订单
func (ob *OrderBook) CancelOrder(orderID string) *Order { ... }
```

```
CancelOrder("myUinqueID-Sell-1-with-100")

asks: 110 -> 5
      100 -> 1      110 -> 5
--------------  ->  --------------
bids: 90  -> 5      90  -> 5
      80  -> 1      80  -> 1

返回 - 被撤销的卖单（价格 100，数量 1）；找不到该 ID 时为 nil

```

## 许可证

MIT License (MIT)

详见 LICENSE 与 AUTHORS 文件
