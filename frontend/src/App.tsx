import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  api,
  errorText,
  type Conversation,
  type Message,
  type Settings,
  type TurnEvent,
} from "./api";
import { Icon, type IconName } from "./Icon";
import { Markdown } from "./Markdown";
import { SettingsDialog } from "./SettingsDialog";

const suggestions: {
  title: string;
  detail: string;
  question: string;
  icon: IconName;
}[] = [
  {
    title: "想选点轻松的",
    detail: "作业少一点，学期从容一点",
    question: "通识选修有什么水课？想找作业少、考核轻松的。",
    icon: "leaf",
  },
  {
    title: "努力也想拿高分",
    detail: "看看给分和考核方式",
    question: "有哪些给分比较高的通识选修？最好不用闭卷考试。",
    icon: "sun",
  },
  {
    title: "人文经典怎么选",
    detail: "听听上过课的同学怎么说",
    question:
      "人文经典类通识选修有哪些比较轻松的课？老师和考核有什么要注意的？",
    icon: "book",
  },
];
type Active = {
  requestId: string;
  conversationId: string;
  user: Message;
  assistant: Message;
  phase: string;
};
function mergeMessages(previous: Message[], incoming: Message[]) {
  const ids = new Set(incoming.map((item) => item.id));
  return [...previous.filter((item) => !ids.has(item.id)), ...incoming];
}

export default function App() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [selected, setSelected] = useState("");
  const selectedRef = useRef("");
  const [history, setHistory] = useState<Message[]>([]);
  const [draft, setDraft] = useState("");
  const [active, setActive] = useState<Active | null>(null);
  const activeRef = useRef<Active | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState("");
  const [jump, setJump] = useState(false);
  const [stopping, setStopping] = useState(false);
  const timeline = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const follow = useRef(true);
  const loadSequence = useRef(0);
  const updateActive = useCallback((next: Active | null) => {
    activeRef.current = next;
    setActive(next);
  }, []);
  const select = useCallback((id: string) => {
    selectedRef.current = id;
    setSelected(id);
    setHistory([]);
    setSidebarOpen(false);
    setError("");
    follow.current = true;
  }, []);

  useEffect(() => {
    let mounted = true;
    Promise.resolve()
      .then(() => api.list())
      .then((items) => {
        if (mounted) setConversations(items);
      })
      .catch((error) => {
        if (mounted) setError(errorText(error));
      });
    Promise.resolve()
      .then(() => api.settings())
      .then((value) => {
        if (mounted) setSettings(value);
      })
      .catch(() => {});
    const unsubscribe = api.subscribe((event: TurnEvent) => {
      const current = activeRef.current;
      if (!current || event.requestId !== current.requestId) return;
      if (
        event.kind === "start" &&
        event.conversation &&
        event.user &&
        event.assistant
      ) {
        setConversations((items) => [
          event.conversation!,
          ...items.filter((item) => item.id !== event.conversationId),
        ]);
        updateActive({
          ...current,
          conversationId: event.conversationId,
          user: event.user,
          assistant: event.assistant,
        });
        if (selectedRef.current === current.conversationId) {
          selectedRef.current = event.conversationId;
          setSelected(event.conversationId);
        }
      } else if (event.kind === "reset")
        updateActive({
          ...current,
          assistant: { ...current.assistant, content: "" },
        });
      else if (event.kind === "delta")
        updateActive({
          ...current,
          assistant: {
            ...current.assistant,
            content: current.assistant.content + event.text,
          },
          phase: "正在回答…",
        });
      else if (event.kind === "status")
        updateActive({ ...current, phase: event.text });
      else if (event.kind === "finish" && event.assistant)
        updateActive({ ...current, assistant: event.assistant, phase: "" });
    });
    return () => {
      mounted = false;
      unsubscribe();
    };
  }, [updateActive]);

  useEffect(() => {
    const sequence = ++loadSequence.current;
    if (!selected) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api
      .messages(selected)
      .then((items) => {
        if (sequence === loadSequence.current) setHistory(items);
      })
      .catch((error) => {
        if (sequence === loadSequence.current) setError(errorText(error));
      })
      .finally(() => {
        if (sequence === loadSequence.current) setLoading(false);
      });
  }, [selected]);
  useLayoutEffect(() => {
    if (follow.current && timeline.current)
      timeline.current.scrollTop = timeline.current.scrollHeight;
  }, [history, active, selected]);
  useEffect(() => {
    if (composer.current) {
      composer.current.style.height = "auto";
      composer.current.style.height =
        Math.min(composer.current.scrollHeight, 160) + "px";
    }
  }, [draft]);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(""), 1800);
    return () => clearTimeout(timer);
  }, [copied]);

  async function send(text = draft) {
    text = text.trim();
    if (!text || activeRef.current || text.length > 4000) return;
    if (settings && !settings.hasAPIKey) {
      setDraft(text);
      setShowSettings(true);
      return;
    }
    const requestId = crypto.randomUUID();
    const empty: Message = {
      id: "pending-" + requestId,
      conversationId: selectedRef.current,
      role: "assistant",
      content: "",
      state: "streaming",
      createdAt: new Date().toISOString(),
    };
    updateActive({
      requestId,
      conversationId: selectedRef.current,
      user: {
        ...empty,
        id: "user-" + requestId,
        role: "user",
        content: text,
        state: "complete",
      },
      assistant: empty,
      phase: "正在连接选课助手…",
    });
    setDraft("");
    setError("");
    setStopping(false);
    follow.current = true;
    try {
      const result = await api.chat(selectedRef.current, text, requestId);
      if (selectedRef.current === result.conversation.id) {
        ++loadSequence.current;
        const user = (activeRef.current as Active | null)?.user;
        setHistory((items) =>
          mergeMessages(
            items,
            user ? [user, result.message] : [result.message],
          ),
        );
        setLoading(false);
        if (result.error) setError(result.error);
      }
      setConversations((items) => [
        result.conversation,
        ...items.filter((item) => item.id !== result.conversation.id),
      ]);
    } catch (error) {
      setError(errorText(error));
      setDraft((value) => value || text);
    } finally {
      if ((activeRef.current as Active | null)?.requestId === requestId)
        updateActive(null);
      setStopping(false);
      composer.current?.focus();
    }
  }
  async function stop() {
    if (!activeRef.current || stopping) return;
    setStopping(true);
    try {
      await api.cancel(activeRef.current.requestId);
    } catch (error) {
      setError(errorText(error));
      setStopping(false);
    }
  }
  async function remove(id: string) {
    try {
      await api.remove(id);
      setConversations((items) => items.filter((item) => item.id !== id));
      if (selectedRef.current === id) select("");
    } catch (error) {
      setError(errorText(error));
    }
  }
  async function copy(message: Message) {
    try {
      await api.copy(message.content);
      setCopied(message.id);
    } catch {
      setError("暂时无法复制，请选中文字复制。");
    }
  }
  const live = active?.conversationId === selected ? active : null;
  const messages = live
    ? mergeMessages(history, [live.user, live.assistant])
    : history;
  const welcome = messages.length === 0 && !loading;
  const currentTitle = conversations.find(
    (item) => item.id === selected,
  )?.title;
  return (
    <div className="app-shell">
      {sidebarOpen && (
        <button
          className="sidebar-backdrop"
          aria-label="收起历史列表"
          onClick={() => setSidebarOpen(false)}
        />
      )}
      <aside
        className={`sidebar ${sidebarOpen ? "is-open" : ""}`}
        aria-label="对话历史"
      >
        <button
          className="brand"
          onClick={() => select("")}
          aria-label="HDU Station 首页"
        >
          <span className="brand-symbol">
            <Icon name="book" size={22} />
          </span>
          <span>
            HDU <b>Station</b>
            <small>你的杭电选课助手</small>
          </span>
        </button>
        <button
          className="new-chat"
          onClick={() => {
            select("");
            composer.current?.focus();
          }}
        >
          <Icon name="plus" size={18} />
          新对话<span>＋</span>
        </button>
        <div className="history-heading">
          最近的对话
          <span>{conversations.length > 0 ? conversations.length : ""}</span>
        </div>
        <nav className="history-list" aria-label="最近的对话">
          {conversations.length === 0 && (
            <p className="history-empty">
              你的选课想法，
              <br />
              会好好留在这里。
            </p>
          )}
          {conversations.map((item) => (
            <div
              key={item.id}
              className={`history-row ${selected === item.id ? "selected" : ""}`}
            >
              <button
                className="history-link"
                onClick={() => select(item.id)}
                aria-current={selected === item.id ? "page" : undefined}
              >
                <Icon name="chat" size={16} />
                <span>{item.title}</span>
                {active?.conversationId === item.id && (
                  <span className="history-pulse" />
                )}
              </button>
              <button
                className="delete-chat icon-button"
                aria-label={`删除对话：${item.title}`}
                title="删除对话"
                onClick={() => remove(item.id)}
                disabled={active?.conversationId === item.id}
              >
                <Icon name="trash" size={15} />
              </button>
            </div>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="campus-note">
            <span className="status-dot online" />
            <span>在杭电，把课选明白。</span>
          </div>
          <button
            className="settings-button"
            onClick={() => setShowSettings(true)}
          >
            <Icon name="settings" size={18} />
            助手设置
            <Icon name="right" size={15} />
          </button>
        </div>
      </aside>
      <main className="main-panel">
        <header className="topbar">
          <div className="topbar-left">
            <button
              className="icon-button menu-button"
              aria-label="打开历史列表"
              onClick={() => setSidebarOpen(true)}
            >
              <Icon name="menu" />
            </button>
            <span className="assistant-title">选课助手</span>
            <span className="title-divider" />
            <span className="topbar-caption">
              {currentTitle ?? "从一个问题开始"}
            </span>
          </div>
          <button
            className="model-badge"
            onClick={() => setShowSettings(true)}
            title="查看助手设置"
          >
            <span
              className={`status-dot ${settings?.hasAPIKey ? "online" : ""}`}
            />
            DeepSeek V4.1 Flash
          </button>
        </header>
        <div
          className={`timeline ${welcome ? "is-welcome" : ""}`}
          ref={timeline}
          onScroll={() => {
            const node = timeline.current;
            if (node) {
              follow.current =
                node.scrollHeight - node.clientHeight - node.scrollTop < 100;
              setJump(!follow.current);
            }
          }}
        >
          {welcome ? (
            <section className="welcome">
              <div className="welcome-label">
                <span className="welcome-emblem">
                  <Icon name="book" size={29} />
                </span>
                <span>HDU STATION · 选课助手</span>
              </div>
              <h1>
                选课这件事，
                <br />
                <span>先听听同学怎么说。</span>
              </h1>
              <p className="welcome-description">
                找轻松的通识课，了解考核和老师口碑。
                <br className="mobile-break" />
                把你的偏好告诉我，一起挑挑看。
              </p>
              <div className="suggestions">
                {suggestions.map((item) => (
                  <button
                    key={item.title}
                    className="suggestion"
                    onClick={() => send(item.question)}
                    disabled={!!active}
                  >
                    <span className="suggestion-icon">
                      <Icon name={item.icon} size={20} />
                    </span>
                    <strong>{item.title}</strong>
                    <span>{item.detail}</span>
                    <Icon name="right" size={17} />
                  </button>
                ))}
              </div>
              <p className="welcome-footnote">
                <span className="small-line" />
                建议来自同学讨论，附原帖供你参考。
              </p>
            </section>
          ) : (
            <div className="conversation-content" aria-label="对话内容">
              {loading && !live && (
                <div className="loading-history" role="status">
                  正在加载对话…
                </div>
              )}
              {messages.map((message) => (
                <article
                  key={message.id}
                  className={`message ${message.role}`}
                  aria-label={message.role === "user" ? "你的问题" : "助手回答"}
                >
                  {message.role === "user" ? (
                    <div className="user-bubble">{message.content}</div>
                  ) : (
                    <>
                      <div className="message-author">
                        <span className="assistant-avatar">
                          <Icon name="book" size={17} />
                        </span>
                        <strong>选课助手</strong>
                      </div>
                      {message.content && (
                        <Markdown text={message.content} onError={setError} />
                      )}
                      {message.state === "streaming" && (
                        <div className="thinking" role="status">
                          <span className="thinking-dots">
                            <i />
                            <i />
                            <i />
                          </span>
                          <span>{live?.phase ?? "正在回答…"}</span>
                        </div>
                      )}
                      {message.state === "cancelled" && (
                        <p className="message-notice">
                          已停止回答，可以继续提问。
                        </p>
                      )}
                      {message.state === "interrupted" && (
                        <p className="message-notice">
                          上次回答被中断，可以重新发送问题。
                        </p>
                      )}
                      {message.state === "error" && (
                        <p className="message-notice error-notice">
                          回答未完成
                        </p>
                      )}
                      {message.state !== "streaming" && (
                        <div className="message-actions">
                          {message.content && (
                            <button
                              className="text-button"
                              onClick={() => copy(message)}
                              aria-label="复制回答"
                            >
                              <Icon
                                name={copied === message.id ? "check" : "copy"}
                                size={14}
                              />
                              {copied === message.id ? "已复制" : "复制"}
                            </button>
                          )}
                          {["error", "interrupted", "cancelled"].includes(
                            message.state,
                          ) && (
                            <button
                              className="text-button"
                              disabled={!!active}
                              onClick={() => {
                                const index = messages.indexOf(message);
                                const question = messages
                                  .slice(0, index)
                                  .reverse()
                                  .find((item) => item.role === "user");
                                if (question) send(question.content);
                              }}
                            >
                              重新回答
                            </button>
                          )}
                        </div>
                      )}
                    </>
                  )}
                </article>
              ))}
            </div>
          )}
        </div>
        <div className="composer-area">
          {jump && (
            <button
              className="jump-button"
              onClick={() => {
                follow.current = true;
                if (timeline.current)
                  timeline.current.scrollTop = timeline.current.scrollHeight;
                setJump(false);
              }}
            >
              回到最新 ↓
            </button>
          )}
          {error && (
            <div className="error-banner" role="alert">
              <span>{error}</span>
              <button aria-label="关闭提示" onClick={() => setError("")}>
                <Icon name="close" size={15} />
              </button>
            </div>
          )}
          {active && !live && (
            <button
              className="other-turn"
              onClick={() => select(active.conversationId)}
            >
              另一条对话正在回答，点击查看 <Icon name="right" size={15} />
            </button>
          )}
          <form
            className={`composer ${active ? "is-busy" : ""}`}
            onSubmit={(event) => {
              event.preventDefault();
              send();
            }}
          >
            <textarea
              ref={composer}
              aria-label="选课问题"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder={
                welcome
                  ? "比如：通识选修有什么水课？"
                  : "继续问问课程、老师或考核…"
              }
              maxLength={4000}
              rows={1}
              onKeyDown={(event) => {
                if (
                  event.key === "Enter" &&
                  !event.shiftKey &&
                  !event.nativeEvent.isComposing &&
                  event.nativeEvent.keyCode !== 229
                ) {
                  event.preventDefault();
                  send();
                }
              }}
            />
            <div className="composer-footer">
              <span>
                <Icon name="chat" size={14} />
                参考同学的真实讨论
              </span>
              {active ? (
                <button
                  type="button"
                  className="send-button stop-button"
                  aria-label="停止回答"
                  onClick={stop}
                  disabled={stopping}
                >
                  <span className="stop-square" />
                </button>
              ) : (
                <button
                  className="send-button"
                  aria-label="发送问题"
                  disabled={!draft.trim()}
                >
                  <Icon name="arrow" size={19} />
                </button>
              )}
            </div>
          </form>
          <p className="composer-hint">
            <span>社区经验仅供参考，开课与选课信息请以教务系统为准。</span>
            <span>Enter 发送 · Shift + Enter 换行</span>
          </p>
        </div>
      </main>
      {showSettings && (
        <SettingsDialog
          settings={settings}
          onClose={() => setShowSettings(false)}
          onSaved={setSettings}
        />
      )}
    </div>
  );
}
