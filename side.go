package orderbook

import (
	"encoding/json"
	"reflect"
)

// Side of the order
// 订单的买卖方向
type Side int

// Sell (asks) or Buy (bids)
// 卖出（卖盘 asks）或买入（买盘 bids）
const (
	Sell Side = iota
	Buy
)

// Opposite returns the other side of the market.
// Opposite 返回市场的另一侧
func (s Side) Opposite() Side {
	if s == Buy {
		return Sell
	}
	return Buy
}

// String implements fmt.Stringer interface
// String 实现 fmt.Stringer 接口
func (s Side) String() string {
	if s == Buy {
		return "buy"
	}

	return "sell"
}

// MarshalJSON implements json.Marshaler interface
// MarshalJSON 实现 json.Marshaler 接口
func (s Side) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler interface
// UnmarshalJSON 实现 json.Unmarshaler 接口
func (s *Side) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case `"buy"`:
		*s = Buy
	case `"sell"`:
		*s = Sell
	default:
		return &json.UnsupportedValueError{
			Value: reflect.New(reflect.TypeOf(data)),
			Str:   string(data),
		}
	}

	return nil
}
