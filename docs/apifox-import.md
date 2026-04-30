# Apifox 导入和调试说明

## 1. 启动本地后端

```bash
cd /Users/lilleap/code/bohack_backend_go
make dev
```

默认地址：

- `http://127.0.0.1:8080`
- 同一套路由也支持 `/api` 前缀：`http://127.0.0.1:8080/api`

## 2. 导入 OpenAPI

在 Apifox 中选择导入 OpenAPI/Swagger 文件，选择：

```text
docs/apifox-openapi.yaml
```

建议导入选项：

- 开启“导入 Servers 为环境”，会生成 `http://127.0.0.1:8080` 和 `http://127.0.0.1:8080/api` 两个环境。
- 开启“导入 Security Scheme”，项目级鉴权会自动按 Bearer Token 配置。
- 开启“自动生成接口用例”，并保留文档里已有的请求示例。

## 3. 配置 Bearer Token

先调用：

```http
POST /auth/login
```

请求示例：

```json
{
  "login": "alice@example.com",
  "password": "123456"
}
```

响应里的 token 在：

```text
data.access_token
```

复制到 Apifox 当前环境或项目 Auth 的 Bearer Token 中。最终请求头格式是：

```http
Authorization: Bearer <access_token>
```

## 4. 注册调试顺序

当前代码的 `POST /auth/register` 实际会校验验证码，所以本地注册建议按这个顺序：

1. 调 `POST /auth/send-verification-code`

```json
{
  "email": "alice@example.com",
  "code_type": "register"
}
```

2. 如果是 `MAIL_MODE=smtp`，去收件箱里拿验证码；如果是 `MAIL_MODE=console`，接口不会返回 `debug_code`，本地调试建议临时设 `REQUIRE_REGISTER_VERIFICATION=false`。
3. 调 `POST /auth/register`，把收到的验证码填到 `verification_code`。

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "123456",
  "verification_code": "123456"
}
```

## 5. 常用调试链路

```text
GET  /healthz
POST /auth/send-verification-code
POST /auth/register
POST /auth/login
GET  /auth/me
GET  /events/current
POST /registration
GET  /registration/status
POST /registration/attachments
```

附件上传使用 `multipart/form-data`：

- `file`: 文件
- `kind`: 可选，默认 `attachment`
- `event_slug`: 可选，默认 `bohack-2026`
