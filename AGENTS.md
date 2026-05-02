# 项目规范

## 项目定位

- 本项目是 LeMail 的 Go + React/ArcoDesign 全新重构版本。
- 兄弟目录 `forsaken-mail` 只作为参考项目，禁止修改其中任何文件。
- 邮件、会话和实时订阅只保存在内存中，禁止引入数据库。
- 验证码提取结果也只保存在内存中，随邮件 TTL 清理；只有验证码项目定义写入 JSON 配置。
- 运行时配置使用 JSON 文件，默认路径是 `config/config.json`，可用 `CONFIG_PATH` 覆盖。
- OpenAI API Key 属于运行时密钥，只能保存在本地配置或环境变量中，管理接口不得回显明文 Key。
- OpenAI 辅助功能用于生成多条可复用验证码正则建议，需要支持 Responses API 和 Chat Completions API；第三方网关不支持 `/responses` 时必须能降级。
- 外部 API Token 只能保存 hash，管理接口不得回显明文 Token。
- 前端构建产物必须写入 `internal/frontend/dist`，由 Go `embed` 打包进二进制。

## 常用命令

- 安装前端依赖：`npm --prefix web install`
- 前端类型检查：`npm --prefix web run typecheck`
- 前端构建：`npm --prefix web run build`
- 后端测试：`go test ./...`
- 后端构建：`go build ./cmd/lemail`
- 完整本地验证：`npm --prefix web run typecheck && npm --prefix web run build && go test ./... && go build ./cmd/lemail`

Windows 本地如遇 Go 临时目录权限问题，可使用：

```powershell
New-Item -ItemType Directory -Force .gotmp,.gocache
$env:GOTMPDIR=(Resolve-Path .gotmp).Path
$env:GOCACHE=(Resolve-Path .gocache).Path
go test ./...
```

## 架构约束

- Go 代码放在 `cmd/` 和 `internal/` 下，包职责要清晰。
- HTTP API 保持 JSON 输入输出，不要随意改变已有字段名和语义。
- SMTP 收信后必须写入内存 Store，并通过实时 Hub 推送给对应邮箱订阅者。
- SMTP 收信后应先完成验证码项目提取，再推送实时邮件事件，保证前端收到事件后能立即刷新验证码。
- WebSocket 私有模式必须校验访问 Cookie 或已有管理员会话。
- 随机邮箱前缀应像真实邮箱用户名，例如 `lucas4821`、`amy904`，不要使用 `mail-` 这类机械前缀。
- 验证码项目由管理员管理，支持收信域名、邮箱前缀、发件人正则、主题、提取来源和验证码正则。
- WebHook 规则只能由管理员管理，避免公开用户滥用或 SSRF 风险。
- `/api/v1` 外部接口必须使用 Bearer Token 或 `X-LeMail-API-Token` 认证，不要依赖浏览器 Cookie。
- 密码必须 hash 后写入配置，禁止保存明文密码。

## 前端规范

- UI 使用 ArcoDesign 组件体系，不引入第二套 UI 框架。
- 管理后台菜单仅在管理员登录后显示；管理员登录/初始化使用独立页面。
- 私有访问登录页也独立显示，不混入主控制台布局。
- 收件台需要直接展示当前邮箱已提取的验证码，并提供一键复制，不要求用户打开邮件正文查找。
- 全部邮件详情应保留验证码项目测试入口，测试必须调用后端真实提取逻辑，避免前端正则行为与 Go 不一致。
- OpenAI 辅助生成正则只能由管理员触发，必须走后端调用 OpenAI，不允许把 API Key 暴露给前端运行时代码。
- 修改 `web/src` 后必须重新运行 `npm --prefix web run build`，并提交更新后的 `internal/frontend/dist`。
- 页面需要兼容桌面和移动端；移动端使用 Drawer 菜单。

## 测试要求

- 修改随机邮箱逻辑时，补充或更新 `internal/httpapi` 测试。
- 修改实时推送、SMTP 或鉴权逻辑时，必须运行并维护 WebSocket/SMTP 相关测试。
- 修改验证码项目或提取逻辑时，必须覆盖 HTML/文本正则提取、内存结果清理和用户邮箱查询。
- 修改 WebHook 逻辑时，必须覆盖规则校验、验证码提取和 payload 发送。
- 修改配置保存逻辑时，必须确认 JSON 配置仍可加载、保存和校验。

## Git 规范

- 每个逻辑阶段完成后创建 Git 提交，保证可回滚。
- Git 提交说明使用中文。
- 不要 amend 旧提交，除非用户明确要求。
- 不要执行 `git reset --hard` 或 `git checkout --` 这类破坏性命令，除非用户明确要求。
- 发现非本人造成的意外改动时，先停止并询问用户。


