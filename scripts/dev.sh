#!/usr/bin/env bash
# webtmux 开发模式脚本
# 用法: ./scripts/dev.sh [端口] [密码]
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

PORT="${1:-8080}"
PASS="${2:-dev123}"

export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=auto

echo "▶ 开发模式启动"
echo "  端口: $PORT"
echo "  密码: $PASS"
echo "  TLS:  关闭"
echo ""

# 确保没有占用端口的进程
if lsof -ti:$PORT >/dev/null 2>&1; then
    echo "  ⚠ 端口 $PORT 已占用，尝试释放..."
    lsof -ti:$PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

go run ./cmd/webtmux \
    -listen ":$PORT" \
    -no-tls \
    -pass "$PASS"
