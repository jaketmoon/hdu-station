import { useEffect, useRef, useState } from "react";
import { api, errorText, type Settings } from "./api";
import { Icon } from "./Icon";
const labels: Record<string, string> = {
  ready: "已连接",
  not_installed: "尚未安装连接组件",
  logged_out: "需要重新登录",
  unsupported: "当前平台暂不支持",
  unavailable: "暂时无法连接",
};
export function SettingsDialog({
  settings,
  onClose,
  onSaved,
}: {
  settings: Settings | null;
  onClose: () => void;
  onSaved: (settings: Settings) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [baseURL, setBaseURL] = useState(
    settings?.baseURL ?? "https://api.deepseek.com",
  );
  const [key, setKey] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  async function save(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      onSaved(await api.save(baseURL, key));
      onClose();
    } catch (error) {
      setError(errorText(error));
    } finally {
      setSaving(false);
    }
  }
  async function connect() {
    setSaving(true);
    setError("");
    try {
      onSaved(
        settings?.qqStatus === "not_installed"
          ? await api.install()
          : await api.settings(),
      );
    } catch (error) {
      setError(errorText(error));
    } finally {
      setSaving(false);
    }
  }
  return (
    <dialog
      ref={dialog}
      className="settings-dialog"
      onCancel={(event) => {
        event.preventDefault();
        if (!saving) onClose();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget && !saving) onClose();
      }}
      aria-labelledby="settings-title"
    >
      <div className="dialog-inner">
        <div className="dialog-heading">
          <div>
            <p className="eyebrow">保持连接</p>
            <h2 id="settings-title">助手设置</h2>
          </div>
          <button
            className="icon-button"
            aria-label="关闭设置"
            onClick={onClose}
            disabled={saving}
          >
            <Icon name="close" />
          </button>
        </div>
        <form onSubmit={save}>
          <div className="model-card">
            <span className="model-icon">
              <Icon name="stars" />
            </span>
            <div>
              <strong>DeepSeek V4.1 Flash</strong>
              <p>为每一次选课，提供一点思路。</p>
            </div>
          </div>
          <label htmlFor="model-address">模型 API 地址</label>
          <input
            id="model-address"
            value={baseURL}
            onChange={(event) => setBaseURL(event.target.value)}
            type="url"
            required
            autoComplete="off"
            spellCheck={false}
          />
          <label htmlFor="model-key">
            API Key{" "}
            {settings?.hasAPIKey && <span className="field-note">已保存</span>}
          </label>
          <input
            id="model-key"
            value={key}
            onChange={(event) => setKey(event.target.value)}
            type="password"
            placeholder={
              settings?.hasAPIKey ? "留空则保留现有 Key" : "输入你的 API Key"
            }
            autoComplete="new-password"
            required={!settings?.hasAPIKey}
          />
          <p className="field-help">凭证仅保存在这台电脑上。</p>
          <div className="connection-row">
            <div>
              <strong>QQ 频道</strong>
              <p>
                <span
                  className={`status-dot ${settings?.qqStatus === "ready" ? "online" : ""}`}
                />
                {labels[settings?.qqStatus ?? "unavailable"]}
              </p>
            </div>
            <button
              type="button"
              className="secondary-button"
              onClick={connect}
              disabled={saving}
            >
              {settings?.qqStatus === "not_installed"
                ? "安装连接组件"
                : "重新检查"}
            </button>
          </div>
          {settings?.qqStatus === "logged_out" && (
            <p className="field-help">
              复用本机腾讯频道 CLI 登录。请在终端完成 QQ
              扫码登录后，点击重新检查。
            </p>
          )}
          <p className="local-note">对话自动保存在本机，可在历史列表中删除。</p>
          {error && (
            <p role="alert" className="inline-error">
              {error}
            </p>
          )}
          <button className="primary-button save-button" disabled={saving}>
            {saving ? "正在连接…" : "保存设置"}
          </button>
        </form>
      </div>
    </dialog>
  );
}
