import { memo, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkCjkFriendly from "remark-cjk-friendly";
import { api } from "./api";

type RevealNode = {
  type: string;
  value?: string;
  children?: RevealNode[];
  position?: { start: { offset?: number }; end: { offset?: number } };
};
// Trim the parsed syntax tree, never raw Markdown. Emphasis and verified links
// keep their structure; tables/code arrive as whole intelligence panels.
function remarkReveal({ limit = Infinity }: { limit?: number }) {
  return (tree: RevealNode) => {
    if (!Number.isFinite(limit)) return;
    function reveal(node: RevealNode): boolean {
      const start = node.position?.start.offset ?? 0;
      if (start >= limit) return false;
      if (node.type === "table" || node.type === "code") return true;
      if (node.children) node.children = node.children.filter(reveal);
      else if (node.value && (node.position?.end.offset ?? 0) > limit) {
        const remaining = limit - start;
        node.value = Array.from(
          new Intl.Segmenter("zh", { granularity: "grapheme" }).segment(
            node.value,
          ),
        )
          .filter(({ index, segment }) => index + segment.length <= remaining)
          .map(({ segment }) => segment)
          .join("");
      }
      if (node.children) return node.children.length > 0;
      return node.value === undefined || node.value.length > 0;
    }
    tree.children = tree.children?.filter(reveal);
  };
}
export const Markdown = memo(function Markdown({
  text,
  onError,
  revealLimit,
}: {
  text: string;
  onError: (message: string) => void;
  revealLimit?: number;
}) {
  const components = useMemo<Components>(
    () => ({
      a: ({ href, children }) => (
        <a
          href={href}
          onClick={(event) => {
            event.preventDefault();
            if (href) api.open(href).catch(() => onError("原帖暂时无法打开。"));
          }}
        >
          {children}
          <span className="external-mark" aria-hidden="true">
            ↗
          </span>
        </a>
      ),
      table: ({ children }) => (
        <div className="table-container">
          <div className="intel-table-heading">
            <span>
              <i /> INTEL / 课程情报
            </span>
            <span>DATA SHEET</span>
          </div>
          <div
            className="table-scroll"
            tabIndex={0}
            aria-label="课程对比表，可左右滚动"
          >
            <table>{children}</table>
          </div>
          <p className="table-hint">左右滑动，查看完整表格与原帖 →</p>
        </div>
      ),
      img: () => null,
    }),
    [onError],
  );
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[
          remarkGfm,
          remarkCjkFriendly,
          [remarkReveal, { limit: revealLimit }],
        ]}
        components={components}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
});
