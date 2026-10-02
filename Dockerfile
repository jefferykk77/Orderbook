# syntax=docker/dockerfile:1

# go.mod 目前声明的是 Go 1.22.1，builder 固定在 1.22 系列。
FROM golang:1.22-alpine AS builder

WORKDIR /src

# 先复制依赖文件，Docker 可以缓存 go mod download 这一层。
COPY go.mod go.sum ./
RUN go mod download

# 服务端只依赖根撮合包、internal 服务层、生成代码和 server 入口。
# 不复制 benchmark、docs、scripts，减少镜像构建上下文对构建的影响。
COPY errors.go order.go orderbook.go orderqueue.go orderside.go side.go ./
COPY api/ api/
COPY internal/ internal/
COPY cmd/server/ cmd/server/

RUN mkdir -p /out

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -mod=readonly \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w" \
      -o /out/orderbook-server \
      ./cmd/server

# 使用 Alpine 保留基础调试工具和根证书；应用进程仍使用非 root 用户运行。
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder --chown=65534:65534 /out/orderbook-server /usr/local/bin/orderbook-server

ENV TRADE_LISTEN_ADDR=:50051

EXPOSE 50051

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/orderbook-server"]
