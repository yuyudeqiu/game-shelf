# TODO 与当前交接

更新：2026-10-21。使用说明见 [README](README.md)，架构与数据口径见
[PROJECT](docs/PROJECT.md)，工作约束见 [AGENTS](AGENTS.md)。
本文件只保留未完成事项、当前状态和最近交接；过程历史查 Git。

## 待办

- [ ] Steam：对照真实账号核对库范围、时长与成就缺口。
- [ ] PSN：验证真实 access token 过期后的自动刷新；目前只有模拟测试覆盖该流程。
- [ ] PSN：继续核对时长、已玩列表范围及奖杯数据准确性。
- [ ] PSN：确认两条《原神》的精确奖杯映射；保持不同 title ID 分别计数。
- [ ] PSN：单独验证购买库、订阅口径和 PS3 / Vita 覆盖。
- [ ] Epic：用户在本机登录后，核对真实游戏库数量、Windows / Mac 与第三方条目、
  DLC / 编辑器 / 工具分类以及未知分类；真实库已同步，完整审计仍待继续。
- [ ] Epic：验证真实 access token 过期续期、轮换令牌及失效重新登录。
- [ ] Epic：对照官方启动器核对累计时长与覆盖缺口；接口已真实返回 11 条非零记录。
- [ ] Epic：单独探测个人成就与最近游玩时间；当前保持 NULL。

## 当前状态

- Steam、PSN、Epic 均已连接并完成真实库同步；Epic 累计时长已接入并完成首次真实同步。
- 2026-10-21 核对：Steam 64 条、PSN 8、Epic 272 个可见游戏，共 344 条。
  Epic 原始记录 293 条；时长接口返回 11 条，9 个可见游戏合计 234.8 小时，
  另 2 条是 DLC 原始记录，不重复计入书架总量。
  PSN 原始记录共 10 条，bilibili、Apple Music 已按确认的媒体分类隐藏，记录仍保留。
- PSN 最近一次同步为 partial：两条《原神》没有唯一奖杯映射；无映射请求、
  详情、分组或封面请求失败，登录正常。完整账号审计仍未完成。
- 成就 / 奖杯详情、中文文本、PSN 本体与附加分组及等级筛选已实现。
  Steam 的 Half-Life、RPG Maker XP 曾缺少可用详情，未推断为零。
- SQLite 当前 schema v4；分类迁移前备份在忽略目录
  `data/backups/before-psn-media-filter.db`，其他历史备份同在该目录。

## 当前交接

- Epic 已接入平台设置：打开官方登录入口，将返回的整段 JSON 或单独的
  authorizationCode 粘贴到本机页面，自动提取后点击连接。服务保存可续期令牌，代码不落盘；账号隔离包含隐藏条目。
- 已实现完整 Library 分页与 Windows / Mac assets 校验、精确 ID 合并、Catalog
  名称 / 分类 / 封面、独立后台同步、`npm run sync:epic`、状态展示与错误脱敏。
  分类过滤保留原始记录，未知分类可见并报告 partial；元数据失败保留旧值。
- Epic 累计时长已接入 Library playtime 接口，按 artifactId 与唯一 appName 对应。
  账号不符、重复 / 非法记录、请求失败时保留旧值并报告 partial；缺失记录保持未知，
  显式零才是零。最近游玩 / 个人成就仍未知；实现依据见 `docs/EPIC.md`。
- 项目远程为 https://github.com/yuyudeqiu/game-shelf ，origin / main；CI 已配置。
  本次 Epic 接入、JSON 自动提取和累计时长作为一个本地签名提交保存；尚未推送。
- 提交身份与签名统一继承用户全局 Git 配置，已移除仓库 user.name / user.email 覆盖，
  避免提交邮箱与 GPG 密钥身份不匹配；只重做本次未推送提交，不改已发布历史。

## 验证与运行

- `go test ./...`、`go test -race -count=1 ./...`、`go vet ./...`、前端构建通过。
  本次新增时长测试覆盖秒数、显式零 / NULL、账号不符、缺字段、负值、溢出、
  重复 artifact、跨 namespace 映射歧义、请求失败与空响应保留。
- 本机真实账号已 verify，时长只读接口返回 11 条非零记录，账号全部匹配；
  使用本机保存的令牌，未输出凭据或原始认证响应。首次真实同步已完成：
  时长请求成功、无歧义映射、0 元数据失败 / 未知分类；28 张封面不可用导致 partial。
  HTTP API 确认 Epic 9 个可见游戏时长共 234.8 小时，未返回时长的游戏保持未知。
- SQLite 保持 schema v4；同步前 SQLite backup 备份在
  `data/backups/before-epic-playtime.db`（忽略目录），历史备份保留。
- 服务已重建并运行于 http://127.0.0.1:8080。游戏库和时长通过本地 HTTP API 验证，
  官方启动器逐款数值对照仍待用户核对。
- 授权表单已支持完整 JSON 自动提取；隔离虚构样本截图在
  `data/qa/epic-json-extraction.png`，此前设置页截图为 `data/qa/epic-settings.png`。
- `git diff --check` 通过。提交前已重跑 Go 测试、vet 与前端构建；本地签名提交，尚未推送。
