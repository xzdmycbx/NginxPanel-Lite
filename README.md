# NginxPanel-Lite

一个**类 1Panel 的轻量多用户面板**,第一阶段专注 **nginx 反向代理**的可视化管理:域名绑定、反向代理、SSL 证书、以及生成的 nginx 配置文件,全部可视化、可编辑。

- 🔐 多用户 + 账号密码登录,**强制 TOTP 两步验证**(首登强制绑定)
- 👮 首次启动注册即管理员;管理员可增删用户、重置普通用户密码/TOTP(**不能管理其他管理员的密码与 TOTP**);用户可改本人密码(账号名不可改)
- 🌿 站点反向代理:域名绑定、**自定义多个 location 路径**(每路径独立的 `proxy_pass`、WebSocket、**可视化缓存开关**、额外指令)、强制 HTTPS、配置预览与备份回滚
- 🗂️ **1Panel 式嵌套 include**:主 `nginx.conf`(面板托管、**不可编辑**)→ 每域名 `site.conf`(**可编辑**)→ 每个 location 单独成文件(**可编辑**,含 `location{}` 块);整站目录原子替换 + `nginx -t` 失败回滚
- 📈 **每个域名独立的 access/error 日志**查看(可刷新、管理员可清空)
- 📜 SSL:**手动上传** + **Let's Encrypt 自动签发/续期**(`go-acme/lego` HTTP-01)
- 🧾 全局操作日志(所有用户可见,记录操作人 / IP / 中文详情)
- 🐳 docker-compose 编排,**nginx 独立容器**,面板经 Docker socket 安全地 `nginx -t` → reload(失败自动回滚)

技术栈:Go(gin + GORM + 纯 Go SQLite)+ React(Vite + Tailwind + shadcn 风格 UI),清新简约圆角主题。

---

## 快速开始

```bash
cp .env.example .env
# 按需修改 .env(本地纯 HTTP 访问请将 PANEL_COOKIE_SECURE 设为 false)
docker compose up -d --build
```

打开 `http://<服务器IP>:8080`:

1. 首次访问进入**初始化页面**,创建第一个管理员账号。
2. 系统强制**绑定两步验证**:用 Authenticator(Google/Microsoft Authenticator、1Password 等)扫码,输入 6 位验证码完成绑定。
3. 进入控制台:**站点管理**新建反向代理,**用户管理**(仅管理员)添加成员,**操作日志**查看全员操作。

终端用户访问的站点由 **nginx 容器**在 `80/443` 提供;面板管理界面在 `8080`。

---

## 架构

```
浏览器 ──8080──▶ panel 容器 (Go)  ──写配置/证书──▶ 共享卷 ◀──读──  nginx 容器 ──80/443──▶ 终端用户
                    │  └─ lego 签发证书(HTTP-01,写 acme-webroot)
                    └─ /var/run/docker.sock ──▶ docker exec nginx -t / -s reload
```

共享卷(两个容器挂载在相同路径,保证生成的路径在 nginx 内有效):

| 卷 | 路径 | 内容 |
|---|---|---|
| `nginx-config` | `/etc/nginx-panel` | `nginx.conf`、`sites/site-<id>/{site.conf,locations/*.conf}`、`logs/site-<id>/{access,error}.log`、`certs/`、`backups/` |
| `acme-webroot` | `/var/www/acme-webroot` | HTTP-01 challenge token |
| `paneldata` | `/data`(仅面板) | `panel.db`、ACME 账户 |

主 `nginx.conf` 由**面板托管并 seed**(不可编辑),nginx 以 `nginx -c /etc/nginx-panel/nginx.conf` 启动;它 `include sites/site-*/site.conf`,每个 `site.conf` 再 `include sites/site-<id>/locations/*.conf`。内置默认 `:80` server 使**首启即可签发证书**。日志写在同一共享卷,面板直接读取(无需额外卷)。

---

## 本地开发

**后端**(需要写共享卷;无 Docker 时用 dry-run 跳过 nginx 控制):

```bash
PANEL_NGINX_DRYRUN=true PANEL_COOKIE_SECURE=false go run ./cmd/server
# 监听 :8080
```

**前端**(Vite 开发服务器,`/api` 代理到后端):

```bash
cd web
npm install
npm run dev      # http://localhost:5173
```

**测试**:

```bash
go test ./...
```

---

## SSL / Let's Encrypt 说明

- 默认使用 **staging 测试环境**(`ACME_STAGING=true` / 站点环境选「测试」),额度近乎无限,先验证流程;正式签发请切到 production。
- HTTP-01 需要**域名已解析到本机**且 **80 端口可达**。
- 证书到期前 30 天**每日自动续期**;也可在站点 SSL 面板手动「立即续期」。

## 安全说明

- **Docker 访问经 socket-proxy 收敛**:面板**不再直挂** `/var/run/docker.sock`,而是通过 `tecnativa/docker-socket-proxy` 只放行 `containers`/`exec`/`version`(socket-proxy 自身以只读挂载真实 socket)。残余风险:`exec` 仍能对容器执行命令,但已无法创建特权容器挂载宿主根目录。panel/proxy 均开启 `no-new-privileges` 并设资源上限。
- 登录态使用 httpOnly + SameSite=Strict 的 JWT Cookie;TOTP 密钥以 AES-GCM 静态加密入库;改密/重置密码会使旧会话失效。
- **强制 TOTP**:未绑定且未通过验证前,任何业务接口都不可访问;enroll/activate 仅接受未绑定会话,杜绝"凭密码重置他人 TOTP"的绕过。
- **两步验证恢复**:管理员可在「用户管理」重置某用户的 TOTP;若**唯一管理员**丢失验证器,用应急命令解锁:
  ```bash
  docker compose exec panel /panel -reset-totp <用户名>
  ```

## 范围(第一阶段)

仅反向代理;单面板副本(SQLite);仅中文界面。后续:静态站点、深色模式、DNS-01 通配符证书、告警通知等。
