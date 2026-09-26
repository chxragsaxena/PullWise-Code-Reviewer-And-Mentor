import { createContext, useContext, type ReactNode } from "react";
import type { ApiClient } from "./client";

const ApiContext = createContext<ApiClient | null>(null);

/** Supplies the API client to the UI, so it can be swapped (real, mock, test). */
export function ApiProvider({ client, children }: { client: ApiClient; children: ReactNode }) {
  return <ApiContext.Provider value={client}>{children}</ApiContext.Provider>;
}

export function useApi(): ApiClient {
  const client = useContext(ApiContext);
  if (!client) throw new Error("useApi must be used inside <ApiProvider>");
  return client;
}
