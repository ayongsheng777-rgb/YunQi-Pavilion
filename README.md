# 云栖阁 YunQi Pavilion

一个放在自己家里的**个人导航主页**：打开浏览器就能看到自己收藏的网站，还能管签到、问 AI、让 AI 助手帮你操作书签。

> 本项目基于 [FlatNas](https://github.com/Garry-QD/FlatNas) 二次开发，原项目 LICENSE（AGPL）完整保留在 `LICENSE` 文件中。另融合了 sun-panel 定制版的 AI、MCP 与插件收藏接口能力。

## 能做什么（大白话版）

| 功能 | 一句话说明 |
|---|---|
| 网址导航 | 把常用网站做成图标放在首页，点一下就打开 |
| 网站签到提醒 | 把每天要签到的网站（如论坛、云服务）登记进来；首页小组件和顶栏铃铛会显示"今天签了几个、还剩几个"；从导航点开过才算签到，签到状态每天自动清零 |
| AI 小助手 | 右下角悬浮按钮，点开就能中文聊天，还能让它帮你找书签 |
| Chrome 插件收藏 | 以后做好的浏览器插件点一下，就能把当前网页存进云栖阁的书签（服务端接口已就绪） |
| MCP（给 AI 助手用） | 把书签、签到、AI 问答开放成标准接口，Claude、Cursor 这类 AI 工具连上就能直接操作 |
| 登录二次验证 | 可选开启手机验证码（OTP）；常用设备可勾选"30 天内免验证"，换设备才需要验证码 |

## 一键 Docker 部署

在装有 Docker 的机器上（比如飞牛 NAS）：

```bash
# 1. 把代码拷到机器上，进目录
cd yunqi-pavilion

# 2. 构建并启动（第一次会下载依赖，多等一会儿）
docker compose up -d --build

# 3. 看日志确认起来了
docker logs -f yunqi-pavilion
```

然后浏览器打开：**http://你的机器IP:23000**

- 首次登录账号：`admin`，密码：`admin`（进去后请马上改密码）
- 数据保存在目录下的 `./data` 文件夹，升级、重装都不会丢

常用命令：

```bash
docker compose down        # 停掉
docker compose up -d        # 启动
docker compose up -d --build  # 更新代码后重新构建
```

## 各功能怎么用

### 网站签到提醒
1. 首页点"编辑"，把**签到提醒**小组件加到页面上
2. 小组件里点"管理站点"，填上要签到的网站（名称、网址、图标、备注）
3. 以后每天从导航点开这些网站，小组件和顶栏铃铛就会变成绿色"已签到"；没点开的显示橙色"待签到"
4. 第二天 0 点自动清零，重新开始记

### AI 小助手
1. 先去"设置 → AI 聊天模型设置"，填上 `api_key`（默认已配好 DeepSeek 的地址和模型名，只差你自己的 key）
2. 点右下角悬浮按钮，中文直接问就行；它还能帮你搜本站的书签

### 登录二次验证（OTP）
1. "设置 → 登录二次验证"点"开始绑定"，用手机验证器 App（如 Google Authenticator）扫二维码
2. 输入 App 上的 6 位数字完成绑定
3. 以后登录：输完账号密码，还要再输 6 位验证码；勾选"30天内这台设备免验证"，这台电脑/手机 30 天内就不用再输了
4. "设置 → 可信设备"里可以看到哪些设备免验证，不想要了点"撤销"

### 钥匙（Token）：插件和 MCP 都用它
- 在"设置 → 插件与 AI 助手的钥匙"里查看当前 token，点"重新生成"可换新（换新后插件和 MCP 客户端也要同步更新）
- 服务端文件位置：`./data/yunqi/api_token.txt`（只有本机能看）

## Chrome 浏览器插件对接说明

> 注意：**插件客户端本身本轮还没做**，下面是给以后开发插件预留的服务端接口说明。

插件用 token 鉴权：请求头 `Authorization: Bearer <token>`，或地址后面加 `?token=<token>`。

| 接口 | 说明 |
|---|---|
| `POST /openapi/v1/bookmark/create` | 收藏网址。参数：`title`（标题，必填）、`url`（网址，必填）、`description`（描述）、`category`（分类名，必填；不存在会自动建） |
| `GET /openapi/v1/categories` | 取全部分类列表 |
| `GET /openapi/v1/version` | 取服务端版本，确认连通性 |

收藏成功返回 `{"success":true,"id":"..."}`；缺必填项返回中文错误，如 `{"error":"标题不能为空"}`。

## MCP 接入说明（给 AI 助手用）

- 地址：`POST http://你的机器IP:23000/mcp`
- 鉴权：请求头 `Authorization: Bearer <token>`（token 见上一节"钥匙"）
- 协议：JSON-RPC 2.0（MCP Streamable HTTP），先调 `initialize`，再调 `tools/list` 看工具
- 工具（共 11 个，都有中文说明）：搜书签 `search_bookmarks`、分类列表 `list_categories`、按分类列书签 `list_bookmarks`、取书签详情 `get_bookmark`、新建书签 `create_bookmark`、改书签 `update_bookmark`、签到站点 `checkin_sites`、今日签到状态 `checkin_today_status`、标记签到 `checkin_mark_done`、AI 问答 `ai_ask`、系统状态 `system_status`

在 Claude / Cursor 里配 MCP 时，URL 填上面地址，Header 加上 Authorization 即可。

## 还需要你做的事

1. **在 GitHub 上建一个空仓库**，名字就叫中文 `云栖阁 YunQi Pavilion`（建好告诉 Muse，帮你把代码推上去）
2. **配 AI 的 key**：去 DeepSeek 官网申请一个 API key，填到"设置 → AI 聊天模型设置"里，AI 聊天才能用
3. 首次登录后**把 admin 密码改掉**；重要数据建议定期备份 `./data` 目录

---

## 开发者备注

- Go module 名仍为 `flatnasgo-backend`（沿用 FlatNas，避免改名风险）；新增后端代码在 `backend/yunqi/`（checkin.go / openapi.go / mcp.go / ai.go / otp.go / yunqi.go），路由在 `backend/main.go` 注册
- 云栖阁数据文件：`server/data/yunqi/`（checkin.json、checkin_records.json、devices.json、otp.json、ai.json、api_token.txt）
- 接口总表见后端报告；前端构建用 `npm run build-only`（vite），`npm run build` 里的 type-check 在基线即有 1 处既有报错（`src/stores/save.ts:254`），与本次改动无关
- 后端验证：`cd backend && go build ./... && go vet ./...` 零报错
