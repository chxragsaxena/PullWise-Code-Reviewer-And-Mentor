import type { Finding } from "../api/types";
import { formatConfidence } from "../lib/format";

/** Says how a finding was produced, so evidence-based and AI findings are distinguishable. */
export function SourceBadge({ finding }: { finding: Pick<Finding, "source" | "confidence"> }) {
  if (finding.source === "deterministic") {
    return (
      <span className="source-badge" title="Computed by code analysis, not an AI guess">
        Deterministic
      </span>
    );
  }
  return (
    <span className="source-badge source-badge--ai" title="Produced by AI reasoning over the code">
      AI · {formatConfidence(finding.confidence)} confidence
    </span>
  );
}
