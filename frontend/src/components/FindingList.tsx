import { Link } from "react-router-dom";
import type { Finding } from "../api/types";
import { groupBySeverity } from "../lib/findings";
import { formatLocation } from "../lib/format";
import { SEVERITY_LABEL } from "../lib/severity";
import { SeverityBadge } from "./SeverityBadge";
import { SourceBadge } from "./SourceBadge";
import "./FindingList.css";

export function FindingList({ reviewId, findings }: { reviewId: string; findings: readonly Finding[] }) {
  return (
    <div className="finding-groups">
      {groupBySeverity(findings).map(({ severity, findings: group }) => (
        <section key={severity} aria-labelledby={`group-${severity}`}>
          <h3 id={`group-${severity}`} className="finding-groups__heading">
            {SEVERITY_LABEL[severity]} <span className="muted">· {group.length}</span>
          </h3>
          <ul className="finding-list">
            {group.map((finding) => (
              <li key={finding.id}>
                <Link to={`/reviews/${reviewId}/findings/${finding.id}`} className="finding-card">
                  <span className="finding-card__top">
                    <SeverityBadge severity={finding.severity} />
                    <span className="finding-card__skill">{finding.skillName}</span>
                    <SourceBadge finding={finding} />
                  </span>
                  <span className="finding-card__title">{finding.checkTitle}</span>
                  <span className="finding-card__found">{finding.found}</span>
                  <code className="finding-card__location">
                    {formatLocation(finding.file, finding.line, finding.endLine)}
                  </code>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
