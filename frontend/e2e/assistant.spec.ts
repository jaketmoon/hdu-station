import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  // Test-only Wails boundary. Production code contains no demo responses or HTTP backend.
  await page.addInitScript(() => {
    const state = {
      id: "",
      messages: [] as any[],
      listeners: [] as ((event: any) => void)[],
      stop: false,
      conversation: null as any,
    };
    const settings = {
      baseURL: "https://api.deepseek.com",
      model: "deepseek-flash",
      hasAPIKey: true,
      qqStatus: "ready",
      dataRoot: "/test",
    };
    (window as any).runtime = {
      EventsOn: (_: string, callback: (event: any) => void) => {
        state.listeners.push(callback);
        return () => {
          state.listeners = state.listeners.filter((item) => item !== callback);
        };
      },
    };
    (window as any).go = {
      main: {
        App: {
          ListConversations: async () =>
            state.conversation ? [state.conversation] : [],
          GetMessages: async () => state.messages,
          GetSettings: async () => settings,
          SaveSettings: async () => settings,
          InstallQQ: async () => settings,
          OpenLink: async () => {},
          CopyText: async () => {},
          DeleteConversation: async () => {
            state.conversation = null;
            state.messages = [];
          },
          Cancel: async () => {
            state.stop = true;
          },
          Chat: async (_: string, question: string, requestId: string) => {
            state.stop = false;
            state.id = "conversation-1";
            const emit = (event: any) =>
              state.listeners.forEach((callback) =>
                callback({ requestId, conversationId: state.id, ...event }),
              );
            const conversation = {
              id: state.id,
              title: question,
              updatedAt: new Date().toISOString(),
            };
            const user = {
              id: "user-1",
              conversationId: state.id,
              role: "user",
              content: question,
              state: "complete",
              createdAt: "",
            };
            const assistant = {
              id: "assistant-1",
              conversationId: state.id,
              role: "assistant",
              content: "",
              state: "streaming",
              createdAt: "",
            };
            state.conversation = conversation;
            state.messages = [user, assistant];
            emit({ kind: "start", conversation, user, assistant });
            emit({ kind: "status", text: "正在阅读同学讨论…" });
            await new Promise((resolve) => setTimeout(resolve, 400));
            let answer =
              "如果你更在意**作业少、考核轻松**，可以先看看下面这几门。\n\n### 可以优先了解\n\n| 课程 | 同学提到的体验 | 参考 |\n| --- | --- | --- |\n| 戏曲鉴赏 | 有同学提到期末以鉴赏作业为主，平时签到不多。不同学期要求可能有变化。 | [原帖](https://pd.qq.com/s/example1) |\n| 中国传统美学导论 | 有同学认可课堂氛围和给分，仍需要认真完成期末作业。 | [原帖](https://pd.qq.com/s/example2) |\n\n**选课前再确认两件事：**授课老师是否相同，以及本学期的考核安排。社区里的“水”是个人感受，不能保证每个人都拿高分。\n\n你更想要不用考试的，还是不用做小组作业的？";
            if (question === "长回答滚动测试")
              answer = Array(5).fill(answer).join("\n\n");
            for (
              let index = 0;
              index < answer.length && !state.stop;
              index += 25
            ) {
              const text = answer.slice(index, index + 25);
              assistant.content += text;
              emit({ kind: "delta", text });
              await new Promise((resolve) => setTimeout(resolve, 15));
            }
            assistant.state = state.stop ? "cancelled" : "complete";
            emit({ kind: "finish", assistant });
            return { conversation, message: assistant };
          },
        },
      },
    };
  });
});

test("welcome, real controls and readable response fit the window", async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toContainText(
    "先听听同学怎么说",
  );
  await expect(
    page.getByRole("textbox", { name: "选课问题" }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-welcome.png`,
  });
  await page.getByRole("button", { name: /想选点轻松的/ }).click();
  await expect(page.getByRole("table")).toBeVisible();
  await expect(page.getByRole("button", { name: "复制回答" })).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "选课问题" }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-answer.png`,
  });
  await page
    .getByRole("button", { name: "查看助手设置" })
    .count()
    .catch(() => 0);
  await page.getByTitle("查看助手设置").click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-settings.png`,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(errors).toEqual([]);
});

test("stop completes promptly, and history is usable on narrow screens", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page
    .getByRole("textbox", { name: "选课问题" })
    .fill("给分高的通识选修有哪些？");
  await page.getByRole("button", { name: "发送问题" }).click();
  await page.getByRole("button", { name: "停止回答" }).click();
  await expect(page.getByText("已停止回答，可以继续提问。")).toBeVisible();
  if (testInfo.project.name === "narrow")
    await page.getByRole("button", { name: "打开历史列表" }).click();
  await expect(page.getByRole("button", { name: "新对话" })).toBeVisible();
  await page.getByRole("button", { name: "新对话" }).click();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
});

test("long streamed answers follow the latest text without moving the composer", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("textbox", { name: "选课问题" }).fill("长回答滚动测试");
  await page.getByRole("button", { name: "发送问题" }).click();
  await expect(page.getByRole("button", { name: "复制回答" })).toBeVisible({
    timeout: 15000,
  });
  await expect(page.getByRole("button", { name: "复制回答" })).toBeInViewport();
  await expect(
    page.getByRole("textbox", { name: "选课问题" }),
  ).toBeInViewport();
  const remaining = await page
    .locator(".timeline")
    .evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop);
  expect(remaining).toBeLessThan(5);
});
