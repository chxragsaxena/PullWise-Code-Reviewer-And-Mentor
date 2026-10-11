import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  server: {
    // Used when VITE_API_MODE=http: forward API and auth calls to the Go
    // backend, so the browser sees everything as one origin (the session
    // cookie needs that — no CORS headers to maintain either).
    proxy: { "/api": "http://localhost:8080", "/auth": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
