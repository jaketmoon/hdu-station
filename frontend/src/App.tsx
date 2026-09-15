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
  type Appearance,
  type ColorTheme,
} from "./api";
import { Icon, type IconName } from "./Icon";
import { TerminalGreeting, Dialogue, useReducedMotion } from "./Dialogue";
import { NightScene } from "./NightScene";
import { SettingsDialog } from "./SettingsDialog";

const suggestions: {
  title: string;
  detail: string;
  question: string;
  icon: IconName;
}[] = [
  {
    title: "想选点轻松的",
    detail: "搜集口碑，锁定低负担好课",
    question: "通识选修有什么水课？想找作业少、考核轻松的。",
    icon: "target",
  },
  {
    title: "努力也想拿高分",
    detail: "侦察给分、老师与考核方式",
    question: "有哪些给分比较高的通识选修？最好不用闭卷考试。",
    icon: "signal",
  },
  {
    title: "空闲时间塞门课",
    detail: "核对课表，寻找不撞课的班级",
    question:
      "结合我本学期的课表，推荐几门能放进空闲时间、作业比较少的通识选修，并核实开课和时间冲突。",
    icon: "clock",
  },
];
const maxConcurrentConversations = 10;
const themeOrder: ColorTheme[] = ["teal", "violet", "porcelain"];
const themeNames: Record<ColorTheme, string> = {
  teal: "青蓝琥珀",
  violet: "灰紫青绿",
  porcelain: "雾白青瓷",
};
type Active = {
  requestId: string;
  conversationId: string;
  user: Message;
  assistant: Message;
  phase: string;
  stopping?: boolean;
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
  const [active, setActive] = useState<Record<string, Active>>({});
  const activeRef = useRef<Record<string, Active>>({});
  const [settings, setSettings] = useState<Settings | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [navigation, setNavigation] = useState<string | null>(null);
  const navigationTarget = useRef<string | null>(null);
  const [copied, setCopied] = useState("");
  const [jump, setJump] = useState(false);
  const [savingAppearance, setSavingAppearance] = useState(false);
  const [appearanceError, setAppearanceError] = useState("");
  const [deleteCandidate, setDeleteCandidate] = useState("");
  const systemReduced = useReducedMotion();
  const appearance = settings?.appearance ?? {
    instantText: false,
  };
  const theme =
    appearance.theme === "violet" || appearance.theme === "porcelain"
      ? appearance.theme
      : "teal";
  const nextTheme =
    themeOrder[(themeOrder.indexOf(theme) + 1) % themeOrder.length];
  const themeName = themeNames[theme];
  const nextThemeName = themeNames[nextTheme];
  useLayoutEffect(() => {
    document.documentElement.dataset.theme = theme;
    return () => {
      delete document.documentElement.dataset.theme;
    };
  }, [theme]);
  const sidebar = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const timeline = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const follow = useRef(true);
  const loadSequence = useRef(0);
  const updateActive = useCallback((next: Active) => {
    const turns = { ...activeRef.current, [next.requestId]: next };
    activeRef.current = turns;
    setActive(turns);
  }, []);
  const removeActive = useCallback((requestId: string) => {
    const turns = { ...activeRef.current };
    delete turns[requestId];
    activeRef.current = turns;
    setActive(turns);
  }, []);
  const select = useCallback((id: string) => {
    const sequence = ++loadSequence.current;
    navigationTarget.current = null;
    setNavigation(null);
    setLoading(false);
    setSidebarOpen(false);
    setError("");
    setJump(false);
    setDeleteCandidate("");
    if (id === selectedRef.current) {
      if (!id) requestAnimationFrame(() => composer.current?.focus());
      return;
    }
    navigationTarget.current = id;
    setNavigation(id);
    const show = (items: Message[]) => {
      if (sequence !== loadSequence.current) return;
      // Keep the current transcript intact until the selected history is ready.
      selectedRef.current = id;
      navigationTarget.current = null;
      follow.current = true;
      setSelected(id);
      setHistory(items);
      setNavigation(null);
      setLoading(false);
      setError("");
      if (!id) requestAnimationFrame(() => composer.current?.focus());
    };
    if (!id || id.startsWith("pending-")) {
      show([]);
      return;
    }
    setLoading(true);
    api
      .messages(id)
      .then(show)
      .catch((error) => {
        if (sequence !== loadSequence.current) return;
        navigationTarget.current = null;
        setNavigation(null);
        setLoading(false);
        setError(errorText(error));
      });
  }, []);

  useEffect(
    () => () => {
      ++loadSequence.current;
    },
    [],
  );
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
      const current = activeRef.current[event.requestId];
      if (!current) return;
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
          phase: "正在传回情报…",
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

  const followDialogue = useCallback(() => {
    if (follow.current && timeline.current)
      timeline.current.scrollTop = timeline.current.scrollHeight;
  }, []);
  useEffect(() => {
    if (!sidebarOpen) return;
    const frame = requestAnimationFrame(() =>
      sidebar.current
        ?.querySelector<HTMLButtonElement>(".sidebar-close")
        ?.focus(),
    );
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setSidebarOpen(false);
        requestAnimationFrame(() => menuButton.current?.focus());
      }
      if (event.key === "Tab") {
        const items = Array.from(
          sidebar.current?.querySelectorAll<HTMLElement>(
            "button:not(:disabled), a[href]",
          ) ?? [],
        ).filter((item) => item.getClientRects().length);
        const first = items[0],
          last = items[items.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    }
    document.addEventListener("keydown", onKey);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener("keydown", onKey);
    };
  }, [sidebarOpen]);
  useEffect(() => {
    function shortcut(event: KeyboardEvent) {
      if (showSettings) return;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        select("");
        composer.current?.focus();
      } else if ((event.metaKey || event.ctrlKey) && event.key === ",") {
        event.preventDefault();
        setShowSettings(true);
      }
    }
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [select, showSettings]);
  async function saveAppearance(next: Appearance) {
    if (savingAppearance) return;
    setSavingAppearance(true);
    setError((current) => (current === appearanceError ? "" : current));
    setAppearanceError("");
    try {
      const saved = await api.appearance(next);
      setSettings((current) =>
        current ? { ...current, appearance: saved } : current,
      );
    } catch (error) {
      setAppearanceError(errorText(error));
      setError(errorText(error));
    } finally {
      setSavingAppearance(false);
    }
  }

  async function send(text = draft) {
    text = text.trim();
    const turns = Object.values(activeRef.current);
    if (
      !text ||
      navigationTarget.current !== null ||
      text.length > 4000 ||
      turns.some((turn) => turn.conversationId === selectedRef.current)
    )
      return;
    if (turns.length >= maxConcurrentConversations) {
      setError("最多同时运行 10 个对话，请等一个回答结束后再发送。");
      return;
    }
    if (settings && !settings.hasAPIKey) {
      setDraft(text);
      setShowSettings(true);
      return;
    }
    const requestId = crypto.randomUUID();
    const conversationId = selectedRef.current;
    // A new chat gets its own local identity before the backend start event.
    const displayId = conversationId || "pending-" + requestId;
    if (!conversationId) {
      selectedRef.current = displayId;
      setSelected(displayId);
    }
    const empty: Message = {
      id: "pending-" + requestId,
      conversationId: displayId,
      role: "assistant",
      content: "",
      state: "streaming",
      createdAt: new Date().toISOString(),
    };
    updateActive({
      requestId,
      conversationId: displayId,
      user: {
        ...empty,
        id: "user-" + requestId,
        role: "user",
        content: text,
        state: "complete",
      },
      assistant: empty,
      phase: "正在建立通讯，等待情报…",
    });
    setDraft("");
    setError("");
    follow.current = true;
    try {
      const result = await api.chat(conversationId, text, requestId);
      if (
        selectedRef.current === displayId ||
        selectedRef.current === result.conversation.id
      ) {
        if (selectedRef.current !== result.conversation.id) {
          selectedRef.current = result.conversation.id;
          setSelected(result.conversation.id);
        }
        const user = activeRef.current[requestId]?.user;
        setHistory((items) =>
          mergeMessages(
            items,
            user ? [user, result.message] : [result.message],
          ),
        );
        if (result.error) setError(result.error);
      }
      setConversations((items) => [
        result.conversation,
        ...items.filter((item) => item.id !== result.conversation.id),
      ]);
    } catch (error) {
      const current = activeRef.current[requestId];
      if (current?.conversationId === selectedRef.current) {
        setError(errorText(error));
        setDraft((value) => value || text);
        if (
          !conversationId &&
          current.conversationId === displayId &&
          navigationTarget.current === null
        ) {
          selectedRef.current = "";
          setSelected("");
        }
      }
    } finally {
      const wasSelected =
        activeRef.current[requestId]?.conversationId === selectedRef.current;
      removeActive(requestId);
      if (wasSelected) composer.current?.focus();
    }
  }
  async function stop() {
    const current = Object.values(activeRef.current).find(
      (turn) => turn.conversationId === selectedRef.current,
    );
    if (!current || current.stopping) return;
    updateActive({ ...current, stopping: true });
    try {
      await api.cancel(current.requestId);
    } catch (error) {
      const remaining = activeRef.current[current.requestId];
      if (remaining) updateActive({ ...remaining, stopping: false });
      if (selectedRef.current === current.conversationId)
        setError(errorText(error));
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
  const turns = Object.values(active);
  const live = turns.find((turn) => turn.conversationId === selected);
  const atCapacity = turns.length >= maxConcurrentConversations;
  const otherTurns = turns.filter((turn) => turn.conversationId !== selected);
  const messages = live
    ? mergeMessages(history, [live.user, live.assistant])
    : history;
  const welcome = selected === "";
  const currentTitle = conversations.find(
    (item) => item.id === selected,
  )?.title;
  return (
    <div className="app-shell">
      {sidebarOpen && (
        <button
          className="sidebar-backdrop"
          aria-label="收起历史列表"
          onClick={() => {
            setSidebarOpen(false);
            requestAnimationFrame(() => menuButton.current?.focus());
          }}
        />
      )}
      <aside
        ref={sidebar}
        className={`sidebar ${sidebarOpen ? "is-open" : ""}`}
        aria-label="对话历史"
      >
        <button
          className="icon-button sidebar-close"
          aria-label="关闭历史列表"
          onClick={() => {
            setSidebarOpen(false);
            requestAnimationFrame(() => menuButton.current?.focus());
          }}
        >
          <Icon name="close" />
        </button>
        <button
          className="brand"
          onClick={() => select("")}
          aria-label="HDU Station 首页"
        >
          <span className="brand-symbol">
            <Icon name="terminal" size={23} />
          </span>
          <span>
            HDU <b>STATION</b>
            <small>杭电 · 选课情报终端</small>
          </span>
        </button>
        <button
          className="new-chat"
          aria-label="新对话"
          onClick={() => {
            select("");
            composer.current?.focus();
          }}
        >
          <Icon name="plus" size={18} />
          新建通讯<span aria-hidden="true">⌘ K</span>
        </button>
        <div className="history-heading">
          通讯档案 <span>{String(conversations.length).padStart(2, "0")}</span>
        </div>
        <nav className="history-list" aria-label="最近的对话">
          {conversations.length === 0 && (
            <div className="history-empty">
              <Icon name="chat" size={32} />
              <p>还没有通讯记录</p>
              <span>每一条线索，都会留在这里。</span>
            </div>
          )}
          {conversations.map((item, index) => (
            <div
              key={item.id}
              className={`history-row ${selected === item.id ? "selected" : ""}`}
            >
              <button
                className="history-link"
                onClick={() => select(item.id)}
                aria-current={selected === item.id ? "page" : undefined}
                aria-busy={navigation === item.id || undefined}
              >
                <span className="history-index" aria-hidden="true">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <span>{item.title}</span>
                {turns.some((turn) => turn.conversationId === item.id) && (
                  <span className="history-pulse" />
                )}
              </button>
              <button
                className="delete-chat icon-button"
                aria-label={`删除对话：${item.title}`}
                title="删除对话"
                onClick={() => setDeleteCandidate(item.id)}
                disabled={turns.some((turn) => turn.conversationId === item.id)}
              >
                <Icon name="trash" size={15} />
              </button>
              {deleteCandidate === item.id && (
                <div className="delete-confirm">
                  <span>删除这条通讯？</span>
                  <button onClick={() => setDeleteCandidate("")}>保留</button>
                  <button
                    onClick={() => {
                      setDeleteCandidate("");
                      remove(item.id);
                    }}
                  >
                    删除
                  </button>
                </div>
              )}
            </div>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="campus-note">
            <span className="status-dot online" />
            <span>LOCAL ARCHIVE</span>
            <span>本机存储</span>
          </div>
          <button
            className="settings-button"
            onClick={() => {
              setSidebarOpen(false);
              setShowSettings(true);
            }}
          >
            <Icon name="settings" size={18} />
            助手设置
            <Icon name="right" size={15} />
          </button>
        </div>
      </aside>
      <main className="main-panel" inert={sidebarOpen}>
        <header className="topbar">
          <div className="topbar-left">
            <button
              className="icon-button menu-button"
              ref={menuButton}
              aria-label="打开历史列表"
              aria-expanded={sidebarOpen}
              onClick={() => setSidebarOpen(true)}
            >
              <Icon name="menu" />
            </button>
            <span className="channel-symbol" aria-hidden="true">
              <Icon name="signal" size={17} />
            </span>
            <span className="assistant-title">通讯频道</span>
            <span className="title-divider" />
            <span className="topbar-caption">
              {currentTitle ?? "等待你的下一条指令"}
            </span>
          </div>
          <div className="topbar-controls">
            <button
              className="model-badge"
              onClick={() => setShowSettings(true)}
              title="查看助手设置"
            >
              <span
                className={`status-dot ${settings?.hasAPIKey ? "online" : ""}`}
              />
              <span>DeepSeek V4.1 Flash</span>
            </button>
            <button
              className="theme-toggle"
              title={`当前：${themeName}；点击切换为${nextThemeName}`}
              aria-label={`切换为${nextThemeName}配色`}
              disabled={!settings || savingAppearance}
              onClick={() =>
                saveAppearance({ ...appearance, theme: nextTheme })
              }
            >
              <span className="theme-swatch" aria-hidden="true">
                <i />
                <i />
              </span>
              <span className="theme-name">{themeName}</span>
            </button>
          </div>
        </header>
        <div className="channel-viewport">
          {loading && (
            <div className="loading-history" role="status">
              <Icon name="chat" size={14} /> 正在读取通讯…
            </div>
          )}
          <div
            className={`timeline ${welcome ? "is-welcome" : ""}`}
            ref={timeline}
            aria-busy={loading || undefined}
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
                <div className="hero-scene">
                  <NightScene />
                  <div className="hero-copy">
                    <TerminalGreeting
                      reduced={systemReduced || appearance.instantText}
                    />
                  </div>
                  <div className="hero-corners" aria-hidden="true" />
                </div>
                <div className="suggestions">
                  {suggestions.map((item, index) => (
                    <button
                      key={item.title}
                      className="suggestion"
                      onClick={() => send(item.question)}
                      disabled={!!live || atCapacity || navigation !== null}
                    >
                      <span className="suggestion-top">
                        <span className="suggestion-icon">
                          <Icon name={item.icon} size={20} />
                        </span>
                        <span className="mission-number">0{index + 1}</span>
                      </span>
                      <strong>{item.title}</strong>
                      <span className="suggestion-detail">{item.detail}</span>
                      <Icon name="right" size={17} />
                    </button>
                  ))}
                </div>
              </section>
            ) : (
              <div className="conversation-content" aria-label="对话内容">
                <div className="transcript-heading">
                  <span>TRANSMISSION LOG</span>
                  <span>
                    通讯记录 /{" "}
                    {String(
                      messages.filter((message) => message.role === "user")
                        .length,
                    ).padStart(2, "0")}
                  </span>
                </div>
                {messages.map((message) => (
                  <article
                    key={message.id}
                    className={`message ${message.role}`}
                    aria-label={
                      message.role === "user" ? "你的问题" : "助手回答"
                    }
                  >
                    {message.role === "user" ? (
                      <div className="user-bubble">
                        <div className="user-label">
                          AGENT <span>发出的指令</span>
                        </div>
                        {message.content}
                      </div>
                    ) : (
                      <>
                        <div className="message-author">
                          <span className="assistant-avatar">
                            <Icon name="terminal" size={17} />
                          </span>
                          <strong>STATION</strong>
                          <span className="author-role">选课情报员</span>
                          <span className="message-state">
                            {message.state === "streaming"
                              ? "● LIVE"
                              : message.state === "complete"
                                ? "已接收 / RECEIVED"
                                : "通讯已结束"}
                          </span>
                        </div>
                        <Dialogue
                          text={message.content}
                          enabled={
                            !appearance.instantText &&
                            !systemReduced &&
                            !["cancelled", "interrupted", "error"].includes(
                              message.state,
                            )
                          }
                          streaming={
                            message.state === "streaming" && !!message.content
                          }
                          onError={setError}
                          onProgress={followDialogue}
                        />
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
                                  name={
                                    copied === message.id ? "check" : "copy"
                                  }
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
                                disabled={!!live || atCapacity}
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
          {otherTurns.length > 0 && (
            <button
              className="other-turn"
              onClick={() => select(otherTurns[0].conversationId)}
            >
              另有 {otherTurns.length} 个对话正在回答（最多 10 个），点击查看{" "}
              <Icon name="right" size={15} />
            </button>
          )}
          <form
            className={`composer ${live ? "is-busy" : ""}`}
            onSubmit={(event) => {
              event.preventDefault();
              send();
            }}
          >
            <div className="command-heading">
              <label htmlFor="command-input">[ COMMAND ]</label>
              {live && (
                <span className="command-indicator">
                  RECEIVING
                  <i />
                </span>
              )}
            </div>
            <div className="command-input-row">
              <span className="command-prompt" aria-hidden="true">
                &gt;
              </span>
              <textarea
                id="command-input"
                ref={composer}
                aria-label="选课问题"
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                placeholder={
                  welcome
                    ? "帮我找周五没早八的专业选修…"
                    : "继续追问，或者补充你的条件…"
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
            </div>
            <div className="composer-footer">
              <span>
                {draft.length > 3200 && (
                  <span className="character-count">{draft.length} / 4000</span>
                )}
              </span>
              {live ? (
                <button
                  type="button"
                  className="send-button stop-button"
                  aria-label="停止回答"
                  onClick={stop}
                  disabled={live.stopping}
                >
                  <span className="stop-square" />
                  <span>{live.stopping ? "正在停止" : "中止通讯"}</span>
                </button>
              ) : (
                <button
                  className="send-button"
                  aria-label="发送问题"
                  disabled={!draft.trim() || atCapacity || navigation !== null}
                >
                  <span>发送指令</span>
                  <Icon name="right" size={17} />
                </button>
              )}
            </div>
          </form>
          <p className="composer-hint">
            <span>社区经验仅供参考，开课与选课信息请以教务系统为准。</span>
          </p>
        </div>
        <footer className="terminal-footer">
          <span>
            <i /> {live ? "RECEIVING TRANSMISSION" : "AWAITING COMMAND"}
          </span>
        </footer>
      </main>
      {showSettings && (
        <SettingsDialog
          settings={settings}
          onClose={() => setShowSettings(false)}
          onSaved={(next) =>
            setSettings((current) => ({
              ...next,
              appearance: current?.appearance ?? next.appearance,
            }))
          }
          appearance={appearance}
          onAppearanceChange={saveAppearance}
          appearanceBusy={savingAppearance}
          systemReduced={systemReduced}
          appearanceError={appearanceError}
        />
      )}
    </div>
  );
}
