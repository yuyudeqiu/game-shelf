import { useEffect, useState } from "react";
import {
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Button } from "./components/ui/button";
interface PSNStatus {
  account_id: string;
  configured: boolean;
  has_token: boolean;
  busy: boolean;
}
export default function PSNSettingsForm({
  onSaved,
  onBusyChange,
}: {
  onSaved: () => void;
  onBusyChange: (value: boolean) => void;
}) {
  const [status, setStatus] = useState<PSNStatus | null>(null);
  const [npsso, setNpsso] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    const load = () =>
      fetch("/api/settings/psn", { signal: controller.signal })
        .then(async (r) => {
          if (!r.ok) throw new Error("无法读取 PSN 配置");
          return r.json() as Promise<PSNStatus>;
        })
        .then((data) => {
          if (!controller.signal.aborted) setStatus(data);
        })
        .catch((e) => {
          if (!controller.signal.aborted) setError(e.message);
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => {
      controller.abort();
      clearInterval(timer);
    };
  }, []);
  async function submit(syncOnly = false) {
    setSaving(true);
    onBusyChange(true);
    setError("");
    setMessage("");
    try {
      const r = await fetch(syncOnly ? "/api/sync/psn" : "/api/settings/psn", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Game-Shelf": "1" },
        body: JSON.stringify(syncOnly ? {} : { npsso }),
      });
      if (!r.ok) {
        const d = await r.json();
        throw new Error(d.error ?? "PSN 连接失败");
      }
      if (!syncOnly) {
        setStatus((await r.json()) as PSNStatus);
        setNpsso("");
      }
      setMessage(
        syncOnly
          ? "已安排 PSN 同步，稍后可在首页查看进度。"
          : "PSN 已连接，正在安排后台同步。NPSSO 不会保存或回传。",
      );
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "无法连接本地服务");
    } finally {
      setSaving(false);
      onBusyChange(false);
    }
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <p className="settings-description">
        同步已玩游戏、游玩时间、最近游玩与奖杯。通过社区接口连接，登录失效后可在这里重新连接。
      </p>
      {status?.configured && (
        <div className="psn-connected">
          <ShieldCheck size={16} />
          <span>
            已连接 PSN <small>账号 ID：{status.account_id}</small>
          </span>
        </div>
      )}
      <ol className="psn-connect-steps">
        <li>
          在浏览器中{" "}
          <a
            href="https://www.playstation.com/"
            target="_blank"
            rel="noreferrer"
          >
            登录 PlayStation <ExternalLink size={11} />
          </a>
          。
        </li>
        <li>
          用同一浏览器{" "}
          <a
            href="https://ca.account.sony.com/api/v1/ssocookie"
            target="_blank"
            rel="noreferrer"
          >
            打开 Sony NPSSO 页面 <ExternalLink size={11} />
          </a>
          。
        </li>
        <li>
          复制 JSON 中 <code>npsso</code> 对应的 64
          位值，粘贴在下面。若返回错误，先确认 Sony 登录成功。
        </li>
      </ol>
      <label className="settings-field">
        NPSSO
        <span className="settings-field-note">
          {status?.has_token
            ? "已保存登录令牌 · 留空沿用，填写新 NPSSO 可重新连接"
            : "登录凭证，仅交给本地服务与 Sony 完成授权"}
        </span>
        <input
          name="psn-npsso"
          type="password"
          autoComplete="off"
          spellCheck={false}
          value={npsso}
          onChange={(e) => setNpsso(e.target.value)}
          minLength={64}
          maxLength={64}
          required={!status?.has_token}
          disabled={loading || saving}
          placeholder={
            status?.has_token
              ? "留空沿用已保存登录"
              : "填写 64 位 NPSSO，只粘贴值本身"
          }
        />
      </label>
      <p className="settings-hint">
        NPSSO 可用于账号登录，请仅在本机填写。服务只保存可刷新的令牌，权限为
        0600；过期时需要重新获取 NPSSO。首版为已玩列表，不代表全部购买库，PS3 /
        PS Vita 覆盖尚未验证。
      </p>
      {loading && (
        <p className="settings-feedback">
          <LoaderCircle size={14} className="spin" />
          正在读取配置…
        </p>
      )}
      {saving && (
        <p className="settings-feedback">
          <LoaderCircle size={14} className="spin" />
          {npsso ? "正在与 Sony 验证登录，请稍候…" : "正在安排同步…"}
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
        <span>NPSSO 和登录令牌不会回传到页面，也不会发送到聊天。</span>
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
        <Button type="submit" disabled={loading || saving || status?.busy}>
          {status?.has_token
            ? npsso
              ? "重新连接并同步"
              : "保存并同步"
            : "连接并同步"}
        </Button>
      </div>
    </form>
  );
}
