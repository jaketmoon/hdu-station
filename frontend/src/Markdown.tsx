import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkCjkFriendly from "remark-cjk-friendly";
import { api } from "./api";
export function Markdown({
  text,
  onError,
}: {
  text: string;
  onError: (message: string) => void;
}) {
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkCjkFriendly]}
        components={{
          a: ({ href, children }) => (
            <a
              href={href}
              onClick={(event) => {
                event.preventDefault();
                if (href)
                  api.open(href).catch(() => onError("原帖暂时无法打开。"));
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
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
