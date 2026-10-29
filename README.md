# webtmux

手机浏览器安全访问远程 tmux 会话。Go 单二进制，xterm.js 前端。

## 功能

- 终端访问：通过浏览器连接 tmux 会话，支持多会话切换
- 移动端优化：底部工具栏（tmux 快捷键、方向键、Ctrl 组合）
- 文件管理器：上传/下载/删除文件，支持拖拽、多根目录、symlink
- 快捷命令：常用命令片段一键执行，支持导入导出
- 系统监控：CPU/内存/磁盘/负载实时仪表盘
- Activity 起始页：登录后显示最近操作记录
- 指纹认证：WebAuthn/Passkey 支持（手机指纹、Windows Hello）
- 安全体系：可选 TLS/mTLS、可信代理、认证限流、TOTP、HMAC Cookie、WebSocket 排他连接
- 反向代理：支持 `base_path`，可挂载在 Nginx 的 `/tmux/` 下

## 安装部署

### 前置条件

- Go 1.25+（推荐用 [mise](https://mise.jdx.dev/) 管理；Go 1.24 引导工具可通过 `GOTOOLCHAIN=auto` 获取）
- tmux 已安装并有运行中的会话

### 生产部署

```bash
# 1. 克隆项目
git clone <repo-url> webtmux && cd webtmux

# 2. 一键部署（构建 + 安装 systemd 服务）
./scripts/deploy.sh

# 3. 首次部署创建配置，设置密码和传输方式
vim ~/.config/webtmux/env
vim ~/.config/webtmux/config.yaml
```

环境配置文件 `~/.config/webtmux/env`：

```bash
# 必填
WEBTMUX_PASS=your-strong-password

# 可选：启用 TOTP 时同时设置开关和密钥
# WEBTMUX_TOTP_ENABLED=true
# WEBTMUX_TOTP_SECRET=YOUR_BASE32_SECRET
# IP 白名单（逗号分隔）、额外文件根目录、错误监控 DSN
# WEBTMUX_IP_WHITELIST=10.0.0.0/8
# WEBTMUX_FILE_ROOTS=/data,/srv
# WEBTMUX_SENTRY_DSN=your-dsn
```

部署模板默认监听 `127.0.0.1:3400`，使用 HTTP，建议同机由 Nginx 提供 HTTPS。传输、路径和限流在 `~/.config/webtmux/config.yaml` 中配置；环境变量可覆盖 YAML。

```bash
# 4. 启动服务
systemctl --user start webtmux

# 5. 开机自启（可选）
loginctl enable-linger $(whoami)
```

服务管理：

```bash
systemctl --user status webtmux    # 查看状态
systemctl --user restart webtmux   # 重启
journalctl --user -u webtmux -f    # 实时日志
```

### 更新部署

```bash
cd webtmux
git pull
./scripts/deploy.sh
```

## 开发

```bash
# 安装 Go 工具链
mise install

# 开发模式启动（端口 8080，无 TLS，密码 dev123）
./scripts/dev.sh

# 自定义端口和密码
./scripts/dev.sh 9090 mypass

# 运行测试
mise run test

# 代码检查
mise run lint
```

开发模式下访问 `http://localhost:8080`。

## 配置

优先级：**CLI flags > 环境变量 > YAML 文件 > 默认值**

使用 `webtmux -config /path/to/config.yaml` 或 `WEBTMUX_CONFIG` 指定文件；不指定时只读取环境变量和默认值。完整示例见 [config.yaml.example](config.yaml.example)。

### CLI 参数

| 参数 | 环境变量 | 默认值 | 说明 |
|------|----------|--------|------|
| `-listen` | `WEBTMUX_LISTEN` | `:8080` | 监听地址 |
| `-pass` | `WEBTMUX_PASS` | — | 登录密码 |
| `-base-path` | `WEBTMUX_BASE_PATH` | 空 | URL 挂载前缀，如 `/tmux` |
| `-tls` | `WEBTMUX_TLS_ENABLED` | false | 开启 TLS，必须指定证书和私钥 |
| `-no-tls` | `WEBTMUX_NO_TLS` | false | 兼容旧配置，与启用 TLS 冲突 |
| `-tls-cert` | `WEBTMUX_TLS_CERT` | 空 | TLS 证书链 PEM 路径 |
| `-tls-key` | `WEBTMUX_TLS_KEY` | 空 | TLS 私钥 PEM 路径 |
| `-tls-client-auth` | `WEBTMUX_TLS_CLIENT_AUTH` | none | `none` / `require_and_verify` |
| `-tls-client-ca` | `WEBTMUX_TLS_CLIENT_CA` | 空 | 严格 mTLS 的客户端 CA PEM 路径 |
| `-trusted-proxy-cidrs` | `WEBTMUX_TRUSTED_PROXY_CIDRS` | 空 | 可信代理 CIDR / IP，逗号分隔 |
| `-max-login-attempts` | `WEBTMUX_MAX_LOGIN_ATTEMPTS` | 5 | 每个客户端 IP 的认证尝试额度 |
| `-login-window` | `WEBTMUX_LOGIN_WINDOW_SEC` | 300 | 统计窗口，秒 |
| `-login-lockout` | `WEBTMUX_LOGIN_LOCKOUT_SEC` | 900 | 锁定时间，秒 |
| `-totp` | `WEBTMUX_TOTP_ENABLED` | false | 启用 TOTP |
| — | `WEBTMUX_TOTP_SECRET` | — | TOTP 密钥 |
| `-ip-whitelist` | `WEBTMUX_IP_WHITELIST` | — | IP 白名单 |
| `-file-roots` | `WEBTMUX_FILE_ROOTS` | — | 文件管理器根目录 |
| — | `WEBTMUX_SENTRY_DSN` | — | 错误监控 DSN |

### config.yaml

```yaml
listen_addr: ":8080"
base_path: ""
tls_enabled: false
tls_client_auth: "none"
trusted_proxy_cidrs: []
auth_pass: "your-strong-password"
totp_enabled: true
totp_secret: "BASE32SECRET"
session_secret: "random-string"
max_login_attempts: 5
login_window_sec: 300
login_lockout_sec: 900
ip_whitelist: []
tmux_socket: ""
file_roots: []
sentry_dsn: ""
```

TLS 默认关闭。直接开启 HTTPS 或严格 mTLS、挂载 Nginx 子路径的完整步骤见 [安全传输与 Nginx 配置](docs/transport-security.md)，可复制 [Nginx 模板](deploy/nginx.conf.example)。

**升级注意：**旧版的自动自签证书和 TLS 失败后回退 HTTP 已取消。原来直接使用 HTTPS 的部署必须在升级前显式设置 `tls_enabled: true`、证书和私钥路径；继续使用原证书也要显式指定。配置或证书无效时启动失败，不回退明文。systemd 模板不再写死 `-listen` 和 `-no-tls`；升级部署脚本会在缺少 YAML 时创建回环 HTTP 默认配置，请先准备好符合现网的 YAML 再运行脚本。

YAML 中的未知字段现在会报错，避免安全开关拼写错误被忽略。升级时请先移除无效或过期字段。

## 安全机制

| 层级 | 机制 | 说明 |
|------|------|------|
| 传输 | TLS 1.2+ / mTLS | 默认关闭；显式证书，严格客户端 CA 校验可选 |
| 代理 | 可信代理 CIDR | 仅信任配置代理的来源 IP 和 HTTPS 转发头 |
| 网络 | IP 白名单 | CIDR / 单 IP，可选 |
| 安全头 | HTTP Headers | X-Content-Type-Options, X-Frame-Options, CSP |
| 认证 | 密码 + TOTP | 登录页面，支持二步验证 |
| 认证 | WebAuthn | 指纹/人脸/Passkey，可替代密码 |
| 会话 | HMAC-SHA256 Cookie | HttpOnly + SameSite=Lax，HTTPS 时 Secure，Path 随挂载路径 |
| 防暴力 | 认证限流 | 默认 5 次/5 分钟，锁定 15 分钟；密码/TOTP 与 WebAuthn 共用提交额度，挑战请求另有同参数额度 |
| WebSocket | 排他连接 | 每个 session 仅允许一个 WS 连接 |
| WebSocket | Origin 检查 | 严格校验请求来源 |
| 文件管理 | 路径校验 | 限制在用户目录内，防止路径遍历 |

### 指纹认证

1. 登录后进入 **Sessions** 标签页
2. 点击 **启用指纹登录**
3. 完成设备指纹验证
4. 下次登录直接使用指纹，无需输入密码

## 内置插件

| 插件 | 功能 |
|------|------|
| 文件管理器 | 浏览/上传/下载/删除文件，支持多根目录、symlink、拖拽上传 |
| 快捷命令 | 常用命令片段管理，支持导入导出 |
| 系统监控 | CPU/内存/磁盘/负载/进程实时监控 |

## 项目结构

```
webtmux/
├── cmd/webtmux/main.go            # 入口
├── internal/
│   ├── config/config.go           # 三级配置（CLI/ENV/YAML）
│   ├── auth/                      # 认证中间件
│   │   ├── middleware.go          # 登录检查 + 安全头
│   │   ├── session.go             # HMAC-SHA256 Cookie
│   │   ├── webauthn.go            # WebAuthn 指纹认证
│   │   ├── ipfilter.go            # IP 白名单
│   │   ├── ratelimit.go           # 速率限制
│   │   └── totp.go                # TOTP 二步验证
│   ├── tmux/                      # tmux 会话管理
│   │   ├── manager.go             # 会话列表/创建/删除
│   │   └── session.go             # PTY 连接
│   └── server/
│       ├── server.go              # HTTP + WebSocket + API
│       ├── tls.go                 # 显式 TLS/mTLS 配置
│       ├── activity.go            # 操作记录
│       ├── filemanager.go         # 文件管理 API
│       ├── system.go              # 系统信息 API
│       └── web/index.html         # 前端（xterm.js + 插件）
├── deploy/
│   ├── webtmux.service            # systemd 服务模板
│   ├── nginx.conf.example         # HTTPS、WebSocket 与子路径代理
│   └── env.example                # 环境配置模板
├── scripts/
│   ├── deploy.sh                  # 生产部署脚本
│   └── dev.sh                     # 开发启动脚本
└── config.yaml.example            # 配置文件示例
```

## License

MIT
