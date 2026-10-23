# 设计决策

更新：2026-10-23。使用说明见 [README](../README.md)，当前进度见 [TODO](../TODO.md)。
这里只记录长期有效的设计和接口注意事项，历史过程查 Git。

## 架构与边界

Go 同步命令 → SQLite ← Go HTTP API ← React / Vite 前端。

- 本地个人只读书架，每个平台一个账号；游戏数据只读，设置接口允许保存凭据及触发同步。
- Go 标准库 HTTP / 路由、小型 SQL repository；前端使用 TypeScript、Tailwind、
  shadcn 风格组件。Go 提供构建后的静态文件，开发时由 Vite 代理。
- 只监听 loopback；封面尽量缓存本地，详情文本和状态从 SQLite 读取。
- 不引入账号系统、跨平台合并、编辑游戏、推荐 / AI、云部署或复杂框架。

## 数据与失败保留

- 当前 schema v5：`games`、`achievement_sets`、`sync_runs`、`achievement_details`。
  平台 ID 为字符串，游戏按 `(platform, platform_game_id)` upsert，不按名称匹配。
- 时间为 UTC Unix 秒，时长为整数秒；NULL 是未知，0 是已知零。
  总量只汇总已知数据并暴露覆盖率；共享成就集只计一次，不把等级积分当奖杯数量。
- 完整拉取并验证分页后才保存库列表；错误、空列表或部分响应不会清空旧记录。
  不因条目本次缺席而删除，因此收藏是累积同步口径。
- 成就详情必须是按 API ID 完整连接定义与个人状态的快照；不完整时保留上次详情。
  数量接口可独立更新摘要；未知解锁时间保持 NULL。
- 详情通常在数据变化或超过 24 小时后刷新。PSN 分组元数据有独立时间戳，
  更新名称不能让个人解锁记录显得更新；分组请求失败保留旧名称。
- 隐藏内容需显式揭示；状态只在当前详情会话保留，并参与搜索，关闭后重置。
  演示数据与真实数据库隔离，不虚构逐条详情或每日活动历史。

## 同步结果展示

- `sync_runs.report_json` 保存结构化结果：`library_saved` 表示本次列表写入已完成，
  `issues` 仅记录非零问题的类型与数量，不包含凭据、原始响应或带认证的 URL。
  运行中 / 历史记录的报告可为 NULL；旧记录保持原摘要，不从文字反推分类。
- 原有 running / success / partial / failed 状态保留。首页根据报告说明影响范围，
  点击展开完成时间与问题，设置入口直接选择对应平台。
- 元数据缺失可能导致新条目未入库，因此不能把列表已写入等同于全部条目已更新。
  封面缺口不表示数值数据同步失败；平台未接入的能力不作为本次异常。
- 列表更新时间优先依据报告的实际保存结果，旧记录沿用 success / partial 判定。
  列表落库后中断继续保留数据、已观察到的问题及未完成标记。

## 本地设置

- Steam 设置原子写入 `settings.json`（0600）；页面保存值优先于初始环境变量。
  空 Key 沿用已有值，修改后无需重启。不得回传或记录密钥、带 Key 的请求 URL。
- PSN 的 NPSSO 仅交换一次，永不落盘；可续期令牌原子写入 `psn-settings.json`
  （0600），不回传前端。验证账号 ID、校验本地绑定后才写游戏数据。
- Epic 临时 authorizationCode 只换一次令牌，不落盘；可续期令牌原子保存至
  `epic-settings.json`（0600）。每次同步在线 verify 身份，绑定检查包含隐藏记录。
  Epic 普通列表 JSON 不作为无账号验证的导入入口。
- JSON POST 校验本机 Host / Origin 与自定义请求头，不开放 CORS；OAuth 不跟随重定向。
  平台同步独立运行，失败互不阻塞。凭据操作流程和参数见 README。

## Steam 注意事项

- `GetOwnedGames` 获取库与时长，包含已玩免费游戏；最近游玩字段可能缺失。
  隐私 / 网络错误不是空库。`GetPlayerAchievements` 与 schema 按 API name 连接。
- schema 可确认无成就；接口失败不可推断为零。中文请求 `l=schinese`。
- 固定封面地址仅作初始猜测；失败时使用商店 appdetails 的 `header_image`，
  保留解析后的真实 URL，避免下次同步重新覆盖成无效地址。

参考：[Web API](https://partner.steamgames.com/doc/webapi_overview)、
[IPlayerService](https://partner.steamgames.com/doc/webapi/IPlayerService)、
[ISteamUserStats](https://partner.steamgames.com/doc/webapi/ISteamUserStats)。

## PSN 注意事项

- 使用基于 [psn-api](https://github.com/achievements-app/psn-api/tree/1e9d9a806fcd884a4ddee8086bfe92594611da9f/src)
  的 Go 原生非官方接口实现；不运行 Node 子进程。参考版本及许可见
  [第三方声明](THIRD_PARTY_NOTICES.md)。
- 已玩列表不等于购买 / 订阅库。按精确 title ID 查奖杯映射，每批五个、失败时逐个回退；
  无映射或多映射保持未知，不用名称猜测，也不创建仅有奖杯的虚拟游戏条目。
- 奖杯集键为 `service:npCommunicationId`；数量合计白金 / 金 / 银 / 铜。
  定义与个人状态按 trophyId 连接，跨全部分组分页获取；分组名称来自 getTitleTrophyGroups。
- 简体中文必须用 `Accept-Language: zh-Hans`；实测 `zh-CN` / `zh-TW` 回退到英语。
  奖杯更新时间不能作为游玩时间；强制刷新文本仍须保留失败保护。
- 保留原始 `source_category`；仅隐藏已确认的 `ps5_web_based_media_app`（bilibili）
  和 `ps5_native_media_app`（Apple Music）。所有游戏视图 / 统计及补全流程统一排除它们，
  原始条目和快照不删除，账号绑定检查仍包含隐藏条目。
- 缺失 / 空分类沿用旧值；初次未知和新分类继续显示，明确重新分类后可恢复展示。
  不按名称、title ID 或有无奖杯判断应用。

参考：[用户接口](https://psn-api.achievements.app/api-docs/users)、
[奖杯接口](https://psn-api.achievements.app/api-docs/user-trophies)、
[手动认证](https://psn-api.achievements.app/authentication/authenticating-manually)。

## Epic 注意事项

- Go 原生实现非官方启动器接口，无 Legendary / Python 运行时依赖；参考接口版本见
  [Epic 接入说明](EPIC.md)。真实账号已完成库同步和时长接口探测，覆盖审计仍待继续。
- Library 全部分页完成后合并 Windows / Mac assets；重复游标、冲突 ID、缺失字段或
  任一平台请求失败时不写库。空库响应保守报错，保留旧数据。
- 游戏 ID 为 namespace、catalogItemId、appName 三元组 JSON 的 SHA-256；
  Windows / Mac 同一组精确 ID 合并平台信息，第三方不可安装条目仍按 Library 保留。
- namespace=ue 和 PRIVATE 沙箱跳过；Catalog 主游戏关系及精确 DLC / mods / 编辑器 /
  工具分类从书架隐藏，原始分类与关系存 source_category JSON，记录仍保留。
  未知分类可见并报告 partial，不按名字过滤；真实账号分类覆盖仍需核对。
- Catalog 元数据失败保留旧行，新条目不虚构标题；分类缺失不覆盖已有分类。
  库条目缺席不删除。封面只使用认可的 Epic CDN 地址并尽量缓存本地。
- 累计时长读取 Library `playtime/account/{accountId}/all`，totalTime 按秒、artifactId
  与唯一 appName 精确对应；逐行验证账号、非负整数与重复 ID，不唯一时保持旧值 / NULL。
  请求失败或缺失记录不置零；明确的 totalTime=0 才是已知零，失败报告 partial。
- 最近游玩和个人成就暂不请求、保持 NULL。后续独立验证成就 GraphQL；
  定义总数与 XP 不代表个人已解锁数。
