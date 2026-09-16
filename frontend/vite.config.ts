import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    // Do not embed assets from removed UI features in the desktop bundle.
    emptyOutDir: true,
    // Keep small pixel fonts as same-origin files for the Wails WebView.
    assetsInlineLimit: (filePath) => filePath.endsWith(".woff2") ? false : undefined,
  },
  test: {
    exclude: ["e2e/**", "node_modules/**"],
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
});
