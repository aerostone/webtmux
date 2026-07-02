# webtmux 安全评审

评审日期: 2026-07-02
评审范围: 全部代码（22 commits）

## 总体评估

安全体系设计良好，采用多层防御策略。发现 **2 个中等问题** 和 **4 个低风险建议**。

| 等级 | 数量 | 说明 |
|------|------|------|
|  高 | 0 | 无 |
|  中 | 2 | 需要修复 |
|  低 | 4 | 建议改进 |

---

##  中等问题

### 1. Session Cookie 缺少 Secure 标志

**文件**: `internal/auth/session.go:38`

```go
http.SetCookie(w, &http.Cookie{
    Name:     cookieName,
    Value:    sig,
    Path:     "/",
    HttpOnly: true,
    SameSite: http.SameSiteLaxMode,
    MaxAge:   int(cookieTTL.Seconds()),
})
```

**问题**: Cookie 未设置 `Secure: true`。当 TLS 启用时，Cookie 可能通过明文 HTTP 传输（如用户误用 http:// 访问）。

**修复**: 当 TLS 启用时自动设置 Secure 标志。

### 2. 密码比较未使用常量时间函数

**文件**: `internal/server/server.go:267`

```go
if req.Password != s.cfg.AuthPass {
```

**问题**: Go 的 `!=` 字符串比较不是常量时间的，理论上存在时序攻击风险。

**缓解**: 速率限制器（5次/5分钟锁定）大幅提高了攻击难度，实际风险较低。

**修复**: 使用 `crypto/subtle.ConstantTimeCompare`。

---

##  低风险建议

### 3. 缺少 Content-Security-Policy 头

**文件**: `internal/server/server.go:170`

当前安全头：
```go
w.Header().Set("X-Content-Type-Options", "nosniff")
w.Header().Set("X-Frame-Options", "DENY")
w.Header().Set("X-XSS-Protection", "1; mode=block")
```

**建议**:
- 添加 CSP 头限制资源加载来源
- 移除 `X-XSS-Protection`（已废弃，某些旧浏览器可能引入问题）

### 4. Shell 命令注入风险（理论性）

**文件**: `internal/server/system.go`

```go
exec.Command("sh", "-c", `top -bn1 | grep "Cpu(s)" | awk '{print $2}'`)
```

**现状**: 所有 shell 命令均为硬编码常量，无用户输入拼接，**当前安全**。

**建议**: 添加注释标明"禁止拼接用户输入"，防止未来维护者引入注入。

### 5. 文件上传无单文件大小限制

**文件**: `internal/server/filemanager.go:137`

```go
r.ParseMultipartForm(100 << 20)  // 100MB 总限制
```

**现状**: 100MB 是总表单限制，未对单个文件设置上限。

**建议**: 在 `io.Copy` 循环中添加单文件大小检查。

### 6. Activity 日志无敏感信息过滤

**文件**: `internal/server/activity.go`

**现状**: 活动日志记录了 IP 地址、文件路径等信息，存储在 `~/.webtmux/activities.json`。

**建议**: 确认 `activities.json` 权限为 0600（当前已是）。

---

## 安全机制验证

| 机制 | 状态 | 说明 |
|------|------|------|
| IP 白名单 | ✅ | 支持 CIDR 和单 IP |
| 速率限制 | ✅ | 5次/5分钟，锁定15分钟，内存存储重启重置 |
| HMAC-SHA256 Cookie | ✅ | HttpOnly + SameSite=Lax，24小时过期 |
| TOTP 二步验证 | ✅ | 可选启用 |
| WebAuthn 指纹 | ✅ | 平台认证器，凭据本地存储 |
| WebSocket Origin 检查 | ✅ | 严格比较 Host |
| WebSocket 排他连接 | ✅ | 每 session 仅一个连接 |
| 路径遍历防护 | ✅ | validatePath 限制在允许目录内 |
| 文件名净化 | ✅ | filepath.Base 防止路径注入 |
| Recovery 中间件 | ✅ | 捕获 panic 返回 500 |
| 日志中间件 | ✅ | 记录 method/path/status/duration/remote |

---

## 修复建议优先级

1. **Session Cookie Secure 标志** - 中等优先级
2. **密码常量时间比较** - 中等优先级
3. **添加 CSP 头** - 低优先级
4. **移除 X-XSS-Protection** - 低优先级
5. **文件上传单文件限制** - 低优先级
