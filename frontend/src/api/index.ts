import type { ApiClient } from "./client";
import { createHttpClient } from "./httpClient";
import { createMockClient } from "./mockClient";

/** Chooses the API implementation from VITE_API_MODE (default: mock). */
export function createDefaultClient(): ApiClient {
  if (import.meta.env.VITE_API_MODE === "http") {
    return createHttpClient(import.meta.env.VITE_API_BASE_URL ?? "");
  }
  return createMockClient();
}
