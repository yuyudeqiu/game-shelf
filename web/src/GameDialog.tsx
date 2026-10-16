import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  Check,
  ExternalLink,
  Eye,
  EyeOff,
  LockKeyhole,
  LoaderCircle,
  Search,
  Trophy,
  X,
} from "lucide-react";
import PSNTrophyBrowser from "./PSNTrophyBrowser";
import { Button } from "./components/ui/button";
import type { Achievement, Game, GameDetail } from "./types";

const date = (t: number | null) =>
  t ? new Date(t * 1000).toLocaleString("zh-CN") : "时间未知";
function AchievementRow({
  item,
  noun,
  revealed,
  onReveal,
}: {
  item: Achievement;
  noun: string;
  revealed: boolean;
  onReveal: () => void;
}) {
  const [failed, setFailed] = useState(false);
  const hidden = item.hidden && !item.achieved && !revealed;
  const icon = item.achieved ? item.icon : item.icon_gray;
  return (
    <li className={`achievement-item ${item.achieved ? "unlocked" : "locked"}`}>
      <div className="achievement-art">
        {icon && !failed && !hidden ? (
          <img
            src={icon}
            alt=""
            loading="lazy"
            onError={() => setFailed(true)}
          />
        ) : hidden ? (
          <LockKeyhole size={22} />
        ) : (
          <Trophy size={22} />
        )}
      </div>
      <div className="achievement-copy">
        <h3>
          {hidden ? `隐藏${noun}` : item.name}{" "}
          {!hidden && item.trophy_type && (
            <span className={`trophy-grade ${item.trophy_type}`}>
              {(
                {
                  bronze: "铜杯",
                  silver: "银杯",
                  gold: "金杯",
                  platinum: "白金",
                } as Record<string, string>
              )[item.trophy_type] ?? item.trophy_type}
            </span>
          )}
          {item.achieved && <Check size={14} aria-label="已解锁" />}
        </h3>
        <p>
          {hidden
            ? "解锁后揭晓，或手动查看详情。"
            : item.description ||
              (item.hidden
                ? "平台未提供此隐藏内容的说明。"
                : "平台未提供说明。")}
        </p>
        {hidden && (
          <button className="reveal-achievement" onClick={onReveal}>
            显示隐藏内容
          </button>
        )}
      </div>
      <div className="achievement-state">
        <span>{item.achieved ? "已解锁" : "未解锁"}</span>
        {item.achieved && <time>{date(item.unlocked_at)}</time>}
      </div>
    </li>
  );
}
export default function GameDialog({
  game,
  cover,
  demo,
  onClose,
}: {
  game: Game;
  cover: ReactNode;
  demo: boolean;
  onClose: () => void;
}) {
  const isPSN = game.platform === "psn";
  const noun = isPSN ? "奖杯" : "成就";
  const [showAllHidden, setShowAllHidden] = useState(false);
  const [revealedIDs, setRevealedIDs] = useState<Set<string>>(() => new Set());
  const [detail, setDetail] = useState<GameDetail | null>(null);
  const [loading, setLoading] = useState(!demo);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [syncMessage, setSyncMessage] = useState("");
  const [syncing, setSyncing] = useState(false);
  const dialog = useRef<HTMLElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialog.current?.focus();
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") close.current();
      if (e.key !== "Tab") return;
      const controls = dialog.current?.querySelectorAll<HTMLElement>(
        "button:not(:disabled),a[href],input",
      );
      if (!controls?.length) return;
      const first = controls[0],
        last = controls[controls.length - 1];
      if (
        e.shiftKey &&
        (document.activeElement === first ||
          document.activeElement === dialog.current)
      ) {
        e.preventDefault();
        last.focus();
      } else if (
        !e.shiftKey &&
        (document.activeElement === last ||
          document.activeElement === dialog.current)
      ) {
        e.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      document.body.style.overflow = overflow;
      window.removeEventListener("keydown", handler);
      previous?.focus();
    };
  }, []);
  useEffect(() => {
    if (demo) return;
    const controller = new AbortController();
    const load = async () => {
      try {
        const r = await fetch(`/api/games/${game.id}`, {
          signal: controller.signal,
        });
        if (!r.ok) throw new Error("游戏详情读取失败，请稍后重试。");
        const data = (await r.json()) as GameDetail;
        if (!controller.signal.aborted) {
          setDetail(data);
          setError("");
        }
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "读取失败");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => {
      controller.abort();
      clearInterval(timer);
    };
  }, [game.id, demo]);
  async function sync() {
    setSyncing(true);
    setSyncMessage("");
    try {
      const r = await fetch(`/api/sync/${game.platform}`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Game-Shelf": "1" },
        body: "{}",
      });
      if (!r.ok) {
        const d = await r.json();
        throw new Error(d.error || "无法开始同步");
      }
      setSyncMessage("已安排平台同步，详情会在获取后自动显示。");
    } catch (e) {
      setSyncMessage(e instanceof Error ? e.message : "无法连接本地服务");
    } finally {
      setSyncing(false);
    }
  }
  const items = detail?.achievements ?? [];
  const hiddenItems = items.filter((a) => a.hidden && !a.achieved);
  const isRevealed = (a: Achievement) =>
    showAllHidden || revealedIDs.has(a.api_name);
  const allHiddenVisible =
    showAllHidden || (hiddenItems.length > 0 && hiddenItems.every(isRevealed));
  const reveal = (id: string) =>
    setRevealedIDs((current) => new Set([...current, id]));
  function toggleAllHidden() {
    if (allHiddenVisible) {
      setShowAllHidden(false);
      setRevealedIDs(new Set());
    } else setShowAllHidden(true);
  }
  const earned = items.filter((a) => a.achieved).length;
  const known = detail?.details_updated_at != null;
  const filtered = items.filter(
    (a) =>
      (filter === "all" || (filter === "earned" ? a.achieved : !a.achieved)) &&
      (a.hidden && !a.achieved && !isRevealed(a)
        ? `隐藏${noun}`
        : `${a.name} ${a.description}`
      )
        .toLocaleLowerCase()
        .includes(query.trim().toLocaleLowerCase()),
  );
  const current = detail?.game ?? game;
  return (
    <div className="modal-backdrop game-backdrop" onClick={onClose}>
      <section
        className="game-dialog"
        ref={dialog}
        role="dialog"
        tabIndex={-1}
        aria-modal="true"
        aria-labelledby="game-detail-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="game-detail-top">
          <span className="eyebrow">
            YOUR GAME ·{" "}
            {game.platform === "steam" ? "STEAM" : game.platform.toUpperCase()}
          </span>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭游戏详情"
          >
            <X size={20} />
          </Button>
        </div>
        <header className="game-detail-hero">
          {cover}
          <div className="game-detail-intro">
            <h2 id="game-detail-title">{current.name}</h2>
            <p>
              {current.playtime_seconds === null
                ? "游玩时间未知"
                : `${new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(current.playtime_seconds / 3600)} 小时游玩`}
              <span>·</span>
              {current.last_played_at
                ? `最近游玩 ${new Date(current.last_played_at * 1000).toLocaleDateString("zh-CN")}`
                : "暂无游玩日期"}
            </p>
            {game.platform === "steam" && (
              <a
                href={`https://steamcommunity.com/stats/${game.platform_game_id}/achievements`}
                target="_blank"
                rel="noreferrer"
              >
                Steam 成就页面 <ExternalLink size={12} />
              </a>
            )}
          </div>
        </header>
        <div className="game-detail-content">
          <div className="achievement-heading">
            <div>
              <h2>
                <Trophy size={18} /> {noun}档案
              </h2>
              <p>
                {known
                  ? `更新于 ${date(detail!.details_updated_at)}`
                  : "每一次小小的完成，都值得留下。"}
              </p>
            </div>
            {known && (
              <strong>
                {earned}
                <span> / {items.length} 已解锁</span>
              </strong>
            )}
          </div>
          {known && items.length > 0 && (
            <>
              <div
                className="detail-progress"
                role="progressbar"
                aria-label={`${noun}完成度`}
                aria-valuenow={earned}
                aria-valuemin={0}
                aria-valuemax={items.length}
              >
                <i style={{ width: `${(earned / items.length) * 100}%` }} />
              </div>
              {hiddenItems.length > 0 && (
                <div className="hidden-content-actions">
                  <Button
                    variant="outline"
                    size="sm"
                    aria-pressed={allHiddenVisible}
                    onClick={toggleAllHidden}
                  >
                    {allHiddenVisible ? (
                      <EyeOff size={14} />
                    ) : (
                      <Eye size={14} />
                    )}
                    {allHiddenVisible
                      ? "收起全部隐藏内容"
                      : `显示全部隐藏内容（${hiddenItems.length}）`}
                  </Button>
                </div>
              )}
              {!isPSN && (
                <div className="achievement-toolbar">
                  <div className="achievement-tabs" aria-label={`${noun}筛选`}>
                    {[
                      ["all", "全部", items.length],
                      ["earned", "已解锁", earned],
                      ["locked", "未解锁", items.length - earned],
                    ].map(([value, label, count]) => (
                      <button
                        key={value}
                        aria-pressed={filter === value}
                        className={filter === value ? "selected" : ""}
                        onClick={() => setFilter(String(value))}
                      >
                        {label} <span>{count}</span>
                      </button>
                    ))}
                  </div>
                  <label className="achievement-search">
                    <Search size={14} />
                    <input
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                      placeholder={`搜索${noun}`}
                      aria-label={`搜索${noun}`}
                    />
                  </label>
                </div>
              )}
            </>
          )}
          {error && (
            <p className="settings-error" role="alert">
              {error} 每 5 秒自动重试。
            </p>
          )}
          {loading ? (
            <div className="achievement-empty">
              <LoaderCircle className="spin" />
              <p>正在读取{noun}档案…</p>
            </div>
          ) : known ? (
            items.length ? (
              isPSN ? (
                <PSNTrophyBrowser
                  items={items}
                  groups={detail?.achievement_groups ?? []}
                  isRevealed={isRevealed}
                  renderItem={(item) => (
                    <AchievementRow
                      key={item.api_name}
                      item={item}
                      noun={noun}
                      revealed={isRevealed(item)}
                      onReveal={() => reveal(item.api_name)}
                    />
                  )}
                />
              ) : filtered.length ? (
                <ul className="achievement-list">
                  {filtered.map((item) => (
                    <AchievementRow
                      key={item.api_name}
                      item={item}
                      noun={noun}
                      revealed={isRevealed(item)}
                      onReveal={() => reveal(item.api_name)}
                    />
                  ))}
                </ul>
              ) : (
                <div className="achievement-empty">
                  <Search />
                  <p>没有匹配的{noun}，试试其他筛选。</p>
                </div>
              )
            ) : (
              <div className="achievement-empty">
                <Trophy />
                <h3>这款游戏没有{noun}</h3>
                <p>已通过平台数据确认。</p>
              </div>
            )
          ) : (
            <div className="achievement-empty">
              <Trophy />
              <h3>
                {demo
                  ? "演示书架暂无逐条成就数据"
                  : game.platform !== "steam" && game.platform !== "psn"
                    ? "这个平台的成就详情尚未接入"
                    : "尚未获取详情"}
              </h3>
              <p>
                {demo
                  ? "回到你的书架，查看真实游戏的成就档案。"
                  : "已有统计会保留；隐私设置、无成就接口或网络问题可能导致详情暂不可用。"}
              </p>
              {!demo &&
                (game.platform === "steam" || game.platform === "psn") && (
                  <Button
                    variant="outline"
                    disabled={syncing}
                    onClick={() => void sync()}
                  >
                    同步{isPSN ? " PSN 奖杯" : " Steam 成就"}
                  </Button>
                )}
            </div>
          )}
          {syncMessage && (
            <p className="settings-feedback" role="status">
              {syncMessage}
            </p>
          )}
        </div>
      </section>
    </div>
  );
}
