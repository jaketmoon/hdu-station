import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const settings = {
      appearance: JSON.parse(
        localStorage.getItem("test-appearance") || '{"instantText":false}',
      ),
      hasAPIKey: true,
      baseURL: "https://api.deepseek.com",
      model: "deepseek-flash",
      campus: { hasCredential: false, status: "logged_out" },
      qqStatus: "ready",
      qqEnabled: true,
      zanao: {
        enabled: false,
        status: "disabled",
        schoolAlias: "",
        hasToken: false,
      },
      xiaohongshu: {
        enabled: false,
        status: "disabled",
        baseURL: "http://127.0.0.1:18060",
        hasAuthToken: false,
      },
    };
    (window as any).go = {
      main: {
        App: {
          GetSettings: async () => settings,
          ListConversations: async () =>
            ["甲", "乙", "丙"].map((id) => ({
              id,
              title: `通讯${id}`,
              updatedAt: "",
            })),
          GetMessages: async (id: string) => [
            {
              id,
              conversationId: id,
              role: "assistant",
              content: `${id}的课程情报`,
              state: "complete",
              createdAt: "",
            },
          ],
          SaveAppearance: async (appearance: any) => {
            settings.appearance = appearance;
            localStorage.setItem("test-appearance", JSON.stringify(appearance));
            return appearance;
          },
        },
      },
    };
  });
});

test("typing preferences persist without VHS controls and respect reduced motion", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".greeting-glyph.is-revealed")).toHaveCount(13);
  await expect(page.locator(".terminal-greeting")).toHaveCSS(
    "font-family",
    '"Station Pixel", sans-serif',
  );
  expect(
    await page.evaluate(async () => {
      await document.fonts.ready;
      return document.fonts.check(
        '36px "Station Pixel"',
        "情报台在线，等待你的指令。",
      );
    }),
  ).toBe(true);
  await expect(page.locator("body")).not.toContainText(
    /ZERO|收到，特工|发现口碑 ·|行动由你决定|HANGZHOU|NIGHT FREQUENCY|选择一项行动|QUICK OPERATIONS|INPUT READY|输入你的选课指令|换行|✦/i,
  );
  await expect(page.locator(".night-scene text")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "录像带特效", exact: true }),
  ).toHaveCount(0);
  await expect(page.locator(".crt-overlay, .signal-transition")).toHaveCount(0);
  await expect(page.locator(".night-scene")).toHaveCSS("filter", "none");
  await page.getByTitle("查看助手设置").click();
  await page
    .getByRole("dialog")
    .locator("summary")
    .filter({ hasText: "显示与动效" })
    .click();
  const typing = page.getByRole("switch", { name: "剧情式逐字对话" });
  await expect(
    page.getByRole("switch", { name: "轻微录像带特效" }),
  ).toHaveCount(0);
  await expect(typing).toHaveAttribute("aria-checked", "true");
  await typing.click();
  await expect(typing).toHaveAttribute("aria-checked", "false");
  await page.keyboard.press("Escape");
  await page.reload();
  await expect(page.locator(".greeting-glyph.is-revealed")).toHaveCount(13);
  await expect(
    page.getByRole("heading", { name: "情报台在线，等待你的指令。" }),
  ).toHaveClass(/instant/);
  await page.getByTitle("查看助手设置").click();
  await page
    .getByRole("dialog")
    .locator("summary")
    .filter({ hasText: "显示与动效" })
    .click();
  await typing.click();
  await expect(typing).toHaveAttribute("aria-checked", "true");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(typing).toBeDisabled();
  await expect(typing).toHaveAttribute("aria-checked", "false");
});

test("slow and out-of-order conversation loads keep the previous frame without flashing home", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  const select = async (name: string) => {
    if (testInfo.project.name === "narrow")
      await page.getByRole("button", { name: "打开历史列表" }).click();
    await page.getByRole("button", { name, exact: true }).click();
  };
  await select("通讯甲");
  await expect(page.getByText("甲的课程情报")).toBeVisible();
  await expect(page.locator(".welcome")).toHaveCount(0);
  await page.evaluate(() => {
    const state = window as any;
    state.__pendingHistory = {};
    state.go.main.App.GetMessages = (id: string) =>
      new Promise((resolve) => {
        state.__pendingHistory[id] = () =>
          resolve([
            {
              id,
              conversationId: id,
              role: "assistant",
              content: `${id}的课程情报`,
              state: "complete",
              createdAt: "",
            },
          ]);
      });
    state.__welcomeFlashed = false;
    state.__homeObserver = new MutationObserver(() => {
      if (document.querySelector(".welcome")) state.__welcomeFlashed = true;
    });
    state.__homeObserver.observe(document.querySelector(".channel-viewport"), {
      childList: true,
      subtree: true,
    });
  });
  await page.getByRole("textbox", { name: "选课问题" }).fill("保留这条草稿");
  await select("通讯乙");
  await expect(page.getByText("甲的课程情报")).toBeVisible();
  await expect(page.getByRole("button", { name: "发送问题" })).toBeDisabled();
  await expect(page.getByRole("status")).toContainText("正在读取通讯");
  await select("通讯丙");
  await page.evaluate(() => (window as any).__pendingHistory["丙"]());
  await expect(page.getByText("丙的课程情报")).toBeVisible();
  await page.evaluate(() => (window as any).__pendingHistory["乙"]());
  await expect(page.getByText("丙的课程情报")).toBeVisible();
  await expect(page.getByText("乙的课程情报")).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "选课问题" })).toHaveValue(
    "保留这条草稿",
  );
  await expect(page.getByRole("button", { name: "发送问题" })).toBeEnabled();
  expect(await page.evaluate(() => (window as any).__welcomeFlashed)).toBe(
    false,
  );
  await page.evaluate(() => (window as any).__homeObserver.disconnect());
  await page.emulateMedia({ reducedMotion: "reduce" });
  await select("通讯甲");
  await page.evaluate(() => (window as any).__pendingHistory["甲"]());
  await expect(page.getByText("甲的课程情报")).toBeVisible();
  expect(
    await page
      .locator(".timeline")
      .evaluate((node) => node.getAnimations().length),
  ).toBe(0);
  await page.keyboard.press("Control+k");
  await expect(
    page.getByRole("heading", { name: "情报台在线，等待你的指令。" }),
  ).toBeVisible();
  await expect(page.getByRole("textbox", { name: "选课问题" })).toBeFocused();
});

test("keyboard navigation closes drawers, restores focus and reaches the command input", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  if (testInfo.project.name === "narrow") {
    const menu = page.getByRole("button", { name: "打开历史列表" });
    await menu.click();
    await expect(
      page.getByRole("button", { name: "关闭历史列表" }),
    ).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(
      page.getByRole("button", { name: "助手设置", exact: true }),
    ).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(menu).toBeFocused();
    await expect(page.getByRole("button", { name: "新对话" })).toBeHidden();
    await menu.click();
    await page.getByRole("button", { name: "助手设置", exact: true }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("main")).not.toHaveAttribute("inert");
  }
  await page.keyboard.press("Control+k");
  await expect(page.getByRole("textbox", { name: "选课问题" })).toBeFocused();
  await page.keyboard.press("Control+,");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
