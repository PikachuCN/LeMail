---
name: lemail-api
description: 使用 LeMail 外部 API 创建临时邮箱，并读取邮件或已提取验证码，适合自动化脚本和代理调用。
---

# LeMail API Skill

当自动化任务需要从正在运行的 LeMail 服务中获取临时邮箱、读取邮件或读取验证码时，使用这个 Skill。

## 必要输入

- `LEMAIL_BASE_URL`：LeMail HTTP 地址，例如 `http://localhost:3000`。
- `LEMAIL_API_TOKEN`：在 LeMail 管理后台配置的外部 API Token。

所有请求都需要携带：

```http
Authorization: Bearer <LEMAIL_API_TOKEN>
```

## 调用流程

1. 读取可用域名：

```bash
curl -H "Authorization: Bearer $LEMAIL_API_TOKEN" \
  "$LEMAIL_BASE_URL/api/v1/config"
```

2. 创建随机邮箱：

```bash
curl -X POST -H "Authorization: Bearer $LEMAIL_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"domain":"localhost"}' \
  "$LEMAIL_BASE_URL/api/v1/mailboxes/random"
```

3. 在目标注册或登录流程中使用返回的 `address`。

4. 轮询已提取验证码：

```bash
curl -H "Authorization: Bearer $LEMAIL_API_TOKEN" \
  "$LEMAIL_BASE_URL/api/v1/mailboxes/<address>/codes"
```

5. 如果没有验证码，再轮询邮件正文：

```bash
curl -H "Authorization: Bearer $LEMAIL_API_TOKEN" \
  "$LEMAIL_BASE_URL/api/v1/mailboxes/<address>/messages"
```

## 响应处理

- `codes[0].code` 通常是最新验证码。
- `messages[0]` 通常是最新邮件。
- 邮件和验证码只保存在内存中，会在 LeMail 的 `mail.retention` 后过期。
- 如果 API 返回 `401`，请刷新或重新配置 LeMail API Token。
- 如果 API 返回 `403`，请在管理后台启用外部 API。

## Python 示例

```python
import os
import time
import requests

base_url = os.environ["LEMAIL_BASE_URL"].rstrip("/")
token = os.environ["LEMAIL_API_TOKEN"]
headers = {"Authorization": f"Bearer {token}"}

mailbox = requests.post(
    f"{base_url}/api/v1/mailboxes/random",
    json={"domain": "localhost"},
    headers=headers,
    timeout=10,
).json()["address"]

for _ in range(30):
    codes = requests.get(
        f"{base_url}/api/v1/mailboxes/{mailbox}/codes",
        headers=headers,
        timeout=10,
    ).json().get("codes", [])
    if codes:
        print(codes[0]["code"])
        break
    time.sleep(2)
```

完整接口说明见 `docs/API.md`。
