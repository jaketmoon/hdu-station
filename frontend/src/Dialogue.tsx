import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Markdown } from "./Markdown";

export function useReducedMotion() {
  const [reduced, setReduced] = useState(
    () =>
      window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false,
  );
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    if (!media) return;
    const update = () => setReduced(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return reduced;
}

// Cursor positions are grapheme boundaries, so Chinese, emoji and combining
// marks arrive intact. The timer survives SSE chunks rather than restarting.
export function useTypewriter(
  text: string,
  enabled: boolean,
  fromStart = false,
  interval = 22,
) {
  const [visible, setVisible] = useState(
    fromStart && enabled ? 0 : text.length,
  );
  const cursor = useRef(visible);
  const source = useRef(text);
  const boundaries = useRef<number[]>([]);
  const running = useRef(enabled);
  const skipped = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const tick = useRef<() => void>(() => {});
  tick.current = () => {
    timer.current = undefined;
    if (!running.current) return;
    const positions = boundaries.current;
    const next = positions.findIndex((position) => position > cursor.current);
    if (next < 0) return;
    const remaining = positions.length - next;
    const step = remaining > 80 ? Math.ceil(remaining / 18) : 1;
    const end = positions[Math.min(next + step - 1, positions.length - 1)];
    cursor.current = end;
    setVisible(end);
    if (end < source.current.length) {
      const punctuation = /[。！？；，.!?;,]/.test(source.current[end - 1]);
      timer.current = setTimeout(
        () => tick.current(),
        punctuation && remaining < 80 ? Math.max(85, interval * 3) : interval,
      );
    }
  };
  useLayoutEffect(() => {
    if (!text.startsWith(source.current)) cursor.current = 0;
    source.current = text;
    boundaries.current = Array.from(
      new Intl.Segmenter("zh", { granularity: "grapheme" }).segment(text),
      ({ index, segment }) => index + segment.length,
    );
    running.current = enabled && !skipped.current;
    if (!running.current) {
      clearTimeout(timer.current);
      timer.current = undefined;
      cursor.current = text.length;
    }
    setVisible(cursor.current);
    if (
      running.current &&
      cursor.current < text.length &&
      timer.current === undefined
    )
      timer.current = setTimeout(() => tick.current(), interval);
  }, [text, enabled, interval]);
  useEffect(
    () => () => {
      clearTimeout(timer.current);
      timer.current = undefined;
    },
    [],
  );
  const revealAll = useCallback(() => {
    skipped.current = true;
    running.current = false;
    clearTimeout(timer.current);
    timer.current = undefined;
    cursor.current = source.current.length;
    setVisible(cursor.current);
  }, []);
  return {
    visible: enabled ? visible : text.length,
    revealing: enabled && visible < text.length,
    revealAll,
  };
}

export const Dialogue = memo(function Dialogue({
  text,
  enabled,
  streaming,
  onError,
  onProgress,
}: {
  text: string;
  enabled: boolean;
  streaming: boolean;
  onError: (message: string) => void;
  onProgress: () => void;
}) {
  const { visible, revealing, revealAll } = useTypewriter(text, enabled);
  useLayoutEffect(onProgress, [visible, onProgress]);
  return (
    <div className="dialogue-body">
      <div aria-hidden={revealing || undefined}>
        <Markdown
          text={text}
          revealLimit={revealing ? visible : undefined}
          onError={onError}
        />
      </div>
      {revealing && (
        <span className="sr-only" role="status">
          正在接收情报，稍后显示完整回答。
        </span>
      )}
      {(revealing || streaming) && (
        <div className="transmission-line">
          <span className="transmission-cursor" aria-hidden="true" />
          <span>{revealing ? "DECODING" : "RECEIVING"}</span>
          {revealing && (
            <button className="reveal-button" onClick={revealAll}>
              立即显示 <span aria-hidden="true">≫</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
});

export function TerminalGreeting({ reduced }: { reduced: boolean }) {
  const text = "情报台在线，等待你的指令。";
  const { visible, revealing } = useTypewriter(text, !reduced, true, 90);
  return (
    <h1
      className={`terminal-greeting ${reduced ? "instant" : ""}`}
      aria-label={text}
    >
      <span className="greeting-text" aria-hidden="true">
        {[text.slice(0, 6), text.slice(6)].map((line, lineIndex) => (
          <span
            className={`greeting-line ${lineIndex ? "accent" : ""}`}
            key={lineIndex}
          >
            {Array.from(line).map((character, index) => (
              <span
                key={index}
                className={`greeting-glyph ${lineIndex * 6 + index < visible ? "is-revealed" : ""}`}
              >
                {character}
              </span>
            ))}
          </span>
        ))}
      </span>
      {revealing && (
        <span className="sr-only" role="status">
          正在接通情报台…
        </span>
      )}
    </h1>
  );
}
