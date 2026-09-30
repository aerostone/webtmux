#!/usr/bin/env bash
# 回滚 pi-tui 刷新补丁（还原 33 → 16）
set -euo pipefail

PI_BIN="$(readlink -f "$(command -v pi)" 2>/dev/null || true)"
if [ -z "$PI_BIN" ]; then echo "错误: 找不到 pi 可执行文件" >&2; exit 1; fi
NODE_MODULES_DIR="$(printf '%s' "$PI_BIN" | sed -n 's|\(.*lib/node_modules\)/.*|\1|p')"
if [ -z "$NODE_MODULES_DIR" ]; then echo "错误: 无法推断 node_modules 根目录" >&2; exit 1; fi
TUI_JS="$NODE_MODULES_DIR/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/tui.js"
BAK="$TUI_JS.bak-refresh-patch"

if [ ! -f "$BAK" ]; then
  echo "未找到备份 $BAK，尝试直接 sed 还原："
  sed -i 's/static MIN_RENDER_INTERVAL_MS = 33; \/\/ \[webtmux-refresh-patch\] 30fps/static MIN_RENDER_INTERVAL_MS = 16;/' "$TUI_JS" || true
else
  cp "$BAK" "$TUI_JS"
  rm -f "$BAK"
fi
node --check "$TUI_JS" && echo "OK: 已还原，语法检查通过。"
