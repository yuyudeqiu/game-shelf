import { useState } from "react";
import type { ReactNode } from "react";
import { Search, Trophy, X } from "lucide-react";
import type { Achievement, AchievementGroup } from "./types";

const grades = [
  ["platinum", "白金"],
  ["gold", "金杯"],
  ["silver", "银杯"],
  ["bronze", "铜杯"],
] as const;
const groupID = (a: Achievement) => a.group_id || "__ungrouped";
const earned = (items: Achievement[]) => items.filter((a) => a.achieved).length;
export default function PSNTrophyBrowser({
  items,
  groups,
  renderItem,
  isRevealed,
}: {
  items: Achievement[];
  groups: AchievementGroup[];
  renderItem: (item: Achievement) => ReactNode;
  isRevealed: (item: Achievement) => boolean;
}) {
  const [selectedGroup, setSelectedGroup] = useState("all");
  const [grade, setGrade] = useState("all");
  const [state, setState] = useState("all");
  const [query, setQuery] = useState("");
  const metadata = new Map(groups.map((g) => [g.id, g]));
  const ids = [...new Set(items.map(groupID))].sort((a, b) =>
    a === "default" ? -1 : b === "default" ? 1 : a.localeCompare(b),
  );
  const name = (id: string) =>
    id === "default"
      ? "本体"
      : id === "__ungrouped"
        ? "分组未知"
        : metadata.get(id)?.name || `附加分组 ${id}`;
  const scoped =
    selectedGroup === "all"
      ? items
      : items.filter((a) => groupID(a) === selectedGroup);
  const typed = scoped.filter(
    (a) =>
      grade === "all" ||
      (grade === "unknown"
        ? !grades.some(([value]) => value === a.trophy_type)
        : a.trophy_type === grade),
  );
  const unlocked = earned(typed);
  const unknown = scoped.filter(
    (a) => !grades.some(([value]) => value === a.trophy_type),
  );
  const matches = items.filter(
    (a) =>
      (selectedGroup === "all" || groupID(a) === selectedGroup) &&
      (grade === "all" ||
        (grade === "unknown"
          ? !grades.some(([value]) => value === a.trophy_type)
          : a.trophy_type === grade)) &&
      (state === "all" || (state === "earned" ? a.achieved : !a.achieved)) &&
      (a.hidden && !a.achieved && !isRevealed(a)
        ? "隐藏奖杯"
        : `${a.name} ${a.description}`
      )
        .toLocaleLowerCase()
        .includes(query.trim().toLocaleLowerCase()),
  );
  function groupButton(id: string, label: string, list: Achievement[]) {
    const count = earned(list),
      percent = list.length ? Math.round((count / list.length) * 100) : 0;
    return (
      <button
        key={id}
        className={`trophy-group-card ${selectedGroup === id ? "selected" : ""}`}
        aria-pressed={selectedGroup === id}
        onClick={() => setSelectedGroup(id)}
      >
        <span className="trophy-group-name">{label}</span>
        {id === "default" && metadata.get(id)?.name && (
          <small>{metadata.get(id)!.name}</small>
        )}
        <span className="trophy-group-count">
          <strong>
            {count}
            <em> / {list.length}</em>
          </strong>
          <span>{percent}%</span>
        </span>
        <span className="trophy-group-track">
          <i style={{ width: `${percent}%` }} />
        </span>
      </button>
    );
  }
  return (
    <div className="psn-trophy-browser">
      <div className="trophy-group-heading">
        <h3>本体与附加内容</h3>
        <span>进度按已解锁奖杯数量计算</span>
      </div>
      <div className="trophy-group-grid" aria-label="奖杯内容分组">
        {groupButton("all", "全部奖杯", items)}
        {ids.map((id) =>
          groupButton(
            id,
            name(id),
            items.filter((a) => groupID(a) === id),
          ),
        )}
      </div>
      <div className="trophy-grade-grid" aria-label="按奖杯等级筛选">
        {grades.map(([value, label]) => {
          const list = scoped.filter((a) => a.trophy_type === value);
          return (
            <button
              key={value}
              className={`trophy-grade-card ${value} ${grade === value ? "selected" : ""}`}
              aria-pressed={grade === value}
              onClick={() => setGrade(grade === value ? "all" : value)}
            >
              <Trophy size={18} />
              <span>{label}</span>
              <strong>
                {earned(list)}
                <small> / {list.length}</small>
              </strong>
            </button>
          );
        })}
      </div>
      {unknown.length > 0 && (
        <button
          className="trophy-unknown-grade"
          aria-pressed={grade === "unknown"}
          onClick={() => setGrade(grade === "unknown" ? "all" : "unknown")}
        >
          等级未知 {earned(unknown)} / {unknown.length}
        </button>
      )}
      <div className="achievement-toolbar">
        <div className="achievement-tabs" aria-label="奖杯解锁筛选">
          {[
            ["all", "全部", typed.length],
            ["earned", "已解锁", unlocked],
            ["locked", "未解锁", typed.length - unlocked],
          ].map(([value, label, count]) => (
            <button
              key={value}
              className={state === value ? "selected" : ""}
              aria-pressed={state === value}
              onClick={() => setState(String(value))}
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
            placeholder="搜索奖杯"
            aria-label="搜索奖杯"
          />
        </label>
      </div>
      {grade !== "all" && (
        <div className="trophy-filter-note">
          <span>
            等级：{grades.find(([value]) => value === grade)?.[1] ?? "未知"} ·
            找到 {matches.length} 个
          </span>
          <button onClick={() => setGrade("all")}>
            <X size={12} />
            清除等级筛选
          </button>
        </div>
      )}
      {matches.length ? (
        ids
          .filter((id) => matches.some((a) => groupID(a) === id))
          .map((id) => {
            const all = items.filter((a) => groupID(a) === id),
              list = matches.filter((a) => groupID(a) === id);
            return (
              <section
                key={id}
                className="trophy-list-group"
                aria-label={`${name(id)}奖杯`}
              >
                <div className="trophy-list-group-heading">
                  <h3>{name(id)}</h3>
                  <span>
                    {earned(all)} / {all.length} 已解锁
                  </span>
                </div>
                <ul className="achievement-list">{list.map(renderItem)}</ul>
              </section>
            );
          })
      ) : (
        <div className="achievement-empty">
          <Search />
          <p>没有匹配的奖杯，试试其他分组或筛选。</p>
        </div>
      )}
    </div>
  );
}
