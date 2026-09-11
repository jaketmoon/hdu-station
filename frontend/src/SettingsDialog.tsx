import { useEffect, useRef, useState } from "react";
import { api, errorText, type Settings } from "./api";
import { Icon } from "./Icon";
const labels: Record<string, string> = {
  ready: "已连接",
  not_installed: "尚未安装连接组件",
  logged_out: "需要重新登录",
  unsupported: "当前平台暂不支持",
  unavailable: "暂时无法连接",
  disabled: "未启用",
  not_configured: "尚未配置",
  auth_failed: "服务 Token 无效",
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
  const [notice, setNotice] = useState("");
  const [zanaoEnabled, setZanaoEnabled] = useState(
    settings?.zanao.enabled ?? false,
  );
  const [schoolAlias, setSchoolAlias] = useState(
    settings?.zanao.schoolAlias ?? "",
  );
  const [zanaoToken, setZanaoToken] = useState("");
  const [clearZanaoToken, setClearZanaoToken] = useState(false);
  const [xhsEnabled, setXhsEnabled] = useState(
    settings?.xiaohongshu.enabled ?? false,
  );
  const [xhsURL, setXhsURL] = useState(
    settings?.xiaohongshu.baseURL ?? "http://127.0.0.1:18060",
  );
  const [xhsToken, setXhsToken] = useState("");
  const [clearXhsToken, setClearXhsToken] = useState(false);
  const initialized = useRef(settings !== null);
  useEffect(() => {
    if (!initialized.current && settings) {
      initialized.current = true;
      setBaseURL(settings.baseURL);
      setZanaoEnabled(settings.zanao.enabled);
      setSchoolAlias(settings.zanao.schoolAlias);
      setXhsEnabled(settings.xiaohongshu.enabled);
      setXhsURL(settings.xiaohongshu.baseURL);
    }
  }, [settings]);
  const zanaoDirty =
    zanaoEnabled !== settings?.zanao.enabled ||
    schoolAlias !== settings?.zanao.schoolAlias ||
    !!zanaoToken ||
    clearZanaoToken;
  const xhsDirty =
    xhsEnabled !== settings?.xiaohongshu.enabled ||
    xhsURL !== settings?.xiaohongshu.baseURL ||
    !!xhsToken ||
    clearXhsToken;
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  async function save(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const saved = await api.save({
        baseURL,
        apiKey: key,
        zanao: {
          enabled: zanaoEnabled,
          schoolAlias,
          token: zanaoToken,
          clearToken: clearZanaoToken,
        },
        xiaohongshu: {
          enabled: xhsEnabled,
          baseURL: xhsURL,
          authToken: xhsToken,
          clearAuthToken: clearXhsToken,
        },
      });
      onSaved(saved);
      setBaseURL(saved.baseURL);
      setSchoolAlias(saved.zanao.schoolAlias);
      setXhsURL(saved.xiaohongshu.baseURL);
      setKey("");
      setZanaoToken("");
      setXhsToken("");
      setClearZanaoToken(false);
      setClearXhsToken(false);
      setNotice("设置已保存，连接状态已更新。");
    } catch (error) {
      setError(errorText(error));
    } finally {
      setSaving(false);
    }
  }
  async function connect() {
    setSaving(true);
    setError("");
    setNotice("");
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
          <fieldset disabled={saving || !settings}>
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
              {settings?.hasAPIKey && (
                <span className="field-note">已保存</span>
              )}
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
            <details className="source-settings">
              <summary>
                <strong>赞哦校园集市</strong>
                <span>
                  {zanaoDirty
                    ? "保存后检查"
                    : labels[settings?.zanao.status ?? "disabled"]}
                </span>
              </summary>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={zanaoEnabled}
                  onChange={(event) => setZanaoEnabled(event.target.checked)}
                />
                启用赞哦搜索
              </label>
              <p className="field-help">搜索所选学校的帖子、历史讨论和评论。</p>
              <label htmlFor="zanao-school">学校别名</label>
              <input
                id="zanao-school"
                value={schoolAlias}
                onChange={(event) => setSchoolAlias(event.target.value)}
                required={zanaoEnabled}
                autoComplete="off"
                spellCheck={false}
                placeholder="填写赞哦请求中的 X-Sc-Alias"
              />
              <label htmlFor="zanao-token">
                赞哦 Token{" "}
                {settings?.zanao.hasToken && (
                  <span className="field-note">已保存</span>
                )}
              </label>
              <input
                id="zanao-token"
                type="password"
                value={zanaoToken}
                onChange={(event) => setZanaoToken(event.target.value)}
                autoComplete="new-password"
                required={
                  zanaoEnabled && (!settings?.zanao.hasToken || clearZanaoToken)
                }
                placeholder={
                  settings?.zanao.hasToken
                    ? "留空则保留现有 Token"
                    : "填写赞哦请求中的 X-Sc-Od"
                }
              />
              <p className="field-help">
                在电脑微信登录赞哦小程序后，从自己的请求头获取学校别名和
                Token。请确认对应杭电。
              </p>
              {settings?.zanao.hasToken && (
                <label className="check-label">
                  <input
                    type="checkbox"
                    checked={clearZanaoToken}
                    onChange={(event) =>
                      setClearZanaoToken(event.target.checked)
                    }
                  />
                  清除已保存的赞哦 Token
                </label>
              )}
              {settings?.zanao.status === "unavailable" && (
                <p className="field-help">
                  请检查 Token 是否过期、学校别名是否正确，或稍后重试。
                </p>
              )}
            </details>
            <details className="source-settings">
              <summary>
                <strong>小红书</strong>
                <span>
                  {xhsDirty
                    ? "保存后检查"
                    : labels[settings?.xiaohongshu.status ?? "disabled"]}
                </span>
              </summary>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={xhsEnabled}
                  onChange={(event) => setXhsEnabled(event.target.checked)}
                />
                启用小红书搜索
              </label>
              <p className="field-help">查找杭电相关笔记，阅读文字与评论。</p>
              <label htmlFor="xhs-address">小红书本机服务地址</label>
              <input
                id="xhs-address"
                type="url"
                value={xhsURL}
                onChange={(event) => setXhsURL(event.target.value)}
                required={xhsEnabled}
                autoComplete="off"
                spellCheck={false}
                placeholder="http://127.0.0.1:18060"
              />
              <label htmlFor="xhs-token">
                服务访问 Token（可选）
                {settings?.xiaohongshu.hasAuthToken && (
                  <span className="field-note">已保存</span>
                )}
              </label>
              <input
                id="xhs-token"
                type="password"
                value={xhsToken}
                onChange={(event) => setXhsToken(event.target.value)}
                autoComplete="new-password"
                placeholder={
                  settings?.xiaohongshu.hasAuthToken
                    ? "留空则保留现有 Token"
                    : "仅在服务启用了访问鉴权时填写"
                }
              />
              {settings?.xiaohongshu.hasAuthToken && (
                <label className="check-label">
                  <input
                    type="checkbox"
                    checked={clearXhsToken}
                    onChange={(event) => setClearXhsToken(event.target.checked)}
                  />
                  清除已保存的服务 Token
                </label>
              )}
              <p className="field-help">
                先在本机运行
                xiaohongshu-mcp，并通过配套登录工具扫码登录。填写服务根地址，无需加
                /mcp。服务访问 Token 对应 AUTH_TOKEN，并非账号 Cookie。
              </p>
              {settings?.xiaohongshu.status === "logged_out" && (
                <p className="field-help">
                  服务已连接，账号尚未登录。请扫码登录后保存并检查。
                </p>
              )}
              {settings?.xiaohongshu.status === "unavailable" && (
                <p className="field-help">
                  请确认本机服务正在运行、端口正确，浏览器已准备好。
                </p>
              )}
            </details>
            <p className="local-note">
              对话自动保存在本机，可在历史列表中删除。
            </p>
            {notice && (
              <p role="status" className="field-help">
                {notice}
              </p>
            )}
            {error && (
              <p role="alert" className="inline-error">
                {error}
              </p>
            )}
            <button className="primary-button save-button" disabled={saving}>
              {saving ? "正在检查连接…" : "保存并检查"}
            </button>
          </fieldset>
        </form>
      </div>
    </dialog>
  );
}
