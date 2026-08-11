import { FormEvent, useState } from 'react'

type Message = {
  id: number
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
    id: 1,
    role: 'assistant',
    content:
      '你好，我是 HDU Station。校园能力、频道检索、网页搜索和代码执行都会在这一个对话里完成。',
  },
]

export default function App() {
  const [draft, setDraft] = useState('')
  const [messages, setMessages] = useState(initialMessages)

  function submit(event: FormEvent) {
    event.preventDefault()
    const content = draft.trim()
    if (!content) return
    setMessages((items) => [
      ...items,
      { id: Date.now(), role: 'user', content },
      {
        id: Date.now() + 1,
        role: 'assistant',
        content: '桌面界面已收到消息，模型与工具运行时将在下一阶段接入。',
      },
    ])
    setDraft('')
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

        <button className="new-chat" type="button">
          <span aria-hidden="true">＋</span>
          新对话
        </button>

        <nav aria-label="对话">
          <p className="section-label">今天</p>
          <button className="conversation conversation--active" type="button">
            <span>选课助手</span>
            <small>刚刚</small>
          </button>
        </nav>

        <footer className="sidebar-footer">
          <div className="local-status">
            <span className="status-dot" aria-hidden="true" />
            <span>
              <strong>本地运行</strong>
              <small>数据保存在这台电脑</small>
            </span>
          </div>
          <button className="settings-button" type="button" aria-label="打开设置">
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
            <span>校园</span>
            <span>频道</span>
            <span>网页</span>
            <span>Sandbox</span>
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
            <span>需要执行代码时会自动使用安全环境</span>
            <button type="submit" disabled={!draft.trim()}>
              发送
            </button>
          </div>
        </form>
      </main>
    </div>
  )
}
