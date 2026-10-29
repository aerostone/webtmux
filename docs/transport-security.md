# 安全传输与 Nginx 配置

webtmux 使用 Go 标准库 TLS，不自定义加密协议或密码套件。默认关闭 TLS；直接启用时最低 TLS 1.2，配置错误或证书、私钥、客户端 CA 加载失败会停止启动，不会自动生成证书或退回 HTTP。修改配置后重启服务，证书更新也需要重启。

## 同机 Nginx：浏览器 HTTPS → 回环 HTTP

这是推荐的同机部署方式。将下列内容写入 `~/.config/webtmux/config.yaml`，密码由同目录 `env` 中的 `WEBTMUX_PASS` 提供：

```yaml
listen_addr: "127.0.0.1:3400"
base_path: "/tmux"
tls_enabled: false
tls_client_auth: "none"
trusted_proxy_cidrs: ["127.0.0.1/32"]
max_login_attempts: 5
login_window_sec: 300
login_lockout_sec: 900
```

复制 [Nginx 模板](../deploy/nginx.conf.example)，替换域名和证书路径，在部署环境运行 `nginx -t` 后重载 Nginx，并重启 webtmux。浏览器访问 `https://terminal.example.com/tmux/`。

模板中的 `proxy_pass http://127.0.0.1:3400` **没有尾部 `/`**：页面、API、WebSocket 和下载都保留 `/tmux/` 前缀，webtmux 自行剥离前缀。不要再加路径 rewrite 或 `proxy_cookie_path`。`/tmux` 会跳转到 `/tmux/`，查询参数保留；Cookie 设置、续期、退出都使用 `/tmux/`。

`base_path` 支持多级 ASCII 路径，如 `/tools/tmux`，不支持查询、片段、百分号编码或 `.` / `..` 路径段。留空或 `/` 表示根路径；根路径部署时把 Nginx 的 `location /tmux/` 改为 `location /`，移除裸 `/tmux` 重定向。

Nginx 应保留 `Host`（包括非标准端口），设置 WebSocket Upgrade，并覆盖 `X-Forwarded-For` 和 `X-Forwarded-Proto`。仅将实际代理地址加入 `trusted_proxy_cidrs`。webtmux 从代理链右侧剥离可信地址，首个不可信地址用于白名单和认证限流；未配代理或直连来源不可信时使用连接来源，忽略转发头。HTTPS 标识仅接受可信直连代理提供的单个 `https` 值，以设置 Secure Cookie。多层代理时，最外层应覆盖客户端传入的转发头，后续可信代理追加真实上一跳。

## 浏览器客户端证书认证

需要浏览器证书时，在模板的 HTTPS `server` 中启用：

```nginx
ssl_client_certificate /etc/nginx/certs/browser-client-ca.pem;
ssl_verify_client on;
```

浏览器导入由此 CA 签发、包含客户端认证用途的证书及私钥。客户端证书验证发生在 Nginx 的 TLS 握手阶段，作用于对应的 TLS 入口；建议使用独立域名。只有 HTTP 80 的入口无法进行浏览器 TLS 证书认证，需要提供 HTTPS 入口。

同机回环上游可以继续 HTTP，webtmux 不需要再次处理浏览器证书。Nginx 客户端证书与浏览器证书属于不同连接，不能把 Nginx 的上游身份视为浏览器身份。证书准入与应用密码/TOTP/WebAuthn 登录叠加，证书不会自动登录应用。

## webtmux 直接提供 TLS 或 mTLS

```yaml
listen_addr: ":3400"
base_path: "/tmux"
auth_pass: "replace-with-a-strong-password"
tls_enabled: true
tls_cert_file: "/etc/webtmux/server-fullchain.pem"
tls_key_file: "/etc/webtmux/server-key.pem"
tls_client_auth: "require_and_verify"
tls_client_ca_file: "/etc/webtmux/client-ca.pem"
trusted_proxy_cidrs: []
```

运行 `webtmux -config /path/to/config.yaml`，访问 `https://证书匹配的域名:3400/tmux/`。服务器证书须匹配访问域名并被客户端信任。严格 mTLS 使用 Go 的 `RequireAndVerifyClientCert`，只允许持有指定 CA 签发、有效且具有客户端认证用途证书的连接。这里只做 CA 证书准入，不做证书到应用用户的映射，也不实现 CRL/OCSP 撤销检查。

只需要服务器 TLS 时，将 `tls_client_auth` 设为 `none`，移除 `tls_client_ca_file`。证书链与私钥必须同时配置；启用 TLS 时不可再设置旧的 `no_tls: true` / `-no-tls`。命令行布尔值支持 `-tls=false`、`-no-tls=false`；配置中的无效布尔值和非正限流参数会报错。

## 跨机器代理的上游 mTLS

后端使用上一节的严格 mTLS 配置，将监听地址改成后端私网地址，并将 `trusted_proxy_cidrs` 限定为 Nginx 的实际出口地址。后端服务器证书的 SAN 包含 `webtmux.internal`，Nginx 能将此域名解析到后端。在 Nginx 模板的 `location` 中替换 `proxy_pass` 并添加：

```nginx
proxy_pass https://webtmux.internal:3400;
proxy_ssl_server_name on;
proxy_ssl_name webtmux.internal;
proxy_ssl_verify on;
proxy_ssl_verify_depth 2;
proxy_ssl_trusted_certificate /etc/nginx/certs/backend-server-ca.pem;
proxy_ssl_certificate /etc/nginx/certs/nginx-client-fullchain.pem;
proxy_ssl_certificate_key /etc/nginx/certs/nginx-client-key.pem;
proxy_ssl_protocols TLSv1.2 TLSv1.3;
```

保留模板中的 Host、转发头和 WebSocket 设置。这里 Nginx 验证后端 CA 与名称，后端验证 Nginx 客户端证书；浏览器证书仍由外侧 Nginx 单独验证。

## 认证限流语义

密码/TOTP 和 WebAuthn 完成登录的提交按客户端 IP 共用 `max_login_attempts` 额度。提交在验证前原子计数，格式错误或缺少认证会话也消耗额度；成功登录重置计数。达到阈值的那次请求仍完成验证，后续请求返回 `429 Too Many Requests`，`Retry-After` 为剩余锁定秒数。

WebAuthn 挑战请求使用同参数的独立额度，同时遵守登录提交的锁定状态，避免一次正常指纹登录被重复计数。成功登录也重置挑战额度。统计窗口和锁定时间分别由 `login_window_sec`、`login_lockout_sec` 控制，三项都必须为正数。修改后重启生效。

限流状态仅保存在当前进程内，重启会清空，多实例不共享。共享出口的客户端也共享 IP 额度；反向代理部署务必正确配置可信代理。Cookie Path 用于正常挂载作用域，同域多个应用如需浏览器安全隔离，应使用不同域名。
