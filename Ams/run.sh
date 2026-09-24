#!/usr/bin/env bash
# 一键启动：编译并运行资产管理系统。首次运行会自动建库并灌入演示数据。
set -euo pipefail

cd "$(dirname "$0")"
ADDR="${AMS_ADDR:-:8080}"

echo "[1/2] 正在编译..."
go build -o ams .

echo "[2/2] 启动服务，浏览器访问 http://localhost${ADDR}"
echo "按 Ctrl+C 退出"
AMS_ADDR="$ADDR" ./ams
