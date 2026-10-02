package service

import (
	"fmt"

	"github.com/shopspring/decimal"
	"orderbook"
	tradepb "orderbook/api/proto/trade"
)

// coreSide 把 proto 的买卖方向转换成撮合核心使用的方向。
// 两边枚举值不同，必须显式映射，不能直接强转。
func coreSide(side tradepb.Side) (orderbook.Side, error) {
	switch side {
	case tradepb.Side_SIDE_BUY:
		return orderbook.Buy, nil
	case tradepb.Side_SIDE_SELL:
		return orderbook.Sell, nil
	default:
		return orderbook.Sell, fmt.Errorf("无效的买卖方向: %s", side)
	}
}

// parseDecimal 解析 proto 中用字符串承载的十进制数。
func parseDecimal(value, field string) (decimalValue decimal.Decimal, err error) {
	decimalValue, err = decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%s 不是合法数字: %w", field, err)
	}
	return decimalValue, nil
}

// cloneOrder 返回业务订单的拷贝，避免 gRPC 序列化和后续成交更新竞争同一块内存。
func cloneOrder(order *tradepb.Order) *tradepb.Order {
	if order == nil {
		return nil
	}
	copied := *order
	return &copied
}

// cloneFill 返回成交记录的拷贝。
func cloneFill(fill *tradepb.Fill) *tradepb.Fill {
	if fill == nil {
		return nil
	}
	copied := *fill
	return &copied
}

// cloneFills 返回成交记录切片的拷贝。
func cloneFills(fills []*tradepb.Fill) []*tradepb.Fill {
	if len(fills) == 0 {
		return nil
	}
	copied := make([]*tradepb.Fill, 0, len(fills))
	for _, fill := range fills {
		copied = append(copied, cloneFill(fill))
	}
	return copied
}
