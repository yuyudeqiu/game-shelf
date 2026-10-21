# Game Shelf

[![CI](https://github.com/yuyudeqiu/game-shelf/actions/workflows/ci.yml/badge.svg)](https://github.com/yuyudeqiu/game-shelf/actions/workflows/ci.yml)

本地个人游戏书架：同步 Steam、PSN、Epic，查看游戏库，以及平台支持的累计时长、成就 / 奖杯。
Epic 同步游戏库、封面与平台返回的累计时长。Go + SQLite 后端，React + TypeScript + Vite 前端。

## 快速启动

需要 Go 1.25+、Node 22.12+ 和 npm。在项目根目录执行：

```sh
npm run setup
npm start
```

打开 http://127.0.0.1:8080 。未连接账号时显示空书架；点击“演示书架”或访问
[演示模式](http://127.0.0.1:8080/?demo=1) 查看示例，演示数据不写入数据库。
演示封面借用 Steam 图片，部分平台版本仅作布局示意。

## 连接平台

### Steam

1. 打开「平台设置 → Steam」，填写 API Key 和 17 位 SteamID64（不是昵称）。
2. 点击「保存并同步」，配置立即生效，无需重启。
3. Key 再次打开时不会回传；留空沿用，填入新值替换。同步结果看首页平台状态。

设置内有 [API Key 申请入口](https://steamcommunity.com/dev/apikey)；游戏详情需允许接口访问。
也可在 `.env` 配置初始 `STEAM_API_KEY` / `STEAM_ID`，页面保存的配置优先。

### PlayStation / PSN

1. 在浏览器登录 [PlayStation](https://www.playstation.com/)。
2. 用同一浏览器打开 [NPSSO 页面](https://ca.account.sony.com/api/v1/ssocookie)。
3. 复制 `npsso` 的 64 位值，粘贴到「平台设置 → PlayStation」，点击「连接并同步」。

不需要 Sony 密码。NPSSO 仅用于交换令牌，本身不保存；账号 ID 自动验证。
服务支持自动刷新短期令牌，登录失效时在页面重新填写 NPSSO。
各平台凭据都只在本机输入，不要发到聊天或提交到 Git。
同一数据目录每个平台绑定一个账号，换账号需使用新的 `DATA_DIR`。

### Epic Games

1. 打开「平台设置 → Epic Games」，点击「登录 Epic 并获取授权代码」。
2. 在 Epic 官方页面完成登录，复制页面返回的整段 JSON（也可只复制 `authorizationCode` 的值）。
3. 粘贴到书架设置，页面会自动提取授权代码，点击「连接并同步」。代码只能使用一次，失败或过期后重新获取。

不需要在书架输入 Epic 密码。临时代码不保存，可续期令牌只保存在本机的
`epic-settings.json`（0600），不回传前端；失效时重新登录。
使用非官方启动器接口，同步游戏库、名称、平台、封面与累计时长；首次同步可能需要几分钟。
累计时长来自 Epic Library 时长接口；未返回记录的游戏保持未知，只有明确返回零才记零。
最近游玩时间与成就目前保持未知。账号范围与分类覆盖仍待进一步核对。
已有 Epic 数据时不能直接换账号，需使用新的 `DATA_DIR`。

## 浏览与同步

- 最近游玩卡片突出累计时长和成就进度；点击卡片或游戏名称打开详情。
- 详情可搜索、筛选和查看解锁日期；PSN 还支持本体 / 附加分组与奖杯等级。
  在列表标题旁显示 / 收起全部隐藏内容；关闭详情后恢复收起。
- 「刷新视图」只读取本地数据库；「平台设置 → 立即同步」向对应平台取新数据。
- 默认每 6 小时自动同步，三个平台独立运行；失败默认最多每 15 分钟重试一次，
  自定义同步周期更短时采用该周期。页面每 60 秒刷新，同步中每 5 秒刷新。
- Steam / PSN 首次同步先保存列表，再补详情与封面；Epic 先验证完整库和元数据，再保存列表与补封面；详情通常缓存 24 小时，数据变化时提前刷新。
  读取详情不请求平台；图标仍从平台 CDN 加载，离线时可能显示占位图。

| 命令 | 用途 |
| --- | --- |
| `npm run sync` | 同步 Steam |
| `npm run sync:psn` | 同步 PSN |
| `npm run sync:epic` | 同步 Epic（先在页面登录） |
| `npm run sync:psn -- -refresh-details` | 跳过 PSN 详情缓存，重取奖杯文本和分组 |
| `npm run covers` | 仅修补缺失封面，不同步游戏指标 |

## 数据口径与限制

- 同款游戏跨平台、不同 title ID 分别保留，不合并。总数是已同步平台条目数。
- 未知值显示为未知，不当作零。时长只合计已知值并显示覆盖率；共享奖杯集只计一次。
- Steam 同步可访问的游戏库；PSN 使用非官方接口，同步**已玩列表**，不代表完整购买库。
  PS3 / Vita、购买与订阅范围，以及更广泛的账号准确性仍待验证。
- Epic 拉取完整 Library 分页并合并 Windows / Mac assets，保留第三方启动器条目。
  DLC / 工具按精确分类隐藏但保留记录，未知分类继续展示并报告部分同步；
  未找到游戏元数据的条目保留此前快照，新条目等待后续重试，不虚构名称。
- PSN 已确认的媒体应用（如 bilibili、Apple Music）退出展示和统计，原始记录仍保留。
  未知分类不直接排除。具体分类规则见 [设计决策](docs/PROJECT.md)。
- 同步失败或部分返回保留旧数据，退款 / 库移除不会自动删除，所以数量可能与平台当前不同。
  `partial` 表示部分数据未获取；库同步时间与成就详情更新时间可能不同。
- 进度按成就 / 奖杯数量计算；奖杯更新时间不当作游玩时间。不生成无历史数据支撑的每日曲线。

## 本地配置与备份

| 配置 / 文件 | 用途 |
| --- | --- |
| `LISTEN_ADDR=127.0.0.1:8080` | 监听地址，只允许 loopback |
| `DATA_DIR=./data`、`SYNC_INTERVAL=6h` | 数据目录与同步周期，可在 `.env` 设置 |
| `data/gameshelf.db`、`data/covers/` | SQLite（含 WAL 伴随文件）与封面缓存 |
| `data/settings.json`、`data/psn-settings.json`、`data/epic-settings.json` | Steam 配置、PSN / Epic 令牌，权限 0600，不回传密钥 |
| `data/steam-account`、`data/psn-account`、`data/epic-account` | 账号绑定 |

`.env` 不执行 shell，也不展开变量。备份前停止服务和同步命令，再复制整个 `data/`；
不要只复制正在写入的 db 主文件。

## 开发

```sh
npm run serve  # 终端 1：Go API
npm run dev    # 终端 2：Vite，代理到 :8080
npm test       # Go 测试、前端类型检查和生产构建
```

后端改动需重启服务；Vite 前端支持热更新。Go 服务使用构建后的 `web/dist`。

GitHub Actions 在推送到 `main` 和提交 PR 时自动检查 Go 格式、模块校验、`go vet`、
竞态测试、后端构建，以及前端锁定依赖安装、类型检查和生产构建；也可手动运行。
CI 使用模拟平台接口，不需要 Steam / PSN / Epic 凭据，不代表真实账号验证。

| HTTP 接口 | 用途 |
| --- | --- |
| `GET /api/dashboard` | 指标、覆盖率、平台状态、最近游玩与时长排行 |
| `GET /api/games` | `q`、`platform`、`sort`、`page` 查询，每页 20 条 |
| `GET /api/games/{id}`、`GET /covers/{filename}` | 本地详情快照、缓存封面 |
| `GET /api/settings/{steam,psn,epic}` | 配置状态，不含密钥 / 令牌 |
| `POST /api/settings/{steam,psn,epic}` | 保存平台配置并安排同步 |
| `POST /api/sync/{steam,psn,epic}` | 安排平台同步 |

`sort` 支持 `recent` / `name` / `playtime` / `achievements`；POST 仅接受符合本机
Host / Origin 校验并携带 `X-Game-Shelf: 1` 的 JSON 请求。

继续开发前阅读 [工作规则](AGENTS.md)、[设计决策](docs/PROJECT.md) 和
[待办与交接](TODO.md)。第三方许可见 [THIRD_PARTY_NOTICES](docs/THIRD_PARTY_NOTICES.md)。

## 许可

项目代码采用 [MIT License](LICENSE)。第三方许可见
[THIRD_PARTY_NOTICES](docs/THIRD_PARTY_NOTICES.md)；游戏封面、图标等素材归各自权利人所有。
