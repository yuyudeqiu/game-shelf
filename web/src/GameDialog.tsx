import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  Check,
  ExternalLink,
  LockKeyhole,
  LoaderCircle,
  Search,
  Trophy,
  X,
} from "lucide-react";
import { Button } from "./components/ui/button";
import type { Achievement, Game, GameDetail } from "./types";

const date = (t: number | null) =>
  t ? new Date(t * 1000).toLocaleString("zh-CN") : "时间未知";
function AchievementRow({ item }: { item: Achievement }) {
  const [revealed, setRevealed] = useState(false);
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
          {hidden ? "隐藏成就" : item.name}{" "}
          {item.achieved && <Check size={14} aria-label="已解锁" />}
        </h3>
        <p>
          {hidden
            ? "解锁后揭晓，或手动查看详情。"
            : item.description ||
              (item.hidden
                ? "Steam 未提供此隐藏成就的说明。"
                : "Steam 未提供成就说明。")}
        </p>
        {hidden && (
          <button
            className="reveal-achievement"
            onClick={() => setRevealed(true)}
          >
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
      const r = await fetch("/api/sync/steam", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Game-Shelf": "1" },
        body: "{}",
      });
      if (!r.ok) {
        const d = await r.json();
        throw new Error(d.error || "无法开始同步");
      }
      setSyncMessage("已安排 Steam 同步，详情会在获取后自动显示。");
    } catch (e) {
      setSyncMessage(e instanceof Error ? e.message : "无法连接本地服务");
    } finally {
      setSyncing(false);
    }
  }
  const items = detail?.achievements ?? [];
  const earned = items.filter((a) => a.achieved).length;
  const known = detail?.details_updated_at != null;
  const filtered = items.filter(
    (a) =>
      (filter === "all" || (filter === "earned" ? a.achieved : !a.achieved)) &&
      (a.hidden && !a.achieved ? "隐藏成就" : `${a.name} ${a.description}`)
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
                <Trophy size={18} /> 成就档案
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
                aria-label="成就完成度"
                aria-valuenow={earned}
                aria-valuemin={0}
                aria-valuemax={items.length}
              >
                <i style={{ width: `${(earned / items.length) * 100}%` }} />
              </div>
              <div className="achievement-toolbar">
                <div className="achievement-tabs" aria-label="成就筛选">
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
                    placeholder="搜索成就"
                    aria-label="搜索成就"
                  />
                </label>
              </div>
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
              <p>正在读取成就档案…</p>
            </div>
          ) : known ? (
            items.length ? (
              filtered.length ? (
                <ul className="achievement-list">
                  {filtered.map((item) => (
                    <AchievementRow key={item.api_name} item={item} />
                  ))}
                </ul>
              ) : (
                <div className="achievement-empty">
                  <Search />
                  <p>没有匹配的成就，试试其他筛选。</p>
                </div>
              )
            ) : (
              <div className="achievement-empty">
                <Trophy />
                <h3>这款游戏没有 Steam 成就</h3>
                <p>已通过平台数据确认。</p>
              </div>
            )
          ) : (
            <div className="achievement-empty">
              <Trophy />
              <h3>
                {demo
                  ? "演示书架暂无逐条成就数据"
                  : game.platform !== "steam"
                    ? "这个平台的成就详情尚未接入"
                    : "尚未获取成就详情"}
              </h3>
              <p>
                {demo
                  ? "回到你的书架，查看真实游戏的成就档案。"
                  : "已有统计会保留；隐私设置、无成就接口或网络问题可能导致详情暂不可用。"}
              </p>
              {!demo && game.platform === "steam" && (
                <Button
                  variant="outline"
                  disabled={syncing}
                  onClick={() => void sync()}
                >
                  同步 Steam 成就
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
