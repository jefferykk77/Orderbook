package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"orderbook"
	tradepb "orderbook/api/proto/trade"
)

// pairKey 表示一个交易组合。当前只开放 BTC/USDT 和 ETH/USDT。
type pairKey struct {
	base  tradepb.Currency
	quote tradepb.Currency
}

// idempotencyKey 是 PlaceOrder 的幂等键。
// 同一个用户复用 request_id 时，即使请求其他字段变了，也返回第一次创建的订单。
type idempotencyKey struct {
	userID    string
	requestID string
}

// orderRecord 是撮合核心之外的订单业务状态。
// OrderBook 只保存仍然挂着的订单，这里补齐原始数量、累计成交和状态机。
type orderRecord struct {
	order *tradepb.Order
	fills []*tradepb.Fill
}

// Service 是 TradingService 的内存版实现。
// 当前先用一把锁保证撮合和元数据一致，压测前再按交易对拆锁。
type Service struct {
	tradepb.UnimplementedTradingServiceServer

	mu       sync.Mutex
	orders   map[string]*orderRecord
	requests map[idempotencyKey]string
	books    map[pairKey]*orderbook.OrderBook
	idPrefix string
	idSeq    uint64
	now      func() time.Time
}

// NewService 创建内存撮合服务。
func NewService() (*Service, error) {
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return nil, fmt.Errorf("生成服务 ID 前缀失败: %w", err)
	}

	return &Service{
		orders:   make(map[string]*orderRecord),
		requests: make(map[idempotencyKey]string),
		books: map[pairKey]*orderbook.OrderBook{
			{base: tradepb.Currency_CURRENCY_BTC, quote: tradepb.Currency_CURRENCY_USDT}: orderbook.NewOrderBook(),
			{base: tradepb.Currency_CURRENCY_ETH, quote: tradepb.Currency_CURRENCY_USDT}: orderbook.NewOrderBook(),
		},
		idPrefix: hex.EncodeToString(randomBytes[:]),
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

type placeInput struct {
	userID     string
	requestID  string
	pair       pairKey
	side       tradepb.Side
	orderType  tradepb.OrderType
	limitPrice decimal.Decimal
	quantity   decimal.Decimal
}

// PlaceOrder 接收订单并调用撮合核心。
func (s *Service) PlaceOrder(ctx context.Context, req *tradepb.PlaceOrderRequest) (*tradepb.PlaceOrderResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.Error(codes.Canceled, "请求已取消")
	}

	input, err := validatePlaceOrder(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	requestKey := idempotencyKey{userID: input.userID, requestID: input.requestID}
	if orderID, exists := s.requests[requestKey]; exists {
		record := s.orders[orderID]
		return &tradepb.PlaceOrderResponse{
			Order: cloneOrder(record.order),
			Fills: cloneFills(record.fills),
		}, nil
	}

	orderID := s.nextID("O")
	now := s.now().UTC()
	record := &orderRecord{
		order: &tradepb.Order{
			OrderId:           orderID,
			RequestId:         input.requestID,
			UserId:            input.userID,
			BaseCurrency:      input.pair.base,
			QuoteCurrency:     input.pair.quote,
			Side:              input.side,
			Type:              input.orderType,
			LimitPrice:        limitPriceString(input),
			OriginalQuantity:  input.quantity.String(),
			ExecutedQuantity:  decimal.Zero.String(),
			RemainingQuantity: input.quantity.String(),
			Status:            tradepb.OrderStatus_ORDER_STATUS_OPEN,
			CreatedAtMs:       now.UnixMilli(),
			UpdatedAtMs:       now.UnixMilli(),
		},
	}
	s.orders[orderID] = record
	s.requests[requestKey] = orderID

	book := s.books[input.pair]
	if book == nil {
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_REJECTED
		record.order.UpdatedAtMs = now.UnixMilli()
		return nil, status.Error(codes.Internal, "交易组合未初始化")
	}

	coreSideValue, err := coreSide(input.side)
	if err != nil {
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_REJECTED
		record.order.UpdatedAtMs = now.UnixMilli()
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if input.orderType == tradepb.OrderType_ORDER_TYPE_LIMIT {
		done, partial, partialQuantity, err := book.ProcessLimitOrder(
			coreSideValue,
			orderID,
			input.quantity,
			input.limitPrice,
		)
		if err != nil {
			markRejected(record, now)
			return nil, status.Error(codes.Internal, fmt.Sprintf("撮合失败: %v", err))
		}

		newFills, err := s.applyFills(orderID, done, partial, partialQuantity, now)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		remaining := decimal.Zero
		if partial != nil && partial.ID() == orderID {
			remaining = partial.Quantity()
		}
		if err := updateTakerState(record, remaining, false, now); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &tradepb.PlaceOrderResponse{
			Order: cloneOrder(record.order),
			Fills: cloneFills(newFills),
		}, nil
	}

	// 市价单没有剩余挂单语义，深度不足时先整体拒绝，避免出现部分成交后不知道如何处理剩余量。
	if _, _, err := book.CalculateMarketPrice(coreSideValue, input.quantity); err != nil {
		if errors.Is(err, orderbook.ErrInsufficientQuantity) {
			markRejected(record, now)
			return &tradepb.PlaceOrderResponse{Order: cloneOrder(record.order)}, nil
		}
		markRejected(record, now)
		return nil, status.Error(codes.Internal, fmt.Sprintf("检查市价深度失败: %v", err))
	}

	done, partial, partialQuantity, quantityLeft, err := book.ProcessMarketOrder(
		coreSideValue,
		input.quantity,
	)
	if err != nil {
		markRejected(record, now)
		return nil, status.Error(codes.Internal, fmt.Sprintf("撮合失败: %v", err))
	}

	newFills, err := s.applyFills(orderID, done, partial, partialQuantity, now)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := updateTakerState(record, quantityLeft, quantityLeft.Sign() > 0, now); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &tradepb.PlaceOrderResponse{
		Order: cloneOrder(record.order),
		Fills: cloneFills(newFills),
	}, nil
}

// CancelOrder 取消仍然挂在订单簿上的订单。
func (s *Service) CancelOrder(ctx context.Context, req *tradepb.CancelOrderRequest) (*tradepb.CancelOrderResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.Error(codes.Canceled, "请求已取消")
	}

	userID, err := requiredText(req.GetUserId(), "user_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	orderID, err := requiredText(req.GetOrderId(), "order_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.findUserOrder(orderID, userID)
	if record == nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}
	if record.order.Status != tradepb.OrderStatus_ORDER_STATUS_OPEN &&
		record.order.Status != tradepb.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED {
		return nil, status.Error(codes.FailedPrecondition, "订单当前状态不能取消")
	}

	book := s.books[pairKey{base: record.order.BaseCurrency, quote: record.order.QuoteCurrency}]
	cancelled := book.CancelOrder(orderID)
	if cancelled == nil {
		return nil, status.Error(codes.Internal, "订单簿中没有找到这笔仍然有效的订单")
	}

	now := s.now().UTC()
	record.order.RemainingQuantity = cancelled.Quantity().String()
	record.order.Status = tradepb.OrderStatus_ORDER_STATUS_CANCELLED
	record.order.UpdatedAtMs = now.UnixMilli()

	return &tradepb.CancelOrderResponse{Order: cloneOrder(record.order)}, nil
}

// GetOrder 返回订单业务状态，不依赖订单是否仍然挂在簿上。
func (s *Service) GetOrder(ctx context.Context, req *tradepb.GetOrderRequest) (*tradepb.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.Error(codes.Canceled, "请求已取消")
	}

	userID, err := requiredText(req.GetUserId(), "user_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	orderID, err := requiredText(req.GetOrderId(), "order_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.findUserOrder(orderID, userID)
	if record == nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}
	return cloneOrder(record.order), nil
}

func validatePlaceOrder(req *tradepb.PlaceOrderRequest) (*placeInput, error) {
	userID, err := requiredText(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	requestID, err := requiredText(req.GetRequestId(), "request_id")
	if err != nil {
		return nil, err
	}

	pair := pairKey{base: req.GetBaseCurrency(), quote: req.GetQuoteCurrency()}
	if !supportedPair(pair) {
		return nil, fmt.Errorf(
			"不支持的交易组合: %s/%s，当前只支持 BTC/USDT 和 ETH/USDT",
			pair.base,
			pair.quote,
		)
	}

	switch req.GetSide() {
	case tradepb.Side_SIDE_BUY, tradepb.Side_SIDE_SELL:
	default:
		return nil, fmt.Errorf("无效的买卖方向: %s", req.GetSide())
	}

	input := &placeInput{
		userID:    userID,
		requestID: requestID,
		pair:      pair,
		side:      req.GetSide(),
		orderType: req.GetType(),
	}

	quantity, err := parseDecimal(req.GetQuantity(), "quantity")
	if err != nil {
		return nil, err
	}
	if quantity.Sign() <= 0 {
		return nil, errors.New("quantity 必须大于 0")
	}
	input.quantity = quantity

	switch input.orderType {
	case tradepb.OrderType_ORDER_TYPE_LIMIT:
		limitPrice, err := parseDecimal(req.GetLimitPrice(), "limit_price")
		if err != nil {
			return nil, err
		}
		if limitPrice.Sign() <= 0 {
			return nil, errors.New("limit_price 必须大于 0")
		}
		input.limitPrice = limitPrice
	case tradepb.OrderType_ORDER_TYPE_MARKET:
	default:
		return nil, fmt.Errorf("无效的订单类型: %s", input.orderType)
	}

	return input, nil
}

func requiredText(value, field string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s 不能为空", field)
	}
	return trimmed, nil
}

func supportedPair(pair pairKey) bool {
	return pair == pairKey{base: tradepb.Currency_CURRENCY_BTC, quote: tradepb.Currency_CURRENCY_USDT} ||
		pair == pairKey{base: tradepb.Currency_CURRENCY_ETH, quote: tradepb.Currency_CURRENCY_USDT}
}

func limitPriceString(input *placeInput) string {
	if input.orderType != tradepb.OrderType_ORDER_TYPE_LIMIT {
		return ""
	}
	return input.limitPrice.String()
}

func (s *Service) nextID(kind string) string {
	s.idSeq++
	return fmt.Sprintf("%s-%s-%d", s.idPrefix, kind, s.idSeq)
}

func (s *Service) findUserOrder(orderID, userID string) *orderRecord {
	record := s.orders[orderID]
	if record == nil || record.order.UserId != userID {
		return nil
	}
	return record
}

func markRejected(record *orderRecord, now time.Time) {
	record.order.Status = tradepb.OrderStatus_ORDER_STATUS_REJECTED
	record.order.UpdatedAtMs = now.UnixMilli()
}

// applyFills 根据撮合核心返回值更新对手方订单，并生成这次请求新增的成交记录。
func (s *Service) applyFills(
	takerOrderID string,
	done []*orderbook.Order,
	partial *orderbook.Order,
	partialQuantity decimal.Decimal,
	now time.Time,
) ([]*tradepb.Fill, error) {
	newFills := make([]*tradepb.Fill, 0)

	for _, matched := range done {
		if matched.ID() == takerOrderID {
			// ProcessLimitOrder 在完全成交时会追加一个带均价的 taker 订单，业务订单不需要用它覆盖限价。
			continue
		}
		fill, err := s.applyMakerFill(matched.ID(), matched.Price(), matched.Quantity(), takerOrderID, now)
		if err != nil {
			return nil, err
		}
		newFills = append(newFills, fill)
	}

	if partial != nil && partial.ID() != takerOrderID && partialQuantity.Sign() > 0 {
		fill, err := s.applyMakerFill(partial.ID(), partial.Price(), partialQuantity, takerOrderID, now)
		if err != nil {
			return nil, err
		}
		newFills = append(newFills, fill)
	}

	return newFills, nil
}

func (s *Service) applyMakerFill(
	makerOrderID string,
	price,
	quantity decimal.Decimal,
	takerOrderID string,
	now time.Time,
) (*tradepb.Fill, error) {
	maker := s.orders[makerOrderID]
	if maker == nil {
		return nil, fmt.Errorf("成交对手订单缺少业务状态: %s", makerOrderID)
	}
	taker := s.orders[takerOrderID]
	if taker == nil {
		return nil, fmt.Errorf("成交发起订单缺少业务状态: %s", takerOrderID)
	}

	executed, err := decimal.NewFromString(maker.order.ExecutedQuantity)
	if err != nil {
		return nil, fmt.Errorf("读取对手订单已成交数量失败: %w", err)
	}
	remaining, err := decimal.NewFromString(maker.order.RemainingQuantity)
	if err != nil {
		return nil, fmt.Errorf("读取对手订单剩余数量失败: %w", err)
	}

	executed = executed.Add(quantity)
	remaining = remaining.Sub(quantity)
	maker.order.ExecutedQuantity = executed.String()
	maker.order.RemainingQuantity = remaining.String()
	if remaining.Sign() <= 0 {
		maker.order.Status = tradepb.OrderStatus_ORDER_STATUS_FILLED
	} else {
		maker.order.Status = tradepb.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
	}
	maker.order.UpdatedAtMs = now.UnixMilli()

	fill := &tradepb.Fill{
		TradeId:      s.nextID("T"),
		MakerOrderId: makerOrderID,
		TakerOrderId: takerOrderID,
		Price:        price.String(),
		Quantity:     quantity.String(),
		ExecutedAtMs: now.UnixMilli(),
	}
	maker.fills = append(maker.fills, fill)
	taker.fills = append(taker.fills, fill)
	return cloneFill(fill), nil
}

func updateTakerState(record *orderRecord, remaining decimal.Decimal, rejected bool, now time.Time) error {
	original, err := decimal.NewFromString(record.order.OriginalQuantity)
	if err != nil {
		return fmt.Errorf("读取订单原始数量失败: %w", err)
	}
	if remaining.Sign() < 0 || remaining.GreaterThan(original) {
		return fmt.Errorf("订单剩余数量非法: %s", remaining)
	}

	executed := original.Sub(remaining)
	record.order.ExecutedQuantity = executed.String()
	record.order.RemainingQuantity = remaining.String()
	switch {
	case rejected:
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_REJECTED
	case remaining.Sign() == 0:
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_FILLED
	case executed.Sign() > 0:
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
	default:
		record.order.Status = tradepb.OrderStatus_ORDER_STATUS_OPEN
	}
	record.order.UpdatedAtMs = now.UnixMilli()
	return nil
}
