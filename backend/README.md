# 兑步 backend (Go)

ADHD 分步记事本服务端。产品行为以 [PRD](https://github.com/vincent-codeworld/duibu-adhd-prd/blob/main/docs/PRD.md) 为准。

## 服务端范围

- Sign in with Apple、账号、邀请码、好友、拉黑
- **已同步**任务 / 阶段 / 奖励 / 验收申请 / 截图元数据
- 应用内通知、举报（`POST /v1/reports` 薄实现：提交回执「已收到」）
- AI 拆解 `POST /v1/ai/decompose`（仅 `title` + `notes`；服务端约 8s 预算，超时/非法降级为模板；无 `AI_API_KEY` 时用本地确定性草稿）

**不做：** 仅设备任务、本地提醒（客户端）、远程推送（v1.1）、支付/商城。

## 本地运行（Postgres 默认）

```bash
cp .env.example .env
docker compose up -d          # Postgres 16，账号 duibu/duibu，库 duibu，端口 5432
# STORE_DRIVER=postgres（默认）时，API 启动会自动跑 migrations/*.sql
GOROOT=$HOME/sdk/go1.23.4 $HOME/sdk/go1.23.4/bin/go run ./cmd/api
# 或：go build -o /tmp/duibu-api ./cmd/api && /tmp/duibu-api
```

环境变量要点：

| 变量 | 默认 | 说明 |
|------|------|------|
| `STORE_DRIVER` | `postgres` | `postgres` 持久化；`memory` 仅进程内（单测/无 Docker 联调） |
| `DATABASE_URL` | `postgres://duibu:duibu@localhost:5432/duibu?sslmode=disable` | `STORE_DRIVER=postgres` 时必通 |
| `AUTH_MODE` | `stub` | stub 登录可写进 Postgres，重启仍在 |
| `JWT_SECRET` | `change-me-in-production` | 联调固定值（与 `.env.example` / 代码默认一致）。**换 secret 后旧 token 会 401，需重登**；数据仍在 |
| `UPLOAD_DIR` | `./data/screenshots` | 验收截图本地落盘目录（MVP，非 S3） |

进程**不会**自动读 `.env`：要么 `export` / `set -a; source .env; set +a`，要么直接依赖上述代码默认。漏配 `.env` 时联调 secret 现已与 example 对齐，避免「有时带 .env、有时不带」导致换签。

健康检查：`GET /healthz` → `{ status, authMode, storeDriver }`

持久化冒烟：

```bash
TOKEN=$(curl -s localhost:8080/v1/auth/dev -H 'Content-Type: application/json'   -d '{"subject":"persist-alice","nickname":"Alice"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["accessToken"])')
curl -s localhost:8080/v1/tasks -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json'   -d '{"title":"持久化任务","notes":"","uploadConsent":true,"dueAt":"2026-12-01T12:00:00Z","stages":[{"title":"一"},{"title":"二"},{"title":"三"}]}'
# 重启 API 后再 GET /v1/tasks，任务应还在；内存店则不会。
```

OpenAPI：`api/openapi.yaml`（当前 **v0.8.1**）

本地构建请用 Go 1.23+（本机可用 `/Users/vincent/sdk/go1.23.4/bin/go`；1.21 在当前 macOS 上会产生无法运行的二进制）。

## Auth stub（联调，默认）

默认 `AUTH_MODE=stub`：

```bash
curl -s localhost:8080/v1/auth/dev -H 'Content-Type: application/json' \
  -d '{"subject":"dev-alice","nickname":"Alice"}'
# 再用返回的 accessToken：
curl -s localhost:8080/v1/me -H "Authorization: Bearer <token>"
```

`POST /v1/auth/apple` 在 stub 模式下把 `identityToken` 当作稳定 subject。登录**不会**上传仅设备任务。

## 真实 Apple 登录（`AUTH_MODE=apple`）

1. Apple Developer：App ID 勾选 Sign in with Apple；MVP Bundle ID 定为 **`com.stepbuddy.app`**（与 App 一致）。
2. （可选）若以后要换 `authorizationCode` / 刷新令牌：创建 Key（Sign in with Apple），下载 `.p8`，记下 Key ID、Team ID。
3. `.env`：

```bash
AUTH_MODE=apple
APPLE_CLIENT_ID=com.stepbuddy.app           # 必填：须等于 identityToken 的 aud（MVP Bundle）
APPLE_CLIENT_IDS=                           # 可选：额外 Services ID，逗号分隔
APPLE_TEAM_ID=                              # 可选：client_secret 用
APPLE_KEY_ID=
APPLE_PRIVATE_KEY_PATH=/secure/AuthKey_XXX.p8
# 或 APPLE_PRIVATE_KEY_PEM="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
JWT_SECRET=change-me-in-production  # 生产务必换成长随机串；联调保持与默认一致
```

4. 重启 API。App 调用：

```http
POST /v1/auth/apple
Content-Type: application/json

{"identityToken":"<ASAuthorizationAppleIDCredential.identityToken UTF-8>","nickname":"可选"}
```

成功：`200` + `{ accessToken, expiresAt, account, created }`（与 stub 相同信封）。  
失败：`401 invalid_apple_token`；未配 `APPLE_CLIENT_ID`：`503 apple_auth_not_configured`。

校验逻辑：拉 `https://appleid.apple.com/auth/keys`，验 RS256，`iss=https://appleid.apple.com`，`aud` ∈ 配置的 Client ID，按 `sub` upsert 账号。

**本机若还没有 .p8 / Team / Key：** 代码路径与单测（mock JWKS）已就绪；上线前由 Vincent 填好 env 即可。当前登录只验 `identityToken`，`.p8` 非阻塞。

**真机冒烟：** 模拟器/真机 Sign in with Apple → App 把 identityToken POST 上来 → `GET /v1/me` 应返回同一账号；二次登录 `created=false`。

`STORE_DRIVER=memory` 重启会清空；默认 Postgres 下账号/任务等会保留。

## 模块顺序

auth → friends → task sync → verification → AI → notices / reports


## invite_code_failures 持久化（v0.8.1）

- 表 `invite_code_failures`（migration 0001 已有）；错误邀请码加好友会记一条失败。
- 近 1 小时失败次数 ≥ `INVITE_CODE_MAX_FAILS_PER_HOUR`（默认 10）→ `429 too_many_failed_invite_attempts`（契约不变）。
- `STORE_DRIVER=postgres`：失败计数跨 API 重启保留；`memory`：仍仅进程内。
- 成功加好友**不会**清历史（与原先一致）。

## dueAt / remindAt + 验收门闩（v0.8.0）

- 创建任务必填 `dueAt`（RFC3339）；阶段可选 `remindAt`（若填写须 ≤ `dueAt`，否则 400）。
- 有 `designatedVerifierAccountId`：`POST /v1/tasks/{id}/complete` → **403** `verification required`；阶段就绪后对 **task_completion** reward 提交验收，**通过后**任务 `status=completed`（无 completionReward 时服务端自动建 `"Task completion"` 门闩奖）。
- 无验收人：自完成路径不变。截图真传契约（v0.7.0）不变。

## 截图真传（v0.7.0）

1. `POST /v1/uploads/screenshots` multipart field `file` → `{ screenshotKey, contentType, byteSize }`（≤4MiB；jpeg/png/webp，按魔数校验）
2. `POST /v1/rewards/{rewardID}/verifications` 带上 `screenshotKey`（须本人未占用的上传）
3. 验收人/申请人：`GET /v1/screenshots/{screenshotKey}` 拉字节；inbox 里仍有 `screenshotKey`，并附 `screenshotUrl`（相对路径）

```bash
# 上传 + 提交 + 拉取（stub auth）
ALICE=$(curl -s localhost:8080/v1/auth/dev -H 'Content-Type: application/json' \
  -d '{"subject":"shot-alice","nickname":"Alice"}')
AT=$(python3 -c 'import sys,json;print(json.load(sys.stdin)["accessToken"])' <<<"$ALICE")
KEY=$(curl -s -X POST localhost:8080/v1/uploads/screenshots -H "Authorization: Bearer $AT" \
  -F "file=@/tmp/tiny.png;type=image/png" | python3 -c 'import sys,json;print(json.load(sys.stdin)["screenshotKey"])')
curl -s -OJ -H "Authorization: Bearer $AT" "localhost:8080/v1/screenshots/$KEY"
```
