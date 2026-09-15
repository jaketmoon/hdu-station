const paths = {
  terminal: "M3 5h18v14H3zM7 9l3 3-3 3m6 0h4",
  signal: "M4 18v2m5-7v7m5-12v12m5-17v17",
  target: "M9 3H3v6m12-6h6v6M3 15v6h6m6 0h6v-6M8 8h8v8H8z",
  clock: "M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
  book: "M4 4h6a3 3 0 0 1 3 3v14a4 4 0 0 0-4-2H4V4Zm16 0h-4a3 3 0 0 0-3 3m7-3v15h-3a4 4 0 0 0-4 2",
  plus: "M12 5v14M5 12h14",
  chat: "M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z",
  settings: "M4 7h16M4 17h16M8 4v6M16 14v6",
  arrow: "M12 19V5m-6 6 6-6 6 6",
  right: "M5 12h14m-5-5 5 5-5 5",
  close: "m6 6 12 12M18 6 6 18",
  menu: "M4 6h16M4 12h16M4 18h16",
  trash: "M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7m4-7v7",
  copy: "M8 8h12v13H8zM4 16V3h12",
  check: "m5 12 4 4L19 6",
  leaf: "M19 3C7 3 2 9 6 15s14 4 13-12ZM5 21 15 9",
  sun: "M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z",
  stars: "m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3Z",
} as const;
export type IconName = keyof typeof paths;
export function Icon({ name, size = 20 }: { name: IconName; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="square"
      strokeLinejoin="miter"
      aria-hidden="true"
    >
      <path d={paths[name]} />
    </svg>
  );
}
