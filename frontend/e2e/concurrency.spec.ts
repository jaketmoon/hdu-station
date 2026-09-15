import { test, expect } from "@playwright/test";

test("independent conversations keep streaming and stop only the visible one", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const listeners: ((event: any) => void)[] = [];
    const conversations: any[] = [];
    const histories = new Map<string, any[]>();
    const cancels = new Map<string, () => void>();
    (window as any).runtime = {
      EventsOn: (_: string, handler: (event: any) => void) => {
        listeners.push(handler);
        return () => {};
      },
    };
    (window as any).go = {
      main: {
        App: {
          GetSettings: async () => ({ hasAPIKey: true }),
          ListConversations: async () => conversations,
          GetMessages: async (id: string) => histories.get(id) || [],
          Cancel: async (id: string) => cancels.get(id)?.(),
          Chat: async (_id: string, question: string, requestId: string) => {
            const id = `conversation-${conversations.length + 1}`;
            const conversation = { id, title: question, updatedAt: "" };
            const user = {
              id: `user-${id}`,
              conversationId: id,
              role: "user",
              content: question,
              state: "complete",
              createdAt: "",
            };
            const assistant = {
              id: `assistant-${id}`,
              conversationId: id,
              role: "assistant",
              content: "",
              state: "streaming",
              createdAt: "",
            };
            conversations.push(conversation);
            histories.set(id, [user, assistant]);
            const emit = (event: any) =>
              listeners.forEach((handler) =>
                handler({ requestId, conversationId: id, text: "", ...event }),
              );
            const stopped = new Promise<void>((resolve) =>
              cancels.set(requestId, resolve),
            );
            emit({
              kind: "start",
              conversation,
              user,
              assistant: { ...assistant },
            });
            assistant.content = `${question}的独立进度`;
            emit({ kind: "delta", text: assistant.content });
            await stopped;
            assistant.state = "cancelled";
            emit({ kind: "finish", assistant });
            cancels.delete(requestId);
            return { conversation, message: assistant };
          },
        },
      },
    };
  });
  await page.goto("/");
  const input = page.getByRole("textbox", { name: "选课问题" });
  const newChat = async () => {
    if ((page.viewportSize()?.width || 0) < 600)
      await page.getByRole("button", { name: "打开历史列表" }).click();
    await page.getByRole("button", { name: /新对话/ }).click();
  };
  await input.fill("发现好课");
  await page.getByRole("button", { name: "发送问题" }).click();
  await expect(page.getByText("发现好课的独立进度")).toBeVisible();
  await newChat();
  await input.fill("查看课表");
  await page.getByRole("button", { name: "发送问题" }).click();
  await expect(page.getByText("查看课表的独立进度")).toBeVisible();
  await expect(page.getByText("发现好课的独立进度")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /另有 1 个对话正在回答/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "停止回答" }).click();
  await expect(page.getByText("已停止回答，可以继续提问。")).toBeVisible();
  await page.getByRole("button", { name: /另有 1 个对话正在回答/ }).click();
  await expect(page.getByText("发现好课的独立进度")).toBeVisible();
  await expect(page.getByRole("button", { name: "停止回答" })).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "删除对话：发现好课" }),
  ).toBeDisabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "停止回答" }).click();
  await expect(page.getByText("已停止回答，可以继续提问。")).toBeVisible();
});
