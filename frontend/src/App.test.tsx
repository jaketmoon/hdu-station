import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { api, type Message, type TurnEvent, type TurnResult } from "./api";

vi.mock("./api", () => ({
  api: {
    list: vi.fn(),
    settings: vi.fn(),
    messages: vi.fn(),
    chat: vi.fn(),
    cancel: vi.fn(),
    remove: vi.fn(),
    save: vi.fn(),
    install: vi.fn(),
    open: vi.fn(),
    copy: vi.fn(),
    subscribe: vi.fn(),
  },
  errorText: (error: unknown) => String(error),
}));
let event: (e: TurnEvent) => void;
const settings = {
  baseURL: "https://api.deepseek.com",
  model: "deepseek-flash",
  hasAPIKey: true,
  campus: { hasCredential: false, status: "logged_out" },
  qqStatus: "ready",
  qqEnabled: true,
  dataRoot: "/test",
  zanao: {
    enabled: false,
    schoolAlias: "",
    hasToken: false,
    status: "disabled",
  },
  xiaohongshu: {
    enabled: false,
    baseURL: "http://127.0.0.1:18060",
    hasAuthToken: false,
    status: "disabled",
  },
};
const message = (
  id: string,
  role: "user" | "assistant",
  content: string,
): Message => ({
  id,
  role,
  content,
  state: "complete",
  conversationId: "c1",
  createdAt: "2026-09-11",
});

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.list).mockResolvedValue([]);
  vi.mocked(api.settings).mockResolvedValue(settings);
  vi.mocked(api.messages).mockResolvedValue([]);
  vi.mocked(api.cancel).mockResolvedValue();
  vi.mocked(api.subscribe).mockImplementation((handler) => {
    event = handler;
    return () => {};
  });
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
});

describe("course assistant", () => {
  it("opens directly into the assistant with three usable prompts", async () => {
    render(<App />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "先听听同学怎么说",
    );
    expect(screen.getByRole("textbox", { name: "选课问题" })).toBeVisible();
    expect(screen.getByRole("button", { name: "发送问题" })).toBeDisabled();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
  });

  it("streams Markdown, stops the right request and preserves its final partial answer", async () => {
    let resolve!: (result: TurnResult) => void;
    vi.mocked(api.chat).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: /想选点轻松的/ }));
    await waitFor(() => expect(api.chat).toHaveBeenCalledTimes(1));
    const requestId = vi.mocked(api.chat).mock.calls[0][2];
    const user = message("u1", "user", "通识选修有什么水课？");
    const assistant = { ...message("a1", "assistant", ""), state: "streaming" };
    const conversation = {
      id: "c1",
      title: "通识选修有什么水课？",
      updatedAt: "",
    };
    act(() =>
      event({
        requestId,
        conversationId: "c1",
        kind: "start",
        text: "",
        conversation,
        user,
        assistant,
      }),
    );
    act(() =>
      event({
        requestId,
        conversationId: "c1",
        kind: "delta",
        text: "**戏曲鉴赏**可以了解一下。",
      }),
    );
    expect(screen.getByText("戏曲鉴赏").tagName).toBe("STRONG");
    fireEvent.click(screen.getByRole("button", { name: "停止回答" }));
    expect(api.cancel).toHaveBeenCalledWith(requestId);
    await act(async () =>
      resolve({
        conversation,
        message: {
          ...assistant,
          state: "cancelled",
          content: "**戏曲鉴赏**可以了解一下。",
        },
      }),
    );
    expect(screen.getByText("已停止回答，可以继续提问。")).toBeVisible();
    expect(screen.getAllByText("戏曲鉴赏")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "重新回答" })).toBeEnabled();
  });

  it("does not send while the Chinese IME is composing or Shift+Enter is used", async () => {
    render(<App />);
    const input = screen.getByRole("textbox", { name: "选课问题" });
    fireEvent.change(input, { target: { value: "通识选修" } });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229, isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(api.chat).not.toHaveBeenCalled();
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
  });

  it("does not mix an old stream into another conversation after switching", async () => {
    vi.mocked(api.list).mockResolvedValue([
      { id: "history", title: "之前的选课问题", updatedAt: "" },
    ]);
    vi.mocked(api.messages).mockImplementation(async (id) =>
      id === "history"
        ? [
            {
              ...message("saved", "assistant", "这是历史回答"),
              conversationId: "history",
            },
          ]
        : [],
    );
    let resolve!: (result: TurnResult) => void;
    vi.mocked(api.chat).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    render(<App />);
    await screen.findByRole("button", { name: "之前的选课问题" });
    fireEvent.click(screen.getByRole("button", { name: /想选点轻松的/ }));
    await waitFor(() => expect(api.chat).toHaveBeenCalled());
    const requestId = vi.mocked(api.chat).mock.calls[0][2];
    const conversation = { id: "c1", title: "新问题", updatedAt: "" };
    const assistant = message("a1", "assistant", "新问题的回答");
    act(() =>
      event({
        requestId,
        conversationId: "c1",
        kind: "start",
        text: "",
        conversation,
        user: message("u1", "user", "新问题"),
        assistant,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "之前的选课问题" }));
    await screen.findByText("这是历史回答");
    act(() =>
      event({
        requestId,
        conversationId: "c1",
        kind: "delta",
        text: "不能串到历史里",
      }),
    );
    await act(async () => resolve({ conversation, message: assistant }));
    expect(screen.getByText("这是历史回答")).toBeVisible();
    expect(screen.queryByText("不能串到历史里")).not.toBeInTheDocument();
  });

  it("retains the draft after a failed send and routes missing keys to settings", async () => {
    vi.mocked(api.settings).mockResolvedValue({
      ...settings,
      hasAPIKey: false,
    });
    render(<App />);
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
    fireEvent.change(screen.getByRole("textbox", { name: "选课问题" }), {
      target: { value: "人文经典推荐" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送问题" }));
    expect(screen.getByRole("dialog", { name: "助手设置" })).toBeVisible();
    expect(screen.getByRole("textbox", { name: "选课问题" })).toHaveValue(
      "人文经典推荐",
    );
    expect(api.chat).not.toHaveBeenCalled();
  });
});

function controlledChats() {
  const pending: {
    requestId: string;
    question: string;
    resolve: (value: TurnResult) => void;
    reject: (error: Error) => void;
  }[] = [];
  vi.mocked(api.chat).mockImplementation(
    (_id, question, requestId) =>
      new Promise((resolve, reject) =>
        pending.push({ requestId, question, resolve, reject }),
      ),
  );
  const start = (index: number) => {
    const turn = pending[index];
    const id = `conversation-${index}`;
    const conversation = { id, title: `问题 ${index}`, updatedAt: "" };
    const user = {
      ...message(`u-${index}`, "user", turn.question),
      conversationId: id,
    };
    const assistant = {
      ...message(`a-${index}`, "assistant", ""),
      conversationId: id,
      state: "streaming",
    };
    act(() =>
      event({
        requestId: turn.requestId,
        conversationId: id,
        kind: "start",
        text: "",
        conversation,
        user,
        assistant,
      }),
    );
    return { conversation, user, assistant };
  };
  const delta = (index: number, text: string, kind = "delta") =>
    act(() =>
      event({
        requestId: pending[index].requestId,
        conversationId: `conversation-${index}`,
        kind,
        text,
      }),
    );
  const send = (text: string) => {
    fireEvent.change(screen.getByRole("textbox", { name: "选课问题" }), {
      target: { value: text },
    });
    fireEvent.click(screen.getByRole("button", { name: "发送问题" }));
  };
  return { pending, start, delta, send };
}

describe("parallel conversations", () => {
  it("streams independently, stops only the selected request, and keeps other requests after completion", async () => {
    const chats = controlledChats();
    render(<App />);
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
    chats.send("第一条问题");
    const first = chats.start(0);
    chats.delta(0, "第一条已有结果");
    fireEvent.click(screen.getByRole("button", { name: /新对话/ }));
    chats.send("第二条问题");
    const second = chats.start(1);
    chats.delta(1, "第二条已有结果");
    chats.delta(0, "不能串到第二条");
    expect(screen.getByText("第二条已有结果")).toBeVisible();
    expect(screen.queryByText(/不能串到第二条/)).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("textbox", { name: "选课问题" }), {
      key: "Enter",
    });
    expect(api.chat).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByRole("button", { name: "停止回答" }));
    expect(api.cancel).toHaveBeenCalledWith(chats.pending[1].requestId);
    fireEvent.click(screen.getByRole("button", { name: "问题 0" }));
    expect(screen.getByRole("button", { name: "停止回答" })).toBeEnabled();
    expect(screen.getByText("第一条已有结果不能串到第二条")).toBeVisible();
    chats.delta(0, "", "reset");
    chats.delta(0, "第一条重新汇总");
    await act(async () =>
      chats.pending[1].resolve({
        conversation: second.conversation,
        message: {
          ...second.assistant,
          state: "cancelled",
          content: "第二条已有结果",
        },
      }),
    );
    expect(screen.getByText("第一条重新汇总")).toBeVisible();
    expect(screen.getByRole("button", { name: "停止回答" })).toBeEnabled();
    await act(async () =>
      chats.pending[0].resolve({
        conversation: first.conversation,
        message: {
          ...first.assistant,
          state: "complete",
          content: "第一条最终回答",
        },
      }),
    );
    chats.delta(0, "过期事件");
    expect(screen.getByText("第一条最终回答")).toBeVisible();
    expect(screen.queryByText("过期事件")).not.toBeInTheDocument();
  });

  it("allows ten active conversations and enables an eleventh after one finishes", async () => {
    const chats = controlledChats();
    render(<App />);
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
    let first!: ReturnType<typeof chats.start>;
    for (let index = 0; index < 10; index++) {
      chats.send(`并行问题 ${index}`);
      const started = chats.start(index);
      if (index === 0) first = started;
      fireEvent.click(screen.getByRole("button", { name: /新对话/ }));
    }
    expect(api.chat).toHaveBeenCalledTimes(10);
    fireEvent.change(screen.getByRole("textbox", { name: "选课问题" }), {
      target: { value: "第十一条" },
    });
    expect(screen.getByRole("button", { name: "发送问题" })).toBeDisabled();
    fireEvent.keyDown(screen.getByRole("textbox", { name: "选课问题" }), {
      key: "Enter",
    });
    expect(api.chat).toHaveBeenCalledTimes(10);
    await act(async () =>
      chats.pending[0].resolve({
        conversation: first.conversation,
        message: { ...first.assistant, state: "complete", content: "完成" },
      }),
    );
    expect(screen.getByRole("button", { name: "发送问题" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "发送问题" }));
    expect(api.chat).toHaveBeenCalledTimes(11);
    expect(api.chat).toHaveBeenLastCalledWith(
      "",
      "第十一条",
      expect.any(String),
    );
  });

  it("does not select a late start or restore a background failure into another draft", async () => {
    const chats = controlledChats();
    render(<App />);
    await waitFor(() => expect(api.settings).toHaveBeenCalled());
    chats.send("慢启动的第一条");
    fireEvent.click(screen.getByRole("button", { name: /新对话/ }));
    chats.send("第二条");
    chats.start(0);
    expect(screen.getByText("第二条")).toBeVisible();
    expect(screen.queryByText("慢启动的第一条")).not.toBeInTheDocument();
    chats.start(1);
    fireEvent.change(screen.getByRole("textbox", { name: "选课问题" }), {
      target: { value: "第二条的草稿" },
    });
    await act(async () => chats.pending[0].reject(new Error("第一条保存失败")));
    expect(screen.getByRole("textbox", { name: "选课问题" })).toHaveValue(
      "第二条的草稿",
    );
    expect(screen.queryByText("Error: 第一条保存失败")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "停止回答" })).toBeEnabled();
  });
});
