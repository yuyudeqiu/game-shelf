# Game Shelf

一个留在本地的个人 Gaming Dashboard。Go + SQLite，React + Vite + Tailwind，
shadcn 风格组件和 Recharts。Steam 首版已实现；PSN / Epic 后续接入。

## 新对话从哪里继续

先读 [AGENTS.md](AGENTS.md)、[项目背景与决策](docs/PROJECT.md) 和 [TODO.md](TODO.md)。
TODO 最下方记录当前进度、验证结果、下一步和尚未确认的事项。

## 快速启动

需要 Go 1.25+、Node 22.12+ 和 npm。在项目根目录执行：

```sh
npm run setup
npm start
```

打开 http://127.0.0.1:8080 。无需凭证即可查看空书架；在页面里选择“演示书架”，
或访问 http://127.0.0.1:8080/?demo=1 查看独立示例数据。演示数据不写入 SQLite，
不会冒充真实同步；演示封面借用 Steam 图片，部分平台版本仅作布局示意。

## 连接 Steam

启动后，点击页面右上角的齿轮或侧栏「平台设置」：

1. 填写 Steam API Key 和 17 位 SteamID64（不是昵称）。
2. 点击「保存并同步」。配置立即生效，无需重启。
3. 再次打开时 Key 不会回传，显示「已保存」；留空可沿用，填写新 Key 可替换。

设置中提供 [Steam API Key 页面](https://steamcommunity.com/dev/apikey) 的入口。
确认 Steam 的游戏详情允许接口访问。不要把 Key 粘贴到聊天或提交到 Git。
保存表示已保存配置，实际连通性由后台同步验证；错误可在首页同步状态查看。
已有账号数据时不能直接换 SteamID，以免混合收藏；新账号请用新的 DATA_DIR。

可选：仍支持 `.env` 中的 `STEAM_API_KEY` / `STEAM_ID` 作为初始配置。
页面保存的配置优先于这两个环境变量。DATA_DIR、端口和同步周期仍可用 `.env` 设置。

服务启动时，如果上次库同步已超过 `SYNC_INTERVAL`（默认 6h），在后台刷新。
运行期间每分钟检查是否到期；浏览器每 60 秒刷新本地视图，同步中每 5 秒刷新。
库同步失败时，后台最多每 15 分钟重试一次（自定义周期更短时采用该周期）。
也可手动执行：

```sh
npm run sync
```

页面上的“刷新视图”只重新读取 SQLite；“平台设置 → 立即同步”可向 Steam 发起同步。
大库首次补成就与图片需要时间；库列表会先保存，最近游玩的游戏优先补全。
单个游戏的成就默认最多每天重查一次，游玩时长变化时会提前更新；缺少逐条详情的游戏也会在下次同步时补齐。

点击最近游玩卡片或游戏列表中的名称，可打开游戏详情：查看逐条成就名称、说明、图标、
已解锁 / 未解锁状态和解锁日期；支持搜索和状态筛选。中文内容以 Steam 实际提供为准。
未解锁的隐藏成就默认收起，可手动展开。详情保存在 SQLite，读取详情不会请求 Steam；
接口不可用时保留上次快照并显示其更新时间。成就图标仍通过 Steam CDN 加载，离线时显示占位图。

图片固定地址失效时，会查询 Steam 商店返回的实际封面 URL（部分游戏含额外的素材目录），
下载后缓存本地并保留解析后的地址。也可以只修补缺失封面，无需重新同步游戏数据：

```sh
npm run covers
```

## 本地文件

- `data/gameshelf.db`：SQLite 数据（含 WAL 伴随文件）。
- `data/covers/`：已缓存的封面；封面可能因下架/素材变化而不可用，有占位图。
- `data/steam-account`：SteamID 绑定，避免切换账号后混合数据。换账号请用新的 `DATA_DIR`。
- `data/settings.json`：页面保存的平台凭证，文件权限 0600；API 不返回 Key。
- `.env`：可选的初始凭证及运行参数，不执行 shell，不展开变量。

默认只允许绑定 loopback IP。`LISTEN_ADDR=127.0.0.1:8080` 可改端口；
`DATA_DIR=./data`、`SYNC_INTERVAL=6h` 可调整。开发期不需要 Docker、Redis 或云服务。
备份时先停止服务和同步命令，再复制 `data/`；不要只复制正在写入的 db 主文件。

## 开发与验证

```sh
# 终端 1：Go API（无需先构建前端）
npm run serve
# 终端 2：Vite，代理 API 到 :8080
npm run dev

# 后端回归测试 + 前端类型检查和生产构建
npm test
```

修改后端后重启 `serve`；Vite 前端支持热更新。

游戏数据只读接口：

- `GET /api/dashboard`：指标、覆盖率、平台状态、最近游玩、时长排行。
- `GET /api/games?q=&platform=all&sort=recent&page=1`：每页 20 条。
  `sort` 支持 `recent` / `name` / `playtime` / `achievements`。
- `GET /api/games/{id}`：游戏信息、本地成就详情快照及其更新时间；尚未获取与确认无成就分别展示。
- `GET /covers/{filename}`：本地封面。

配置与同步接口（仅允许本机页面发起 JSON 请求）：

- `GET /api/settings/steam`：SteamID、是否有 Key、配置来源和同步状态，不含 Key。
- `POST /api/settings/steam`：保存并安排同步；空 Key 表示沿用已保存的 Key。
- `POST /api/sync/steam`：安排一次同步。

## 数据口径与边界

同款跨平台分别计数；总数是已同步平台条目数。未知字段为 null，已知零为 0。
时长仅汇总已知数据，成就/奖杯按关联的独立集合汇总，避免共享奖杯集重复计数。
不把 PSN 等级积分当作奖杯数量。没有每日历史，因此不生成每日游玩曲线。

同步不会因空列表、隐私错误、网络错误而清空旧数据；退款/库移除不会自动删除旧条目，
所以这里是累积同步收藏，未必与平台此刻库数量完全一致。部分失败会保留旧值并显示
partial 状态。上次更新时间指库同步时间，成就可能更旧。

Steam 使用公共 API；PSN / Epic 的非官方接口尚未实现。无法在未配置凭证的情况下
验证真实账号可见性和数据准确性，详细进度见 TODO。

布局截图：[演示首页](docs/screenshots/dashboard-demo.png)、[390px 游戏卡片](docs/screenshots/mobile-library.png)。
