package orderbook

import (
	"container/list"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// OrderQueue stores and manage chain of orders
// OrderQueue 存储并管理同一价格上的订单链
type OrderQueue struct {
	volume decimal.Decimal
	price  decimal.Decimal
	orders *list.List
}

// NewOrderQueue creates and initialize OrderQueue object
// NewOrderQueue 创建并初始化 OrderQueue 对象
func NewOrderQueue(price decimal.Decimal) *OrderQueue {
	return &OrderQueue{
		price:  price,
		volume: decimal.Zero,
		orders: list.New(),
	}
}

// Len returns amount of orders in queue
// Len 返回队列中的订单数量
func (oq *OrderQueue) Len() int {
	return oq.orders.Len()
}

// Price returns price level of the queue
// Price 返回该队列的价格档位
func (oq *OrderQueue) Price() decimal.Decimal {
	return oq.price
}

// Volume returns total orders volume
// Volume 返回队列中订单的总数量
func (oq *OrderQueue) Volume() decimal.Decimal {
	return oq.volume
}

// Head returns top order in queue
// Head 返回队列中的队首订单
func (oq *OrderQueue) Head() *list.Element {
	return oq.orders.Front()
}

// Tail returns bottom order in queue
// Tail 返回队列中的队尾订单
func (oq *OrderQueue) Tail() *list.Element {
	return oq.orders.Back()
}

// Append adds order to tail of the queue
// Append 将订单加到队列尾部
func (oq *OrderQueue) Append(o *Order) *list.Element {
	oq.volume = oq.volume.Add(o.Quantity())
	return oq.orders.PushBack(o)
}

// Update sets up new order to list value
// Update 用新订单替换链表节点中的值
func (oq *OrderQueue) Update(e *list.Element, o *Order) *list.Element {
	oq.volume = oq.volume.Sub(e.Value.(*Order).Quantity())
	oq.volume = oq.volume.Add(o.Quantity())
	e.Value = o
	return e
}

// Remove removes order from the queue and link order chain
// Remove 从队列中移除订单并重新链接订单链
func (oq *OrderQueue) Remove(e *list.Element) *Order {
	oq.volume = oq.volume.Sub(e.Value.(*Order).Quantity())
	return oq.orders.Remove(e).(*Order)
}

// String implements fmt.Stringer interface
// String 实现 fmt.Stringer 接口
func (oq *OrderQueue) String() string {
	sb := strings.Builder{}
	iter := oq.orders.Front()
	sb.WriteString(fmt.Sprintf("\nqueue length: %d, price: %s, volume: %s, orders:", oq.Len(), oq.Price(), oq.Volume()))
	for iter != nil {
		order := iter.Value.(*Order)
		str := fmt.Sprintf("\n\tid: %s, volume: %s, time: %s", order.ID(), order.Quantity(), order.Price())
		sb.WriteString(str)
		iter = iter.Next()
	}
	return sb.String()
}

// MarshalJSON implements json.Marshaler interface
// MarshalJSON 实现 json.Marshaler 接口
func (oq *OrderQueue) MarshalJSON() ([]byte, error) {
	iter := oq.Head()

	var orders []*Order
	for iter != nil {
		orders = append(orders, iter.Value.(*Order))
		iter = iter.Next()
	}

	return json.Marshal(
		&struct {
			Volume decimal.Decimal `json:"volume"`
			Price  decimal.Decimal `json:"price"`
			Orders []*Order        `json:"orders"`
		}{
			Volume: oq.Volume(),
			Price:  oq.Price(),
			Orders: orders,
		},
	)
}

// UnmarshalJSON implements json.Unmarshaler interface
// UnmarshalJSON 实现 json.Unmarshaler 接口
func (oq *OrderQueue) UnmarshalJSON(data []byte) error {
	obj := struct {
		Volume decimal.Decimal `json:"volume"`
		Price  decimal.Decimal `json:"price"`
		Orders []*Order        `json:"orders"`
	}{}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	oq.volume = obj.Volume
	oq.price = obj.Price
	oq.orders = list.New()
	for _, order := range obj.Orders {
		oq.orders.PushBack(order)
	}
	return nil
}
