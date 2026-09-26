import { useCallback, useEffect, useState } from "react";

export interface AsyncState<T> {
  data: T | null;
  error: Error | null;
  /** True while a request is in flight, including reloads that keep old data. */
  loading: boolean;
  reload: () => void;
}

/**
 * Runs `load` when `deps` change and tracks its result. Stale responses from a
 * superseded request are ignored. Existing data stays visible during a reload.
 */
export function useAsync<T>(load: () => Promise<T>, deps: readonly unknown[]): AsyncState<T> {
  const [state, setState] = useState<{ data: T | null; error: Error | null; loading: boolean }>({
    data: null,
    error: null,
    loading: true,
  });
  const [reloadCount, setReloadCount] = useState(0);
  // Stable identity, so callers can safely list it in effect dependencies.
  const reload = useCallback(() => setReloadCount((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    setState((prev) => ({ ...prev, error: null, loading: true }));

    load().then(
      (data) => {
        if (!cancelled) setState({ data, error: null, loading: false });
      },
      (error: unknown) => {
        if (!cancelled) {
          setState({ data: null, error: error instanceof Error ? error : new Error(String(error)), loading: false });
        }
      },
    );

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- callers own the dependency list
  }, [...deps, reloadCount]);

  return { ...state, reload };
}
