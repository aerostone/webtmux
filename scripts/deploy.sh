#!/usr/bin/env bash
# webtmux 生产部署脚本
# 用法: ./scripts/deploy.sh
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BINARY_NAME="webtmux"
INSTALL_PATH="/usr/local/bin/$BINARY_NAME"
SERVICE_FILE="$PROJECT_DIR/deploy/webtmux.service"
SYSTEMD_DIR="$HOME/.config/systemd/user"
ENV_DIR="$HOME/.config/webtmux"
ENV_FILE="$ENV_DIR/env"

cd "$PROJECT_DIR"

# ─── 1. 构建 ───
echo "▶ 构建 $BINARY_NAME ..."
export GOTOOLCHAIN=auto
# 国内用户可设置 GOPROXY=https://goproxy.cn,direct
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
go build -o "$BINARY_NAME" ./cmd/webtmux

if [ ! -f "$BINARY_NAME" ]; then
    echo "✗ 构建失败" >&2
    exit 1
fi
echo "  ✓ 构建成功: $(ls -lh $BINARY_NAME | awk '{print $5}')"

# ─── 2. 安装二进制 ───
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
echo "▶ 安装到 $INSTALL_DIR/$BINARY_NAME ..."
mkdir -p "$INSTALL_DIR"
cp "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod 755 "$INSTALL_DIR/$BINARY_NAME"
echo "  ✓ 已安装"

# ─── 3. 安装 systemd 服务 ───
echo "▶ 安装 systemd 服务 ..."
mkdir -p "$SYSTEMD_DIR"
# 替换 ExecStart 路径为实际安装路径
sed "s|/usr/local/bin/$BINARY_NAME|$INSTALL_DIR/$BINARY_NAME|g" "$SERVICE_FILE" > "$SYSTEMD_DIR/webtmux.service"
systemctl --user daemon-reload
echo "  ✓ 服务文件已安装"

# ─── 4. 检查环境配置 ───
if [ ! -f "$ENV_FILE" ]; then
    echo "▶ 创建环境配置 ..."
    mkdir -p "$ENV_DIR"
    cp "$PROJECT_DIR/deploy/env.example" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    echo "  ⚠ 请编辑 $ENV_FILE 设置密码"
    echo "    然后运行: systemctl --user start webtmux"
    exit 0
fi
echo "  ✓ 环境配置已存在"

# ─── 5. 重启服务 ───
echo "▶ 重启 webtmux ..."
systemctl --user restart webtmux
sleep 2

if systemctl --user is-active --quiet webtmux; then
    echo "  ✓ webtmux 已启动"
    echo ""
    echo "  端口: 3400"
    echo "  日志: journalctl --user -u webtmux -f"
    echo "  状态: systemctl --user status webtmux"
else
    echo "  ✗ 启动失败，查看日志:" >&2
    echo "    journalctl --user -u webtmux -n 20" >&2
    exit 1
fi
