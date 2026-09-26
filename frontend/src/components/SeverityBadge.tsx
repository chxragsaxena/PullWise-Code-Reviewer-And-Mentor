import type { Severity } from "../api/types";
import { SEVERITY_LABEL } from "../lib/severity";

/** Severity is always spelled out, never conveyed by color alone. */
export function SeverityBadge({ severity }: { severity: Severity }) {
  return <span className={`badge badge--${severity}`}>{SEVERITY_LABEL[severity]}</span>;
}
