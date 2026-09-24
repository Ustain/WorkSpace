# 多阶段构建：编译产物是一个静态单文件，前端与数据库驱动都已内嵌。
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 纯 Go 的 SQLite 驱动，无需 CGO，可直接产出静态二进制。
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ams .

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /out/ams /app/ams

# SQLite 数据文件挂出来，容器重建不丢数据。
VOLUME ["/app/data"]
ENV AMS_ADDR=":8080" AMS_DB="data/ams.db"
EXPOSE 8080

ENTRYPOINT ["/app/ams"]
