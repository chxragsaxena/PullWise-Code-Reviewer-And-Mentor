import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  server: {
    // Used when VITE_API_MODE=http: proxy API and auth to the Go backend so the cookie stays same-origin.
    proxy: { "/api": "http://localhost:8080", "/auth": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
