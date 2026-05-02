# LeMail 外部 API

本文档描述给注册脚本、自动化工具和其他软件调用的 `/api/v1` 接口。浏览器 UI 使用的 `/api/mailbox` 和 `/api/admin` 接口保持不变。

## 认证

先在管理后台“系统配置 -> 外部 API 调用”中启用 API，并设置 API Token。服务端只保存 Token 的 bcrypt hash。

API Token 拥有读取当前内存邮件和验证码的权限，请按服务器级密钥保管，不要放入前端页面或公开仓库。

所有 `/api/v1` 请求都需要携带以下任一请求头：

```http
Authorization: Bearer <API_TOKEN>
```

或：

```http
X-LeMail-API-Token: <API_TOKEN>
```

错误响应统一为：

```json
{ "error": "invalid api token" }
```

## 接口列表

### 读取配置

```http
GET /api/v1/config
```

响应：

```json
{
  "version": "v1",
  "domains": ["localhost"],
  "retention": "1h",
  "accessMode": "public"
}
```

### 生成随机邮箱

```http
POST /api/v1/mailboxes/random
Content-Type: application/json

{ "domain": "localhost" }
```

`domain` 可省略，省略时使用配置中的第一个域名。

响应：

```json
{
  "localPart": "lucas4821",
  "domain": "localhost",
  "address": "lucas4821@localhost"
}
```

### 校验自定义邮箱

```http
POST /api/v1/mailboxes
Content-Type: application/json

{ "localPart": "dev100", "domain": "localhost" }
```

也可以直接传入完整地址：

```json
{ "address": "dev100@localhost" }
```

响应：

```json
{
  "localPart": "dev100",
  "domain": "localhost",
  "address": "dev100@localhost"
}
```

### 读取邮箱邮件

```http
GET /api/v1/mailboxes/{address}/messages
```

示例：

```bash
curl -H "Authorization: Bearer $LEMAIL_API_TOKEN" \
  http://localhost:3000/api/v1/mailboxes/dev100@localhost/messages
```

响应：

```json
{
  "messages": [
    {
      "id": "msg-id",
      "from": "service@example.com",
      "to": ["dev100@localhost"],
      "subject": "Login code",
      "text": "Your code is 123456",
      "html": "<p>Your code is <b>123456</b></p>",
      "headers": { "Subject": ["Login code"] },
      "raw": "raw email source",
      "receivedAt": "2026-05-02T10:00:00Z"
    }
  ]
}
```

### 读取邮箱验证码

```http
GET /api/v1/mailboxes/{address}/codes
```

响应：

```json
{
  "codes": [
    {
      "id": "code-id",
      "projectId": "cp_login",
      "projectName": "登录验证码",
      "mailbox": "dev100@localhost",
      "code": "123456",
      "subject": "Login code",
      "from": "service@example.com",
      "receivedAt": "2026-05-02T10:00:00Z",
      "messageId": "msg-id"
    }
  ]
}
```

验证码按接收时间倒序返回，通常取数组第一项即可。

### 读取单封邮件详情

```http
GET /api/v1/messages/{messageId}
```

响应：

```json
{
  "message": {
    "id": "msg-id",
    "from": "service@example.com",
    "to": ["dev100@localhost"],
    "subject": "Login code",
    "text": "Your code is 123456",
    "html": "",
    "headers": {},
    "raw": "raw email source",
    "receivedAt": "2026-05-02T10:00:00Z"
  }
}
```

## 自动化建议

1. 调用 `POST /api/v1/mailboxes/random` 获取邮箱地址。
2. 把邮箱地址传给目标注册流程。
3. 每 2-5 秒轮询 `GET /api/v1/mailboxes/{address}/codes`。
4. 如果验证码项目未命中，再读取 `messages` 或 `messages/{id}` 自行解析正文。
5. 邮件和验证码只保存在内存中，超过配置的 `mail.retention` 后会自动清理。
