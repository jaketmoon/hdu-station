import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Markdown } from "./Markdown";

describe("Chinese Markdown", () => {
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
