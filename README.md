# LeMail

> 临时邮箱也可以很优雅：收邮件、抓验证码、看投递日志，一只小邮差全都帮你搞定。

[GitHub 仓库](https://github.com/PikachuCN/LeMail) · 作者：**PikachuCN** · Go + React + ArcoDesign · GPL-2.0-only

LeMail 是一个轻量级、自托管的临时邮箱系统。它会在你的服务器上开一个 SMTP 收信口，把邮件暂存在内存里，再用 WebSocket 推到浏览器；如果遇到 ChatGPT / OpenAI 这类验证码邮件，还能自动从正文里把验证码抠出来，放到界面上让你一键复制。

适合这些场景：

- 注册、登录、测试时临时收验证码，不想翻邮箱。
- 给团队自建一个可控、可清理、不落数据库的临时邮箱。
- 调试邮件投递链路，知道邮件到底有没有打到你的服务器。
- 给自动化脚本提供“生成邮箱 -> 读取邮件 -> 读取验证码”的 API。

## 亮点

- **开箱即用的临时邮箱**：随机邮箱、自定义前缀、多域名、实时推送都支持。
- **验证码自动提取**：管理员可创建“ChatGPT 登录”等项目，用正则自动抓取验证码。
- **AI 生成正则建议**：配置 OpenAI Key 后，可让 AI 根据邮件样本生成可复用的 Go 正则。
- **投递调试**：SMTP 连接、HELO、MAIL FROM、RCPT、DATA、拒收原因都能在后台看到。
- **管理员中台**：ArcoDesign 左侧菜单 + 卡片表格布局，管理邮件、配置、WebHook、API。
- **无数据库**：邮件、验证码、会话都在内存里，默认 1 小时清理；配置写入 JSON。
- **单二进制发布**：前端已 embed 到 Go 二进制，Release 下载后不需要 Node.js。
- **多种部署姿势**：Docker、systemd 一键脚本、宝塔反向代理都可以。

## 快速开始

### 方式一：下载 Release 直接跑

从 [Releases](https://github.com/PikachuCN/LeMail/releases) 下载对应系统的 zip，解压后：

```bash
cp config/config.example.json config/config.json
./lemail
```

Windows：

```powershell
copy config\config.example.json config\config.json
.\lemail.exe
```

默认入口：

- Web 控制台：`http://localhost:3000`
- SMTP 服务：示例配置默认 `0.0.0.0:2525`，生产公网收信建议改为 `0.0.0.0:25`

首次打开“管理员登录”页面时，如果还没有管理员密码，会进入初始化流程。

### 方式二：一键部署到 Linux systemd

适合云服务器、宝塔服务器、纯 Linux 主机。脚本会下载最新 Release，安装到 `/opt/lemail`，配置放到 `/etc/lemail/config.json`，并创建 `lemail.service`。

```bash
curl -fsSL https://raw.githubusercontent.com/PikachuCN/LeMail/main/scripts/install.sh -o /tmp/lemail-install.sh
sudo bash /tmp/lemail-install.sh
```

带域名一键部署：

```bash
curl -fsSL https://raw.githubusercontent.com/PikachuCN/LeMail/main/scripts/install.sh -o /tmp/lemail-install.sh
sudo LEMAIL_DOMAIN=mail.example.com bash /tmp/lemail-install.sh
```

常用环境变量：

```bash
LEMAIL_VERSION=v1.0              # 默认 latest
LEMAIL_DOMAIN=mail.example.com   # 写入初始收信域名
LEMAIL_HTTP_ADDR=0.0.0.0:3000    # Web 监听地址
LEMAIL_SMTP_ADDR=0.0.0.0:25      # SMTP 监听地址
LEMAIL_OPEN_FIREWALL=1           # 自动尝试放行 firewalld/ufw 端口
```

服务管理：

```bash
sudo systemctl status lemail --no-pager
sudo systemctl restart lemail
sudo journalctl -u lemail -f
sudo nano /etc/lemail/config.json
```

一键脚本会使用 systemd capability 允许 `lemail` 普通用户监听 25 端口，不需要让程序长期以 root 身份运行。

### 方式三：Docker Compose

```bash
git clone https://github.com/PikachuCN/LeMail.git
cd LeMail
cp config/config.example.json config/config.json
docker compose up -d --build
```

默认映射：

- Web：宿主机 `3000` -> 容器 `3000`
- SMTP：宿主机 `25` -> 容器 `2525`
- 配置：宿主机 `./config` -> 容器 `/app/config`

公网收信时，服务器的 25/tcp 必须能被外部邮件服务器访问。

## 宝塔面板部署

宝塔可以负责域名、SSL、Nginx 反向代理；LeMail 负责真正收邮件。两者分工如下：

- 浏览器访问：`https://mail.example.com` -> 宝塔 Nginx -> `127.0.0.1:3000`
- 邮件投递：发信方 SMTP -> 服务器 `25/tcp` -> LeMail

> 注意：SMTP 25 端口不是 HTTP 流量，不能靠网站反向代理转发；必须让公网 25/tcp 直接到 LeMail。

### 1. 在宝塔终端安装 LeMail

打开宝塔“终端”，执行：

```bash
curl -fsSL https://raw.githubusercontent.com/PikachuCN/LeMail/main/scripts/install.sh -o /tmp/lemail-install.sh
sudo LEMAIL_DOMAIN=mail.example.com bash /tmp/lemail-install.sh
```

如果你的宝塔环境没有 `sudo`，直接用 root 执行：

```bash
LEMAIL_DOMAIN=mail.example.com bash /tmp/lemail-install.sh
```

### 2. 在宝塔安全里放行端口

在宝塔面板“安全”里放行：

- `25/tcp`：公网 SMTP 收信必须用。
- `3000/tcp`：如果不做反向代理才需要外部访问；如果只走 Nginx，可只让本机访问。
- `80/tcp`、`443/tcp`：给网页和 SSL 使用。

同时检查云厂商安全组，也要放行 25、80、443。很多云厂商默认屏蔽 25 端口，如果被屏蔽，LeMail 再努力也收不到公网邮件。

### 3. 添加网站并配置反向代理

在宝塔“网站”里添加站点，例如 `mail.example.com`，然后设置反向代理：

- 目标 URL：`http://127.0.0.1:3000`
- 发送域名：`$host`
- 开启 WebSocket 支持（如果面板有这个开关）

如果需要手写 Nginx 片段，可参考：

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

### 4. 配置 SSL

在宝塔站点 SSL 页面申请证书。Web 页面走 HTTPS 后，浏览器会自动使用 WSS 连接实时推送。

### 5. 配置 DNS

假设你要使用 `mail.example.com` 收信：

```dns
mail.example.com.      A      <服务器 IP>
mx.mail.example.com.   A      <服务器 IP>
mail.example.com.      MX 10  mx.mail.example.com.
```

MX 应该指向主机名，不要直接写 IP。比如不要写成 `mail.example.com MX 10 1.2.3.4`。

宝塔相关操作可参考官方文档：

- 宝塔文档入口：https://docs.bt.cn/
- 反向代理：https://docs.bt.cn/user-guide/site/php/site-config/reverse-proxy
- 系统防火墙端口规则：https://docs.bt.cn/user-guide/security/firewall/port-rule

## DNS 配置要点

公网收信最容易踩坑，记住这几条：

1. `mail.domains` 必须包含你要收信的域名。
2. MX 记录必须指向主机名，主机名再用 A/AAAA 指向服务器 IP。
3. 服务器 25/tcp 必须开放，云厂商安全组也要开放。
4. 如果“投递调试”里没有任何 TCP 连接，问题通常在 DNS、端口或发信方投递侧。
5. 如果看到 `RCPT 拒收`，检查域名是否配置、邮箱前缀是否被保留。

## ChatGPT 首封验证码收不到？

先别急着怀疑正则。按这个顺序排查：

1. 看“全部邮件”：如果第一封邮件根本不存在，说明它没成功进入 LeMail。
2. 看“投递调试”：请求验证码的时间点附近有没有 `TCP 连接`、`RCPT 接受`、`邮件写入`。
3. 没有连接：查 MX、25 端口、安全组、云厂商 25 限制。
4. 有拒收：查 `mail.domains` 和保留前缀。
5. 邮件存在但没有验证码：在“全部邮件 -> 邮件详情”里测试验证码项目，必要时用 AI 生成正则建议。

## 配置文件

默认读取 `config/config.json`，也可用环境变量指定：

```bash
CONFIG_PATH=/etc/lemail/config.json ./lemail
```

核心字段：

```json
{
  "server": { "httpAddr": "0.0.0.0:3000" },
  "smtp": { "addr": "0.0.0.0:25" },
  "mail": {
    "domains": ["mail.example.com"],
    "retention": "1h",
    "reservedLocalParts": ["admin", "postmaster", "root"]
  },
  "access": { "mode": "public", "passwordHash": "" },
  "admin": { "username": "admin", "passwordHash": "" },
  "api": { "enabled": false, "tokenHash": "" },
  "openai": {
    "enabled": false,
    "apiKey": "",
    "baseURL": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "timeout": "15s",
    "apiMode": "auto"
  },
  "webhooks": [],
  "codeProjects": []
}
```

密码、API Token 会保存为 bcrypt hash；OpenAI API Key 只在服务端配置中保存，管理接口不会回显。

## 页面导览

- **收件台**：生成邮箱、复制地址、看邮件、复制自动提取出的验证码。
- **邮件详情**：查看 Text、HTML、Raw 和邮件头。
- **管理概览**：看域名、邮件数、验证码数、运行状态。
- **系统配置**：改 HTTP/SMTP、域名、访问模式、API Token、OpenAI 配置。
- **验证码项目**：创建 ChatGPT / OpenAI 等验证码提取规则。
- **WebHook 规则**：把验证码推送到你的机器人、自动化平台或自建接口。
- **投递调试**：看 SMTP 投递事件，定位“邮件到底有没有到”。
- **全部邮件**：管理员查看内存里的所有邮件，并测试验证码提取。

## 验证码自动提取

ChatGPT / OpenAI 邮件可用类似配置：

- 发件人正则：`(?i)openai\.com`
- 提取来源：`HTML`
- 验证码正则：`(?is)<h1[^>]*>\s*(\d{6})\s*</h1>|enter this code:\s*(\d{6})`

如果邮件里有：

```html
<p>enter this code:</p>
<h1>892832</h1>
```

LeMail 会直接把 `892832` 显示到“自动提取验证码”卡片里。新邮件到达时不会强行弹出正文，避免打断你复制验证码。

## 外部 API

在“系统配置 -> 外部 API 调用”启用 API 并设置 Token 后，可用：

```http
Authorization: Bearer <你的 API Token>
```

常用接口：

- `GET /api/v1/config`
- `POST /api/v1/mailboxes/random`
- `POST /api/v1/mailboxes`
- `GET /api/v1/mailboxes/{address}/messages`
- `GET /api/v1/mailboxes/{address}/codes`
- `GET /api/v1/messages/{messageId}`

更多说明见 `docs/API.md`，给自动化代理使用的说明见 `skills/lemail-api/SKILL.md`。

## 开发命令

```bash
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run build
go test ./...
go build ./cmd/lemail
```

Windows 如果 Go 测试遇到临时目录权限问题：

```powershell
New-Item -ItemType Directory -Force .gotmp,.gocache
$env:GOTMPDIR=(Resolve-Path .gotmp).Path
$env:GOCACHE=(Resolve-Path .gocache).Path
go test ./...
```

## Release

推送 `v*` tag 会触发 GitHub Actions：

- 构建 Linux、Windows、macOS 的 amd64 和 arm64 二进制。
- 统一生成 `.zip` 和 `checksums.txt`。
- 压缩包内包含 `config/config.example.json`。
- 发布 GitHub Releases，并构建 GHCR Docker 镜像。

## 许可证

GPL-2.0-only。作者：PikachuCN。
