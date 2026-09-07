package orderbook

import (
	"container/list"
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// OrderBook implements standard matching algorithm
// OrderBook 实现标准撮合算法
type OrderBook struct {
	orders map[string]*list.Element // orderID -> *Order (*list.Element.Value.(*Order))
	// 订单 ID → *Order（*list.Element.Value.(*Order)）

	asks *OrderSide
	bids *OrderSide
}

// NewOrderBook creates Orderbook object
// NewOrderBook 创建 OrderBook 对象
func NewOrderBook() *OrderBook {
	return &OrderBook{
		orders: map[string]*list.Element{},
		bids:   NewOrderSide(),
		asks:   NewOrderSide(),
	}
}

// PriceLevel contains price and volume in depth
// PriceLevel 表示深度中某一档的价格与数量
type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

// restingBook is the opposite side of the book an incoming order consumes,
// restingBook 是进场订单要吃掉的对手盘，
// walked from best price outward. Matching depends on this surface, not on
// 从最优价向外遍历。撮合依赖该接口，而不是
// bids vs asks.
// 直接区分买盘还是卖盘。
type restingBook interface {
	Len() int
	Best() *OrderQueue
	Next(price decimal.Decimal) *OrderQueue
}

type restingSide struct {
	book *OrderSide
	best func() *OrderQueue
	next func(decimal.Decimal) *OrderQueue
}

func (t restingSide) Len() int { return t.book.Len() }

func (t restingSide) Best() *OrderQueue { return t.best() }

func (t restingSide) Next(price decimal.Decimal) *OrderQueue { return t.next(price) }

func (ob *OrderBook) restingBook(side Side) restingBook {
	book := ob.GetOrderSide(side.Opposite())
	if side == Buy {
		return restingSide{book: book, best: book.MinPriceQueue, next: book.GreaterThan}
	}
	return restingSide{book: book, best: book.MaxPriceQueue, next: book.LessThan}
}

func (s Side) crosses(limit, best decimal.Decimal) bool {
	if s == Buy {
		return limit.GreaterThanOrEqual(best)
	}
	return limit.LessThanOrEqual(best)
}

// ProcessMarketOrder immediately gets definite quantity from the order book with market price
// ProcessMarketOrder 按市价立即从订单簿吃掉指定数量
// Arguments:
// 参数：
//
//	side     - what do you want to do (ob.Sell or ob.Buy)
//	side     - 买卖方向（ob.Sell 或 ob.Buy）
//	quantity - how much quantity you want to sell or buy
//	quantity - 希望买入或卖出的数量
//	* to create new decimal number you should use decimal.New() func
//	* 创建 decimal 请使用 decimal.New()
//	  read more at https://github.com/shopspring/decimal
//	  详见 https://github.com/shopspring/decimal
//
// Return:
// 返回值：
//
//	error        - not nil if price is less or equal 0
//	error        - 价格小于等于 0 时不为 nil
//	done         - not nil if your market order produces ends of anoter orders, this order will add to
//	done         - 若市价单导致其他订单完全成交，这些订单会加入
//	               the "done" slice
//	               "done" 切片
//	partial      - not nil if your order has done but top order is not fully done
//	partial      - 若你的订单已处理但对手盘最优订单未完全成交，则不为 nil
//	partialQuantityProcessed - if partial order is not nil this result contains processed quatity from partial order
//	partialQuantityProcessed - 当 partial 不为 nil 时，表示该部分成交订单已成交的数量
//	quantityLeft - more than zero if it is not enought orders to process all quantity
//	quantityLeft - 盘口深度不足以吃完全部数量时大于 0
func (ob *OrderBook) CalculatePriceAfterExecution(side Side, quantity decimal.Decimal) (price decimal.Decimal, err error) {
	price = decimal.Zero
	resting := ob.restingBook(side)
	level := resting.Best()
	for quantity.Sign() > 0 && level != nil {
		levelVolume := level.Volume()
		levelPrice := level.Price()
		if quantity.GreaterThanOrEqual(levelVolume) {
			price = levelPrice
			quantity = quantity.Sub(levelVolume)
			level = resting.Next(levelPrice)
		} else {
			price = levelPrice
			quantity = decimal.Zero
		}
	}

	return
}

func (ob *OrderBook) ProcessMarketOrder(side Side, quantity decimal.Decimal) (done []*Order, partial *Order, partialQuantityProcessed, quantityLeft decimal.Decimal, err error) {
	if quantity.Sign() <= 0 {
		return nil, nil, decimal.Zero, decimal.Zero, ErrInvalidQuantity
	}

	resting := ob.restingBook(side)
	for quantity.Sign() > 0 && resting.Len() > 0 {
		bestPrice := resting.Best()
		ordersDone, partialDone, partialProcessed, quantityLeft := ob.processQueue(bestPrice, quantity)
		done = append(done, ordersDone...)
		partial = partialDone
		partialQuantityProcessed = partialProcessed
		quantity = quantityLeft
	}

	quantityLeft = quantity
	return
}

// ProcessLimitOrder places new order to the OrderBook
// ProcessLimitOrder 将新订单放入订单簿
// Arguments:
// 参数：
//
//	side     - what do you want to do (ob.Sell or ob.Buy)
//	side     - 买卖方向（ob.Sell 或 ob.Buy）
//	orderID  - unique order ID in depth
//	orderID  - 盘口中的唯一订单 ID
//	quantity - how much quantity you want to sell or buy
//	quantity - 希望买入或卖出的数量
//	price    - no more expensive (or cheaper) this price
//	price    - 限价：买入不高于该价，卖出不低于该价
//	* to create new decimal number you should use decimal.New() func
//	* 创建 decimal 请使用 decimal.New()
//	  read more at https://github.com/shopspring/decimal
//	  详见 https://github.com/shopspring/decimal
//
// Return:
// 返回值：
//
//	error   - not nil if quantity (or price) is less or equal 0. Or if order with given ID is exists
//	error   - 数量（或价格）小于等于 0，或给定 ID 的订单已存在时不为 nil
//	done    - not nil if your order produces ends of anoter order, this order will add to
//	done    - 若你的订单导致其他订单完全成交，这些订单会加入
//	          the "done" slice. If your order have done too, it will be places to this array too
//	          "done" 切片。若你的订单本身也完全成交，同样会放入该数组
//	partial - not nil if your order has done but top order is not fully done. Or if your order is
//	partial - 若你的订单已处理但对手盘最优订单未完全成交；或你的订单部分成交后
//	          partial done and placed to the orderbook without full quantity - partial will contain
//	          以剩余数量挂入订单簿，则 partial 为该剩余订单
//	          your order with quantity to left
//	          （即你这笔订单的剩余数量）
//	partialQuantityProcessed - if partial order is not nil this result contains processed quatity from partial order
//	partialQuantityProcessed - 当 partial 不为 nil 时，表示该部分成交订单已成交的数量
func (ob *OrderBook) ProcessLimitOrder(side Side, orderID string, quantity, price decimal.Decimal) (done []*Order, partial *Order, partialQuantityProcessed decimal.Decimal, err error) {
	if _, ok := ob.orders[orderID]; ok {
		return nil, nil, decimal.Zero, ErrOrderExists
	}

	if quantity.Sign() <= 0 {
		return nil, nil, decimal.Zero, ErrInvalidQuantity
	}

	if price.Sign() <= 0 {
		return nil, nil, decimal.Zero, ErrInvalidPrice
	}

	quantityToTrade := quantity
	own := ob.GetOrderSide(side)
	resting := ob.restingBook(side)

	bestPrice := resting.Best()
	for quantityToTrade.Sign() > 0 && resting.Len() > 0 && side.crosses(price, bestPrice.Price()) {
		ordersDone, partialDone, partialQty, quantityLeft := ob.processQueue(bestPrice, quantityToTrade)
		done = append(done, ordersDone...)
		partial = partialDone
		partialQuantityProcessed = partialQty
		quantityToTrade = quantityLeft
		bestPrice = resting.Best()
	}

	if quantityToTrade.Sign() > 0 {
		o := NewOrder(orderID, side, quantityToTrade, price, time.Now().UTC())
		if len(done) > 0 {
			partialQuantityProcessed = quantity.Sub(quantityToTrade)
			partial = o
		}
		ob.orders[orderID] = own.Append(o)
	} else {
		totalQuantity := decimal.Zero
		totalPrice := decimal.Zero

		for _, order := range done {
			totalQuantity = totalQuantity.Add(order.Quantity())
			totalPrice = totalPrice.Add(order.Price().Mul(order.Quantity()))
		}

		if partialQuantityProcessed.Sign() > 0 {
			totalQuantity = totalQuantity.Add(partialQuantityProcessed)
			totalPrice = totalPrice.Add(partial.Price().Mul(partialQuantityProcessed))
		}

		done = append(done, NewOrder(orderID, side, quantity, totalPrice.Div(totalQuantity), time.Now().UTC()))
	}
	return
}

func (ob *OrderBook) processQueue(orderQueue *OrderQueue, quantityToTrade decimal.Decimal) (done []*Order, partial *Order, partialQuantityProcessed, quantityLeft decimal.Decimal) {
	quantityLeft = quantityToTrade

	for orderQueue.Len() > 0 && quantityLeft.Sign() > 0 {
		headOrderEl := orderQueue.Head()
		headOrder := headOrderEl.Value.(*Order)

		if quantityLeft.LessThan(headOrder.Quantity()) {
			partial = NewOrder(headOrder.ID(), headOrder.Side(), headOrder.Quantity().Sub(quantityLeft), headOrder.Price(), headOrder.Time())
			partialQuantityProcessed = quantityLeft
			orderQueue.Update(headOrderEl, partial)
			quantityLeft = decimal.Zero
		} else {
			quantityLeft = quantityLeft.Sub(headOrder.Quantity())
			done = append(done, ob.CancelOrder(headOrder.ID()))
		}
	}

	return
}

// Order returns order by id
// Order 按 ID 返回订单
func (ob *OrderBook) Order(orderID string) *Order {
	e, ok := ob.orders[orderID]
	if !ok {
		return nil
	}

	return e.Value.(*Order)
}

// Depth returns price levels and volume at price level
// Depth 返回各价格档位及其数量
func (ob *OrderBook) Depth() (asks, bids []*PriceLevel) {
	level := ob.asks.MaxPriceQueue()
	for level != nil {
		asks = append(asks, &PriceLevel{
			Price:    level.Price(),
			Quantity: level.Volume(),
		})
		level = ob.asks.LessThan(level.Price())
	}

	level = ob.bids.MaxPriceQueue()
	for level != nil {
		bids = append(bids, &PriceLevel{
			Price:    level.Price(),
			Quantity: level.Volume(),
		})
		level = ob.bids.LessThan(level.Price())
	}
	return
}

// CancelOrder removes order with given ID from the order book
// CancelOrder 按 ID 从订单簿中移除订单
func (ob *OrderBook) CancelOrder(orderID string) *Order {
	e, ok := ob.orders[orderID]
	if !ok {
		return nil
	}

	delete(ob.orders, orderID)

	return ob.GetOrderSide(e.Value.(*Order).Side()).Remove(e)
}

// CalculateMarketPrice returns total market price for requested quantity
// CalculateMarketPrice 返回指定数量对应的市价总额
// if err is not nil price returns total price of all levels in side
// 若 err 不为 nil，price 返回该侧所有档位的总额
func (ob *OrderBook) CalculateMarketPrice(side Side, quantity decimal.Decimal) (price decimal.Decimal, quant decimal.Decimal, err error) {
	price = decimal.Zero
	quant = decimal.Zero
	resting := ob.restingBook(side)
	level := resting.Best()
	for quantity.Sign() > 0 && level != nil {
		levelVolume := level.Volume()
		levelPrice := level.Price()
		if quantity.GreaterThanOrEqual(levelVolume) {
			price = price.Add(levelPrice.Mul(levelVolume))
			quantity = quantity.Sub(levelVolume)
			quant = quant.Add(levelVolume)
			level = resting.Next(levelPrice)
		} else {
			price = price.Add(levelPrice.Mul(quantity))
			quant = quant.Add(quantity)
			quantity = decimal.Zero
		}
	}
	if quantity.Sign() > 0 {
		err = ErrInsufficientQuantity
	}

	return
}

// String implements fmt.Stringer interface
// String 实现 fmt.Stringer 接口
func (ob *OrderBook) String() string {
	return ob.asks.String() + "\r\n------------------------------------" + ob.bids.String()
}

// MarshalJSON implements json.Marshaler interface
// MarshalJSON 实现 json.Marshaler 接口
func (ob *OrderBook) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		&struct {
			Asks *OrderSide `json:"asks"`
			Bids *OrderSide `json:"bids"`
		}{
			Asks: ob.asks,
			Bids: ob.bids,
		},
	)
}

// UnmarshalJSON implements json.Unmarshaler interface
// UnmarshalJSON 实现 json.Unmarshaler 接口
func (ob *OrderBook) UnmarshalJSON(data []byte) error {
	obj := struct {
		Asks *OrderSide `json:"asks"`
		Bids *OrderSide `json:"bids"`
	}{}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	ob.asks = obj.Asks
	ob.bids = obj.Bids
	ob.orders = map[string]*list.Element{}

	for _, order := range ob.asks.Orders() {
		ob.orders[order.Value.(*Order).ID()] = order
	}

	for _, order := range ob.bids.Orders() {
		ob.orders[order.Value.(*Order).ID()] = order
	}

	return nil
}
