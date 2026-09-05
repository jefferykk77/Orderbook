package orderbook

import (
	"encoding/json"
	"testing"
)

func TestSideJSON(t *testing.T) {
	data := struct {
		S Side `json:"side"`
	}{}

	data.S = Buy
	resultBuy, _ := json.Marshal(data)
	t.Log(string(resultBuy))

	data.S = Sell
	resultSell, _ := json.Marshal(&data)
	t.Log(string(resultSell))

	_ = json.Unmarshal(resultBuy, &data)
	t.Log(data)

	_ = json.Unmarshal(resultSell, &data)
	t.Log(data)

	err := json.Unmarshal([]byte(`{"side":"fake"}`), &data)
	if err == nil {
		t.Fatal("can unmarshal unsupported value")
	}
}

func TestSideOpposite(t *testing.T) {
	if Buy.Opposite() != Sell {
		t.Fatalf("Buy.Opposite() = %s, want sell", Buy.Opposite())
	}
	if Sell.Opposite() != Buy {
		t.Fatalf("Sell.Opposite() = %s, want buy", Sell.Opposite())
	}
	if Buy.Opposite().Opposite() != Buy {
		t.Fatal("Opposite should be involutive")
	}
}
