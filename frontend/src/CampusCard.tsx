import { useEffect, useRef, useState } from "react";
import { api, errorText, type CampusConnection, type CampusLogin } from "./api";

const labels: Record<string, string> = {
  logged_out: "尚未登录",
  saved: "已登录",
  expired: "登录已过期",
  unsupported: "当前平台暂不支持",
};

const help: Record<string, string> = {
  saved: "可核实本学期开课；读取本人课表后，可筛选不撞课的班级。",
  expired: "请重新完成网页授权以更新登录信息。",
};

export function CampusCard({
  connection,
  disabled,
  onChange,
}: {
  connection: CampusConnection;
  disabled: boolean;
  onChange: (connection: CampusConnection) => void;
}) {
  const [login, setLogin] = useState<CampusLogin | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [remaining, setRemaining] = useState(0);
  const mounted = useRef(true);
  const generation = useRef(0);
  const current = useRef(onChange);
  current.current = onChange;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const loginID = login?.id;
  useEffect(() => {
    if (!loginID) return;
    const id = loginID;
    const version = generation.current;
    const expiresAt = login?.expiresAt ?? Date.now();
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      const seconds = Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
      setRemaining(seconds);
      if (!seconds && !stopped) {
        stopped = true;
        setLogin((value) =>
          value?.id === id ? { ...value, status: "expired" } : value,
        );
        void api.cancelCampusLogin(id).catch(() => {});
      }
    };
    const poll = async () => {
      try {
        const result = await api.pollCampusLogin(id);
        if (stopped || version !== generation.current) return;
        if (result.status === "waiting") {
          timer = setTimeout(poll, 1000);
          return;
        }
        // Once approved, updating local login state may outlive the
        // device request's deadline. It must not expire the saved login.
        clearInterval(interval);
        setLogin(result);
        if (result.status === "ready") {
          setBusy(true);
          current.current({ hasCredential: true, status: "saved" });
          setNotice("HDU CLI 登录已保存。");
          const checked = await api.checkCampus();
          if (stopped || version !== generation.current) return;
          current.current(checked);
          setNotice(
            checked.status === "saved"
              ? "HDU CLI 登录已保存。"
              : "登录状态已更新。",
          );
          setBusy(false);
        } else if (result.message) setError(result.message);
        stopped = true;
        clearInterval(interval);
      } catch (error) {
        if (stopped || version !== generation.current) return;
        stopped = true;
        setBusy(false);
        clearInterval(interval);
        setError(errorText(error));
        setLogin((value) =>
          value?.id === id ? { ...value, status: "error" } : value,
        );
        void api.cancelCampusLogin(id).catch(() => {});
      }
    };
    tick();
    const interval = setInterval(tick, 1000);
    if (!stopped) void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
      clearInterval(interval);
      void api.cancelCampusLogin(id).catch(() => {});
    };
    // A result updates the existing session; it must not restart polling.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loginID]);

  async function begin() {
    generation.current++;
    setBusy(true);
    setError("");
    setNotice("");
    setLogin(null);
    try {
      const result = await api.beginCampusLogin();
      if (!mounted.current) {
        await api.cancelCampusLogin(result.id);
        return;
      }
      setLogin(result);
    } catch (error) {
      if (mounted.current) setError(errorText(error));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function run(action: "check" | "logout" | "open" | "cancel") {
    if (action !== "open") generation.current++;
    setBusy(true);
    setError("");
    try {
      if (action === "open" && login) await api.openCampusLogin(login.id);
      else if (action === "cancel" && login) {
        const result = await api.cancelCampusLogin(login.id);
        if (result.status === "ready") {
          const checked = await api.checkCampus();
          if (mounted.current) {
            current.current(checked);
            setLogin(null);
            setNotice("HDU CLI 登录已完成。");
          }
        } else if (mounted.current) {
          setLogin(null);
          setNotice("已取消本次校园授权。");
        }
      } else if (action === "check" || action === "logout") {
        const result = await (action === "check"
          ? api.checkCampus()
          : api.logoutCampus());
        if (mounted.current) {
          current.current(result);
          setNotice(result.message ?? "登录状态已更新。");
        }
      }
    } catch (error) {
      if (mounted.current) setError(errorText(error));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  const waiting = login?.status === "waiting";
  return (
    <details className="source-settings campus-settings">
      <summary>
        <strong>HDU CLI 登录</strong>
        <span>
          {waiting ? "等待网页授权" : (labels[connection.status] ?? "状态未知")}
        </span>
      </summary>
      <p className="field-help">
        授权读取课程信息和本人课表，核实本学期开课，并把合适的课放入空闲位置。
      </p>
      <div className="source-actions">
        <button
          type="button"
          className="primary-button"
          onClick={begin}
          disabled={
            disabled || busy || waiting || connection.status === "unsupported"
          }
        >
          {busy && !waiting
            ? "处理中…"
            : connection.hasCredential
              ? "重新授权"
              : "网页授权"}
        </button>
        <button
          type="button"
          className="text-button"
          onClick={() => void run("check")}
          disabled={disabled || busy || waiting}
        >
          刷新状态
        </button>
        {connection.hasCredential && (
          <button
            type="button"
            className="source-clear"
            onClick={() => void run("logout")}
            disabled={disabled || busy || waiting}
          >
            退出登录
          </button>
        )}
      </div>
      {waiting && (
        <div className="source-login" role="status">
          <strong>请在浏览器中确认授权</strong>
          <p className="field-help">核对网页上的授权码</p>
          <code className="campus-user-code">{login.userCode}</code>
          <p className="field-help">
            仅申请课程信息与本人课表读取权限 · 剩余 {Math.floor(remaining / 60)}{" "}
            分 {remaining % 60} 秒
          </p>
          <div className="source-actions">
            <button
              type="button"
              className="text-button"
              disabled={busy}
              onClick={() => void run("open")}
            >
              再次打开授权页
            </button>
            <button
              type="button"
              className="source-clear"
              disabled={busy}
              onClick={() => void run("cancel")}
            >
              取消授权
            </button>
          </div>
        </div>
      )}
      {login?.status === "expired" && (
        <p role="status" className="field-help">
          授权请求已过期，请重新发起。
        </p>
      )}
      {login?.status === "denied" && (
        <p role="status" className="field-help">
          你已拒绝本次授权，可以重新发起。
        </p>
      )}
      {login?.status === "cancelled" && (
        <p role="status" className="field-help">
          本次授权已取消。
        </p>
      )}
      <p className="field-help">
        授权仅保存在这台电脑上。
        {help[connection.status] ?? "社区选课讨论无需校园登录。"}
      </p>
      {connection.status === "saved" && !connection.scheduleAccess && (
        <p role="status" className="field-help">
          当前登录尚未记录课表读取权限。点击“重新授权”后，即可按本人课表筛选。
        </p>
      )}
      {notice && (
        <p role="status" className="field-help">
          {notice}
        </p>
      )}
      {error && (
        <p role="alert" className="source-error">
          {error}
        </p>
      )}
    </details>
  );
}
