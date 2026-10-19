# TODO 与当前交接

更新：2026-10-19。使用说明见 [README](README.md)，架构与数据口径见
[PROJECT](docs/PROJECT.md)，工作约束见 [AGENTS](AGENTS.md)。
本文件只保留未完成事项、当前状态和最近交接；过程历史查 Git。

## 待办

- [ ] Steam：对照真实账号核对库范围、时长与成就缺口。
- [ ] PSN：验证真实 access token 过期后的自动刷新；目前只有模拟测试覆盖该流程。
- [ ] PSN：继续核对时长、已玩列表范围及奖杯数据准确性。
- [ ] PSN：确认两条《原神》的精确奖杯映射；保持不同 title ID 分别计数。
- [ ] PSN：单独验证购买库、订阅口径和 PS3 / Vita 覆盖。
- [ ] Epic：账号探测后选择原生接入或 Legendary JSON 导入；先做列表和封面，
  检查 Windows / Mac 覆盖并过滤 DLC / 工具；不支持的指标保持 null。

## 当前状态

- Steam、PSN 均已连接并完成真实同步；Epic 尚未接入。
- 2026-10-19 核对：Steam 64 条、PSN 8 个可见游戏，共 72 条。
  PSN 原始记录共 10 条，bilibili、Apple Music 已按确认的媒体分类隐藏，记录仍保留。
- PSN 最近一次同步为 partial：两条《原神》没有唯一奖杯映射；无映射请求、
  详情、分组或封面请求失败，登录正常。完整账号审计仍未完成。
- 成就 / 奖杯详情、中文文本、PSN 本体与附加分组及等级筛选已实现。
  Steam 的 Half-Life、RPG Maker XP 曾缺少可用详情，未推断为零。
- SQLite 当前 schema v4；分类迁移前备份在忽略目录
  `data/backups/before-psn-media-filter.db`，其他历史备份同在该目录。

## 最近交付

- 最近游玩改为横向记录卡：小封面、大号累计时长和成就数量、完成度；
  桌面两列、窄屏单列。平台及 PS4 / PS5 标签放大到 12px，并提高对比度。
- 显示 / 收起全部隐藏内容移到筛选和搜索下方、详情列表标题旁；手机上下排列。
  仍作用于当前游戏全部隐藏内容，切换分组保留状态，关闭详情重置。
- 单条隐藏内容的显示按钮占用说明行，消除多余行高；长说明正常换行。
- 本次提交包含以上前端调整及文档精简；前一提交为 `1c67356`（PSN 媒体应用过滤）。
  未推送远程。

## 验证与运行

- 本次：`go test ./...`、`npm --prefix web run build`、`git diff --check` 通过。
  前端仍有约 667KB 的图表包体积提示，不影响构建。
- 浏览器已检查桌面及 390px：卡片排版、平台字号、零值、无横向溢出、点击详情；
  Steam / PSN 隐藏内容开关、分组切换、空搜索结果及条目行高。
- 后端媒体过滤此前已通过 Go race、go vet 及迁移 / 失败保留 / 账号隔离回归。
- 当前本地服务：`./bin/gameshelf serve`，http://127.0.0.1:8080。
  已构建前端资源；纯前端修改无需重启。测试页已关闭，临时视口已恢复。
- 本次截图在忽略目录 `data/qa/`：`recent-platform-readable.png`、
  `recent-record-cards-mobile.png`、`hidden-toggle-position-*.png`、
  `achievement-row-height-*.png`。凭据、数据库与截图不入 Git。
