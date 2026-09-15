import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

const loginQR =
  "data:image/png;base64," +
  readFileSync(new URL("./fixtures/login-qr.png", import.meta.url)).toString(
    "base64",
  );

test.beforeEach(async ({ page }) => {
  // Test-only Wails boundary. Production code contains no demo responses or HTTP backend.
  await page.addInitScript((loginQR) => {
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
      campus: {
        hasCredential: false,
        status: "logged_out",
        scheduleAccess: false,
      },
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
          SaveSettings: async (input: any) => {
            Object.assign(settings, {
              baseURL: input.baseURL,
              hasAPIKey: !!input.apiKey || settings.hasAPIKey,
            });
            Object.assign(settings.zanao, {
              enabled: input.zanao.enabled,
              schoolAlias: input.zanao.schoolAlias,
              hasToken:
                !!input.zanao.token ||
                (!input.zanao.clearToken && settings.zanao.hasToken),
              status: input.zanao.enabled ? "ready" : "disabled",
            });
            return {
              ...settings,
              zanao: { ...settings.zanao },
              xiaohongshu: { ...settings.xiaohongshu },
            };
          },
          CheckCampus: async () => ({ ...settings.campus }),
          BeginCampusLogin: async () => ({
            id: "campus-login",
            status: "waiting",
            userCode: "ABCD-EFGH",
            expiresAt: Date.now() + 600000,
          }),
          PollCampusLogin: async () => {
            await new Promise((resolve) => setTimeout(resolve, 2000));
            settings.campus = {
              hasCredential: true,
              status: "saved",
              scheduleAccess: true,
            };
            return {
              id: "campus-login",
              status: "ready",
              expiresAt: Date.now() + 600000,
            };
          },
          OpenCampusLogin: async () => {},
          CancelCampusLogin: async () => ({
            id: "campus-login",
            status: "cancelled",
            expiresAt: 0,
          }),
          LogoutCampus: async () => {
            settings.campus = {
              hasCredential: false,
              status: "logged_out",
              scheduleAccess: false,
            };
            return { ...settings.campus };
          },
          CheckSource: async (source: string) =>
            source === "qq"
              ? { enabled: settings.qqEnabled, status: settings.qqStatus }
              : {
                  enabled: settings.xiaohongshu.enabled,
                  status: settings.xiaohongshu.status,
                },
          SetSourceEnabled: async (source: string, enabled: boolean) => {
            if (source === "qq") {
              settings.qqEnabled = enabled;
              settings.qqStatus = enabled ? "ready" : "disabled";
              return { enabled, status: settings.qqStatus };
            }
            settings.xiaohongshu.enabled = enabled;
            settings.xiaohongshu.status = enabled ? "logged_out" : "disabled";
            return { enabled, status: settings.xiaohongshu.status };
          },
          BeginSourceLogin: async (source: string) => ({
            id: source + "-login",
            status: "waiting",
            // Valid test-only PNG; the production image comes from the login provider.
            image: loginQR,
            expiresAt: Date.now() + 240000,
          }),
          PollSourceLogin: async (id: string) => {
            const scanned = (window as any).__loginScanned;
            const source = id.startsWith("qq") ? "qq" : "xiaohongshu";
            if (scanned) {
              if (source === "qq") settings.qqStatus = "ready";
              else settings.xiaohongshu.status = "ready";
            }
            return { id, status: scanned ? "ready" : "waiting" };
          },
          CancelSourceLogin: async () => {},
          ClearSourceCredentials: async (source: string) => {
            if (source === "qq") {
              settings.qqStatus = settings.qqEnabled
                ? "logged_out"
                : "disabled";
              return { enabled: settings.qqEnabled, status: settings.qqStatus };
            }
            settings.xiaohongshu.status = settings.xiaohongshu.enabled
              ? "logged_out"
              : "disabled";
            return {
              enabled: settings.xiaohongshu.enabled,
              status: settings.xiaohongshu.status,
            };
          },
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
              id: `user-${state.messages.length + 1}`,
              conversationId: state.id,
              role: "user",
              content: question,
              state: "complete",
              createdAt: "",
            };
            const assistant = {
              id: `assistant-${state.messages.length + 1}`,
              conversationId: state.id,
              role: "assistant",
              content: "",
              state: "streaming",
              createdAt: "",
            };
            state.conversation = conversation;
            state.messages.push(user, assistant);
            emit({ kind: "start", conversation, user, assistant });
            emit({ kind: "status", text: "正在阅读同学讨论…" });
            if (question === "停止回答测试") {
              // Keep the fixture in flight until Cancel, independent of how
              // long the narrow layout takes to settle before Playwright clicks.
              const deadline = Date.now() + 10000;
              while (!state.stop && Date.now() < deadline)
                await new Promise((resolve) => setTimeout(resolve, 20));
            } else await new Promise((resolve) => setTimeout(resolve, 400));
            let answer =
              "如果你更在意**作业少、考核轻松**，可以先看看下面这几门。\n\n### 可以优先了解\n\n| 课程 | 同学提到的体验 | 参考 |\n| --- | --- | --- |\n| 戏曲鉴赏 | 有同学提到期末以鉴赏作业为主，平时签到不多。不同学期要求可能有变化。 | [原帖](https://pd.qq.com/s/example1) |\n| 中国传统美学导论 | 有同学认可课堂氛围和给分，仍需要认真完成期末作业。 | [原帖](https://pd.qq.com/s/example2) |\n\n**选课前再确认两件事：**授课老师是否相同，以及本学期的考核安排。社区里的“水”是个人感受，不能保证每个人都拿高分。\n\n你更想要不用考试的，还是不用做小组作业的？";
            if (
              question.includes("空闲时间") ||
              question.includes("本学期开课")
            )
              answer =
                "已核实教务默认学期：2026–2027 学年第1学期。\n\n| 课程（课程号） | 老师 | 上课时间 | 课表筛选 |\n| --- | --- | --- | --- |\n| 影视音乐赏析（001） | 测试老师 | 星期二第10–11节，第1–17周 | 可放入空闲位置 |\n| 戏曲鉴赏（002） | 测试老师 | 星期三第6–7节，第1–17周 | 与已选课程冲突 |\n\n网上的“影视音乐鉴赏”可能对应“影视音乐赏析”（001）。建议先考虑影视音乐赏析周二10–11节的班；是否有余量和选课资格仍需以教务系统为准。";
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
  }, loginQR);
});

test("campus course and schedule authorization fits both windows", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByTitle("查看助手设置").click();
  const dialog = page.getByRole("dialog");
  const campus = dialog.locator(".campus-settings");
  await campus.locator("summary").click();
  await expect(campus.locator("summary")).toContainText("HDU CLI 登录");
  await expect(campus.locator("summary")).toContainText("尚未登录");
  await campus.getByRole("button", { name: "网页授权" }).click();
  await expect(campus.getByText("ABCD-EFGH")).toBeVisible();
  await campus
    .getByRole("button", { name: "再次打开授权页" })
    .scrollIntoViewIfNeeded();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-campus-authorizing.png`,
  });
  await expect(campus.locator("summary")).toContainText("已登录");
  await expect(campus).toContainText(
    "可核实本学期开课；读取本人课表后，可筛选不撞课的班级。",
  );
  await expect(campus).not.toContainText("查询课程类别");
  await campus.scrollIntoViewIfNeeded();
  expect(
    await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
  ).toBeTruthy();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-campus-settings.png`,
  });
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: /想选点轻松的/ }).click();
  await expect(page.getByRole("button", { name: "复制回答" })).toBeVisible();
  await page.getByTitle("查看助手设置").click();
  await campus.locator("summary").click();
  await expect(campus.getByRole("button", { name: "重新授权" })).toBeVisible();
  await campus.getByRole("button", { name: "退出登录" }).click();
  await expect(campus.locator("summary")).toContainText("尚未登录");
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
  await page.getByRole("textbox", { name: "选课问题" }).fill("停止回答测试");
  await page.getByRole("button", { name: "发送问题" }).click();
  await page.getByRole("button", { name: "停止回答", exact: true }).click();
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

test("optional sources can be configured and checked in both window sizes", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByTitle("查看助手设置").click();
  const dialog = page.getByRole("dialog");
  await dialog.locator("summary").filter({ hasText: "赞哦校园集市" }).click();
  await page.getByRole("checkbox", { name: "启用赞哦搜索" }).check();
  await page.getByLabel("学校别名", { exact: true }).fill("test-campus");
  await page.locator("#zanao-token").fill("test-only-token");
  await expect(
    dialog.locator("summary").filter({ hasText: "赞哦校园集市" }),
  ).toContainText("保存后检查");
  await page.locator("#zanao-school").scrollIntoViewIfNeeded();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-zanao-settings.png`,
  });
  const xhs = dialog.getByRole("region", { name: "小红书来源" });
  await xhs.locator(".source-disclosure").click();
  await xhs.getByRole("switch", { name: "启用小红书搜索" }).click();
  await page.getByRole("button", { name: "保存并检查" }).click();
  await expect(page.getByText("设置已保存，连接状态已更新。")).toBeVisible();
  await expect(
    dialog.locator("summary").filter({ hasText: "赞哦校园集市" }),
  ).toContainText("已连接");
  await expect(xhs.locator(".source-status")).toContainText("需要重新登录");
  await expect(page.locator("#zanao-token")).toHaveValue("");
  await expect(page.getByLabel("小红书本机服务地址")).toHaveCount(0);
  expect(
    await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBeTruthy();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page
    .getByRole("button", { name: "保存并检查" })
    .scrollIntoViewIfNeeded();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-xiaohongshu-settings.png`,
  });
  await page.keyboard.press("Escape");
  await page.getByTitle("查看助手设置").click();
  await expect(
    page
      .getByRole("dialog")
      .locator("summary")
      .filter({ hasText: "赞哦校园集市" }),
  ).toContainText("已连接");
});

for (const source of ["QQ 频道", "小红书"]) {
  test(`${source}: compact card, QR login, credential clearing and saved toggle`, async ({
    page,
  }, testInfo) => {
    await page.goto("/");
    await page.getByTitle("查看助手设置").click();
    const card = page.getByRole("region", { name: `${source}来源` });
    await expect(
      card.getByRole("button", { name: `重新检查${source}` }),
    ).toBeVisible();
    await expect(card.getByRole("switch")).not.toBeVisible();
    await card.locator(".source-disclosure").click();
    const toggle = card.getByRole("switch");
    if ((await toggle.getAttribute("aria-checked")) === "false")
      await toggle.click();
    await card.getByRole("button", { name: "重新连接", exact: true }).click();
    const qr = card.getByRole("img", { name: `${source}登录二维码` });
    await expect(qr).toBeVisible();
    await expect(qr).toBeInViewport({ ratio: 1 });
    await expect(
      card.getByText("在手机上确认登录，凭证会自动保存。"),
    ).toBeInViewport();
    expect(
      await qr.evaluate(
        (img: HTMLImageElement) => img.complete && img.naturalWidth > 0,
      ),
    ).toBeTruthy();
    expect(
      await page
        .getByRole("dialog")
        .evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBeTruthy();
    await page.screenshot({
      path: `test-results/${testInfo.project.name}-${source === "QQ 频道" ? "qq" : "xhs"}-login.png`,
    });
    await page.evaluate(() => {
      (window as any).__loginScanned = true;
    });
    await expect(card.getByRole("status")).toContainText("登录成功", {
      timeout: 6000,
    });
    await expect(qr).not.toBeVisible();
    await expect(card.locator(".source-status")).toHaveText("已连接");
    await card.getByRole("button", { name: "清除登录凭证" }).click();
    await expect(card.getByRole("status")).toContainText("登录凭证已清除");
    await expect(card.locator(".source-status")).toHaveText("需要重新登录");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await page.keyboard.press("Escape");
    await page.getByTitle("查看助手设置").click();
    await expect(card.locator(".source-status")).toHaveText("未启用");
    await expect(card.getByRole("switch")).not.toBeVisible();
  });
}

test("course fit recommendation and course IDs remain readable", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByRole("button", { name: /空闲时间塞门课/ }).click();
  await expect(page.getByRole("button", { name: "复制回答" })).toBeVisible();
  await expect(page.getByRole("table")).toContainText("影视音乐赏析（001）");
  await expect(page.getByRole("table")).toContainText("与已选课程冲突");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: `test-results/${testInfo.project.name}-course-fit.png`,
  });
});
