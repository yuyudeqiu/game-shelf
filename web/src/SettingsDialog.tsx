import { useEffect, useRef, useState } from "react";
import {
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
  X,
} from "lucide-react";
import EpicSettingsForm from "./EpicSettingsForm";
import PSNSettingsForm from "./PSNSettingsForm";
import { Button } from "./components/ui/button";

interface Status {
  steam_id: string;
  has_key: boolean;
  configured: boolean;
  source: string;
  busy: boolean;
}
export default function SettingsDialog({
  onClose,
  onSaved,
  demo,
}: {
  onClose: () => void;
  onSaved: () => void;
  demo: boolean;
}) {
  const [platform, setPlatform] = useState<"steam" | "psn" | "epic">("steam");
  const [status, setStatus] = useState<Status | null>(null);
  const [key, setKey] = useState("");
  const [id, setID] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const dialog = useRef<HTMLElement>(null);
  useEffect(() => {
    const controller = new AbortController();
    if (demo) {
      setLoading(false);
      return;
    }
    fetch("/api/settings/steam", { signal: controller.signal })
      .then(async (r) => {
        if (!r.ok) throw new Error("无法读取平台设置，请确认本地服务已启动");
        return r.json() as Promise<Status>;
      })
      .then((s) => {
        setStatus(s);
        setID(s.steam_id);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [demo]);
  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) onClose();
      if (e.key === "Tab") {
        const controls = dialog.current?.querySelectorAll<HTMLElement>(
          "button:not(:disabled),a[href],input:not(:disabled)",
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
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, [onClose, saving]);
  async function submit(syncOnly = false) {
    setSaving(true);
    setError("");
    setMessage("");
    try {
      const r = await fetch(
        syncOnly ? "/api/sync/steam" : "/api/settings/steam",
        {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Game-Shelf": "1" },
          body: JSON.stringify(syncOnly ? {} : { api_key: key, steam_id: id }),
        },
      );
      if (!r.ok) {
        const data = await r.json();
        throw new Error(data.error ?? "保存失败，请重试");
      }
      if (!syncOnly) {
        setStatus((await r.json()) as Status);
        setKey("");
      }
      setMessage(
        syncOnly
          ? "已安排同步，稍后可在首页查看进度。"
          : "配置已保存，已安排后台同步，无需重启。",
      );
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "无法连接本地服务");
    } finally {
      setSaving(false);
    }
  }
  return (
    <div
      className="modal-backdrop"
      onClick={() => {
        if (!saving) onClose();
      }}
    >
      <section
        ref={dialog}
        className="help-modal settings-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="settings-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-heading">
          <div>
            <p className="eyebrow">PLATFORM SETTINGS</p>
            <h2 id="settings-title">连接你的游戏平台</h2>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="关闭平台设置"
            disabled={saving}
            onClick={onClose}
            autoFocus
          >
            <X size={18} />
          </Button>
        </div>
        <div className="settings-platform-tabs" aria-label="选择配置平台">
          <button
            type="button"
            disabled={saving}
            aria-pressed={platform === "steam"}
            onClick={() => setPlatform("steam")}
          >
            Steam
          </button>
          <button
            type="button"
            disabled={saving}
            aria-pressed={platform === "psn"}
            onClick={() => setPlatform("psn")}
          >
            PlayStation
          </button>
          <button type="button" disabled={saving} aria-pressed={platform === "epic"} onClick={() => setPlatform("epic")}>
            Epic Games
          </button>
        </div>
        {demo ? (
          <div className="data-notes">
            <p>演示模式不保存配置。回到你的书架后，即可连接 Steam、PSN 或 Epic。</p>
            <a className="settings-text-link" href="/">
              返回我的书架 →
            </a>
          </div>
        ) : platform === "epic" ? (
          <EpicSettingsForm onSaved={onSaved} onBusyChange={setSaving} />
        ) : platform === "psn" ? (
          <PSNSettingsForm onSaved={onSaved} onBusyChange={setSaving} />
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <p className="settings-description">
              填入一次，以后打开书架就能自动更新。
            </p>
            <label className="settings-field">
              Steam API Key
              <span className="settings-field-note">
                {status?.has_key
                  ? "已保存 · 留空沿用现有 Key"
                  : "仅保存在这台电脑，保存后不会显示完整 Key"}
              </span>
              <input
                type="password"
                name="steam-api-key"
                autoComplete="new-password"
                spellCheck={false}
                placeholder={
                  status?.has_key
                    ? "已保存，输入新 Key 可替换"
                    : "填写 32 位 API Key"
                }
                value={key}
                onChange={(e) => setKey(e.target.value)}
                disabled={loading || saving}
                maxLength={64}
                required={!status?.has_key}
              />
            </label>
            <a
              className="settings-text-link"
              href="https://steamcommunity.com/dev/apikey"
              target="_blank"
              rel="noreferrer"
            >
              前往 Steam 获取 API Key
              <ExternalLink size={12} />
            </a>
            <label className="settings-field">
              SteamID64
              <span className="settings-field-note">
                你的 17 位数字账号 ID，非昵称
              </span>
              <input
                name="steam-id"
                inputMode="numeric"
                autoComplete="off"
                placeholder="例如 76561198000000000"
                value={id}
                onChange={(e) => setID(e.target.value)}
                disabled={loading || saving}
                pattern="[0-9]{17}"
                maxLength={17}
                required
              />
            </label>
            <p className="settings-hint">
              Steam 个人主页地址为 /profiles/数字 时，这串数字就是
              SteamID64。请确认 Steam 的「游戏详情」允许访问。
            </p>
            {loading && (
              <p className="settings-feedback">
                <LoaderCircle size={14} className="spin" />
                正在读取配置…
              </p>
            )}
            {error && (
              <p className="settings-feedback settings-error" role="alert">
                {error}
              </p>
            )}
            {message && (
              <p className="settings-feedback settings-success" role="status">
                {message}
              </p>
            )}
            <div className="settings-privacy">
              <ShieldCheck size={15} />
              <span>凭证只交给本地服务，用于读取你的游戏数据。</span>
            </div>
            <div className="modal-footer">
              {status?.configured ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={saving || status.busy}
                  onClick={() => void submit(true)}
                >
                  <RefreshCw size={13} />
                  立即同步
                </Button>
              ) : (
                <span />
              )}
              <Button type="submit" disabled={loading || saving}>
                {saving ? <LoaderCircle size={14} className="spin" /> : null}
                保存并同步
              </Button>
            </div>
          </form>
        )}
      </section>
    </div>
  );
}
