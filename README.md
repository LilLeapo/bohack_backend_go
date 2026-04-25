# BoHack Backend (Go)

zq到此一游
当前 Go 后端已实现：

- 用户注册
- 用户登录
- 发送邮箱验证码
- 忘记密码重置
- 用户修改密码
- 获取当前登录用户
- 更新个人资料
- 当前活动查询
- 活动列表 / 活动详情公开查询
- 用户提交报名
- 用户修改报名
- 用户取消报名
- 用户查询自己的报名状态
- 用户上传 / 查看 / 删除报名附件
- 管理员创建 / 更新活动
- 管理员查看报名列表 / 报名详情
- 管理员查看报名附件
- 管理员审核报名
- 启动时自动确保 PostgreSQL / SQLite 表存在

## 运行

### Go 环境

本机已通过 Homebrew 安装 Go：

```bash
go version
```

Go 安装的命令行工具会放在 `~/go/bin`，该路径已写入 `~/.zshrc`。

### PostgreSQL 开发环境

开发环境会读取：

- `/home/admin/code/auth_db/postgres.env`
- 当前目录下可选的 `.env`

启动：

```bash
cd /Users/lilleap/code/bohack_backend_go
cp .env.example .env
bash ./run-dev.sh
```

默认监听：

- `http://127.0.0.1:8080`

### SQLite 本地调试

不需要启动 PostgreSQL，直接运行：

```bash
cd /Users/lilleap/code/bohack_backend_go
bash ./run-sqlite-dev.sh
```

默认会使用：

- 数据库文件：`./storage/bohack-dev.sqlite`
- 附件目录：`./storage/registration_attachments`
- 邮件模式：`MAIL_MODE=console`

也可以指定数据库路径：

```bash
SQLITE_PATH=./storage/debug.sqlite bash ./run-sqlite-dev.sh
```

服务启动时会自动建表并创建默认活动。

## 主要接口

- `POST /auth/register`
- `POST /auth/login`
- `POST /auth/send-verification-code`
- `POST /auth/forgot-password/send-code`
- `POST /auth/forgot-password/reset`
- `POST /auth/change-password`
- `GET /auth/me`
- `GET /user/profile`
- `PATCH /user/profile`
- `GET /events/current`
- `GET /events`
- `GET /events/{slug}`
- `POST /registration`
- `PUT /registration`
- `PATCH /registration`
- `DELETE /registration`
- `GET /registration/status`
- `GET /registration/attachments`
- `POST /registration/attachments`
- `DELETE /registration/attachments/{attachmentID}`
- `GET /registration/attachments/{attachmentID}/download`
- `POST /registrations`
- `PUT /registrations`
- `PATCH /registrations`
- `GET /registrations/me`
- `DELETE /registrations/me`
- `GET /registrations/me/attachments`
- `POST /registrations/me/attachments`
- `DELETE /registrations/me/attachments/{attachmentID}`
- `GET /registrations/me/attachments/{attachmentID}/download`
- `GET /admin/events`
- `GET /admin/events/{eventID}`
- `POST /admin/events`
- `PATCH /admin/events/{eventID}`
- `GET /admin/registrations`
- `GET /admin/registrations/{registrationID}`
- `GET /admin/registrations/{registrationID}/attachments`
- `PATCH /admin/registrations/{registrationID}/review`
- `GET /healthz`

本地开发时也支持同一套 `/api/*` 前缀别名，例如：

- `/api/auth/login`
- `/api/registration`
- `/api/admin/registrations`
- `/api/healthz`

## 兼容说明

- 备份库导入的用户密码哈希是 `bcrypt`，当前 Go 登录逻辑可直接兼容。
- 注册接口兼容额外传入 `verification_code` / `verificationCode` 字段，目前会忽略，不阻塞旧前端迁移。
- 当 `REQUIRE_REGISTER_VERIFICATION=true` 时，注册接口会校验 `verification_code` / `verificationCode`。
- 验证码发送接口兼容：
  - `code_type` / `codeType`
  - `purpose`
  - `type`
- 忘记密码重置接口兼容：
  - `verification_code` / `verificationCode` / `code`
  - `new_password` / `newPassword`
- 报名接口同时接受以下字段别名：
  - `event_slug` / `eventSlug`
  - `real_name` / `realName`
  - `team_name` / `teamName`
  - `role_preference` / `rolePreference`
- 个人资料更新接口同时接受 `avatar_url` / `avatarUrl`。
- 活动管理接口同时接受：
  - `is_current` / `isCurrent`
  - `registration_open_at` / `registrationOpenAt`
  - `registration_close_at` / `registrationCloseAt`
- 附件上传使用 `multipart/form-data`，字段：
  - `file`
  - `kind`
  - 可选 `event_slug` / `eventSlug`

## 管理员接口约束

- `/admin/*` 需要先登录。
- 当前使用 `users.is_admin = true` 作为管理员判定。
- 活动支持 `is_current`，公开 `/events/current` 会优先返回数据库中标记为当前且已发布的活动。
- 附件文件默认存放在本地目录，由鉴权下载接口返回，不直接公开目录。
- 邮件支持两种模式：
  - `MAIL_MODE=console`
  - `MAIL_MODE=smtp`
- `console` 模式下会把验证码写到服务日志，并在接口响应里返回 `debug_code`，仅适合开发环境。
- 审核状态支持：
  - `submitted`
  - `under_review`
  - `approved`
  - `rejected`
  - `cancelled`
- 活动状态支持：
  - `draft`
  - `published`
  - `archived`

## 环境变量

示例见 `.env.example`。

- `ATTACHMENT_DIR`:
  默认 `./storage/registration_attachments`
- `MAX_UPLOAD_MB`:
  默认 `20`
- `MAIL_MODE`:
  默认 `console`
- `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM`:
  当 `MAIL_MODE=smtp` 时使用
- `VERIFICATION_CODE_EXPIRE_MINUTES`:
  默认 `10`
- `VERIFICATION_CODE_MIN_INTERVAL_SECONDS`:
  默认 `60`
- `REQUIRE_REGISTER_VERIFICATION`:
  默认 `false`
