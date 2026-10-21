# Epic 接入调查与实施顺序

更新：2026-10-21。已实现游戏库、封面和累计时长；本机真实账号已完成时长同步：11 条非零记录，其中 9 个可见游戏合计 234.8 小时；另 2 条 DLC 不计总量。完整覆盖核对仍待继续。

## 建议

优先参考 Legendary 的启动器接口，用 Go 原生实现；Legendary JSON 仅保留为诊断 / 备选导入方向。
这样能沿用现有平台设置、独立同步、令牌续期、账号绑定和本地封面缓存，运行时不增加 Python 依赖。
原生接口也是非官方启动器接口，需要以真实账号探测结果作为实施依据。

Epic 官方 Ecom 文档描述的是开发者对自己产品及内容的所有权检查；
本次未找到适合第三方个人书架直接读取整个账号游戏库的官方公开 API。
EOS 成就接口也不能直接视为跨所有游戏的个人成就接口。

## 已确认的源码能力

参考 Legendary 固定提交 `42f6bdeadde3a9526dc8eb713763476999ac217b`：

- OAuth 支持临时 authorization code 换令牌、refresh token 续期及 verify 校验身份。
- Library Items 接口使用 `records`、`responseMetadata.nextCursor` 分页。
- Launcher Assets 可按 Windows / Mac 获取，用于补充平台与精确 app ID，不能单独代表完整库。
- Catalog 按 namespace + catalogItemId 获取标题、分类与图片等元数据。
- 存在 Epic Store 成就定义和个人状态 GraphQL 查询；是否可用于本账号、覆盖哪些游戏仍待实测。
- `list --json` 输出游戏数组及嵌套 DLC，不带已验证的账号身份。
  即使附加手填 account ID，也不能替代写入前的在线身份验证。

本机常见位置未发现 Legendary 命令、Legendary 配置、Heroic 配置或 Epic Launcher 安装。
这不代表所有可能路径均不存在；没有读取任何现有令牌。

## 当前实现

已实现本地 Epic 设置与临时代码登录、OAuth verify、账号绑定、令牌续期、
完整库与 Windows / Mac assets 校验、Catalog 中文名称 / 分类 / 图片、封面缓存，
并接入独立后台同步、页面状态和 `npm run sync:epic`。
同步在正式写库前执行账号核验和完整分页检查；没有另设可绕过账号绑定的 JSON 导入入口。

- `epic.go` 只负责协议和数据校验，`epic_settings.go` 负责本地凭据与同步。
- 用户通过设置页的 Epic 官方登录链接获取一次性 `authorizationCode`；本机 POST 换令牌，
  输入支持完整返回 JSON 或单独代码，页面自动提取 authorizationCode，后端也验证并提取；
  返回内容和代码均不落盘。可续期令牌原子写入忽略目录中的 `epic-settings.json`，权限 0600。
- 库分页检查必需字段、游标循环、精确 ID 冲突，Windows / Mac 任一请求失败不写库。
  显式空库也保守报错。账号绑定检查直接查询数据库，包含所有隐藏记录。
- 游戏 ID 使用 namespace、catalogItemId、appName 数组 JSON 的 SHA-256，避免分隔符歧义和
  封面文件名问题。不同精确 ID 继续独立，同组 ID 合并 Windows / Mac 标签。
- Catalog 的 ID 和 namespace 必须匹配请求；元数据失败跳过更新、保留旧行。
  没有旧数据时不编造名称；请求失败或未知分类报告 partial。
- `source_category` 保存 Catalog 分类路径和 mainGameItem ID 的 JSON。
  有 mainGameItem 或精确分类为 addons、addons/dlc、addons/launchable、mods、editors、
  applications/editors、applications/tools、tools 时从所有书架读取隐藏，记录仍保留。
  ue namespace 和 PRIVATE 沙箱不纳入库。
  仅 games / applications/games 识别为已知游戏分类；其他分类继续展示并报告待核对。
  这些规则只有 fixture 覆盖，必须用真实账号核实实际分类值及覆盖。
- 封面取已知 Epic CDN 的明确图片类型，缓存请求不带 Authorization，不跟随重定向。
  已有标题 / 封面 / 分类不会因上游请求失败丢失；条目缺席不删除。
- 累计时长使用 Library GET `playtime/account/{accountId}/all`，响应数组每项为
  accountId、artifactId、totalTime。以秒存储，artifactId 与本次完整库的唯一 appName
  精确对应；不同 namespace 的同名 app ID 出现歧义时不映射、不按游戏名称匹配。
  所有记录必须账号一致、totalTime 为非负 int64、artifactId 不重复，否则保留旧时长。
  没有记录保持 NULL，显式零才是零；接口失败只标记 partial，不阻塞库和封面。
  单位依据 Playnite Epic 插件将 totalTime 直接赋给秒制 Playtime 字段，另有真实账号响应验证。
- 最近游玩和个人成就不请求、保持 NULL；不把成就定义、XP、空响应当个人数量。

模拟测试覆盖认证与临时代码不落盘、令牌权限和续期轮换、API 脱敏、本机 POST 校验、
隐藏记录账号隔离、同步幂等、分页失败 / 游标循环 / ID 冲突保留旧库、
平台合并、第三方条目保留、分类与重新分类、NULL / 零、元数据与封面失败。

## 真实账号验收与后续

用户在本机登录后先核对库数量、Windows / Mac 与第三方条目、被隐藏条目及未知分类，
再验证 access token 真实过期续期和重新登录。公开源码 / fixture 通过不代表真实接口已通过。
当前本机已有 Epic 登录；本次使用本机令牌内存中验证账号并只读探测时长，没有输出凭据或原始认证响应。

成就后续独立探测 GraphQL 的 errors / null、精确成就 ID、分页完整性、隐藏字段、
中文文本和解锁时间；累计时长已接入；最近游玩时间仍需可靠来源。
原生接口无法用于本账号时再评估 Legendary 备选；普通 JSON 只能预览，
正式导入仍需可信账号身份及完整性校验。

## 参考

- [Epic Ecom 官方说明](https://dev.epicgames.com/docs/epic-games-store/services/ecom/ecom-overview)
- [EOS 成就官方说明](https://dev.epicgames.com/docs/epic-online-services/player-and-game-data/achievements-interface/achievements-reference)
- [Legendary 认证、库、Catalog、成就请求](https://github.com/legendary-gl/legendary/blob/42f6bdeadde3a9526dc8eb713763476999ac217b/legendary/api/egs.py)
- [Legendary 列表与第三方条目处理](https://github.com/legendary-gl/legendary/blob/42f6bdeadde3a9526dc8eb713763476999ac217b/legendary/core.py)
- [Legendary JSON 导出](https://github.com/legendary-gl/legendary/blob/42f6bdeadde3a9526dc8eb713763476999ac217b/legendary/cli.py)

协议由 Go 独立实现，仅参考公开请求格式；没有引入、翻译或运行 Legendary 实现代码。
Legendary 的 GPLv3 许可与参考关系见第三方声明。

时长协议参考：[Playnite Epic 客户端](https://github.com/JosefNemec/PlayniteExtensions/blob/886468f65724e6fd104f3a09847a17ba57cfb430/source/Libraries/EpicLibrary/Services/EpicAccountClient.cs)、[时长映射](https://github.com/JosefNemec/PlayniteExtensions/blob/886468f65724e6fd104f3a09847a17ba57cfb430/source/Libraries/EpicLibrary/EpicLibrary.cs)、[秒制 Playtime 字段](https://api.playnite.link/docs/api/Playnite.SDK.Models.Game.html#Playnite_SDK_Models_Game_Playtime)。仅参考公开协议与数据口径，Go 实现独立编写。
