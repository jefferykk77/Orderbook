package orderbook

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Order strores information about request
// Order 保存一笔委托的信息
type Order struct {
	side      Side
	id        string
	timestamp time.Time
	quantity  decimal.Decimal
	price     decimal.Decimal
}

// MarketView represents order book in a glance
// MarketView 是订单簿的概览
type MarketView struct {
	Asks map[string]decimal.Decimal `json:"asks"`
	Bids map[string]decimal.Decimal `json:"bids"`
}

// NewOrder creates new constant object Order
// NewOrder 创建不可变的 Order 对象
func NewOrder(orderID string, side Side, quantity, price decimal.Decimal, timestamp time.Time) *Order {
	return &Order{
		id:        orderID,
		side:      side,
		quantity:  quantity,
		price:     price,
		timestamp: timestamp,
	}
}

// ID returns orderID field copy
// ID 返回 orderID 字段的副本
func (o *Order) ID() string {
	return o.id
}

// Side returns side of the order
// Side 返回订单的买卖方向
func (o *Order) Side() Side {
	return o.side
}

// Quantity returns quantity field copy
// Quantity 返回 quantity 字段的副本
func (o *Order) Quantity() decimal.Decimal {
	return o.quantity
}

// Price returns price field copy
// Price 返回 price 字段的副本
func (o *Order) Price() decimal.Decimal {
	return o.price
}

// Time returns timestamp field copy
// Time 返回 timestamp 字段的副本
func (o *Order) Time() time.Time {
	return o.timestamp
}

// String implements Stringer interface
// String 实现 Stringer 接口
func (o *Order) String() string {
	return fmt.Sprintf("\n\"%s\":\n\tside: %s\n\tquantity: %s\n\tprice: %s\n\ttime: %s\n", o.ID(), o.Side(), o.Quantity(), o.Price(), o.Time())
}

// MarshalJSON implements json.Marshaler interface
// MarshalJSON 实现 json.Marshaler 接口
func (o *Order) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		&struct {
			S         Side            `json:"side"`
			ID        string          `json:"id"`
			Timestamp time.Time       `json:"timestamp"`
			Quantity  decimal.Decimal `json:"quantity"`
			Price     decimal.Decimal `json:"price"`
		}{
			S:         o.Side(),
			ID:        o.ID(),
			Timestamp: o.Time(),
			Quantity:  o.Quantity(),
			Price:     o.Price(),
		},
	)
}

// UnmarshalJSON implements json.Unmarshaler interface
// UnmarshalJSON 实现 json.Unmarshaler 接口
func (o *Order) UnmarshalJSON(data []byte) error {
	obj := struct {
		S         Side            `json:"side"`
		ID        string          `json:"id"`
		Timestamp time.Time       `json:"timestamp"`
		Quantity  decimal.Decimal `json:"quantity"`
		Price     decimal.Decimal `json:"price"`
	}{}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	o.side = obj.S
	o.id = obj.ID
	o.timestamp = obj.Timestamp
	o.quantity = obj.Quantity
	o.price = obj.Price
	return nil
}

// GetOrderSide gets the orderside along with its orders in one side of the market
// GetOrderSide 获取市场一侧的 OrderSide 及其订单
func (ob *OrderBook) GetOrderSide(side Side) *OrderSide {
	switch side {
	case Buy:
		return ob.bids
	default:
		return ob.asks
	}
}

// MarketOverview gives an overview of the market including the quantities and prices of each side in the market
// MarketOverview 给出市场概览，包含两侧各自的数量与价格
// asks:   qty   price       bids:  qty   price
// 卖盘：  数量   价格         买盘： 数量   价格
//         0.2   14                 0.9   13
//         0.1   14.5               5     14
//         0.8   16                 2     16
func (ob *OrderBook) MarketOverview() *MarketView {

	return &MarketView{
		Asks: compileOrders(ob.asks),
		Bids: compileOrders(ob.bids),
	}
}

// compileOrders compiles orders in the following format
// compileOrders 按下列格式汇总订单
func compileOrders(orders *OrderSide) map[string]decimal.Decimal {
	// show queue
	// 展示队列
	queue := make(map[string]decimal.Decimal)

	if orders != nil {
		level := orders.MaxPriceQueue()
		for level != nil {
			if q, exists := queue[level.Price().String()]; exists {
				queue[level.Price().String()] = q.Add(level.Volume())
			} else {
				queue[level.Price().String()] = level.Volume()
			}

			level = orders.LessThan(level.Price())
		}

	}

	return queue
}
