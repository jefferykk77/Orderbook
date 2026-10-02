package orderbook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/shopspring/decimal"
)

func newBenchmarkBook(levels, ordersPerLevel int) *OrderBook {
	book := NewOrderBook()
	quantity := decimal.New(10, 0)
	orderID := 0

	for level := 0; level < levels; level++ {
		askPrice := decimal.New(int64(100+level), 0)
		bidPrice := decimal.New(int64(99-level), 0)
		for i := 0; i < ordersPerLevel; i++ {
			_, _, _, err := book.ProcessLimitOrder(Sell, "ask-"+strconv.Itoa(orderID), quantity, askPrice)
			if err != nil {
				panic(err)
			}
			_, _, _, err = book.ProcessLimitOrder(Buy, "bid-"+strconv.Itoa(orderID), quantity, bidPrice)
			if err != nil {
				panic(err)
			}
			orderID++
		}
	}

	return book
}

func newPartialFillBook(quantity int64) *OrderBook {
	book := NewOrderBook()
	_, _, _, err := book.ProcessLimitOrder(
		Sell,
		"resting-ask",
		decimal.New(quantity, 0),
		decimal.New(100, 0),
	)
	if err != nil {
		panic(err)
	}
	return book
}

func BenchmarkLimitOrderNoCross(b *testing.B) {
	book := newBenchmarkBook(50, 10)
	quantity := decimal.New(1, 0)
	price := decimal.New(1, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := strconv.Itoa(i)
		if _, _, _, err := book.ProcessLimitOrder(Buy, id, quantity, price); err != nil {
			b.Fatal(err)
		}
		if book.CancelOrder(id) == nil {
			b.Fatal("order was not placed")
		}
	}
}

func BenchmarkLimitOrderMatch(b *testing.B) {
	book := newPartialFillBook(1 << 60)
	quantity := decimal.New(1, 0)
	price := decimal.New(100, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := book.ProcessLimitOrder(Buy, strconv.Itoa(i), quantity, price); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarketOrderPartialFill(b *testing.B) {
	book := newPartialFillBook(1 << 60)
	quantity := decimal.New(1, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, _, err := book.ProcessMarketOrder(Buy, quantity); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateMarketPrice(b *testing.B) {
	book := newBenchmarkBook(50, 10)
	quantity := decimal.New(250, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := book.CalculateMarketPrice(Buy, quantity); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDepth(b *testing.B) {
	book := newBenchmarkBook(50, 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if asks, bids := book.Depth(); len(asks) != 50 || len(bids) != 50 {
			b.Fatalf("unexpected depth: asks=%d bids=%d", len(asks), len(bids))
		}
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	book := newBenchmarkBook(50, 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(book); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelIndependentBooks(b *testing.B) {
	quantity := decimal.New(1, 0)
	price := decimal.New(100, 0)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		book := newPartialFillBook(1 << 60)
		for i := 0; pb.Next(); i++ {
			if _, _, _, err := book.ProcessLimitOrder(Buy, strconv.Itoa(i), quantity, price); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkBookString(b *testing.B) {
	book := newBenchmarkBook(50, 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprint(book)
	}
}
