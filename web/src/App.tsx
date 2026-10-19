import { useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import {
  ArrowDownWideNarrow,
  ArrowRight,
  ArrowUpRight,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clock3,
  Database,
  Disc3,
  Gamepad2,
  LayoutDashboard,
  LibraryBig,
  LoaderCircle,
  Monitor,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  Sparkles,
  Trophy,
  WifiOff,
  X,
} from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Button } from "./components/ui/button";
import SettingsDialog from "./SettingsDialog";
import GameDialog from "./GameDialog";
import { demoDashboard, demoLibrary } from "./demo";
import type {
  Dashboard,
  Game,
  Library,
  Platform,
  PlatformStatus,
} from "./types";

const labels: Record<Platform, string> = {
  steam: "Steam",
  psn: "PlayStation",
  epic: "Epic Games",
};
const colors: Record<Platform, string> = {
  steam: "#91b9e5",
  psn: "#b1a1e6",
  epic: "#e1c392",
};
const isDemo = new URLSearchParams(window.location.search).get("demo") === "1";
const num = (n: number) =>
  new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(n);
function hours(value: number | null) {
  return value === null
    ? "—"
    : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(
        value / 3600,
      );
}
function ago(value: number | null) {
  if (value === null) return "暂无记录";
  const days = Math.floor(Math.max(0, Date.now() / 1000 - value) / 86400);
  if (days === 0) return "今天";
  if (days === 1) return "昨天";
  if (days < 30) return `${days} 天前`;
  return new Date(value * 1000).toLocaleDateString("zh-CN", {
    month: "short",
    day: "numeric",
    year:
      new Date(value * 1000).getFullYear() !== new Date().getFullYear()
        ? "numeric"
        : undefined,
  });
}
function exactTime(value: number | null) {
  return value === null
    ? "尚未同步"
    : new Date(value * 1000).toLocaleString("zh-CN");
}
async function request<T>(url: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });
  if (!response.ok)
    throw new Error("读取本地数据失败，请确认 Go 服务正在运行。");
  return response.json() as Promise<T>;
}

function PlatformMark({
  platform,
  small = false,
}: {
  platform: Platform;
  small?: boolean;
}) {
  return (
    <span
      className={`platform-mark ${platform} ${small ? "small" : ""}`}
      aria-hidden="true"
    >
      {platform === "steam" ? (
        <Disc3 size={small ? 12 : 18} />
      ) : platform === "psn" ? (
        <Gamepad2 size={small ? 12 : 18} />
      ) : (
        <span className="epic-type">E</span>
      )}
    </span>
  );
}
function Badge({
  platform,
  edition,
}: {
  platform: Platform;
  edition?: string | null;
}) {
  return (
    <span
      className="platform-badge"
      style={{ "--platform-color": colors[platform] } as CSSProperties}
    >
      <PlatformMark platform={platform} small />
      {edition ?? labels[platform]}
    </span>
  );
}
function Cover({ game, className = "" }: { game: Game; className?: string }) {
  const [failed, setFailed] = useState(false);
  const src = game.cover_path
    ? `/covers/${encodeURIComponent(game.cover_path)}`
    : game.cover_url;
  useEffect(() => setFailed(false), [src]);
  return (
    <div
      className={`cover ${className}`}
      style={{ "--cover-accent": colors[game.platform] } as CSSProperties}
    >
      {src && !failed ? (
        <img
          src={src}
          alt={`${game.name} 封面`}
          loading="lazy"
          onError={() => setFailed(true)}
        />
      ) : (
        <div className="cover-fallback">
          <Gamepad2 size={30} />
          <span>{game.name}</span>
        </div>
      )}
    </div>
  );
}
function Progress({
  game,
  compact = false,
}: {
  game: Game;
  compact?: boolean;
}) {
  const known = game.earned_count !== null && game.total_count !== null;
  const percent =
    known && game.total_count! > 0
      ? Math.min(100, (game.earned_count! / game.total_count!) * 100)
      : 0;
  if (!known)
    return (
      <span className="unknown" title="平台尚未提供，或尚未成功获取">
        —
      </span>
    );
  if (game.total_count === 0)
    return <span className="unknown no-achievement">无成就</span>;
  return (
    <div className={`achievement-progress ${compact ? "compact" : ""}`}>
      <span>
        <Trophy size={12} />
        {game.earned_count}
        <span className="muted">/ {game.total_count}</span>
      </span>
      <div
        className="progress-track"
        role="progressbar"
        aria-label={`${game.name} 成就进度`}
        aria-valuemin={0}
        aria-valuemax={game.total_count!}
        aria-valuenow={game.earned_count!}
      >
        <i style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}
function RecentGameCard({ game, onOpen }: { game: Game; onOpen: () => void }) {
  const noun = game.platform === "psn" ? "奖杯" : "成就";
  const known = game.earned_count !== null && game.total_count !== null;
  const noAchievements = known && game.total_count === 0;
  const percent = known && game.total_count! > 0
    ? Math.min(100, (game.earned_count! / game.total_count!) * 100)
    : null;
  return (
    <article className="recent-card">
      <button
        className="recent-detail-link"
        aria-label={`查看 ${game.name} 成就详情`}
        onClick={onOpen}
      />
      <div className="recent-art"><Cover game={game} /></div>
      <div className="recent-body">
        <div className="recent-heading">
          <h3 title={game.name}>{game.name}</h3>
          <span className="recent-time" title={exactTime(game.last_played_at)}>
            {ago(game.last_played_at)}
          </span>
        </div>
        <div className="recent-platform">
          <PlatformMark platform={game.platform} small />
          <span>{labels[game.platform]}</span>
          {game.edition_platform && game.edition_platform !== labels[game.platform] && (
            <span className="recent-edition">{game.edition_platform}</span>
          )}
        </div>
        <div className="recent-metrics">
          <div className="recent-playtime">
            <div className="recent-value">
              <strong>{hours(game.playtime_seconds)}</strong>
              {game.playtime_seconds !== null && <span>小时</span>}
            </div>
            <span className="recent-metric-label">
              <Clock3 size={12} />{game.playtime_seconds === null ? "游玩时长未知" : "累计游玩"}
            </span>
          </div>
          <div className="recent-achievements">
            <div className={`recent-value ${noAchievements ? "recent-value-empty" : ""}`}>
              {noAchievements ? <strong>无{noun}</strong> : (
                <>
                  <strong>{game.earned_count === null ? "—" : num(game.earned_count)}</strong>
                  <span>/ {game.total_count === null ? "—" : num(game.total_count)}</span>
                </>
              )}
            </div>
            <span className="recent-metric-label">
              <Trophy size={12} />{known ? (noAchievements ? "平台已确认" : `${noun}已解锁`) : `${noun}数据未完整获取`}
            </span>
            {percent !== null && (
              <div className="recent-completion">
                <div
                  className="progress-track"
                  role="progressbar"
                  aria-label={`${game.name} ${noun}进度`}
                  aria-valuemin={0}
                  aria-valuemax={game.total_count!}
                  aria-valuenow={game.earned_count!}
                >
                  <i style={{ width: `${percent}%` }} />
                </div>
                <span>{Math.round(percent)}%</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </article>
  );
}
function platformState(p: PlatformStatus) {
  if (isDemo) return "演示数据";
  if (!p.supported) return "待接入";
  if (!p.configured) return p.count ? "本地缓存" : "未连接";
  if (p.run?.status === "running") return "同步中";
  if (p.run?.status === "failed") return "同步失败";
  if (p.run?.status === "partial") return "部分已更新";
  return p.last_library_sync ? "已同步" : "等待同步";
}

export default function App() {
  const [dashboard, setDashboard] = useState<Dashboard | null>(
    isDemo ? demoDashboard() : null,
  );
  const [library, setLibrary] = useState<Library | null>(
    isDemo ? demoLibrary("", "all", "recent", 1) : null,
  );
  const [q, setQ] = useState("");
  const [search, setSearch] = useState("");
  const [platform, setPlatform] = useState("all");
  const [sort, setSort] = useState("recent");
  const [page, setPage] = useState(1);
  const [refresh, setRefresh] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [libraryError, setLibraryError] = useState(false);
  const [loading, setLoading] = useState(!isDemo);
  const [listLoading, setListLoading] = useState(!isDemo);
  const [help, setHelp] = useState(false);
  const modalRef = useRef<HTMLElement>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [selectedGame, setSelectedGame] = useState<Game | null>(null);

  useEffect(() => {
    const t = setTimeout(() => {
      setSearch(q);
      setPage(1);
    }, 200);
    return () => clearTimeout(t);
  }, [q]);
  useEffect(() => {
    if (isDemo) return;
    const controller = new AbortController();
    setLoading(true);
    request<Dashboard>("/api/dashboard", controller.signal)
      .then((data) => {
        setDashboard(data);
        setError(null);
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "数据读取失败");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [refresh]);
  useEffect(() => {
    if (isDemo) {
      setLibrary(demoLibrary(search, platform, sort, page));
      return;
    }
    const controller = new AbortController();
    setListLoading(true);
    const params = new URLSearchParams({
      q: search,
      platform,
      sort,
      page: String(page),
    });
    request<Library>(`/api/games?${params}`, controller.signal)
      .then((data) => {
        setLibrary(data);
        setLibraryError(false);
      })
      .catch(() => {
        if (!controller.signal.aborted) setLibraryError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setListLoading(false);
      });
    return () => controller.abort();
  }, [search, platform, sort, page, refresh]);
  const syncing =
    dashboard?.platforms.some((p) => p.run?.status === "running") ?? false;
  useEffect(() => {
    if (isDemo) return;
    const t = setInterval(
      () => setRefresh((n) => n + 1),
      syncing ? 5000 : 60000,
    );
    return () => clearInterval(t);
  }, [syncing]);
  useEffect(() => {
    if (!help) return;
    const previousFocus =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setHelp(false);
      if (e.key === "Tab") {
        const controls = modalRef.current?.querySelectorAll<HTMLElement>(
          "button:not(:disabled),a[href],input,select",
        );
        if (!controls?.length) return;
        const first = controls[0],
          last = controls[controls.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
      document.body.style.overflow = previousOverflow;
      previousFocus?.focus();
    };
  }, [help]);

  const metrics = dashboard?.metrics;
  const lastSync =
    Math.max(
      0,
      ...(dashboard?.platforms.map((p) => p.last_library_sync ?? 0) ?? []),
    ) || null;
  const hasGames = (metrics?.games ?? 0) > 0;
  const platformData =
    dashboard?.platforms.map((p) => ({
      name: labels[p.platform],
      count: p.count,
      fill: colors[p.platform],
    })) ?? [];
  const topData =
    dashboard?.top_played.map((g) => ({
      name: g.name,
      hours: Number((g.playtime_seconds! / 3600).toFixed(1)),
    })) ?? [];

  function filterPlatform(value: string) {
    setPlatform(value);
    setPage(1);
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="#overview" aria-label="Game Shelf 首页">
          <span className="brand-symbol">
            <LibraryBig size={22} />
          </span>
          <span>
            game shelf<span className="brand-dot">.</span>
          </span>
        </a>
        <div className="workspace-tag">
          <span className="status-dot" />
          PERSONAL SPACE
        </div>
        <nav aria-label="页面导航">
          <p className="nav-label">我的空间</p>
          <a className="nav-link active" href="#overview">
            <LayoutDashboard size={17} />
            总览<span className="nav-key">01</span>
          </a>
          <a className="nav-link" href="#library">
            <LibraryBig size={17} />
            游戏库
            <span className="nav-count">{metrics ? metrics.games : "—"}</span>
          </a>
          <a className="nav-link" href="#recent">
            <Clock3 size={17} />
            最近游玩
          </a>
        </nav>
        <div className="platform-nav">
          <p className="nav-label">游戏平台</p>
          {(["steam", "psn", "epic"] as Platform[]).map((p) => {
            const item = dashboard?.platforms.find((s) => s.platform === p);
            return (
              <a
                key={p}
                href="#library"
                className="platform-nav-link"
                onClick={() => filterPlatform(p)}
              >
                <PlatformMark platform={p} />
                <span>{labels[p]}</span>
                <span className="nav-count">{item?.count ?? "—"}</span>
              </a>
            );
          })}
        </div>
        <div className="sidebar-bottom">
          <button
            className="help-link"
            onClick={() => setSettingsOpen(true)}
            aria-label="平台设置"
          >
            <Settings2 size={15} />
            平台设置
            <ArrowUpRight size={13} />
          </button>
          <div className="local-card">
            <Database size={17} />
            <div>
              <strong>留在你的本地</strong>
              <p>你的收藏，你的数据。</p>
            </div>
            <span className="status-dot" />
          </div>
          <button className="help-link" onClick={() => setHelp(true)}>
            <CircleHelp size={15} />
            关于数据与同步
            <ArrowUpRight size={13} />
          </button>
          <div className="profile">
            <div className="profile-avatar">
              <Gamepad2 size={20} />
            </div>
            <div>
              <strong>Player One</strong>
              <span>个人游戏档案</span>
            </div>
            <span className="version">v0.1</span>
          </div>
        </div>
      </aside>

      <div className="main-shell">
        <header className="topbar">
          <div className="breadcrumb">
            <span>我的空间</span>
            <ChevronRight size={13} />
            <strong>总览</strong>
          </div>
          <div className="topbar-right">
            <Button
              variant="ghost"
              size="icon"
              aria-label="平台设置"
              onClick={() => setSettingsOpen(true)}
            >
              <Settings2 size={16} />
            </Button>
            <span className="local-pill">
              <ShieldCheck size={13} />
              LOCAL ONLY
            </span>
            <span className="header-date">
              {new Date().toLocaleDateString("zh-CN", {
                month: "long",
                day: "numeric",
                weekday: "long",
              })}
            </span>
          </div>
        </header>
        <main>
          {isDemo && (
            <div className="demo-banner">
              <span>
                <Sparkles size={15} />
                演示模式 · 以下为示例数据，不会写入你的数据库
              </span>
              <a href="/">
                返回我的书架
                <ArrowRight size={14} />
              </a>
            </div>
          )}
          {error && (
            <div className="error-banner" role="alert">
              <WifiOff size={17} />
              <span>
                {error}
                {dashboard ? " 当前保留上次读取的内容。" : ""}
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setRefresh((n) => n + 1)}
              >
                重试
              </Button>
            </div>
          )}
          <section className="page-heading" id="overview">
            <div>
              <p className="eyebrow">
                <span />
                YOUR PERSONAL GAMING JOURNAL
              </p>
              <h1>
                好游戏，都在这里<span>。</span>
              </h1>
              <p className="page-description">
                收藏的世界，投入的时间，还有每一次小小的成就。
              </p>
            </div>
            <div className="heading-actions">
              <span title={exactTime(lastSync)}>
                <span className={`status-dot ${syncing ? "pulse" : ""}`} />
                {isDemo
                  ? "示例档案"
                  : syncing
                    ? "正在同步游戏平台…"
                    : lastSync
                      ? `${ago(lastSync)}已更新`
                      : "等待第一次同步"}
              </span>
              <Button
                variant="outline"
                onClick={() => setRefresh((n) => n + 1)}
                disabled={loading || isDemo}
                title="重新读取本地数据库；平台同步由后台定时任务或本地命令执行"
              >
                <RefreshCw size={14} className={loading ? "spin" : ""} />
                刷新视图
              </Button>
            </div>
          </section>

          <section className="stats-grid" aria-label="游戏统计">
            <article className="stat-card">
              <div className="stat-title">
                已同步游戏
                <span className="stat-icon">
                  <LibraryBig size={16} />
                </span>
              </div>
              <div className="stat-value">
                {metrics ? num(metrics.games) : "—"}
                <span>款</span>
              </div>
              <div className="stat-footer platform-counts">
                {dashboard?.platforms.map((p) => (
                  <span key={p.platform}>
                    <i style={{ background: colors[p.platform] }} />
                    {labels[p.platform]} <strong>{p.count}</strong>
                  </span>
                )) ?? <span>正在读取本地收藏</span>}
              </div>
            </article>
            <article className="stat-card">
              <div className="stat-title">
                累计游玩时间
                <span className="stat-icon">
                  <Clock3 size={16} />
                </span>
              </div>
              <div className="stat-value">
                {metrics && metrics.playtime_known
                  ? hours(metrics.playtime_seconds)
                  : "—"}
                <span>小时</span>
              </div>
              <div className="stat-footer">
                <span className="tiny-track">
                  <i
                    style={{
                      width: `${metrics?.games ? (metrics.playtime_known / metrics.games) * 100 : 0}%`,
                    }}
                  />
                </span>
                <span>
                  时长覆盖{" "}
                  <strong>
                    {metrics?.playtime_known ?? 0} / {metrics?.games ?? 0}
                  </strong>{" "}
                  款
                </span>
              </div>
            </article>
            <article className="stat-card">
              <div className="stat-title">
                已获得成就 / 奖杯
                <span className="stat-icon trophy">
                  <Trophy size={16} />
                </span>
              </div>
              <div className="stat-value">
                {metrics && metrics.achievements_known
                  ? num(metrics.earned)
                  : "—"}
                <span>个</span>
              </div>
              <div className="stat-footer">
                <span>
                  成就覆盖{" "}
                  <strong>
                    {metrics?.achievements_known ?? 0} / {metrics?.games ?? 0}
                  </strong>{" "}
                  款
                </span>
                <span
                  className="stat-earned-detail"
                  title={dashboard?.platforms
                    .map((p) => `${labels[p.platform]} ${p.earned}`)
                    .join(" · ")}
                >
                  查看口径 <CircleHelp size={12} />
                </span>
              </div>
            </article>
          </section>

          <div className="connection-strip">
            {dashboard?.platforms.map((p) => (
              <div key={p.platform} className="connection-item">
                <PlatformMark platform={p.platform} small />
                <span>{labels[p.platform]}</span>
                <span
                  className={`connection-state ${p.run?.status === "failed" ? "failed" : ""}`}
                  title={
                    p.run?.error_summary ??
                    (p.supported
                      ? p.scope === "played"
                        ? "游戏库来源：平台已玩列表"
                        : "游戏库来源：平台拥有列表"
                      : "将在后续版本接入")
                  }
                >
                  {p.run?.status === "running" ? (
                    <LoaderCircle size={10} className="spin" />
                  ) : (
                    <i />
                  )}
                  {platformState(p)}
                </span>
              </div>
            ))}
            <button onClick={() => setHelp(true)}>
              同步说明
              <ArrowUpRight size={12} />
            </button>
          </div>

          {!hasGames && !loading && !error && (
            <section className="setup-card">
              <div className="setup-illustration">
                <LibraryBig size={38} />
                <span>01</span>
              </div>
              <div className="setup-copy">
                <p className="eyebrow">让书架有你的样子</p>
                <h2>从连接 Steam 开始。</h2>
                <p>
                  填入 Steam Key 和 SteamID，保存后自动同步。
                  <br />
                  你的收藏、时长和成就会出现在这里。
                </p>
                <div className="setup-actions">
                  <Button onClick={() => setSettingsOpen(true)}>
                    配置 Steam
                    <ArrowRight size={14} />
                  </Button>
                  <a href="/?demo=1">
                    先逛逛演示书架
                    <ArrowUpRight size={13} />
                  </a>
                </div>
              </div>
              <span className="setup-decoration" aria-hidden="true">
                LET’S
                <br />
                PLAY.
              </span>
            </section>
          )}

          <section id="recent" className="recent-section">
            <div className="section-heading">
              <div>
                <h2>
                  <span className="section-marker" />
                  最近游玩
                </h2>
                <p>上次的冒险，停在了哪里？</p>
              </div>
              <a
                href="#library"
                onClick={() => {
                  filterPlatform("all");
                  setSort("recent");
                }}
              >
                全部游戏
                <ArrowRight size={14} />
              </a>
            </div>
            {dashboard?.recent.length ? (
              <div className="recent-grid">
                {dashboard.recent.map((g) => (
                  <RecentGameCard
                    key={g.id}
                    game={g}
                    onOpen={() => setSelectedGame(g)}
                  />
                ))}
              </div>
            ) : (
              <div className="empty-recent">
                <Clock3 size={25} />
                <span>
                  {loading
                    ? "正在读取游玩记录…"
                    : hasGames
                      ? "还没有可用的最近游玩时间"
                      : "你的下一场冒险，会出现在这里。"}
                </span>
                <p>仅展示平台提供了最近游玩时间的游戏</p>
              </div>
            )}
          </section>

          <section className="charts-grid" aria-label="游戏统计图表">
            <article className="chart-card">
              <div className="chart-heading">
                <div>
                  <h2>收藏版图</h2>
                  <p>各平台已同步游戏数量</p>
                </div>
                <Monitor size={17} />
              </div>
              {hasGames ? (
                <div className="platform-chart">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={platformData}
                      layout="vertical"
                      margin={{ top: 12, left: 0, right: 34, bottom: 0 }}
                      barSize={15}
                    >
                      <CartesianGrid horizontal={false} stroke="#ffffff09" />
                      <XAxis
                        type="number"
                        allowDecimals={false}
                        axisLine={false}
                        tickLine={false}
                        tick={{ fill: "#6f776e", fontSize: 10 }}
                      />
                      <YAxis
                        type="category"
                        dataKey="name"
                        width={86}
                        axisLine={false}
                        tickLine={false}
                        tick={{ fill: "#afb5ac", fontSize: 11 }}
                      />
                      <Tooltip
                        cursor={{ fill: "#ffffff04" }}
                        contentStyle={tooltipStyle}
                        formatter={(value) => [`${value} 款`, "游戏数量"]}
                      />
                      <Bar dataKey="count" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <ChartEmpty label="同步后，看看你的收藏分布" />
              )}
              <div className="chart-note">同款游戏在不同平台分别计数</div>
            </article>
            <article className="chart-card">
              <div className="chart-heading">
                <div>
                  <h2>
                    时间都给了谁<span className="top-label">TOP 5</span>
                  </h2>
                  <p>按已知累计游玩时间排序</p>
                </div>
                <Clock3 size={17} />
              </div>
              {topData.length ? (
                <div className="time-chart">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={topData}
                      layout="vertical"
                      margin={{ top: 4, left: 0, right: 28, bottom: 0 }}
                      barSize={9}
                    >
                      <XAxis type="number" hide />
                      <YAxis
                        type="category"
                        dataKey="name"
                        width={140}
                        axisLine={false}
                        tickLine={false}
                        tick={{ fill: "#b4bbaf", fontSize: 11 }}
                        tickFormatter={(value) =>
                          value.length > 20 ? `${value.slice(0, 19)}…` : value
                        }
                      />
                      <Tooltip
                        cursor={{ fill: "#ffffff04" }}
                        contentStyle={tooltipStyle}
                        formatter={(value) => [`${value} h`, "累计游玩"]}
                      />
                      <Bar
                        dataKey="hours"
                        fill="#c3ed8b"
                        radius={[0, 3, 3, 0]}
                        background={{ fill: "#ffffff04", radius: 3 }}
                      />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <ChartEmpty label="有了时长记录，就有了答案" />
              )}
              <div className="chart-note">缺失时长的游戏不参与排行</div>
            </article>
          </section>

          <section id="library" className="library-section">
            <div className="section-heading">
              <div>
                <h2>
                  <span className="section-marker" />
                  我的游戏库
                  <span className="count-chip">{metrics?.games ?? 0}</span>
                </h2>
                <p>一个安静的地方，放下所有想玩的世界。</p>
              </div>
              <span className="read-only">
                <BookOpen size={13} />
                只读收藏
              </span>
            </div>
            <div className="library-panel">
              <div className="library-toolbar">
                <label className="search-field">
                  <Search size={16} />
                  <input
                    placeholder="搜索游戏名称…"
                    aria-label="搜索游戏名称"
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                  />
                  {q && (
                    <button onClick={() => setQ("")} aria-label="清空搜索">
                      <X size={13} />
                    </button>
                  )}
                </label>
                <div className="library-controls">
                  <div
                    className="filter-tabs"
                    role="group"
                    aria-label="按平台筛选"
                  >
                    {(["all", "steam", "psn", "epic"] as const).map((p) => (
                      <button
                        key={p}
                        onClick={() => filterPlatform(p)}
                        className={platform === p ? "selected" : ""}
                      >
                        {p === "all"
                          ? "全部"
                          : p === "psn"
                            ? "PSN"
                            : p === "epic"
                              ? "Epic"
                              : "Steam"}
                      </button>
                    ))}
                  </div>
                  <label className="sort-select">
                    <ArrowDownWideNarrow size={14} />
                    <select
                      aria-label="游戏排序"
                      value={sort}
                      onChange={(e) => {
                        setSort(e.target.value);
                        setPage(1);
                      }}
                    >
                      <option value="recent">最近游玩</option>
                      <option value="playtime">游玩时间</option>
                      <option value="name">游戏名称</option>
                      <option value="achievements">成就进度</option>
                    </select>
                  </label>
                </div>
              </div>
              {libraryError ? (
                <div className="library-empty" role="alert">
                  <WifiOff size={24} />
                  <h3>游戏库读取失败</h3>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setRefresh((n) => n + 1)}
                  >
                    重新读取
                  </Button>
                </div>
              ) : (
                <div
                  className={`table-scroll ${listLoading ? "is-loading" : ""}`}
                  aria-busy={listLoading}
                >
                  <table>
                    <thead>
                      <tr>
                        <th>游戏名称</th>
                        <th>平台</th>
                        <th>游玩时间</th>
                        <th>成就 / 奖杯</th>
                        <th>最近游玩</th>
                      </tr>
                    </thead>
                    <tbody>
                      {library?.games.map((g) => (
                        <tr key={g.id}>
                          <td className="game-name-cell">
                            <button
                              className="game-cell game-detail-link"
                              aria-label={`查看 ${g.name} 成就详情`}
                              onClick={() => setSelectedGame(g)}
                            >
                              <Cover game={g} />
                              <span>
                                <strong>{g.name}</strong>
                                <small>
                                  {g.source_scope === "played"
                                    ? "已玩游戏"
                                    : "游戏库"}
                                  <span> · </span>
                                  {labels[g.platform]}
                                </small>
                              </span>
                            </button>
                          </td>
                          <td className="platform-cell" data-label="平台">
                            <Badge
                              platform={g.platform}
                              edition={g.edition_platform}
                            />
                          </td>
                          <td className="playtime-cell" data-label="游玩时间">
                            {hours(g.playtime_seconds)}
                            {g.playtime_seconds !== null && <span> h</span>}
                          </td>
                          <td
                            className="achievement-cell"
                            data-label="成就 / 奖杯"
                          >
                            <Progress game={g} />
                          </td>
                          <td
                            className="last-played"
                            data-label="最近游玩"
                            title={exactTime(g.last_played_at)}
                          >
                            {g.last_played_at ? ago(g.last_played_at) : "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {!library?.games.length && (
                    <div className="library-empty">
                      <LibraryBig size={28} />
                      <h3>
                        {listLoading
                          ? "正在读取游戏库"
                          : hasGames
                            ? "没有找到匹配的游戏"
                            : "书架已经准备好了"}
                      </h3>
                      <p>
                        {hasGames
                          ? "试试其他名称或平台。"
                          : "连接 Steam 后，你的游戏会自动出现在这里。"}
                      </p>
                      {(q || platform !== "all") && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setQ("");
                            filterPlatform("all");
                          }}
                        >
                          清除筛选
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              )}
              <div className="table-footer">
                <span>
                  显示{" "}
                  {library?.total
                    ? (library.page - 1) * library.page_size + 1
                    : 0}
                  –
                  {library
                    ? Math.min(library.page * library.page_size, library.total)
                    : 0}
                  ，共 {library?.total ?? 0} 款
                  <span className="null-note"> · — 表示数据未知</span>
                </span>
                <div className="pagination">
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="上一页"
                    disabled={!library || library.page <= 1 || listLoading}
                    onClick={() => setPage((library?.page ?? 1) - 1)}
                  >
                    <ChevronLeft size={15} />
                  </Button>
                  <span>
                    {library?.page ?? 1} / {library?.pages ?? 1}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="下一页"
                    disabled={
                      !library || library.page >= library.pages || listLoading
                    }
                    onClick={() => setPage((library?.page ?? 1) + 1)}
                  >
                    <ChevronRight size={15} />
                  </Button>
                </div>
              </div>
            </div>
          </section>
          <footer className="page-footer">
            <span>
              <span className="brand-dot">●</span> GAME SHELF
              <span className="footer-separator">/</span>为热爱留一个位置。
            </span>
            <span>
              <Database size={12} />
              本地存储 · 只读展示
            </span>
          </footer>
        </main>
      </div>

      {settingsOpen && (
        <SettingsDialog
          demo={isDemo}
          onClose={() => setSettingsOpen(false)}
          onSaved={() => {
            setRefresh((n) => n + 1);
            setTimeout(() => setRefresh((n) => n + 1), 1500);
          }}
        />
      )}
      {selectedGame && (
        <GameDialog
          key={selectedGame.id}
          game={selectedGame}
          cover={<Cover game={selectedGame} />}
          demo={isDemo}
          onClose={() => setSelectedGame(null)}
        />
      )}
      {help && (
        <div className="modal-backdrop" onClick={() => setHelp(false)}>
          <section
            ref={modalRef}
            className="help-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="help-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="modal-heading">
              <div>
                <p className="eyebrow">LOCAL FIRST</p>
                <h2 id="help-title">连接你的游戏档案</h2>
              </div>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setHelp(false)}
                aria-label="关闭说明"
                autoFocus
              >
                <X size={18} />
              </Button>
            </div>
            <p className="settings-description">
              在平台设置中填写 Steam Key / SteamID64，或连接 PlayStation 的
              NPSSO，保存后直接同步，无需编辑文件或重启服务。
            </p>
            <Button
              onClick={() => {
                setHelp(false);
                setSettingsOpen(true);
              }}
            >
              打开平台设置
              <ArrowRight size={14} />
            </Button>
            <div className="data-notes">
              <h3>关于这里的数字</h3>
              <p>
                总数按平台条目计数，同款跨平台分别保留。时长与成就只汇总已获取的数据；未知显示为
                —，已知零显示为 0。成就与奖杯按独立集合计数，不混用 PSN
                等级积分。
              </p>
              <p>
                PSN 使用社区接口，同步已玩记录；Epic
                尚待接入。同步失败保留旧记录；封面可能需要联网补充。演示模式的数据独立于你的本地数据库。
              </p>
            </div>
            <div className="modal-footer">
              <a href={isDemo ? "/" : "/?demo=1"}>
                {isDemo ? "返回我的书架" : "浏览演示书架"}
                <ArrowRight size={14} />
              </a>
              <Button size="sm" onClick={() => setHelp(false)}>
                知道了
              </Button>
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
function ChartEmpty({ label }: { label: string }) {
  return (
    <div className="chart-empty">
      <div>
        <i />
        <i />
        <i />
        <i />
        <i />
      </div>
      <p>{label}</p>
    </div>
  );
}
const tooltipStyle = {
  background: "#222622",
  border: "1px solid #3c4438",
  borderRadius: 8,
  color: "#e6ebdf",
  fontSize: 12,
};
