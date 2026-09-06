package orderbook

import (
	"container/list"
	"encoding/json"
	"fmt"
	"strings"

	rbtx "github.com/emirpasic/gods/examples/redblacktreeextended"
	rbt "github.com/emirpasic/gods/trees/redblacktree"
	"github.com/shopspring/decimal"
)

// OrderSide implements facade to operations with order queue
// OrderSide 是对订单队列操作的门面
type OrderSide struct {
	priceTree *rbtx.RedBlackTreeExtended
	prices    map[string]*OrderQueue

	volume    decimal.Decimal
	numOrders int
	depth     int
}

func rbtComparator(a, b interface{}) int {
	return a.(decimal.Decimal).Cmp(b.(decimal.Decimal))
}

// NewOrderSide creates new OrderSide manager
// NewOrderSide 创建新的 OrderSide 管理器
func NewOrderSide() *OrderSide {
	return &OrderSide{
		priceTree: &rbtx.RedBlackTreeExtended{
			Tree: rbt.NewWith(rbtComparator),
		},
		prices: map[string]*OrderQueue{},
		volume: decimal.Zero,
	}
}

// Len returns amount of orders
// Len 返回订单数量
func (os *OrderSide) Len() int {
	return os.numOrders
}

// Depth returns depth of market
// Depth 返回市场深度（价档数）
func (os *OrderSide) Depth() int {
	return os.depth
}

// Volume returns total amount of quantity in side
// Volume 返回该侧的数量合计
func (os *OrderSide) Volume() decimal.Decimal {
	return os.volume
}

// Append appends order to definite price level
// Append 将订单追加到指定价格档位
func (os *OrderSide) Append(o *Order) *list.Element {
	price := o.Price()
	strPrice := price.String()

	priceQueue, ok := os.prices[strPrice]
	if !ok {
		priceQueue = NewOrderQueue(o.Price())
		os.prices[strPrice] = priceQueue
		os.priceTree.Put(price, priceQueue)
		os.depth++
	}
	os.numOrders++
	os.volume = os.volume.Add(o.Quantity())
	return priceQueue.Append(o)
}

// Remove removes order from definite price level
// Remove 从指定价格档位移除订单
func (os *OrderSide) Remove(e *list.Element) *Order {
	price := e.Value.(*Order).Price()
	strPrice := price.String()

	priceQueue := os.prices[strPrice]
	o := priceQueue.Remove(e)

	if priceQueue.Len() == 0 {
		delete(os.prices, strPrice)
		os.priceTree.Remove(price)
		os.depth--
	}

	os.numOrders--
	os.volume = os.volume.Sub(o.Quantity())
	return o
}

// MaxPriceQueue returns maximal level of price
// MaxPriceQueue 返回最高价格档位
func (os *OrderSide) MaxPriceQueue() *OrderQueue {
	if os.depth > 0 {
		if value, found := os.priceTree.GetMax(); found {
			return value.(*OrderQueue)
		}
	}
	return nil
}

// MinPriceQueue returns maximal level of price
// MinPriceQueue 返回最低价格档位
func (os *OrderSide) MinPriceQueue() *OrderQueue {
	if os.depth > 0 {
		if value, found := os.priceTree.GetMin(); found {
			return value.(*OrderQueue)
		}
	}
	return nil
}

// LessThan returns nearest OrderQueue with price less than given
// LessThan 返回价格低于给定值的最近 OrderQueue
func (os *OrderSide) LessThan(price decimal.Decimal) *OrderQueue {
	tree := os.priceTree.Tree
	node := tree.Root

	var floor *rbt.Node
	for node != nil {
		if tree.Comparator(price, node.Key) > 0 {
			floor = node
			node = node.Right
		} else {
			node = node.Left
		}
	}

	if floor != nil {
		return floor.Value.(*OrderQueue)
	}

	return nil
}

// GreaterThan returns nearest OrderQueue with price greater than given
// GreaterThan 返回价格高于给定值的最近 OrderQueue
func (os *OrderSide) GreaterThan(price decimal.Decimal) *OrderQueue {
	tree := os.priceTree.Tree
	node := tree.Root

	var ceiling *rbt.Node
	for node != nil {
		if tree.Comparator(price, node.Key) < 0 {
			ceiling = node
			node = node.Left
		} else {
			node = node.Right
		}
	}

	if ceiling != nil {
		return ceiling.Value.(*OrderQueue)
	}

	return nil
}

// Orders returns all of *list.Element orders
// Orders 返回全部订单的 *list.Element
func (os *OrderSide) Orders() (orders []*list.Element) {
	for _, price := range os.prices {
		iter := price.Head()
		for iter != nil {
			orders = append(orders, iter)
			iter = iter.Next()
		}
	}
	return
}

// String implements fmt.Stringer interface
// String 实现 fmt.Stringer 接口
func (os *OrderSide) String() string {
	sb := strings.Builder{}

	level := os.MaxPriceQueue()
	for level != nil {
		sb.WriteString(fmt.Sprintf("\n%s -> %s", level.Price(), level.Volume()))
		level = os.LessThan(level.Price())
	}

	return sb.String()
}

// MarshalJSON implements json.Marshaler interface
// MarshalJSON 实现 json.Marshaler 接口
func (os *OrderSide) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		&struct {
			NumOrders int                    `json:"numOrders"`
			Depth     int                    `json:"depth"`
			Prices    map[string]*OrderQueue `json:"prices"`
		}{
			NumOrders: os.numOrders,
			Depth:     os.depth,
			Prices:    os.prices,
		},
	)
}

// UnmarshalJSON implements json.Unmarshaler interface
// UnmarshalJSON 实现 json.Unmarshaler 接口
func (os *OrderSide) UnmarshalJSON(data []byte) error {
	obj := struct {
		NumOrders int                    `json:"numOrders"`
		Depth     int                    `json:"depth"`
		Prices    map[string]*OrderQueue `json:"prices"`
	}{}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	os.numOrders = obj.NumOrders
	os.depth = obj.Depth
	os.prices = obj.Prices
	os.priceTree = &rbtx.RedBlackTreeExtended{
		Tree: rbt.NewWith(rbtComparator),
	}

	for price, queue := range os.prices {
		os.priceTree.Put(decimal.RequireFromString(price), queue)
	}

	return nil
}
