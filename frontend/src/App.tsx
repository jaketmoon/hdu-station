import { FormEvent, useEffect, useState } from 'react'

import {
  BootstrapState,
  Conversation,
  getStationBindings,
  PersistedMessage,
  SettingsState,
  SandboxStatus,
  TencentChannelStatus,
  ToolAudit,
  getStationEvents,
} from './api'

type Message = {
  id: string
  role: 'assistant' | 'user'
  content: string
}

const suggestions = [
  '看看这学期能选什么课',
  '帮我找几门评价不错的课',
  '推荐事少、给分友好的水课',
]

const initialMessages: Message[] = [
  {
    id: 'welcome',
    role: 'assistant',
    content:
      '你好，我是 HDU Station。校园能力、频道检索、网页搜索和代码执行都会在这一个对话里完成。',
  },
]

const initialConversation: Conversation = {
  id: 'local-course-selection',
  title: '选课助手',
  createdAt: '',
  updatedAt: '',
}

function toMessages(items: PersistedMessage[]): Message[] {
  return items.map(({ id, role, content }) => ({ id, role, content }))
}

function auditLabel(audit: ToolAudit): string {
  const names: Record<string, string> = {
    hdu_academic_class_search: '校园课程检索',
    hdu_academic_course_selection: '本人选课查询',
    hdu_academic_schedule: '学期课表查询',
    hdu_academic_schedule_now: '今明课表查询',
    tencent_search_guild_feed: '频道帖子检索',
    web_search: '网页搜索',
    web_fetch: '网页读取',
  }
  return names[audit.toolName] ?? audit.toolName
}

export default function App() {
  const [draft, setDraft] = useState('')
  const [messages, setMessages] = useState(initialMessages)
  const [bootstrap, setBootstrap] = useState<BootstrapState | null>(null)
  const [conversations, setConversations] = useState<Conversation[]>([
    initialConversation,
  ])
  const [currentConversationID, setCurrentConversationID] = useState(
    initialConversation.id,
  )
  const [isSending, setIsSending] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')
  const [sandboxStatus, setSandboxStatus] = useState<SandboxStatus | null>(null)
  const [sandboxBusy, setSandboxBusy] = useState(false)
  const [sandboxMessage, setSandboxMessage] = useState('')
  const [tencentBusy, setTencentBusy] = useState(false)
  const [tencentStatus, setTencentStatus] = useState<TencentChannelStatus | null>(null)
  const [toolAudits, setToolAudits] = useState<ToolAudit[]>([])
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settings, setSettings] = useState<SettingsState | null>(null)
  const [selectedProvider, setSelectedProvider] = useState('openai')
  const [apiKey, setApiKey] = useState('')
  const [campusKey, setCampusKey] = useState('')
  const [model, setModel] = useState('')
  const [baseURL, setBaseURL] = useState('')
  const [protocol, setProtocol] = useState('responses')
  const [settingsMessage, setSettingsMessage] = useState('')

  useEffect(() => {
    const api = getStationBindings()
    const events = getStationEvents()
    const onToken = (payload: { conversationId: string; text: string }) => {
      if (!payload?.text) return
      setMessages((items) => {
        let streamIndex = -1
        for (let index = items.length - 1; index >= 0; index -= 1) {
          if (items[index].id.startsWith('stream-') && items[index].role === 'assistant') {
            streamIndex = index
            break
          }
        }
        if (streamIndex < 0) return items
        const next = [...items]
        next[streamIndex] = {
          ...next[streamIndex],
          content: next[streamIndex].content + payload.text,
        }
        return next
      })
    }
    events?.EventsOn('station:chat-token', onToken)
    if (!api) {
      return () => {
        events?.EventsOff?.('station:chat-token')
      }
    }

    let cancelled = false
    void api
      .Bootstrap()
      .then((state) => {
        if (!cancelled) setBootstrap(state)
      })
      .catch(() => undefined)

    void api
      .SandboxStatus()
      .then((status) => {
        if (!cancelled) setSandboxStatus(status)
      })
      .catch(() => undefined)

    void api
      .TencentChannelStatus()
      .then((status) => {
        if (!cancelled) setTencentStatus(status)
      })
      .catch(() => undefined)

    void api
      .Conversations()
      .then((items) => {
        if (cancelled || items.length === 0) return
        setConversations(items)
        setCurrentConversationID(items[0].id)
        return api.Messages(items[0].id)
      })
      .then((items) => {
        if (!cancelled && items && items.length > 0) {
          setMessages(toMessages(items))
        }
        if (!cancelled && items && items.length > 0) {
          void api.ToolAudits(items[0].conversationId).then(setToolAudits).catch(() => undefined)
        }
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      events?.EventsOff?.('station:chat-token')
    }
  }, [])

  function selectConversation(conversationID: string) {
    setCurrentConversationID(conversationID)
    const api = getStationBindings()
    if (!api) {
      setMessages(initialMessages)
      return
    }
    void api
      .Messages(conversationID)
      .then((items) => setMessages(items.length > 0 ? toMessages(items) : initialMessages))
      .catch(() => undefined)
    void api.ToolAudits(conversationID).then(setToolAudits).catch(() => setToolAudits([]))
  }

  async function createConversation() {
    const api = getStationBindings()
    try {
      const conversation = api
        ? await api.CreateConversation('新对话')
        : {
            ...initialConversation,
            id: `local-${Date.now()}`,
            title: '新对话',
          }
      setConversations((items) => [conversation, ...items])
      setCurrentConversationID(conversation.id)
      setMessages([])
      setToolAudits([])
    } catch {
      // Keep the existing conversation available if the desktop bridge is unavailable.
    }
  }

  async function openSettings() {
    setSettingsOpen(true)
    setSettingsMessage('')
    const api = getStationBindings()
    if (!api) {
      setSettingsMessage('浏览器预览没有桌面绑定，请在 HDU Station 应用中配置。')
      return
    }
    try {
      const next = await api.Settings()
      setSettings(next)
      const provider = next.providers.find((item) => item.name === selectedProvider) ?? next.providers[0]
      if (provider) {
        setSelectedProvider(provider.name)
        setModel(provider.model)
        setBaseURL(provider.baseURL)
        setProtocol(provider.protocol)
      }
    } catch {
      setSettingsMessage('无法读取本地设置。')
    }
  }

  function selectProvider(name: string) {
    setSelectedProvider(name)
    const provider = settings?.providers.find((item) => item.name === name)
    if (!provider) return
    setModel(provider.model)
    setBaseURL(provider.baseURL)
    setProtocol(provider.protocol)
  }

  async function saveSettings(event: FormEvent) {
    event.preventDefault()
    const api = getStationBindings()
    if (!api) {
      setSettingsMessage('浏览器预览没有桌面绑定，请在 HDU Station 应用中配置。')
      return
    }
    try {
      await api.SaveModelProvider(selectedProvider, apiKey, model, baseURL, protocol)
      if (campusKey.trim()) await api.SaveCampusKey(campusKey)
      const next = await api.Settings()
      setSettings(next)
      setApiKey('')
      setCampusKey('')
      setBootstrap(await api.Bootstrap())
      setSettingsMessage('设置已保存到本机。')
    } catch {
      setSettingsMessage('设置保存失败，请检查地址、协议和必填项。')
    }
  }

  async function installSandbox() {
    const api = getStationBindings()
    if (!api) {
      setSandboxMessage('浏览器预览没有桌面绑定。')
      return
    }
    setSandboxBusy(true)
    setSandboxMessage('正在准备本地隔离环境……')
    try {
      await api.InstallSandbox()
      setSandboxStatus(await api.SandboxStatus())
      setSandboxMessage('Sandbox 已安装；首次执行时会启动隔离环境。')
    } catch {
      setSandboxMessage('Sandbox 安装失败，请检查平台能力和镜像配置。')
    } finally {
      setSandboxBusy(false)
    }
  }

  async function clearAllData() {
    const api = getStationBindings()
    if (!api) {
      setSettingsMessage('浏览器预览没有桌面绑定。')
      return
    }
    if (!window.confirm('这会删除本机配置、会话、工作区和 Sandbox，且需要重启应用。继续吗？')) return
    try {
      await api.ClearAllData()
      setSettingsMessage('本机数据已清除，请重启 HDU Station。')
    } catch {
      setSettingsMessage('清除本机数据失败，Sandbox 可能仍在运行。')
    }
  }

  async function installTencentCLI() {
    const api = getStationBindings()
    if (!api) {
      setSettingsMessage('浏览器预览没有桌面绑定。')
      return
    }
    setTencentBusy(true)
    setSettingsMessage('正在从官方 npm registry 下载腾讯频道 CLI……')
    try {
      await api.InstallTencentCLI()
      const status = await api.TencentChannelStatus()
      setTencentStatus(status)
      setSettingsMessage(status.notice)
    } catch {
      setSettingsMessage('腾讯频道 CLI 安装失败，请检查平台支持和网络连接。')
    } finally {
      setTencentBusy(false)
    }
  }

  async function clearCampusCredential() {
    const api = getStationBindings()
    if (!api || !settings?.campusAuth.configured) return
    if (!window.confirm('只移除本机保存的杭电校园 Key，保留会话、模型设置和 Sandbox。继续吗？')) return
    try {
      await api.ClearCampusCredential()
      setSettings(await api.Settings())
      setBootstrap(await api.Bootstrap())
      setCampusKey('')
      setSettingsMessage('杭电校园 Key 已从本机移除。')
    } catch {
      setSettingsMessage('移除杭电校园 Key 失败。')
    }
  }

  async function importHduhelpCLICredential() {
    const api = getStationBindings()
    if (!api) {
      setSettingsMessage('浏览器预览没有桌面绑定。')
      return
    }
    if (!window.confirm('读取当前用户 hduhelp-cli 的本机 PAT，并替换 Station 保存的校园 Key。不会同步或修改 hduhelp-cli。继续吗？')) return
    try {
      await api.ImportHduhelpCLICredential()
      setSettings(await api.Settings())
      setBootstrap(await api.Bootstrap())
      setCampusKey('')
      setSettingsMessage('已导入当前 hduhelp-cli 的校园 PAT 到 Station。')
    } catch {
      setSettingsMessage('导入校园 PAT 失败；请确认 hduhelp-cli 已登录且凭据未过期。')
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    const content = draft.trim()
    if (!content || isSending) return
    const response = '当前是浏览器预览模式；请在 HDU Station 桌面应用中发送消息以调用本地模型和工具。'
    const api = getStationBindings()
    const useDesktopChat =
      api &&
      currentConversationID !== initialConversation.id &&
      !currentConversationID.startsWith('local-')
    setMessages((items) => [
      ...items,
      { id: `local-user-${Date.now()}`, role: 'user', content },
    ])
    setDraft('')
    setErrorMessage('')

    if (useDesktopChat) {
      setIsSending(true)
      const streamID = `stream-${Date.now()}`
      setMessages((items) => [...items, { id: streamID, role: 'assistant', content: '' }])
      try {
        const assistant = await api.ChatStream(currentConversationID, content)
        setMessages((items) =>
          items.map((item) => (item.id === streamID ? toMessages([assistant])[0] : item)),
        )
        void api.ToolAudits(currentConversationID).then(setToolAudits).catch(() => undefined)
      } catch {
        setMessages((items) => items.filter((item) => item.id !== streamID))
        setErrorMessage('模型暂时不可用，请检查设置中的 API Key 和模型配置。')
      } finally {
        setIsSending(false)
      }
      return
    }

    setMessages((items) => [
      ...items,
      { id: `local-assistant-${Date.now()}`, role: 'assistant', content: response },
    ])
  }

  return (
    <div className="station-shell">
      <aside className="sidebar">
        <header className="brand">
          <span className="brand-mark" aria-hidden="true">
            H
          </span>
          <span>
            <strong>HDU Station</strong>
            <small>本地校园 AI 工作站</small>
          </span>
        </header>

        <button className="new-chat" type="button" onClick={() => void createConversation()}>
          <span aria-hidden="true">＋</span>
          新对话
        </button>

        <nav aria-label="对话">
          <p className="section-label">今天</p>
          {conversations.map((conversation) => (
            <button
              className={`conversation${
                conversation.id === currentConversationID ? ' conversation--active' : ''
              }`}
              key={conversation.id}
              type="button"
              onClick={() => selectConversation(conversation.id)}
            >
              <span>{conversation.title}</span>
              <small>{conversation.id === currentConversationID ? '当前' : '历史'}</small>
            </button>
          ))}
        </nav>

        <footer className="sidebar-footer">
          <div className="local-status">
            <span className="status-dot" aria-hidden="true" />
            <span>
              <strong>本地运行</strong>
              <small>数据保存在这台电脑</small>
            </span>
          </div>
          <button className="settings-button" type="button" aria-label="打开设置" onClick={() => void openSettings()}>
            设置
          </button>
        </footer>
      </aside>

      <main className="workspace">
        <header className="workspace-header">
          <div>
            <p className="eyebrow">UNIFIED CAMPUS ASSISTANT</p>
            <h1>选课助手</h1>
          </div>
          <div className="capability-status" aria-label="能力状态">
            <span>
              校园{bootstrap ? (
                bootstrap.configuration.campusAuth.state === 'pat_verified'
                  ? ' · PAT 已验证'
                  : bootstrap.configuration.campusAuth.state === 'scope_missing'
                    ? ' · 缺少权限'
                    : bootstrap.configuration.campusAuth.state === 'pat_rejected'
                      ? ' · Key 被拒绝'
                  : bootstrap.configuration.campusAuth.configured
                    ? ' · PAT 已配置'
                    : ' · 待配置'
              ) : ''}
            </span>
            <span>频道{tencentStatus ? (tencentStatus.searchAvailable ? ' · 前置条件满足' : ' · 未就绪') : ''}</span>
            <span>网页</span>
            <span>
              Sandbox{sandboxStatus ? (sandboxStatus.ready ? ' · 就绪' : sandboxStatus.supported ? ' · 待安装' : ' · 不支持') : ''}
            </span>
          </div>
        </header>

        <section className="timeline" aria-live="polite">
          <div className="welcome-card">
            <div className="welcome-icon" aria-hidden="true">
              ✦
            </div>
            <div>
              <p className="eyebrow">HDU STATION</p>
              <h2>一个入口，处理所有校园信息</h2>
              <p>
                直接说你想完成的事情。Station 会自行决定查询校园、检索杭电频道、搜索网页，或在隔离环境中执行代码。
              </p>
            </div>
          </div>

          <div className="messages">
            {messages.map((message) => (
              <article className={`message message--${message.role}`} key={message.id}>
                <span className="message-role">
                  {message.role === 'assistant' ? 'Station' : '你'}
                </span>
                <p>{message.content}</p>
              </article>
            ))}
          </div>

          {toolAudits.length > 0 && (
            <div className="tool-audits" aria-label="工具调用记录">
              {toolAudits.slice(-8).map((audit) => (
                <span className={`tool-audit tool-audit--${audit.status}`} key={audit.id}>
                  <span aria-hidden="true">
                    {audit.status === 'completed' ? '✓' : audit.status === 'failed' ? '!' : '·'}
                  </span>
                  {auditLabel(audit)}
                  {audit.status === 'started' ? '处理中' : audit.status === 'failed' ? '失败' : '已完成'}
                </span>
              ))}
            </div>
          )}

          <div className="suggestions" aria-label="示例问题">
            {suggestions.map((suggestion) => (
              <button key={suggestion} type="button" onClick={() => setDraft(suggestion)}>
                {suggestion}
              </button>
            ))}
          </div>
        </section>

        <form className="composer" onSubmit={submit}>
          <textarea
            aria-label="给 HDU Station 发消息"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="问课程、通知、比赛、科研或就业信息……"
            rows={2}
          />
          <div className="composer-footer">
            <span aria-live="polite">
              {errorMessage || (isSending ? '正在请求模型……' : '需要执行代码时会自动使用安全环境')}
            </span>
          <button type="submit" disabled={!draft.trim() || isSending}>
              {isSending ? '请求中' : '发送'}
            </button>
          </div>
        </form>

        {settingsOpen && (
          <div className="settings-panel" role="dialog" aria-label="本地设置">
            <div className="settings-panel__header">
              <div>
                <p className="eyebrow">LOCAL CONFIGURATION</p>
                <h2>本地设置</h2>
              </div>
              <button type="button" onClick={() => setSettingsOpen(false)}>
                关闭
              </button>
            </div>
            <div className="sandbox-settings">
              <div>
                <strong>隔离执行环境</strong>
                <span>{sandboxStatus?.reason ?? '尚未读取平台状态。'}</span>
              </div>
              <button type="button" onClick={() => void installSandbox()} disabled={sandboxBusy || sandboxStatus?.supported === false}>
                {sandboxBusy ? '安装中' : '安装 Sandbox'}
              </button>
            </div>
            <div className="sandbox-settings">
              <div>
                <strong>杭电校园认证</strong>
                <span>{settings?.campusAuth.notice ?? '当前仅支持本机配置校园 Key。'}</span>
              </div>
              <span className="settings-status-badge" aria-label={settings?.campusAuth.state}>
                {!settings?.campusAuth.configured
                  ? '未配置'
                  : settings.campusAuth.state === 'pat_verified'
                    ? 'PAT 已验证'
                    : settings.campusAuth.state === 'scope_missing'
                      ? '缺少权限'
                      : settings.campusAuth.state === 'pat_rejected'
                        ? 'Key 被拒绝'
                        : 'PAT 已配置'}
              </span>
            </div>
            {settings?.campusAuth.configured && (
              <div className="sandbox-settings">
                <div>
                  <strong>移除校园 Key</strong>
                  <span>只删除 Station 保存的 Neo 凭据，不影响其他本地数据。</span>
                </div>
                <button type="button" onClick={() => void clearCampusCredential()}>移除</button>
              </div>
            )}
            <div className="sandbox-settings">
              <div>
                <strong>导入 hduhelp-cli PAT</strong>
                <span>按需读取当前用户已登录的 hduhelp-cli 凭据，导入后仍由 Station 独立保存。</span>
              </div>
              <button type="button" onClick={() => void importHduhelpCLICredential()}>导入</button>
            </div>
            <div className="sandbox-settings">
              <div>
                <strong>腾讯频道检索</strong>
                <span>{tencentStatus?.notice ?? '首次启用时从官方 registry 下载固定版本平台二进制，不需要 Node。'}</span>
              </div>
              <button type="button" onClick={() => void installTencentCLI()} disabled={tencentBusy || tencentStatus?.cliInstalled === true}>
                {tencentBusy ? '下载中' : '安装 CLI'}
              </button>
            </div>
            {sandboxMessage && <p className="sandbox-settings__message" aria-live="polite">{sandboxMessage}</p>}
            <form className="settings-form" onSubmit={saveSettings}>
              <label>
                模型供应商
                <select value={selectedProvider} onChange={(event) => selectProvider(event.target.value)}>
                  {(settings?.providers ?? [
                    { name: 'openai', type: 'openai', protocol: 'responses', baseURL: '', model: '', configured: false },
                    { name: 'anthropic', type: 'anthropic', protocol: 'messages', baseURL: '', model: '', configured: false },
                  ]).map((provider) => (
                    <option key={provider.name} value={provider.name}>
                      {provider.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                API Key <small>留空表示保持当前 Key</small>
                <input type="password" value={apiKey} onChange={(event) => setApiKey(event.target.value)} autoComplete="off" />
              </label>
              <label>
                模型
                <input value={model} onChange={(event) => setModel(event.target.value)} placeholder="例如 gpt-4.1-mini" />
              </label>
              <label>
                Base URL
                <input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://api.openai.com/v1" />
              </label>
              <label>
                协议
                <select value={protocol} onChange={(event) => setProtocol(event.target.value)}>
                  {selectedProvider === 'anthropic' ? (
                    <option value="messages">Anthropic Messages</option>
                  ) : (
                    <>
                      <option value="responses">OpenAI Responses</option>
                      <option value="chat_completions">OpenAI Chat Completions</option>
                    </>
                  )}
                </select>
              </label>
              <label>
                杭电校园 Key <small>留空表示保持当前 Key</small>
                <input type="password" value={campusKey} onChange={(event) => setCampusKey(event.target.value)} autoComplete="off" />
              </label>
              <div className="settings-form__footer">
                <span aria-live="polite">{settingsMessage}</span>
                <button type="submit">保存设置</button>
              </div>
            </form>
            <div className="settings-danger-zone">
              <strong>清理本机数据</strong>
              <span>删除 Station 自己创建的配置、会话、工作区和 Sandbox；不会删除腾讯 CLI 自己管理的登录凭据。</span>
              <button type="button" onClick={() => void clearAllData()}>清除全部数据</button>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
