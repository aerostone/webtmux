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
- 安全体系：IP 白名单、登录限流、TOTP、HMAC Cookie、WebSocket 排他连接

## 安装部署

### 前置条件

- Go 1.24+（推荐用 [mise](https://mise.jdx.dev/) 管理）
- tmux 已安装并有运行中的会话

### 生产部署

```bash
# 1. 克隆项目
git clone <repo-url> webtmux && cd webtmux

# 2. 一键部署（构建 + 安装 systemd 服务）
./scripts/deploy.sh

# 3. 首次运行会提示设置密码，编辑环境配置
vim ~/.config/webtmux/env
```

环境配置文件 `~/.config/webtmux/env`：

```bash
# 必填
WEBTMUX_PASS=your-strong-password

# 可选
WEBTMUX_TOTP_SECRET=          # TOTP 密钥（启用二步验证）
WEBTMUX_IP_WHITELIST=         # IP 白名单，逗号分隔
WEBTMUX_FILE_ROOTS=           # 文件管理器额外根目录
WEBTMUX_SENTRY_DSN=           # GlitchTip/Sentry 错误监控 DSN
```

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

### CLI 参数

| 参数 | 环境变量 | 默认值 | 说明 |
|------|----------|--------|------|
| `-listen` | `WEBTMUX_LISTEN` | `:8080` | 监听地址 |
| `-pass` | `WEBTMUX_PASS` | — | 登录密码 |
| `-no-tls` | `WEBTMUX_NO_TLS` | false | 禁用 TLS |
| `-tls-cert` | `WEBTMUX_TLS_CERT` | 自动生成 | TLS 证书路径 |
| `-tls-key` | `WEBTMUX_TLS_KEY` | 自动生成 | TLS 私钥路径 |
| `-totp-secret` | `WEBTMUX_TOTP_SECRET` | — | TOTP 密钥 |
| `-ip-whitelist` | `WEBTMUX_IP_WHITELIST` | — | IP 白名单 |
| `-file-roots` | `WEBTMUX_FILE_ROOTS` | — | 文件管理器根目录 |
| `-sentry-dsn` | `WEBTMUX_SENTRY_DSN` | — | 错误监控 DSN |

### config.yaml

```yaml
listen_addr: ":8080"
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

## 安全机制

| 层级 | 机制 | 说明 |
|------|------|------|
| 传输 | TLS 1.2+ | 默认启用，证书自动生成在 `~/.webtmux/` |
| 网络 | IP 白名单 | CIDR / 单 IP，可选 |
| 安全头 | HTTP Headers | X-Content-Type-Options, X-Frame-Options, CSP |
| 认证 | 密码 + TOTP | 登录页面，支持二步验证 |
| 认证 | WebAuthn | 指纹/人脸/Passkey，可替代密码 |
| 会话 | HMAC-SHA256 Cookie | HttpOnly + SameSite=Lax |
| 防暴力 | 速率限制 | 5 次/5 分钟，锁定 15 分钟 |
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
│       ├── tls.go                 # TLS 证书自动生成
│       ├── activity.go            # 操作记录
│       ├── filemanager.go         # 文件管理 API
│       ├── system.go              # 系统信息 API
│       └── web/index.html         # 前端（xterm.js + 插件）
├── deploy/
│   ├── webtmux.service            # systemd 服务模板
│   └── env.example                # 环境配置模板
├── scripts/
│   ├── deploy.sh                  # 生产部署脚本
│   └── dev.sh                     # 开发启动脚本
└── config.yaml.example            # 配置文件示例
```

## License

MIT
