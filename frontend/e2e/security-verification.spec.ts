import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test("security QR fits settings and remains pending until search recovery", async ({
  page,
}, info) => {
  const image =
    "data:image/png;base64," +
    readFileSync(new URL("./fixtures/login-qr.png", import.meta.url)).toString(
      "base64",
    );
  await page.addInitScript((image) => {
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
        enabled: true,
        baseURL: "http://127.0.0.1:18060",
        hasAuthToken: true,
        status: "verification_required",
      },
    };
    (window as any).runtime = { EventsOn: () => () => {} };
    (window as any).go = {
      main: {
        App: {
          ListConversations: async () => [],
          GetSettings: async () => settings,
          BeginSourceVerification: async () => ({
            id: "security-fixture",
            kind: "verification",
            status: "waiting",
            image,
            expiresAt: Date.now() + 45000,
          }),
          PollSourceLogin: async () => ({
            id: "security-fixture",
            status: (window as any).__securityRecovered ? "ready" : "waiting",
          }),
          CancelSourceLogin: async () => {},
        },
      },
    };
  }, image);
  await page.goto("/");
  await page.getByTitle("查看助手设置").click();
  const card = page.getByRole("region", { name: "小红书来源" });
  await expect(card).toContainText("需要安全验证");
  await card.locator(".source-disclosure").click();
  await card.getByRole("button", { name: "安全验证", exact: true }).click();
  const qr = card.getByRole("img", { name: "小红书安全验证二维码" });
  await expect(qr).toBeVisible();
  await expect(card).toContainText("等待扫码验证身份");
  const box = await qr.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await page.screenshot({
    path: `test-results/${info.project.name}-security-verification.png`,
  });
  await page.evaluate(() => {
    (window as any).__securityRecovered = true;
  });
  await expect(card).toContainText("安全验证已通过，已确认搜索恢复可用。");
  await expect(qr).not.toBeVisible();
});
