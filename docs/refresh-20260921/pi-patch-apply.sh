#!/usr/bin/env bash
# pi-tui 刷新补丁：MIN_RENDER_INTERVAL_MS 16 → 33 (60fps → 30fps)
# 背景：pi TUI 流式输出时每帧都按行粒度整行重发，tmux 3.0 无法抑制，
#       经 webtmux 后形成 ~20KB/s、~160 行/秒的持续刷新洪流。
#       30fps 对阅读完全无感，帧数(=字节数)直接减半。
# 用法：bash apply.sh   （可重复执行；重复执行会幂等跳过）
# 回滚：bash revert.sh
set -euo pipefail

# 动态定位全局 pi 安装下的 pi-tui dist
PI_BIN="$(readlink -f "$(command -v pi)" 2>/dev/null || true)"
if [ -z "$PI_BIN" ]; then
  echo "错误: 找不到 pi 可执行文件" >&2
  exit 1
fi
# pi 可能是符号链接 → 真实文件在 pi-coding-agent/dist/bundle/ 下；
# 用路径里的 lib/node_modules 锚点回推 node_modules 根目录
NODE_MODULES_DIR="$(printf '%s' "$PI_BIN" | sed -n 's|\(.*lib/node_modules\)/.*|\1|p')"
if [ -z "$NODE_MODULES_DIR" ]; then
  echo "错误: 无法从 $PI_BIN 推断 node_modules 根目录（pi 版本可能已变化，请人工检查）" >&2
  exit 1
fi
TUI_JS="$NODE_MODULES_DIR/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/tui.js"
if [ ! -f "$TUI_JS" ]; then
  echo "错误: 找不到 $TUI_JS（pi 版本可能已变化，请人工检查）" >&2
  exit 1
fi

if grep -q 'MIN_RENDER_INTERVAL_MS = 33;' "$TUI_JS"; then
  echo "已经是 33，无需操作。"
  exit 0
fi

MATCHES=$(grep -c 'static MIN_RENDER_INTERVAL_MS = 16;' "$TUI_JS" || true)
if [ "$MATCHES" -ne 1 ]; then
  echo "错误: 期望在 $TUI_JS 中找到 1 处 'MIN_RENDER_INTERVAL_MS = 16'，实际 $MATCHES 处" >&2
  exit 1
fi

cp "$TUI_JS" "$TUI_JS.bak-refresh-patch"
sed -i 's/static MIN_RENDER_INTERVAL_MS = 16;/static MIN_RENDER_INTERVAL_MS = 33; \/\/ [webtmux-refresh-patch] 30fps/' "$TUI_JS"
node --check "$TUI_JS" && echo "OK: $TUI_JS 已打补丁 (16→33)，语法检查通过。"
echo "注意: 需要重启 pi 会话生效；npm 升级 pi 后补丁会丢失，届时重跑本脚本即可。"
