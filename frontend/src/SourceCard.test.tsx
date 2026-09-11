import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, type SourceLogin } from "./api";
import { SourceCard } from "./SourceCard";

vi.mock("./api", () => ({
  api: {
    checkSource: vi.fn(),
    enableSource: vi.fn(),
    beginLogin: vi.fn(),
    pollLogin: vi.fn(),
    cancelLogin: vi.fn(),
    clearSource: vi.fn(),
  },
  errorText: (error: unknown) =>
    error instanceof Error ? error.message : String(error),
}));
const png =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jO1sAAAAASUVORK5CYII=";
function Harness() {
  const [connection, setConnection] = useState({
    enabled: true,
    status: "logged_out",
  });
  return (
    <SourceCard
      source="xiaohongshu"
      name="小红书"
      connection={connection}
      disabled={false}
      onChange={(_, next) => setConnection(next)}
    />
  );
}
function expand() {
  fireEvent.click(screen.getByRole("button", { expanded: false }));
}
async function reconnect() {
  fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
  await screen.findByRole("img", { name: "小红书登录二维码" });
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.beginLogin).mockImplementation(async () => ({
    id: "login-1",
    status: "waiting",
    image: png,
    expiresAt: Date.now() + 240000,
  }));
  vi.mocked(api.pollLogin).mockResolvedValue({
    id: "login-1",
    status: "waiting",
  });
  vi.mocked(api.cancelLogin).mockResolvedValue();
  vi.mocked(api.clearSource).mockResolvedValue({
    enabled: true,
    status: "logged_out",
  });
});
afterEach(() => {
  vi.useRealTimers();
});

describe("source account settings", () => {
  it("starts compact and checks only its source", async () => {
    vi.mocked(api.checkSource).mockResolvedValue({
      enabled: true,
      status: "ready",
    });
    render(<Harness />);
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新检查小红书" }));
    await screen.findByText("已连接");
    expect(api.checkSource).toHaveBeenCalledWith("xiaohongshu");
    expand();
    expect(screen.getByRole("switch")).toBeChecked();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
  it("detects saved login, updates the connection, and clears the account", async () => {
    let scanned!: (login: SourceLogin) => void;
    vi.mocked(api.pollLogin).mockImplementation(
      () =>
        new Promise((resolve) => {
          scanned = resolve;
        }),
    );
    render(<Harness />);
    expand();
    await reconnect();
    expect(screen.getByRole("button", { name: "清除登录凭证" })).toBeDisabled();
    await act(async () => {
      scanned({ id: "login-1", status: "ready" });
    });
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("已连接")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("凭证已保存在本机");
    fireEvent.click(screen.getByRole("button", { name: "清除登录凭证" }));
    await waitFor(() =>
      expect(api.clearSource).toHaveBeenCalledWith("xiaohongshu"),
    );
    expect(screen.getByText("需要重新登录")).toBeVisible();
  });
  it("persists the enabled switch independently of the model form", async () => {
    vi.mocked(api.enableSource).mockResolvedValue({
      enabled: false,
      status: "disabled",
    });
    render(<Harness />);
    expand();
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => expect(screen.getByRole("switch")).not.toBeChecked());
    expect(api.enableSource).toHaveBeenCalledWith("xiaohongshu", false);
    expect(screen.getByText("未启用")).toBeVisible();
  });
  it("expires a QR and stops polling while leaving reconnect available", async () => {
    vi.useFakeTimers();
    vi.mocked(api.beginLogin).mockResolvedValue({
      id: "login-1",
      status: "waiting",
      image: png,
      expiresAt: Date.now() + 1000,
    });
    render(<Harness />);
    expand();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100);
    });
    expect(screen.getByText("二维码已过期")).toBeVisible();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(api.cancelLogin).toHaveBeenCalledWith("login-1");
    expect(screen.getByRole("button", { name: "重新连接" })).toBeEnabled();
    const calls = vi.mocked(api.pollLogin).mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(api.pollLogin).toHaveBeenCalledTimes(calls);
  });
  it("cancels on close and ignores a late login result", async () => {
    let scanned!: (login: SourceLogin) => void;
    vi.mocked(api.pollLogin).mockImplementation(
      () =>
        new Promise((resolve) => {
          scanned = resolve;
        }),
    );
    const changed = vi.fn();
    const view = render(
      <SourceCard
        source="qq"
        name="QQ 频道"
        connection={{ enabled: true, status: "logged_out" }}
        disabled={false}
        onChange={changed}
      />,
    );
    expand();
    fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
    await screen.findByRole("img");
    view.unmount();
    await act(async () => {
      scanned({ id: "login-1", status: "ready" });
    });
    expect(api.cancelLogin).toHaveBeenCalledWith("login-1");
    expect(changed).not.toHaveBeenCalled();
  });
  it("cancels a QR that arrives after closing and shows retriable connection errors", async () => {
    let prepared!: (login: SourceLogin) => void;
    vi.mocked(api.beginLogin).mockImplementation(
      () =>
        new Promise((resolve) => {
          prepared = resolve;
        }),
    );
    const view = render(<Harness />);
    expand();
    fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
    view.unmount();
    await act(async () => {
      prepared({
        id: "late",
        status: "waiting",
        image: png,
        expiresAt: Date.now() + 240000,
      });
    });
    expect(api.cancelLogin).toHaveBeenCalledWith("late");
    expect(api.pollLogin).not.toHaveBeenCalled();
    vi.mocked(api.beginLogin).mockRejectedValue(new Error("连接暂不可用"));
    render(<Harness />);
    expand();
    fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("连接暂不可用");
    expect(screen.getByRole("button", { name: "重新连接" })).toBeEnabled();
  });
  it("shows an unsupported platform without offering unavailable account operations", () => {
    render(
      <SourceCard
        source="qq"
        name="QQ 频道"
        connection={{ enabled: true, status: "unsupported" }}
        disabled={false}
        onChange={vi.fn()}
      />,
    );
    expand();
    expect(screen.getByText("当前平台暂不支持")).toBeVisible();
    expect(screen.getByRole("switch")).toBeDisabled();
    expect(screen.getByRole("button", { name: "重新连接" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "清除登录凭证" })).toBeDisabled();
  });
});
