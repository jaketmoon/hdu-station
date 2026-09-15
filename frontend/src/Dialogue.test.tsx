import { StrictMode } from "react";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TerminalGreeting, Dialogue, useTypewriter } from "./Dialogue";
import { Markdown } from "./Markdown";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("terminal dialogue", () => {
  it("reveals the pixel greeting one character at a time under StrictMode and cleans up", () => {
    const { container, unmount } = render(
      <StrictMode>
        <TerminalGreeting reduced={false} />
      </StrictMode>,
    );
    expect(
      screen.getByRole("heading", { name: "情报台在线，等待你的指令。" }),
    ).toBeInTheDocument();
    expect(container.querySelectorAll(".is-revealed")).toHaveLength(0);
    act(() => vi.advanceTimersByTime(90));
    expect(container.querySelectorAll(".is-revealed")).toHaveLength(1);
    expect(container.querySelector(".is-revealed")).toHaveTextContent("情");
    act(() => vi.advanceTimersByTime(3000));
    expect(container.querySelectorAll(".is-revealed")).toHaveLength(13);
    expect(screen.queryByRole("status")).toBeNull();
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
  it("shows the whole greeting immediately when typing is reduced", () => {
    const { container } = render(<TerminalGreeting reduced />);
    expect(container.querySelectorAll(".is-revealed")).toHaveLength(13);
    expect(vi.getTimerCount()).toBe(0);
  });
  it("keeps complete graphemes and keeps advancing across rapidly arriving chunks", () => {
    const { result, rerender, unmount } = renderHook(
      ({ text }) => useTypewriter(text, true),
      { initialProps: { text: "" } },
    );
    rerender({ text: "A👩‍💻中é" });
    act(() => vi.advanceTimersByTime(22));
    expect(result.current.visible).toBe(1);
    rerender({ text: "A👩‍💻中é后" });
    act(() => vi.advanceTimersByTime(10));
    rerender({ text: "A👩‍💻中é后来" });
    act(() => vi.advanceTimersByTime(12));
    expect("A👩‍💻中é后来".slice(0, result.current.visible)).toBe("A👩‍💻");
    act(() => result.current.revealAll());
    rerender({ text: "A👩‍💻中é后来追加" });
    expect(result.current.visible).toBe("A👩‍💻中é后来追加".length);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
  it("shows history immediately, resets replaced summaries, and flushes stopped text", () => {
    const { result, rerender } = renderHook(
      ({ text, enabled }) => useTypewriter(text, enabled),
      { initialProps: { text: "已有的历史情报", enabled: true } },
    );
    expect(result.current.revealing).toBe(false);
    rerender({ text: "新的可靠汇总", enabled: true });
    expect(result.current.visible).toBe(0);
    act(() => vi.advanceTimersByTime(22));
    expect(result.current.visible).toBe(1);
    rerender({ text: "新的可靠汇总", enabled: false });
    expect(result.current.visible).toBe(6);
    expect(result.current.revealing).toBe(false);
  });
  it("lets users skip typing and continues to display future chunks immediately", () => {
    const props = {
      enabled: true,
      streaming: true,
      onError: vi.fn(),
      onProgress: vi.fn(),
    };
    const { rerender } = render(<Dialogue {...props} text="" />);
    rerender(
      <Dialogue {...props} text="**课程情报**已整理，继续接收后续线索。" />,
    );
    fireEvent.click(screen.getByRole("button", { name: /立即显示/ }));
    expect(screen.getByText("课程情报").tagName).toBe("STRONG");
    rerender(
      <Dialogue
        {...props}
        text="**课程情报**已整理，继续接收后续线索。追加完毕。"
      />,
    );
    expect(screen.getByText(/追加完毕/)).toBeVisible();
  });
  it("reveals parsed emphasis and whole tables without exposing Markdown delimiters", () => {
    const { container, rerender } = render(
      <Markdown
        text="**选课情报**，后续文字。"
        revealLimit={4}
        onError={vi.fn()}
      />,
    );
    expect(container.querySelector("strong")).toHaveTextContent("选课");
    expect(container.textContent).not.toContain("**");
    const table = "| 课程 | 状态 |\n| --- | --- |\n| 戏曲 | 可选 |";
    rerender(<Markdown text={table} revealLimit={1} onError={vi.fn()} />);
    expect(screen.getByRole("table")).toHaveTextContent("可选");
  });
});
