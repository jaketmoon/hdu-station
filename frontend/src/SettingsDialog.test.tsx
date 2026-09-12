import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { SettingsDialog } from "./SettingsDialog";
import { api, type Settings, type CampusLogin } from "./api";

vi.mock("./api", () => ({
  api: {
    save: vi.fn(),
    beginCampusLogin: vi.fn(),
    pollCampusLogin: vi.fn(),
    cancelCampusLogin: vi.fn(),
    openCampusLogin: vi.fn(),
    checkCampus: vi.fn(),
    logoutCampus: vi.fn(),
  },
  errorText: String,
}));

const settings: Settings = {
  baseURL: "https://api.deepseek.com",
  model: "deepseek-flash",
  hasAPIKey: true,
  campus: { hasCredential: false, status: "logged_out" },
  qqStatus: "disabled",
  qqEnabled: false,
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
const pending = (): CampusLogin => ({
  id: "campus-login",
  status: "waiting",
  userCode: "ABCD-EFGH",
  expiresAt: Date.now() + 600000,
});

beforeEach(() => {
  vi.resetAllMocks();
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  vi.mocked(api.beginCampusLogin).mockResolvedValue(pending());
  vi.mocked(api.cancelCampusLogin).mockResolvedValue({
    ...pending(),
    status: "cancelled",
  });
  vi.mocked(api.checkCampus).mockResolvedValue({
    hasCredential: true,
    status: "saved",
  });
});

it("opens official web authorization and shows the saved local login without a PAT input", async () => {
  let approve!: (result: CampusLogin) => void;
  vi.mocked(api.pollCampusLogin).mockReturnValue(
    new Promise((resolve) => {
      approve = resolve;
    }),
  );
  const onSaved = vi.fn();
  render(
    <SettingsDialog settings={settings} onClose={vi.fn()} onSaved={onSaved} />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  expect(screen.queryByLabelText(/校园 PAT/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "网页授权" }));
  expect(await screen.findByText("ABCD-EFGH")).toBeVisible();
  expect(screen.getByRole("button", { name: "网页授权" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "再次打开授权页" }));
  await waitFor(() =>
    expect(api.openCampusLogin).toHaveBeenCalledWith("campus-login"),
  );
  await act(async () => {
    approve({ ...pending(), status: "ready" });
  });
  await waitFor(() =>
    expect(onSaved).toHaveBeenLastCalledWith({
      ...settings,
      campus: { hasCredential: true, status: "saved" },
    }),
  );
  expect(screen.getByText("HDU CLI 登录已保存。")).toBeVisible();
  expect(api.save).not.toHaveBeenCalled();
});

it("keeps an approved login while a login state update outlives the request deadline", async () => {
  vi.useFakeTimers();
  try {
    const login = { ...pending(), expiresAt: Date.now() + 500 };
    vi.mocked(api.beginCampusLogin).mockResolvedValue(login);
    vi.mocked(api.pollCampusLogin).mockResolvedValue({
      ...login,
      status: "ready",
    });
    let checked!: (value: Settings["campus"]) => void;
    vi.mocked(api.checkCampus).mockReturnValue(
      new Promise((resolve) => {
        checked = resolve;
      }),
    );
    const onSaved = vi.fn();
    render(
      <SettingsDialog
        settings={settings}
        onClose={vi.fn()}
        onSaved={onSaved}
      />,
    );
    fireEvent.click(screen.getByText("HDU CLI 登录"));
    fireEvent.click(screen.getByRole("button", { name: "网页授权" }));
    await act(async () => {});
    expect(api.checkCampus).toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(
      screen.queryByText("授权请求已过期，请重新发起。"),
    ).not.toBeInTheDocument();
    await act(async () => {
      checked({ hasCredential: true, status: "saved" });
    });
    expect(onSaved).toHaveBeenLastCalledWith({
      ...settings,
      campus: { hasCredential: true, status: "saved" },
    });
  } finally {
    vi.useRealTimers();
  }
});

it("cancels on close and ignores a late approval", async () => {
  let approve!: (result: CampusLogin) => void;
  vi.mocked(api.pollCampusLogin).mockReturnValue(
    new Promise((resolve) => {
      approve = resolve;
    }),
  );
  const onSaved = vi.fn();
  const view = render(
    <SettingsDialog settings={settings} onClose={vi.fn()} onSaved={onSaved} />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  fireEvent.click(screen.getByRole("button", { name: "网页授权" }));
  await screen.findByText("ABCD-EFGH");
  view.unmount();
  await act(async () => {
    approve({ ...pending(), status: "ready" });
  });
  expect(api.cancelCampusLogin).toHaveBeenCalledWith("campus-login");
  expect(onSaved).not.toHaveBeenCalled();
});

it("cancels an authorization that starts after the dialog is closed", async () => {
  let started!: (result: CampusLogin) => void;
  vi.mocked(api.beginCampusLogin).mockReturnValue(
    new Promise((resolve) => {
      started = resolve;
    }),
  );
  const view = render(
    <SettingsDialog settings={settings} onClose={vi.fn()} onSaved={vi.fn()} />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  fireEvent.click(screen.getByRole("button", { name: "网页授权" }));
  view.unmount();
  await act(async () => {
    started(pending());
  });
  expect(api.cancelCampusLogin).toHaveBeenCalledWith("campus-login");
});

it.each([
  ["denied", "你已拒绝本次授权，可以重新发起。"],
  ["expired", "授权请求已过期，请重新发起。"],
])("shows %s and lets the user retry", async (status, message) => {
  vi.mocked(api.pollCampusLogin).mockResolvedValue({ ...pending(), status });
  render(
    <SettingsDialog settings={settings} onClose={vi.fn()} onSaved={vi.fn()} />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  fireEvent.click(screen.getByRole("button", { name: "网页授权" }));
  expect(await screen.findByText(message)).toBeVisible();
  expect(screen.getByRole("button", { name: "网页授权" })).toBeEnabled();
});

it("logs out immediately and model settings contain no campus credential fields", async () => {
  const connected = {
    ...settings,
    campus: { hasCredential: true, status: "saved" },
  };
  const onSaved = vi.fn();
  vi.mocked(api.logoutCampus).mockResolvedValue(settings.campus);
  vi.mocked(api.save).mockResolvedValue(connected);
  render(
    <SettingsDialog settings={connected} onClose={vi.fn()} onSaved={onSaved} />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  fireEvent.click(screen.getByRole("button", { name: "退出登录" }));
  await waitFor(() => expect(onSaved).toHaveBeenLastCalledWith(settings));
  fireEvent.click(screen.getByRole("button", { name: "保存并检查" }));
  await waitFor(() => expect(api.save).toHaveBeenCalled());
  const saved = vi.mocked(api.save).mock.calls[0][0];
  expect(saved).not.toHaveProperty("campusKey");
  expect(saved).not.toHaveProperty("clearCampusKey");
  expect(saved.zanao.enabled).toBe(false);
});

it("makes unsupported browser authorization unavailable", () => {
  render(
    <SettingsDialog
      settings={{
        ...settings,
        campus: { hasCredential: false, status: "unsupported" },
      }}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByText("HDU CLI 登录"));
  expect(screen.getByRole("button", { name: "网页授权" })).toBeDisabled();
});
