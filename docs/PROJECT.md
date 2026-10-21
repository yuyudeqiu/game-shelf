# 设计决策

更新：2026-10-21。使用说明见 [README](../README.md)，当前进度见 [TODO](../TODO.md)。
这里只记录长期有效的设计和接口注意事项，历史过程查 Git。

## 架构与边界

Go 同步命令 → SQLite ← Go HTTP API ← React / Vite 前端。

- 本地个人只读书架，每个平台一个账号；游戏数据只读，设置接口允许保存凭据及触发同步。
- Go 标准库 HTTP / 路由、小型 SQL repository；前端使用 TypeScript、Tailwind、
  shadcn 风格组件。Go 提供构建后的静态文件，开发时由 Vite 代理。
- 只监听 loopback；封面尽量缓存本地，详情文本和状态从 SQLite 读取。
- 不引入账号系统、跨平台合并、编辑游戏、推荐 / AI、云部署或复杂框架。

## 数据与失败保留

- 当前 schema v4：`games`、`achievement_sets`、`sync_runs`、`achievement_details`。
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

## 本地设置

- Steam 设置原子写入 `settings.json`（0600）；页面保存值优先于初始环境变量。
  空 Key 沿用已有值，修改后无需重启。不得回传或记录密钥、带 Key 的请求 URL。
- PSN 的 NPSSO 仅交换一次，永不落盘；可续期令牌原子写入 `psn-settings.json`
  （0600），不回传前端。验证账号 ID、校验本地绑定后才写游戏数据。
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

## Epic 接入方向

先探测账号再选原生接口或 [Legendary](https://github.com/legendary-gl/legendary) JSON 导入。
先做列表、名称和封面；检查 Windows / Mac 及不可安装的第三方条目，排除 DLC / 编辑器 / 工具。
时长和成就无可靠来源时保持 NULL。具体待办统一放在 TODO。
