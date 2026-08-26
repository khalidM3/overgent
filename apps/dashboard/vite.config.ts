import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: process.env.STICKGUY_DASHBOARD_API_ORIGIN ? {
      "/api": { target: process.env.STICKGUY_DASHBOARD_API_ORIGIN, changeOrigin: true, rewrite: (path) => path.replace(/^\/api/, "") },
    } : undefined,
  },
  test: {
    environment: "jsdom",
    include: ["test/**/*.test.ts", "test/**/*.test.tsx"],
    setupFiles: ["./test/setup.ts"],
  },
});
