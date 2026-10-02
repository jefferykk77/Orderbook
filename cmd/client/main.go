package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mbobakov/grpc-consul-resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	tradepb "orderbook/api/proto/trade"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("用法: client <place|cancel|get> [参数]")
	}
	command := strings.ToLower(strings.TrimSpace(os.Args[1]))
	if command != "place" && command != "cancel" && command != "get" {
		return fmt.Errorf("不支持的命令: %s，可用命令: place、cancel、get", command)
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	serverAddr := flags.String("server", "consul://127.0.0.1:8500/orderbook?healthy=true", "服务端地址或 Consul 服务发现地址")
	userID := flags.String("user", "", "用户 ID")
	requestID := flags.String("request", "", "下单幂等键，同一个用户不能复用")
	base := flags.String("base", "BTC", "基础货币: BTC 或 ETH")
	quote := flags.String("quote", "USDT", "计价货币，当前只能是 USDT")
	side := flags.String("side", "buy", "买卖方向: buy 或 sell")
	orderType := flags.String("type", "limit", "订单类型: limit 或 market")
	price := flags.String("price", "", "限价，十进制字符串")
	quantity := flags.String("quantity", "", "数量，十进制字符串")
	orderID := flags.String("order", "", "订单 ID")
	timeout := flags.Duration("timeout", 5*time.Second, "请求超时时间")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("存在未识别的参数: %s", flags.Arg(0))
	}
	if *userID == "" {
		return fmt.Errorf("-user 不能为空")
	}

	conn, err := grpc.NewClient(
		*serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy": "round_robin"}`),
	)
	if err != nil {
		return fmt.Errorf("创建 gRPC 客户端失败: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := tradepb.NewTradingServiceClient(conn)

	switch command {
	case "place":
		resp, err := client.PlaceOrder(ctx, &tradepb.PlaceOrderRequest{
			RequestId:     *requestID,
			UserId:        *userID,
			BaseCurrency:  parseCurrency(*base),
			QuoteCurrency: parseCurrency(*quote),
			Side:          parseSide(*side),
			Type:          parseOrderType(*orderType),
			LimitPrice:    *price,
			Quantity:      *quantity,
		})
		if err != nil {
			return err
		}
		return printJSON(resp)
	case "cancel":
		resp, err := client.CancelOrder(ctx, &tradepb.CancelOrderRequest{
			UserId:  *userID,
			OrderId: *orderID,
		})
		if err != nil {
			return err
		}
		return printJSON(resp)
	case "get":
		resp, err := client.GetOrder(ctx, &tradepb.GetOrderRequest{
			UserId:  *userID,
			OrderId: *orderID,
		})
		if err != nil {
			return err
		}
		return printJSON(resp)
	default:
		return fmt.Errorf("不支持的命令: %s，可用命令: place、cancel、get", command)
	}
}

func parseCurrency(value string) tradepb.Currency {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "BTC":
		return tradepb.Currency_CURRENCY_BTC
	case "ETH":
		return tradepb.Currency_CURRENCY_ETH
	case "USDT":
		return tradepb.Currency_CURRENCY_USDT
	default:
		return tradepb.Currency_CURRENCY_UNSPECIFIED
	}
}

func parseSide(value string) tradepb.Side {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "buy":
		return tradepb.Side_SIDE_BUY
	case "sell":
		return tradepb.Side_SIDE_SELL
	default:
		return tradepb.Side_SIDE_UNSPECIFIED
	}
}

func parseOrderType(value string) tradepb.OrderType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "limit":
		return tradepb.OrderType_ORDER_TYPE_LIMIT
	case "market":
		return tradepb.OrderType_ORDER_TYPE_MARKET
	default:
		return tradepb.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func printJSON(message proto.Message) error {
	data, err := protojson.MarshalOptions{Multiline: true}.Marshal(message)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(data))
	return nil
}
