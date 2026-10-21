import { useEffect, useState } from "react";
import {
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Button } from "./components/ui/button";
interface EpicStatus {
  account_id: string;
  configured: boolean;
  has_token: boolean;
  busy: boolean;
  login_url: string;
}
export default function EpicSettingsForm({
  onSaved,
  onBusyChange,
}: {
  onSaved: () => void;
  onBusyChange: (value: boolean) => void;
}) {
  const [status, setStatus] = useState<EpicStatus | null>(null);
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    const load = () =>
      fetch("/api/settings/epic", { signal: controller.signal })
        .then(async (r) => {
          if (!r.ok) throw new Error("无法读取 Epic 配置");
          return r.json() as Promise<EpicStatus>;
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
    if (!syncOnly) setCode("");
    onBusyChange(true);
    setError("");
    setMessage("");
    try {
      const r = await fetch(syncOnly ? "/api/sync/epic" : "/api/settings/epic", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Game-Shelf": "1" },
        body: JSON.stringify(syncOnly ? {} : { authorization_code: code }),
      });
      if (!r.ok) {
        const d = await r.json();
        throw new Error(d.error ?? "Epic 连接失败");
      }
      if (!syncOnly) {
        setStatus((await r.json()) as EpicStatus);
        setCode("");
      }
      setMessage(
        syncOnly
          ? "已安排 Epic 同步，稍后可在首页查看进度。"
          : "Epic 已连接，正在安排后台同步。临时代码不会保存或回传。",
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
        同步 Epic 游戏库、封面与平台记录的累计时长。最近游玩时间与成就暂未接入，平台未返回时长的游戏保持未知。
      </p>
      {status?.configured && (
        <div className="psn-connected">
          <ShieldCheck size={16} />
          <span>
            已连接 Epic <small>账号 ID：{status.account_id}</small>
          </span>
        </div>
      )}
      <ol className="psn-connect-steps">
        <li>在浏览器中
          {status?.login_url ? (
            <a href={status.login_url} target="_blank" rel="noreferrer">
              登录 Epic 并获取授权代码 <ExternalLink size={11} />
            </a>
          ) : "打开 Epic 登录入口（正在读取配置）"}。
        </li>
        <li>完成登录后，复制页面返回的整段 JSON 并粘贴到下面，书架会自动提取 <code>authorizationCode</code>。也可以只粘贴代码。</li>
        <li>代码只能使用一次，过期或连接失败后重新打开登录入口获取。</li>
      </ol>
      <label className="settings-field">
        授权内容
        <span className="settings-field-note">
          {status?.has_token
            ? "已保存登录令牌 · 日常更新点击「立即同步」，重新登录时填写新代码"
            : "登录凭证，仅交给本地服务与 Epic 完成授权"}
        </span>
        <input
          name="epic-authorization-code"
          type="password"
          autoComplete="off"
          spellCheck={false}
          value={code}
          onChange={(e) => {
            const input = e.target.value;
            setError("");
            setMessage("");
            if (input.trimStart().startsWith("{")) {
              try {
                const response: unknown = JSON.parse(input);
                if (response && typeof response === "object" &&
                    "authorizationCode" in response &&
                    typeof response.authorizationCode === "string" &&
                    /^[A-Za-z0-9_-]{16,1024}$/.test(response.authorizationCode.trim())) {
                  setCode(response.authorizationCode.trim());
                  setMessage("已从 JSON 提取授权代码，点击「连接并同步」即可。");
                  return;
                }
              } catch { /* Keep input for validation by the local service. */ }
            }
            setCode(input);
          }}
          minLength={16}
          maxLength={4096}
          required
          disabled={loading || saving || status?.busy}
          placeholder={
            status?.has_token
              ? "粘贴新的完整 JSON 或授权代码"
              : "粘贴整段 JSON，自动提取授权代码"
          }
        />
      </label>
      <p className="settings-hint">
        代码仅用于本次授权，服务在本机保存可续期令牌；失效后需要重新登录。
        每个平台绑定一个账号，更换已有数据的账号需使用新的数据目录。
        首次同步会先核对完整库与 Windows / Mac 条目，再补封面，可能需要几分钟。
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
          正在连接 Epic 或安排同步，请稍候…
        </p>
      )}
      {status?.busy && !saving && (
        <p className="settings-feedback" role="status"><LoaderCircle size={14} className="spin" />Epic 正在同步，可关闭设置并在首页查看结果。</p>
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
        <span>临时代码与令牌仅交给本机服务和 Epic。请不要发到聊天。</span>
      </div>
      <div className="modal-footer">
        {status?.configured ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={loading || saving || status.busy}
            onClick={() => void submit(true)}
          >
            <RefreshCw size={13} />
            立即同步
          </Button>
        ) : (
          <span />
        )}
        <Button type="submit" disabled={loading || saving || status?.busy || !code.trim()}>
          {status?.has_token ? "重新连接并同步" : "连接并同步"}
        </Button>
      </div>
    </form>
  );
}
