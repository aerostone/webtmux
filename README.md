# webtmux

手机浏览器安全访问远程 tmux 会话。Go 单二进制，xterm.js 前端。

## 快速启动

```bash
# 构建
mise run build
# 或
go build -o webtmux ./cmd/webtmux

# 启动（默认 TLS 加密，证书自动生成在 ~/.webtmux/）
./webtmux -pass your-strong-password

# 禁用 TLS（不推荐）
./webtmux -pass your-password -no-tls
```

手机浏览器访问 `https://<server-ip>:8080`（首次访问需接受自签名证书）

## TLS 加密

**默认启用**，无需手动配置。首次启动时自动生成 ECDSA-P256 自签名证书，持久化在 `~/.webtmux/`：

```
~/.webtmux/
├── cert.pem    # 证书（10年有效期）
└── key.pem     # 私钥（权限 0600）
```

| 场景 | 行为 |
|------|------|
| 首次启动 | 自动生成证书，启动 HTTPS + WSS |
| 再次启动 | 复用已有证书 |
| `--no-tls` | 禁用 TLS，使用明文 HTTP + WS |
| 自定义证书 | `--tls-cert` / `--tls-key` 指定 |

```bash
# 自定义证书（如 Let's Encrypt）
./webtmux -tls-cert /etc/letsencrypt/live/domain/cert.pem \
          -tls-key /etc/letsencrypt/live/domain/privkey.pem \
          -pass your-password
```

## 配置

优先级：**CLI flags > 环境变量 > YAML 文件 > 默认值**

### config.yaml

```yaml
listen_addr: ":8080"
tls_cert_file: ""                  # 留空自动使用 ~/.webtmux/cert.pem
tls_key_file: ""                   # 留空自动使用 ~/.webtmux/key.pem
no_tls: false                      # 禁用 TLS（不推荐）
auth_pass: "your-strong-password"
totp_enabled: true
totp_secret: "BASE32SECRET"
session_secret: "random-string"
max_login_attempts: 5
login_window_sec: 300
login_lockout_sec: 900
ip_whitelist: []
tmux_socket: ""
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `WEBTMUX_LISTEN` | 监听地址 |
| `WEBTMUX_PASS` | 登录密码 |
| `WEBTMUX_CONFIG` | YAML 配置路径 |
| `WEBTMUX_TLS_CERT` | TLS 证书路径 |
| `WEBTMUX_TLS_KEY` | TLS 私钥路径 |
| `WEBTMUX_SOCKET` | tmux socket 路径 |
| `WEBTMUX_IP_WHITELIST` | 逗号分隔 IP 白名单 |
| `WEBTMUX_SESSION_SECRET` | Cookie 签名密钥 |

## 安全机制

| 层级 | 机制 |
|------|------|
| 1. 传输 | TLS 1.2+（默认启用，WSS 全程加密） |
| 2. 网络 | IP 白名单（CIDR / 单 IP，可选） |
| 3. 安全头 | X-Content-Type-Options, X-Frame-Options, CSP |
| 4. 认证 | 登录页面（密码 + 可选 TOTP 二步验证） |
| 5. 会话 | HMAC-SHA256 签名 Cookie（HttpOnly + SameSite=Strict） |
| 6. 防暴力 | 速率限制（默认 5 次/5 分钟，锁定 15 分钟） |
| 7. WebSocket | 严格 Origin 检查 |
| 8. 进程 | 以启动用户 UID 运行，不越权 |

## 会话管理

前端 **Sessions** 标签页：

- 创建新会话：输入名称 → 点击 **+ New**
- 连接会话：点击 **Connect** → 跳转到 Terminal 标签
- 杀死会话：点击 **Kill**

## 插件系统

前端支持通过 `registerPlugin()` 注册业务插件。在浏览器 Console 中测试：

```javascript
registerPlugin({
  name: 'Quick Commands',
  description: '常用命令快捷按钮',
  render() {
    const d = document.createElement('div');
    ['htop', 'df -h', 'free -m'].forEach(cmd => {
      const b = document.createElement('button');
      b.className = 'btn btn-outline';
      b.style.margin = '4px';
      b.textContent = cmd;
      b.addEventListener('click', () => { send(cmd + '\r') });
      d.appendChild(b);
    });
    return d;
  }
});
```

## 开发

```bash
mise install
mise run test
mise run lint
mise run dev
```

## 项目结构

```
webtmux/
├── cmd/webtmux/main.go           # 入口（自动 TLS）
├── internal/
│   ├── config/config.go          # 三级配置
│   ├── auth/                     # 认证（IP/TOTP/Session/RateLimit）
│   ├── tmux/                     # tmux 会话管理
│   └── server/
│       ├── server.go             # HTTP + WSS + API
│       ├── tls.go                # TLS 证书自动生成/持久化
│       └── web/index.html        # 前端（xterm.js + 插件）
├── mise.toml
└── config.yaml.example
```

## License

MIT
