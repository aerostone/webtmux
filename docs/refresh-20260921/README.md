# pi vs codex 刷新差异排查（2026-09-21）

## 结论（实测）

用 pty 以 104x38 挂 `tmux attach-session -t 0`（与 webtmux attach 路径完全相同）抓 25s：

- 总量 **510,819 B ≈ 20.4 KB/s**；
- **4025 次 `ESC[row;colH` 绝对定位（~161/s）**，每次都是「定位 + 整行 104 列文本 + 属性复位」≈115B；
- `ESC[2J`×1（仅 attach 首屏）、`ESC[3J`×0、`ESC[2K`×0 → 流式期间 **0 次整屏重绘**；
- 0 相对光标移动。

| | pi (TuiMainScreen, regular 模式) | codex (ratatui) |
|---|---|---|
| 渲染粒度 | **行级 diff**：任何一行变 1 个字符 → 重发整行(~115B) | **单元格级 diff**：只发真正变化的 cell |
| 帧率 | 60fps（`MIN_RENDER_INTERVAL_MS=16`） | ~10-20fps |
| 完成内容 | 留在视口内，之后每次仍参与 diff/重发 | 一次性写入 scrollback，不再触碰 |
| 实测/估算速率 | **~20KB/s**（实测） | ~1-3KB/s |

链路放大：pi 行级输出 → tmux 3.0 客户端 diff 也是**行级**（且会剥离
`ESC[?2026h/l`、`ESC[2K`），任何一行变化必然整行重发 → webtmux 原样透传
→ 浏览器 xterm 按行重绘。tmux 这一层无法抑制"真变化"，只能改源头。

## 本次改动

### webtmux 服务端 `internal/server/server.go`
1. **慢客户端 2s 写超时 → 主动断连**（场景1）：WS 写阻塞>2s 说明浏览器/网络
   严重落后。不再无限积压（积压会在网络恢复后"冲进来"造成快进式补刷），
   直接 markDead；客户端自动重连，新 attach 由 tmux 重发整屏 = 一帧干净画面。
2. **刷新遥测**（场景3：不依赖你配合跑 pi）：每次 WS 断连打一行 INFO：
   `out=总字节 clear2J=整屏清屏次数 clear3J=清scrollback次数 maxBurst1s=1秒最大突发 resizes=客户端resize次数`。
   看日志即可量化"刷新有多凶"。

### webtmux 客户端 `internal/server/web/index.html`
1. **resize 滞回**（A-1，场景2 的 resize 黑闪）：250ms settle + 回摆抵消 +
   小幅变化(Δrows≤2)再等 1.5s 确认。URL 栏收展/手势抖动不再触发
   `tmux resize → pi 高度变化 → fullRender 整屏黑闪`。
2. **后台 5s 主动断 WS**（A-2，场景1）：页面隐藏>5s 自动断开（服务端立即
   停止泵数据、tmux attach 释放）；回到前台自动重连 + tmux 整屏重发。
   不弹 overlay（自断抑制），用户无感。
3. **合帧窗口 32ms → 50ms**（A-4，场景2）：xterm 最大绘制频率 30fps→20fps，
   画面更"静"。改 `RX_COALESCE_MS` 一个常量可调。

### 环境
- `~/.bashrc` 追加 `alias pi='PI_DEBUG_REDRAW=1 pi'`：新 pi 会话的每次
  fullRender 会记原因到 `~/.pi/agent/logs/…/pi-debug.log`（resize /
  width/height changed / 首帧）。alias 可能被绕过，所以服务端遥测是主力。

### pi 侧补丁（**未应用**，等你点头）
`pi-patch-apply.sh`：把 pi-tui `MIN_RENDER_INTERVAL_MS` 16→33（60→30fps）。
实测 pi 流式期间 0 次 fullRender，主要洪就是 60fps 行级增量帧，帧率减半
= 字节减半（~20KB/s → ~10KB/s）。单行改动、可回滚（`pi-patch-revert.sh`）。
clearOnShrink 经确认**默认关闭**（你的 settings.json 无 `terminal.clearOnShrink`），
不需要额外 patch。

## 验证方法

1. `go build ./... && go test ./...` 已通过。
2. 重启 webtmux 后用手机/浏览器连一个正在流式输出的 pi：
   - 观察屏幕是否更"静"（20fps + 无 resize 黑闪）；
   - 断网 1 分钟再恢复：应看到"断开→自动重连→一帧完整画面"，而不是
     快进补刷一大段；
   - 手机锁屏 30s 再解锁：同上；
   - webtmux 日志里找 `ws disconnect: session=… out=… clear2J=… resizes=…`
     对比改动前后。
3. pi 补丁（可选）：`bash docs/refresh-20260921/pi-patch-apply.sh`，重开一个
   pi 会话，再用同样的 pty 抓取脚本对比字节速率（预期 ~10KB/s）。

## 后续可选（未做）

- **tmux 3.0 → 3.4/3.5**：新版有行滚动序列优化（`S`/`T`），纯滚动更省字节。
  需单独排期（动 tmux server 会影响所有会话）。
- **WebSocket permessage-deflate**：终端文本 3-10 倍压缩，坏网场景积压
  字节再降一个量级。gorilla v1.5.3 直接支持，改动极小。
- **pi 上游**：行级 diff + 60fps 对远端终端不友好，建议向上游反馈
  （30fps + 单元格级 diff 两个方向）。
