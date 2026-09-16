import { useEffect, useRef, useState } from "react";
import {
  api,
  errorText,
  type Settings,
  type LoginSource,
  type SourceConnection,
  type Appearance,
} from "./api";
import { Icon } from "./Icon";
import { SourceCard } from "./SourceCard";
import { CampusCard } from "./CampusCard";
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
  appearance,
  onAppearanceChange,
  appearanceBusy,
  systemReduced,
  appearanceError,
}: {
  settings: Settings | null;
  onClose: () => void;
  onSaved: (settings: Settings) => void;
  appearance?: Appearance;
  onAppearanceChange?: (appearance: Appearance) => Promise<void>;
  appearanceBusy?: boolean;
  systemReduced?: boolean;
  appearanceError?: string;
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
  const initialized = useRef(settings !== null);
  useEffect(() => {
    if (!initialized.current && settings) {
      initialized.current = true;
      setBaseURL(settings.baseURL);
      setZanaoEnabled(settings.zanao.enabled);
      setSchoolAlias(settings.zanao.schoolAlias);
    }
  }, [settings]);
  const zanaoDirty =
    zanaoEnabled !== settings?.zanao.enabled ||
    schoolAlias !== settings?.zanao.schoolAlias ||
    !!zanaoToken ||
    clearZanaoToken;
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
      });
      onSaved(saved);
      setBaseURL(saved.baseURL);
      setSchoolAlias(saved.zanao.schoolAlias);
      setKey("");
      setZanaoToken("");
      setClearZanaoToken(false);
      setNotice("设置已保存，连接状态已更新。");
    } catch (error) {
      setError(errorText(error));
    } finally {
      setSaving(false);
    }
  }
  const latest = useRef(settings);
  latest.current = settings;
  function updateSource(source: LoginSource, connection: SourceConnection) {
    const value = latest.current;
    if (!value) return;
    const next =
      source === "qq"
        ? {
            ...value,
            qqEnabled: connection.enabled,
            qqStatus: connection.status,
          }
        : { ...value, xiaohongshu: { ...value.xiaohongshu, ...connection } };
    latest.current = next;
    onSaved(next);
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
            <p className="eyebrow">TERMINAL CONFIGURATION / 终端配置</p>
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
        {appearanceError && (
          <p role="alert" className="inline-error">
            {appearanceError}
          </p>
        )}
        {appearance && onAppearanceChange && (
          <details className="appearance-settings">
            <summary>
              <span>
                <Icon name="terminal" size={17} />
                显示与动效
              </span>
              <span>DISPLAY</span>
            </summary>
            <div className="appearance-option">
              <div>
                <strong>剧情式逐字对话</strong>
                <p>
                  {systemReduced
                    ? "系统已减弱动效，回答会直接显示。"
                    : "逐字解码情报；随时可以立即显示。"}
                </p>
              </div>
              <button
                type="button"
                className="source-switch"
                role="switch"
                aria-label="剧情式逐字对话"
                aria-checked={!appearance.instantText && !systemReduced}
                disabled={appearanceBusy || systemReduced}
                onClick={() =>
                  onAppearanceChange({
                    ...appearance,
                    instantText: !appearance.instantText,
                  })
                }
              >
                <span />
              </button>
            </div>
            <p className="field-help">修改后自动保存到本机。</p>
          </details>
        )}
        <form onSubmit={save}>
          <fieldset disabled={saving || !settings}>
            <div className="model-card">
              <span className="model-icon">
                <Icon name="stars" />
              </span>
              <div>
                <strong>DeepSeek V4.1 Flash</strong>
                <p>情报分析核心 · 课程线索由此汇合</p>
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
            <CampusCard
              connection={
                settings?.campus ?? {
                  hasCredential: false,
                  status: "logged_out",
                }
              }
              disabled={saving || !settings}
              onChange={(campus) => {
                if (!latest.current) return;
                const next = { ...latest.current, campus };
                latest.current = next;
                onSaved(next);
              }}
            />
            <div className="sources-heading">搜索来源</div>
            <SourceCard
              source="qq"
              name="QQ 频道"
              connection={{
                enabled: settings?.qqEnabled ?? true,
                status: settings?.qqStatus ?? "unavailable",
              }}
              disabled={saving || !settings}
              onChange={updateSource}
            />
            <SourceCard
              source="xiaohongshu"
              name="小红书"
              connection={{
                enabled: settings?.xiaohongshu.enabled ?? false,
                status: settings?.xiaohongshu.status ?? "disabled",
              }}
              disabled={saving || !settings}
              onChange={updateSource}
            />
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
