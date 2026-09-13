import { useEffect, useRef, useState } from "react";
import {
  api,
  errorText,
  type LoginSource,
  type SourceConnection,
  type SourceLogin,
} from "./api";
import { Icon } from "./Icon";

const labels: Record<string, string> = {
  ready: "已连接",
  verification_required: "需要安全验证",
  not_installed: "尚未连接",
  logged_out: "需要重新登录",
  unsupported: "当前平台暂不支持",
  unavailable: "暂时无法连接",
  disabled: "未启用",
  auth_failed: "连接验证失败",
};

export function SourceCard({
  source,
  name,
  connection,
  disabled,
  onChange,
}: {
  source: LoginSource;
  name: string;
  connection: SourceConnection;
  disabled: boolean;
  onChange: (source: LoginSource, connection: SourceConnection) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [login, setLogin] = useState<SourceLogin | null>(null);
  const [remaining, setRemaining] = useState(0);
  const mounted = useRef(true);
  const loginPanel = useRef<HTMLDivElement>(null);
  const current = useRef({ connection, onChange });
  current.current = { connection, onChange };
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  function connected(status: string, verification = false) {
    const { connection, onChange } = current.current;
    onChange(source, {
      enabled: connection.enabled,
      status: connection.enabled
        ? status === "saved"
          ? "unavailable"
          : "ready"
        : "disabled",
    });
    setNotice(
      verification
        ? "安全验证已通过，已确认搜索恢复可用。"
        : status === "saved"
          ? "登录凭证已保存，可稍后重新检查连接。"
          : `登录成功，凭证已保存在本机。${source === "xiaohongshu" ? "搜索可能仍需单独的安全验证。" : connection.enabled ? "可以开始搜索了。" : "启用后即可参与搜索。"}`,
    );
  }

  useEffect(() => {
    if (!login || login.status !== "waiting") return;
    const id = login.id;
    const expiresAt = login.expiresAt ?? Date.now();
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      const seconds = Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
      setRemaining(seconds);
      if (seconds === 0) {
        stopped = true;
        setLogin((value) =>
          value?.id === id
            ? { ...value, status: "expired", image: undefined }
            : value,
        );
      }
    };
    const poll = async () => {
      try {
        const result = await api.pollLogin(id);
        if (stopped) return;
        if (result.status === "waiting") {
          timer = setTimeout(poll, 3000);
          return;
        }
        setLogin((value) =>
          value?.id === id ? { ...value, ...result, image: undefined } : value,
        );
        if (result.status === "ready" || result.status === "saved")
          connected(result.status, login.kind === "verification");
        else if (result.status === "verification_required") {
          current.current.onChange(source, {
            ...current.current.connection,
            status: "verification_required",
          });
          setNotice("已登录，但搜索需要安全验证，请点击安全验证。");
        } else if (result.status === "cancelled")
          setNotice("登录已取消，可以重新连接。");
      } catch (error) {
        if (stopped) return;
        setError(errorText(error));
        setLogin((value) =>
          value?.id === id
            ? { ...value, status: "error", image: undefined }
            : value,
        );
      }
    };
    tick();
    const interval = setInterval(tick, 1000);
    if (!stopped) void poll();
    return () => {
      stopped = true;
      clearInterval(interval);
      clearTimeout(timer);
      void api.cancelLogin(id).catch(() => {});
    };
    // Connection changes are read through current; a pending login is never restarted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [login?.id, login?.status]);

  async function action(
    kind: "check" | "enable" | "connect" | "verify" | "clear",
    enabled?: boolean,
  ) {
    setBusy(kind);
    setError("");
    setNotice("");
    if (kind !== "check") setLogin(null);
    try {
      if (kind === "connect" || kind === "verify") {
        const result =
          kind === "verify"
            ? await api.beginVerification()
            : await api.beginLogin(source);
        if (!mounted.current) {
          void api.cancelLogin(result.id).catch(() => {});
          return;
        }
        setRemaining(
          Math.max(0, Math.ceil(((result.expiresAt ?? 0) - Date.now()) / 1000)),
        );
        setLogin(result);
        if (source === "xiaohongshu" && result.status === "waiting") {
          const { connection, onChange } = current.current;
          onChange(source, {
            enabled: connection.enabled,
            status: connection.enabled
              ? result.kind === "verification"
                ? "verification_required"
                : "logged_out"
              : "disabled",
          });
        }
        if (result.status === "ready" || result.status === "saved")
          connected(result.status, result.kind === "verification");
      } else {
        const result =
          kind === "check"
            ? await api.checkSource(source)
            : kind === "clear"
              ? await api.clearSource(source)
              : await api.enableSource(source, !!enabled);
        if (!mounted.current) return;
        current.current.onChange(source, result);
        setNotice(
          kind === "clear"
            ? "登录凭证已清除，下次使用时请重新扫码。"
            : kind === "enable"
              ? result.enabled
                ? "已启用此来源。"
                : "已关闭此来源，登录凭证仍保留。"
              : "",
        );
      }
    } catch (error) {
      if (mounted.current) setError(errorText(error));
    } finally {
      if (mounted.current) setBusy("");
    }
  }
  const waiting = login?.status === "waiting";
  const verification = login?.kind === "verification" || busy === "verify";
  useEffect(() => {
    if (expanded && (waiting || busy === "connect" || busy === "verify")) {
      loginPanel.current?.scrollIntoView?.({ block: "nearest" });
    }
  }, [expanded, waiting, busy]);
  const locked = disabled || !!busy;
  const unsupported = connection.status === "unsupported";
  return (
    <section className="source-card" aria-label={`${name}来源`}>
      <div className="source-card-header">
        <button
          type="button"
          className="source-disclosure"
          aria-expanded={expanded}
          aria-controls={`${source}-controls`}
          onClick={() => setExpanded(!expanded)}
        >
          <span className={`source-chevron ${expanded ? "expanded" : ""}`}>
            <Icon name="right" size={13} />
          </span>
          <span>
            <strong>{name}</strong>
            <span className="source-status">
              <span
                className={`status-dot ${connection.status === "ready" && !waiting ? "online" : ""}`}
              />
              {waiting
                ? verification
                  ? "等待扫码验证身份"
                  : "等待扫码登录"
                : (labels[connection.status] ?? "正在检查…")}
            </span>
          </span>
        </button>
        <button
          type="button"
          className="secondary-button source-check"
          aria-label={`重新检查${name}`}
          onClick={() => void action("check")}
          disabled={locked || waiting}
        >
          {busy === "check" ? "检查中…" : "重新检查"}
        </button>
      </div>
      <div
        id={`${source}-controls`}
        className="source-controls"
        hidden={!expanded}
      >
        <div className="source-enable-row">
          <span id={`${source}-enable-label`}>启用{name}搜索</span>
          <button
            type="button"
            role="switch"
            className="source-switch"
            aria-checked={connection.enabled}
            aria-labelledby={`${source}-enable-label`}
            disabled={locked || waiting || unsupported}
            onClick={() => void action("enable", !connection.enabled)}
          >
            <span />
          </button>
        </div>
        <p className="field-help">
          {source === "qq"
            ? "阅读杭电频道里的课程讨论和同学评价。"
            : "查找杭电相关笔记，阅读文字与评论。"}
        </p>
        {connection.status === "verification_required" && !waiting && (
          <p className="field-help">
            账号已登录，但搜索需要额外验证。请点击安全验证，用已登录该账号的小红书
            App 扫码。
          </p>
        )}
        <div className="source-actions">
          {source === "xiaohongshu" && (
            <button
              type="button"
              className="secondary-button"
              disabled={locked || waiting || unsupported}
              onClick={() => void action("verify")}
            >
              {busy === "verify" ? "正在获取验证二维码…" : "安全验证"}
            </button>
          )}
          <button
            type="button"
            className="secondary-button"
            onClick={() => void action("connect")}
            disabled={locked || waiting || unsupported}
          >
            {busy === "connect" ? "正在准备二维码…" : "重新连接"}
          </button>
          <button
            type="button"
            className="source-clear"
            onClick={() => void action("clear")}
            disabled={
              locked ||
              waiting ||
              unsupported ||
              connection.status === "not_installed"
            }
          >
            {busy === "clear" ? "清除中…" : "清除登录凭证"}
          </button>
        </div>
        {(busy === "connect" ||
          busy === "verify" ||
          waiting ||
          login?.status === "expired") && (
          <div
            ref={loginPanel}
            className="source-login"
            aria-label={`${name}${verification ? "安全验证" : "扫码登录"}`}
          >
            <div className="source-qr">
              {waiting && login?.image ? (
                <img
                  src={login.image}
                  alt={`${name}${verification ? "安全验证" : "登录"}二维码`}
                  width={184}
                  height={184}
                />
              ) : (
                <div className="qr-placeholder">
                  <Icon
                    name={login?.status === "expired" ? "close" : "stars"}
                    size={30}
                  />
                  <span>
                    {login?.status === "expired"
                      ? "二维码已过期"
                      : "正在获取二维码…"}
                  </span>
                </div>
              )}
            </div>
            <strong>
              {login?.status === "expired"
                ? "重新获取后即可扫码"
                : `打开${source === "qq" ? "QQ" : "小红书"}扫一扫`}
            </strong>
            <p className="field-help">
              {login?.status === "expired"
                ? verification
                  ? "点击安全验证，获取新的验证二维码。"
                  : "点击重新连接，获取新的二维码。"
                : verification
                  ? "使用已登录该账号的小红书 App 扫码验证身份。验证后会检查搜索是否恢复。"
                  : "在手机上确认登录，凭证会自动保存。"}
            </p>
            {waiting && (
              <>
                <span className="qr-countdown">
                  等待扫码 · {Math.floor(remaining / 60)}:
                  {String(remaining % 60).padStart(2, "0")} 后失效
                </span>
                <button
                  type="button"
                  className="source-clear"
                  onClick={() => {
                    setLogin(null);
                    setNotice("登录已取消，可以重新连接。");
                  }}
                >
                  取消等待
                </button>
              </>
            )}
          </div>
        )}
        {connection.status === "unavailable" && !login && !busy && (
          <p className="field-help">
            {source === "xiaohongshu"
              ? "小红书连接组件暂不可用，请启动本机组件后重新检查。"
              : "暂时无法连接，请检查网络后重试。"}
          </p>
        )}
        {error && (
          <p className="source-error" role="alert">
            {error}
          </p>
        )}
        {notice && (
          <p className="source-notice" role="status">
            {notice}
          </p>
        )}
      </div>
      {!expanded && error && (
        <p className="source-error" role="alert">
          {error}
        </p>
      )}
      {!expanded && notice && (
        <p className="source-notice" role="status">
          {notice}
        </p>
      )}
    </section>
  );
}
