import type { ReactNode } from "react";
import "./PageState.css";

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="page-state" role="status">
      <span className="spinner" aria-hidden="true" />
      <span>{label}…</span>
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: Error; onRetry?: () => void }) {
  return (
    <div className="page-state page-state--error" role="alert">
      <strong>Something went wrong</strong>
      <span>{error.message}</span>
      {onRetry && (
        <button type="button" className="button button--secondary" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  );
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="page-state">
      <strong>{title}</strong>
      {children && <span>{children}</span>}
    </div>
  );
}
