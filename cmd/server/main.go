package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	tradepb "orderbook/api/proto/trade"
	"orderbook/internal/service"
)

func main() {
	listenAddr := os.Getenv("TRADE_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":50051"
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", listenAddr, err)
	}
	listenPort := listener.Addr().(*net.TCPAddr).Port

	advertiseAddr := os.Getenv("TRADE_ADVERTISE_ADDR")
	if advertiseAddr == "" {
		ip, err := outboundIP()
		if err != nil {
			log.Fatalf("获取出口 IP 失败: %v", err)
		}
		advertiseAddr = ip.String()
	}
	if net.ParseIP(advertiseAddr) == nil {
		log.Fatalf("TRADE_ADVERTISE_ADDR 必须是 IP 地址: %s", advertiseAddr)
	}

	tradingService, err := service.NewService()
	if err != nil {
		log.Fatalf("初始化撮合服务失败: %v", err)
	}

	grpcServer := grpc.NewServer()
	tradepb.RegisterTradingServiceServer(grpcServer, tradingService)

	// 使用 gRPC 官方健康检查协议。空字符串表示整个 server 的总状态，
	// 后续 Consul 的 GRPC check 就检查这个状态。
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// 先把健康检查摘掉，再等待存量请求结束。
		healthServer.Shutdown()
		grpcServer.GracefulStop()
	}()

	// 服务和健康检查都注册完成后，再把服务实例写入 Consul。
	if err := registerConsulService(advertiseAddr, listenPort); err != nil {
		log.Fatalf("注册 Consul 服务失败: %v", err)
	}

	// 服务和健康检查都注册完成，进入正式接流状态。
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	log.Printf("撮合服务开始监听 %s", listenAddr)
	if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Fatalf("gRPC 服务退出: %v", err)
	}
}

func registerConsulService(advertiseAddr string, port int) error {
	config := api.DefaultConfig()
	if consulAddr := os.Getenv("TRADE_CONSUL_ADDR"); consulAddr != "" {
		config.Address = consulAddr
	}

	consulClient, err := api.NewClient(config)
	if err != nil {
		return fmt.Errorf("创建 Consul 客户端失败: %w", err)
	}

	serviceName := os.Getenv("TRADE_CONSUL_SERVICE")
	if serviceName == "" {
		serviceName = "orderbook"
	}
	grpcAddr := net.JoinHostPort(advertiseAddr, strconv.Itoa(port))

	service := &api.AgentServiceRegistration{
		ID:      grpcAddr,
		Name:    serviceName,
		Tags:    []string{"grpc", serviceName},
		Address: advertiseAddr,
		Port:    port,
		Check: &api.AgentServiceCheck{
			GRPC:                           grpcAddr,
			Timeout:                        "5s",
			Interval:                       "10s",
			DeregisterCriticalServiceAfter: "10m",
		},
	}

	if err := consulClient.Agent().ServiceRegister(service); err != nil {
		return fmt.Errorf("注册服务失败: %w", err)
	}
	log.Printf("服务 %s 已注册到 Consul，健康检查地址 %s", serviceName, grpcAddr)
	return nil
}

// outboundIP 通过 UDP 连接获取本机出口 IP，不会真正发出业务流量。
func outboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
