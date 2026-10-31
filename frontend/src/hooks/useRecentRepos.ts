import { useApi } from "../api/ApiProvider";
import { useAsync } from "./useAsync";

/** The repositories the signed-in user used before, newest first. Suggestions are optional: any failure is empty. */
export function useRecentRepos(): string[] {
  const api = useApi();
  const recent = useAsync(() => api.listRecentRepos(), [api]);
  return recent.data ?? [];
}
