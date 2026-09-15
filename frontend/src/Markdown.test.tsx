import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Markdown } from "./Markdown";

describe("Chinese Markdown", () => {
  it("preserves hard line breaks and separators in received intelligence", () => {
    const { container } = render(
      <Markdown
        text={"第一行  \n第二行\n\n---\n\n下一条情报"}
        onError={vi.fn()}
      />,
    );
    expect(container.querySelector("br")).not.toBeNull();
    expect(container.querySelector("hr")).not.toBeNull();
  });
  it("keeps a table's horizontal reading position while later text is revealed", () => {
    const text =
      "| 课程 | 状态 |\n| --- | --- |\n| 戏曲 | 可选 |\n\n继续接收后续情报。";
    const onError = vi.fn();
    const { container, rerender } = render(
      <Markdown text={text} revealLimit={1} onError={onError} />,
    );
    const scroller = container.querySelector(".table-scroll")!;
    scroller.scrollLeft = 45;
    rerender(
      <Markdown text={text} revealLimit={text.length - 2} onError={onError} />,
    );
    expect(container.querySelector(".table-scroll")?.scrollLeft).toBe(45);
  });
  it("renders bold Chinese punctuation without exposing delimiters", () => {
    const { container } = render(
      <Markdown
        text={"**选课前再确认两件事：**授课老师和考核方式。"}
        onError={vi.fn()}
      />,
    );
    expect(container.querySelector("strong")).toHaveTextContent(
      "选课前再确认两件事：",
    );
    expect(container.textContent).not.toContain("**");
  });
  it("does not execute HTML or render unsafe links and remote tracking images", () => {
    const { container } = render(
      <Markdown
        text={
          "<script>alert(1)</script>\n\n[点这里](javascript:alert(1))\n\n![图片](https://tracker.example/pixel)"
        }
        onError={vi.fn()}
      />,
    );
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("点这里").getAttribute("href")).not.toContain(
      "javascript:",
    );
  });
});
